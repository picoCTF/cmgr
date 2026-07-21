package cmgr

import "testing"

// TestRotatedPrevChecksum pins the retention rotation: the just-replaced
// generation becomes the rollback target when content changes, and an
// unchanged rebuild leaves the existing target alone.
func TestRotatedPrevChecksum(t *testing.T) {
	cases := []struct {
		name                          string
		oldChecksum, newChecksum, cur uint32
		want                          uint32
	}{
		{"content changed retains replaced generation", 0xA, 0xB, 0x9, 0xA},
		{"unchanged rebuild leaves target untouched", 0xA, 0xA, 0x9, 0x9},
		{"first build has no rollback target", 0, 0xB, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rotatedPrevChecksum(tc.oldChecksum, tc.newChecksum, tc.cur); got != tc.want {
				t.Errorf("rotatedPrevChecksum(%#x,%#x,%#x) = %#x, want %#x",
					tc.oldChecksum, tc.newChecksum, tc.cur, got, tc.want)
			}
		})
	}
}

// TestDisplacedPruneCandidate pins the --prune-old guard, including the
// A->B->A flip-flop case where the displaced generation equals the just-rebuilt
// current one and must NOT be pruned.
func TestDisplacedPruneCandidate(t *testing.T) {
	cases := []struct {
		name                            string
		oldChecksum, newChecksum, displ uint32
		want                            bool
	}{
		{"normal displacement is a candidate", 0xB, 0xC, 0xA, true},
		{"no content change is not a candidate", 0xB, 0xB, 0xA, false},
		{"nothing displaced is not a candidate", 0xB, 0xC, 0, false},
		{"flip-flop keeps the current generation", 0xB, 0xA, 0xA, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := displacedPruneCandidate(tc.oldChecksum, tc.newChecksum, tc.displ); got != tc.want {
				t.Errorf("displacedPruneCandidate(%#x,%#x,%#x) = %v, want %v",
					tc.oldChecksum, tc.newChecksum, tc.displ, got, tc.want)
			}
		})
	}
}

// retentionTestChallenge returns a minimal custom challenge with a known source
// checksum, suitable for openBuild/finalizeBuild and the migration backfill.
func retentionTestChallenge(id ChallengeId, sourceChecksum uint32) *ChallengeMetadata {
	return &ChallengeMetadata{
		Id:             id,
		Name:           "Retention",
		Namespace:      "test",
		ChallengeType:  "custom",
		SourceChecksum: sourceChecksum,
		Path:           "/tmp/test/problem.md",
		Hosts:          []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:        map[string]PortInfo{},
	}
}

// TestFinalizeBuildPersistsChecksums guards the durable half of the rotation
// contract: finalizeBuild must write both checksum and prevchecksum, so the
// refcount guard (which reads them back) sees the true generation pair. If the
// SET clauses were dropped, the round-tripped values would be wrong.
func TestFinalizeBuildPersistsChecksums(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	const format = "flag{%s}"
	challenge := retentionTestChallenge("test/finalize-checksums", 0x11112222)
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          7,
		Format:        format,
		Challenge:     "test/finalize-checksums",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}

	const wantChecksum = uint32(0xCAFEBABE)
	const wantPrev = uint32(0x0BADF00D)
	build.Flag = "flag{persisted}"
	build.Checksum = wantChecksum
	build.PrevChecksum = wantPrev
	build.Images = []Image{{Host: "challenge"}}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	got, err := mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}
	if got.Checksum != wantChecksum {
		t.Errorf("persisted checksum = %#x, want %#x", got.Checksum, wantChecksum)
	}
	if got.PrevChecksum != wantPrev {
		t.Errorf("persisted prevchecksum = %#x, want %#x", got.PrevChecksum, wantPrev)
	}
}

// TestOpenBuildStampsChecksum verifies that a freshly opened build already
// carries its content checksum (derived from the challenge source checksum and
// flag format), so an in-flight build is visible to contentReferenced before
// finalizeBuild runs.
func TestOpenBuildStampsChecksum(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	const format = "flag{%s}"
	const sourceChecksum = uint32(0x0102ABCD)
	challenge := retentionTestChallenge("test/openbuild-stamp", sourceChecksum)
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          3,
		Format:        format,
		Challenge:     "test/openbuild-stamp",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	if want := contentChecksum(sourceChecksum, format); build.Checksum != want {
		t.Errorf("openBuild stamped checksum %#x, want %#x", build.Checksum, want)
	}
}

// TestContentReferencedFailsSafeOnError verifies the documented safety
// property: on a query error contentReferenced returns true (keep the images)
// rather than false (delete them). Closing the database forces the error.
func TestContentReferencedFailsSafeOnError(t *testing.T) {
	mgr := setupTestManager(t)

	bMeta := &BuildMetadata{Challenge: "test/whatever", Seed: 1, Format: "flag{%s}", Checksum: 0x1234}
	mgr.db.Close() // subsequent queries error

	if !mgr.contentReferenced(bMeta, 0) {
		t.Error("expected contentReferenced to fail safe (true) on a query error")
	}
}

// TestContentReferencedIgnoresFormat verifies that two builds sharing
// (challenge, seed, checksum) but differing in flag format still count as
// referencing the same images: a docker tag is (challenge, seed, checksum,
// host), so format must not narrow the key.
func TestContentReferencedIgnoresFormat(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := retentionTestChallenge("test/format-agnostic", 0x55aa)
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	const seed = 9
	const checksum = uint32(0xFACEFEED)
	idA := insertTestBuild(t, mgr, "event-a", "test/format-agnostic", "flag{%s}", seed, checksum)
	insertTestBuild(t, mgr, "event-b", "test/format-agnostic", "ctf{%s}", seed, checksum)

	// A tag-identity query keyed on challenge+seed+checksum must see the
	// different-format sibling as a reference.
	shared := &BuildMetadata{Challenge: "test/format-agnostic", Seed: seed, Format: "flag{%s}", Checksum: checksum}
	if !mgr.contentReferenced(shared, idA) {
		t.Error("expected a different-format build with the same checksum to count as a reference")
	}
}

// TestMigrateBuildChecksumsResumable verifies that the backfill is data-driven
// and idempotent: a row left at checksum=0 after the column already exists (as
// an interrupted migration would leave it) is still backfilled on a later run,
// rather than being skipped forever by a column-existence guard.
func TestMigrateBuildChecksumsResumable(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	const format = "flag{%s}"
	const sourceChecksum = uint32(0x99887766)
	challenge := retentionTestChallenge("test/resumable", sourceChecksum)
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// A row stranded at checksum=0 despite the column already existing.
	id := insertTestBuild(t, mgr, "event", "test/resumable", format, 4, 0)

	// The backfill re-runs (m.cli is nil, so retagging is skipped) and must
	// resolve the stranded row.
	if err := mgr.migrateBuildChecksums(mgr.db); err != nil {
		t.Fatalf("migrateBuildChecksums failed: %s", err)
	}

	var got uint32
	if err := mgr.db.Get(&got, "SELECT checksum FROM builds WHERE id = ?;", id); err != nil {
		t.Fatalf("failed to read checksum: %s", err)
	}
	if want := contentChecksum(sourceChecksum, format); got != want {
		t.Errorf("resumed backfill checksum = %#x, want %#x", got, want)
	}

	// Idempotent: a second pass changes nothing and still succeeds.
	if err := mgr.migrateBuildChecksums(mgr.db); err != nil {
		t.Fatalf("second migrateBuildChecksums pass failed: %s", err)
	}
}
