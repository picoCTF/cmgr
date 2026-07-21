package cmgr

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestContentChecksum(t *testing.T) {
	base := contentChecksum(0xdeadbeef, "flag{%s}")
	if base != contentChecksum(0xdeadbeef, "flag{%s}") {
		t.Error("contentChecksum is not deterministic")
	}
	if base == contentChecksum(0xdeadbef0, "flag{%s}") {
		t.Error("expected different checksum for a different source checksum")
	}
	if base == contentChecksum(0xdeadbeef, "ctf{%s}") {
		t.Error("expected different checksum for a different flag format")
	}
}

func TestDockerIdContentAddressed(t *testing.T) {
	image := Image{Host: "challenge"}

	a := &BuildMetadata{Id: 1, Seed: 3, Checksum: 0xdeadbeef}
	if got, want := a.dockerId(image), "s3-deadbeef-challenge"; got != want {
		t.Errorf("dockerId = %q, want %q", got, want)
	}

	// Negative seeds must still yield a tag with a valid leading character.
	b := &BuildMetadata{Id: 2, Seed: -7, Checksum: 0xdeadbeef}
	if got, want := b.dockerId(image), "s-7-deadbeef-challenge"; got != want {
		t.Errorf("dockerId = %q, want %q", got, want)
	}

	// The local build id must not influence the tag: identical content built
	// under different ids (e.g. by different cmgr databases) shares a tag.
	c := &BuildMetadata{Id: 99, Seed: 3, Checksum: 0xdeadbeef}
	if a.dockerId(image) != c.dockerId(image) {
		t.Error("dockerId must not depend on the build id")
	}
}

// insertTestBuild inserts a builds row directly and returns its id.
func insertTestBuild(t *testing.T, mgr *Manager, schema, challenge, format string, seed int, checksum uint32) BuildId {
	t.Helper()
	res, err := mgr.db.Exec(
		`INSERT INTO builds(flag, format, seed, checksum, hasartifacts, lastsolved, challenge, schema, instancecount)
		 VALUES ('flag{x}', ?, ?, ?, 0, 0, ?, ?, 1);`,
		format, seed, checksum, challenge, schema)
	if err != nil {
		t.Fatalf("failed to insert build: %s", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to read build id: %s", err)
	}
	return BuildId(id)
}

func TestContentReferenced(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := &ChallengeMetadata{
		Id:            "test/shared-content",
		Name:          "Shared Content",
		Namespace:     "test",
		ChallengeType: "custom",
		Path:          "/tmp/test/problem.md",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
	}
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	const format = "flag{%s}"
	const seed = 42
	const checksum = uint32(0xabad1dea)

	// Two schemas holding the same (challenge, seed, format, checksum) tuple
	// resolve to the same docker tags.
	idA := insertTestBuild(t, mgr, "event-a", "test/shared-content", format, seed, checksum)
	idB := insertTestBuild(t, mgr, "event-b", "test/shared-content", format, seed, checksum)

	shared := &BuildMetadata{Challenge: "test/shared-content", Seed: seed, Format: format, Checksum: checksum}
	if !mgr.contentReferenced(shared, idA) {
		t.Error("expected content to be referenced while a second build shares the tuple")
	}

	if _, err := mgr.db.Exec("DELETE FROM builds WHERE id = ?;", idB); err != nil {
		t.Fatalf("failed to delete build: %s", err)
	}
	if mgr.contentReferenced(shared, idA) {
		t.Error("expected content to be unreferenced once the sharing build is gone")
	}

	// A build of the same challenge and seed but a different generation
	// (checksum) has different tags and must not count as a reference.
	insertTestBuild(t, mgr, "event-b", "test/shared-content", format, seed, checksum+1)
	if mgr.contentReferenced(shared, idA) {
		t.Error("expected a different-checksum build not to count as a reference")
	}
}

// TestBuildsChecksumMigration exercises the legacy-database migration path: a
// builds table from before the checksum column existed must gain the column
// and have its rows backfilled from the owning challenge's source checksum
// and the row's flag format. (Image retagging is skipped because the test
// manager has no docker client.)
func TestBuildsChecksumMigration(t *testing.T) {
	dbFile, err := os.CreateTemp("", "cmgr-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	t.Cleanup(func() { removeDBFiles(dbFile.Name()) })

	seedDB, err := sqlx.Open("sqlite3", dbFile.Name())
	if err != nil {
		t.Fatalf("failed to open seed database: %s", err)
	}

	const sourceChecksum = uint32(0x1234abcd)
	const format = "flag{%s}"

	// The pre-checksum shape of the challenges/builds/images tables.
	legacy := `
	CREATE TABLE challenges (
		id TEXT NOT NULL PRIMARY KEY,
		name TEXT NOT NULL,
		namespace TEXT NOT NULL,
		challengetype TEXT NOT NULL,
		description TEXT NOT NULL,
		details TEXT,
		sourcechecksum INT NOT NULL,
		metadatachecksum INT NOT NULL,
		path TEXT NOT NULL,
		solvescript INTEGER NOT NULL,
		templatable INTEGER NOT NULL,
		maxusers INTEGER NOT NULL,
		category TEXT,
		points INTEGER NOT NULL
	);
	CREATE TABLE builds (
		id INTEGER PRIMARY KEY,
		flag TEXT NOT NULL,
		format TEXT NOT NULL,
		seed INTEGER NOT NULL,
		hasartifacts INTEGER NOT NULL,
		lastsolved INTEGER,
		challenge TEXT NOT NULL,
		schema TEXT NOT NULL,
		instancecount INT NOT NULL,
		UNIQUE(schema, format, challenge, seed),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
	);
	CREATE TABLE images (
		id INTEGER PRIMARY KEY,
		build INTEGER NOT NULL,
		host TEXT NOT NULL,
		FOREIGN KEY (build) REFERENCES builds (id)
	);`
	if _, err := seedDB.Exec(legacy); err != nil {
		t.Fatalf("failed to create legacy schema: %s", err)
	}
	if _, err := seedDB.Exec(
		`INSERT INTO challenges(id, name, namespace, challengetype, description, sourcechecksum, metadatachecksum, path, solvescript, templatable, maxusers, points)
		 VALUES ('test/legacy', 'Legacy', 'test', 'custom', '', ?, 0, '/tmp/legacy/problem.md', 0, 0, 0, 0);`,
		sourceChecksum); err != nil {
		t.Fatalf("failed to seed challenge: %s", err)
	}
	if _, err := seedDB.Exec(
		`INSERT INTO builds(id, flag, format, seed, hasartifacts, lastsolved, challenge, schema, instancecount)
		 VALUES (1, 'flag{x}', ?, 7, 0, 0, 'test/legacy', 'event', 1);`, format); err != nil {
		t.Fatalf("failed to seed build: %s", err)
	}
	if _, err := seedDB.Exec("INSERT INTO images(build, host) VALUES (1, 'challenge');"); err != nil {
		t.Fatalf("failed to seed image: %s", err)
	}
	seedDB.Close()

	mgr := new(Manager)
	mgr.log = newLogger(DISABLED)
	os.Setenv(DB_ENV, dbFile.Name())
	defer os.Unsetenv(DB_ENV)
	if err := mgr.initDatabase(); err != nil {
		t.Fatalf("initDatabase failed: %s", err)
	}
	defer mgr.db.Close()

	var got uint32
	if err := mgr.db.Get(&got, "SELECT checksum FROM builds WHERE id = 1;"); err != nil {
		t.Fatalf("failed to read migrated checksum: %s", err)
	}
	if want := contentChecksum(sourceChecksum, format); got != want {
		t.Errorf("migrated checksum = %#x, want %#x", got, want)
	}

	// The migrated row must round-trip through the normal lookup path.
	bMeta, err := mgr.lookupBuildMetadata(1)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}
	if bMeta.Checksum != contentChecksum(sourceChecksum, format) {
		t.Errorf("lookup returned checksum %#x, want %#x", bMeta.Checksum, contentChecksum(sourceChecksum, format))
	}
}
