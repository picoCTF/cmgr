package cmgr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

const schemaQuery string = `
	CREATE TABLE IF NOT EXISTS challenges (
		id TEXT NOT NULL PRIMARY KEY,
		name TEXT NOT NULL,
		namespace TEXT NOT NULL,
		challengetype TEXT NOT NULL,
		description TEXT NOT NULL,
		details TEXT,
		sourcechecksum INT NOT NULL,
		metadatachecksum INT NOT NULL,
		path TEXT NOT NULL,
		solvescript INTEGER NOT NULL CHECK(solvescript == 0 OR solvescript == 1),
		templatable INTEGER NOT NULL CHECK(templatable == 0 OR templatable == 1),
		maxusers INTEGER NOT NULL CHECK(maxusers >= 0),
		category TEXT,
		points INTEGER NOT NULL CHECK(points >= 0)
	);

	CREATE TABLE IF NOT EXISTS hints (
		challenge TEXT NOT NULL,
		idx INT NOT NULL,
		hint TEXT NOT NULL,
		PRIMARY KEY (challenge, idx),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tags (
		challenge TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY (challenge, tag),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS tagIndex ON tags(LOWER(tag));

	CREATE TABLE IF NOT EXISTS attributes (
		challenge TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (challenge, key),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS attributeIndex ON attributes(LOWER(key));

	CREATE TABLE IF NOT EXISTS hosts (
		challenge TEXT NOT NULL,
		name TEXT NOT NULL,
		idx INT NOT NULL,
		target TEXT NOT NULL,
		PRIMARY KEY (challenge, name),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
		    ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS hostsIndex ON hosts(challenge);

	CREATE TABLE IF NOT EXISTS portNames (
		challenge TEXT NOT NULL,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER NOT NULL CHECK (port > 0 AND port < 65536),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE CASCADE ON DELETE CASCADE,
		FOREIGN KEY (challenge, host) REFERENCES hosts (challenge, name)
		    ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS portNamesIndex ON portNames(challenge);

	CREATE TABLE IF NOT EXISTS builds (
		id INTEGER PRIMARY KEY,
		flag TEXT NOT NULL,
		format TEXT NOT NULL,
		seed INTEGER NOT NULL,
		hasartifacts INTEGER NOT NULL CHECK (hasartifacts = 0 OR hasartifacts = 1),
		lastsolved INTEGER,
		challenge TEXT NOT NULL,
		schema TEXT NOT NULL,
		instancecount INT NOT NULL,
		UNIQUE(schema, format, challenge, seed),
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE RESTRICT ON DELETE RESTRICT
	);

	CREATE INDEX IF NOT EXISTS schemaIndex on builds(schema);

	CREATE TABLE IF NOT EXISTS images (
		id INTEGER PRIMARY KEY,
		build INTEGER NOT NULL,
		host TEXT NOT NULL,
		FOREIGN KEY (build) REFERENCES builds (id)
		    ON UPDATE RESTRICT ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS imagePorts (
		image INTEGER NOT NULL,
		port TEXT NOT NULL,
		FOREIGN KEY (image) REFERENCES images (id)
			ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS lookupData (
		build INTEGER NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		FOREIGN KEY (build) REFERENCES builds (id)
			ON UPDATE RESTRICT ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS instances (
		id INTEGER PRIMARY KEY,
		lastsolved INTEGER,
		build INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_finalized INTEGER NOT NULL DEFAULT 0 CHECK(is_finalized IN (0,1)),
		FOREIGN KEY (build) REFERENCES builds (id)
			ON UPDATE RESTRICT ON DELETE RESTRICT
	);

	CREATE TABLE IF NOT EXISTS portAssignments (
		instance INTEGER NOT NULL,
		name TEXT NOT NULL,
		port INTEGER NOT NULL CHECK (port > 0 AND port < 65536),
		FOREIGN KEY (instance) REFERENCES instances (id)
			ON UPDATE RESTRICT ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS containers (
		instance INTEGER NOT NULL,
		id TEXT NOT NULL PRIMARY KEY,
		FOREIGN KEY (instance) REFERENCES instances (id)
			ON UPDATE RESTRICT ON DELETE CASCADE
	);

	-- There are currently not any network-level challenge options, so this table is not created.
	-- However, this is kept as a placeholder in case additional options are added in the future.
	--
	-- CREATE TABLE IF NOT EXISTS networkOptions (
	--	challenge INTEGER NOT NULL,
	--	FOREIGN KEY (challenge) REFERENCES challenges (id)
	--		ON UPDATE CASCADE ON DELETE CASCADE
	--);

	CREATE TABLE IF NOT EXISTS containerOptions (
		challenge INTEGER NOT NULL,
		host TEXT NOT NULL,
		init INTEGER NOT NULL CHECK(init == 0 OR init == 1),
		cpus TEXT NOT NULL,
		memory TEXT NOT NULL,
		ulimits TEXT NOT NULL,
		pidslimit INTEGER NOT NULL,
		readonlyrootfs INTEGER NOT NULL CHECK(readonlyrootfs == 0 OR readonlyrootfs == 1),
		droppedcaps TEXT NOT NULL,
		nonewprivileges INTEGER NOT NULL CHECK(nonewprivileges == 0 OR nonewprivileges == 1),
		diskquota TEXT NOT NULL,
		cgroupparent TEXT NOT NULL,
		capimmutable INTEGER NOT NULL CHECK(capimmutable == 0 OR capimmutable == 1) DEFAULT 0,
		FOREIGN KEY (challenge) REFERENCES challenges (id)
			ON UPDATE CASCADE ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS instanceBuildIndex ON instances(build);
	CREATE INDEX IF NOT EXISTS portAssignmentInstanceIndex ON portAssignments(instance);
	CREATE INDEX IF NOT EXISTS portAssignmentPortIndex ON portAssignments(port);
	CREATE INDEX IF NOT EXISTS containerInstanceIndex ON containers(instance);
	CREATE INDEX IF NOT EXISTS imageBuildIndex ON images(build);
	CREATE INDEX IF NOT EXISTS imagePortImageIndex ON imagePorts(image);
	CREATE INDEX IF NOT EXISTS lookupDataBuildIndex ON lookupData(build);
`

// Connects to the desired database (creating it if it does not exist) and then
// ensures that the necessary tables and indexes exist and that the sqlite
// engine is enforcing foreign key constraints.
func (m *Manager) initDatabase() error {
	dbPath, isSet := os.LookupEnv(DB_ENV)
	if !isSet {
		dbPath = "cmgr.db"
	}

	// _busy_timeout=50 gives SQLite up to 50ms to retry acquiring a lock before
	// returning SQLITE_BUSY; avoids instant failures under concurrent access.
	// In WAL mode, _synchronous=NORMAL preserves database consistency but can lose
	// the most recent committed transactions on a crash or power loss (potentially
	// more than one), in exchange for better performance than FULL.
	dsn := dbPath + "?_fk=true&_journal_mode=WAL&_busy_timeout=50&_synchronous=NORMAL"
	if walEnv, ok := os.LookupEnv(DB_WAL_ENV); ok && (walEnv == "false" || walEnv == "0" || walEnv == "off") {
		dsn = dbPath + "?_fk=true&_busy_timeout=50"
	}

	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		m.log.errorf("could not open database: %s", err)
		return err
	}

	// File exists and is a valid sqlite database
	m.dbPath = dbPath

	_, err = db.Exec(schemaQuery)
	if err != nil {
		m.log.errorf("could not set database schema: %s", err)
		return err
	}
	// Migrate older DBs: add capimmutable if it is not already present.
	var capimmutableColumnCount int
	err = db.QueryRow("SELECT COUNT(1) FROM pragma_table_info('containerOptions') WHERE name='capimmutable';").Scan(&capimmutableColumnCount)
	if err != nil {
		m.log.errorf("could not inspect containerOptions schema: %s", err)
		return err
	}
	if capimmutableColumnCount == 0 {
		_, err = db.Exec("ALTER TABLE containerOptions ADD COLUMN capimmutable INTEGER NOT NULL DEFAULT 0;")
		if err != nil {
			m.log.errorf("could not migrate containerOptions.capimmutable column: %s", err)
			return err
		}
	}

	// Bring an older `instances` table up to the current schema. created_at
	// (DEFAULT CURRENT_TIMESTAMP) and is_finalized were both added after the
	// original release. SQLite cannot ALTER ... ADD COLUMN a non-constant
	// default such as CURRENT_TIMESTAMP, so any DB missing either column is
	// migrated by rebuilding the table to match the canonical schema exactly.
	var createdAtCols, isFinalizedCols int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('instances') WHERE name = 'created_at';").Scan(&createdAtCols)
	if err != nil {
		m.log.errorf("could not check instances table schema: %s", err)
		return err
	}
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('instances') WHERE name = 'is_finalized';").Scan(&isFinalizedCols)
	if err != nil {
		m.log.errorf("could not check instances table schema for is_finalized: %s", err)
		return err
	}

	// A pre-rebuild migration added is_finalized via ALTER ... DEFAULT 1, so on
	// those upgraded DBs openInstance (which omits is_finalized) creates rows
	// that are born finalized — the unfinalized-launch GC then never reclaims a
	// crashed launch. The canonical schema uses DEFAULT 0, so a non-zero default
	// also triggers a rebuild to restore it.
	var isFinalizedDefault sql.NullString
	if isFinalizedCols > 0 {
		err = db.QueryRow("SELECT dflt_value FROM pragma_table_info('instances') WHERE name = 'is_finalized';").Scan(&isFinalizedDefault)
		if err != nil {
			m.log.errorf("could not check instances.is_finalized default: %s", err)
			return err
		}
	}
	staleIsFinalizedDefault := isFinalizedCols > 0 && isFinalizedDefault.String != "0"

	if createdAtCols == 0 || isFinalizedCols == 0 || staleIsFinalizedDefault {
		if err = rebuildInstancesTable(db, createdAtCols > 0, isFinalizedCols > 0); err != nil {
			m.log.errorf("could not migrate instances table: %s", err)
			return err
		}
	}

	// instanceCreatedAtIndex is created here rather than in schemaQuery, so it
	// must be ensured for fresh databases too (the rebuild path recreates it).
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS instanceCreatedAtIndex ON instances(created_at);")
	if err != nil {
		m.log.errorf("could not create instanceCreatedAtIndex index: %s", err)
		return err
	}

	var fkeysEnforced bool
	err = db.QueryRow("PRAGMA foreign_keys;").Scan(&fkeysEnforced)
	if err != nil {
		m.log.errorf("could not check for foreign key support: %s", err)
		return err
	}

	if !fkeysEnforced {
		m.log.errorf("foreign keys not enabled")
		return errors.New("foreign keys not enabled")
	}

	m.db = db

	// Seed the per-Manager RNG if not already initialized (e.g., when called
	// directly in tests without going through NewManager).
	if m.rand == nil {
		m.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return nil
}

// rebuildInstancesTable recreates the `instances` table with the canonical
// schema and copies existing rows across. It migrates databases that predate
// the created_at and/or is_finalized columns, which cannot simply be added via
// ALTER TABLE ... ADD COLUMN because created_at's DEFAULT CURRENT_TIMESTAMP is a
// non-constant default that SQLite rejects in ADD COLUMN.
//
// `instances` is the target of ON DELETE CASCADE foreign keys (portAssignments,
// containers), so the implicit DELETE that DROP TABLE performs would cascade and
// destroy those rows if foreign keys were enforced. The rebuild therefore runs
// with foreign_keys disabled. Because that pragma is per-connection and cannot
// change inside a transaction, the whole operation is pinned to a single
// connection: disable FKs, rebuild in a transaction, verify integrity with
// foreign_key_check, commit, then re-enable FKs.
func rebuildInstancesTable(db *sqlx.DB, hasCreatedAt, hasIsFinalized bool) (retErr error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF;"); err != nil {
		return err
	}
	// Restore enforcement before the connection returns to the pool (LIFO: runs
	// before conn.Close). go-sqlite3 sets _fk only at open, not on checkout, so
	// a connection left with foreign_keys=OFF would silently skip enforcement on
	// reuse. Surface a re-enable failure so initDatabase aborts instead.
	defer func() {
		if _, e := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON;"); e != nil && retErr == nil {
			retErr = fmt.Errorf("could not re-enable foreign keys after rebuilding instances table: %w", e)
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const createNew = `CREATE TABLE instances_migrate_new (
		id INTEGER PRIMARY KEY,
		lastsolved INTEGER,
		build INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_finalized INTEGER NOT NULL DEFAULT 0 CHECK(is_finalized IN (0,1)),
		FOREIGN KEY (build) REFERENCES builds (id)
			ON UPDATE RESTRICT ON DELETE RESTRICT
	);`
	if _, err = tx.ExecContext(ctx, createNew); err != nil {
		return err
	}

	// Synthesize values only for columns the old table lacks: backfill a missing
	// created_at to now, and treat legacy instances as finalized (they were
	// already launched), matching the semantics of the original incremental
	// migration. When created_at already exists, copy it verbatim — preserving
	// NULL, which the rest of the code treats as a valid "unknown" timestamp
	// rather than rewriting it to now.
	createdExpr := "CURRENT_TIMESTAMP"
	if hasCreatedAt {
		createdExpr = "created_at"
	}
	finalizedExpr := "1"
	if hasIsFinalized {
		finalizedExpr = "is_finalized"
	}
	copyStmt := fmt.Sprintf(
		"INSERT INTO instances_migrate_new (id, lastsolved, build, created_at, is_finalized) "+
			"SELECT id, lastsolved, build, %s, %s FROM instances;",
		createdExpr, finalizedExpr,
	)
	if _, err = tx.ExecContext(ctx, copyStmt); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, "DROP TABLE instances;"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "ALTER TABLE instances_migrate_new RENAME TO instances;"); err != nil {
		return err
	}

	// Recreate the indexes that were dropped along with the old table.
	if _, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS instanceBuildIndex ON instances(build);"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS instanceCreatedAtIndex ON instances(created_at);"); err != nil {
		return err
	}

	// Confirm the rebuild left no dangling foreign-key references before
	// committing (rows were copied with enforcement off).
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return err
	}
	var violation string
	if rows.Next() {
		// foreign_key_check yields: table, rowid, parent (referenced table), fkid.
		var (
			table  string
			rowid  sql.NullInt64
			parent string
			fkid   int
		)
		if err = rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			rows.Close()
			return err
		}
		rowidStr := "NULL"
		if rowid.Valid {
			rowidStr = strconv.FormatInt(rowid.Int64, 10)
		}
		violation = fmt.Sprintf("row %s in %q references a missing row in %q (fk %d)", rowidStr, table, parent, fkid)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if violation != "" {
		return fmt.Errorf("foreign key check failed after rebuilding instances table: %s", violation)
	}

	return tx.Commit()
}

func (m *Manager) getReversePortMap(id ChallengeId) (map[string]string, error) {
	rpm := make(map[string]string)

	res := []struct {
		Name string
		Port int
	}{}

	err := m.db.Select(&res, `SELECT name, port FROM portNames WHERE challenge=?;`, id)
	if err != nil {
		m.log.errorf("could not get challenge ports: %s", err)
		return nil, err
	}

	for _, entry := range res {
		rpm[fmt.Sprintf("%d/tcp", entry.Port)] = entry.Name
	}

	m.log.debugf("reverse port map for %s: %v", id, rpm)

	return rpm, nil
}

func (m *Manager) usedPortSet() (map[int]struct{}, error) {
	var ports []int
	err := m.db.Select(&ports, "SELECT port FROM portAssignments;")

	portSet := make(map[int]struct{})
	for _, port := range ports {
		portSet[port] = struct{}{}
	}

	return portSet, err
}

func (m *Manager) usedPortBitset() ([]uint64, error) {
	if m.portLow == 0 {
		return nil, nil
	}

	numPorts := m.portHigh - m.portLow + 1
	bitset := make([]uint64, (numPorts+63)/64)

	rows, err := m.db.Query("SELECT port FROM portAssignments WHERE port BETWEEN ? AND ?", m.portLow, m.portHigh)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		p := port - m.portLow
		bitset[p/64] |= (1 << (uint(p) % 64))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return bitset, nil
}

func (m *Manager) safeToRefresh(new *ChallengeMetadata) bool {
	old, err := m.lookupChallengeMetadata(new.Id)
	if err != nil {
		return false
	}

	sameType := old.ChallengeType == new.ChallengeType
	sameOptions := reflect.DeepEqual(old.ChallengeOptions, new.ChallengeOptions)

	safe := sameType && sameOptions

	return safe
}

func (m *Manager) dumpState() ([]*ChallengeMetadata, error) {
	challenges, err := m.listChallenges()
	if err != nil {
		return nil, err
	}

	for i, challenge := range challenges {
		meta, err := m.lookupChallengeMetadata(challenge.Id)
		if err != nil {
			return nil, err
		}

		meta.Builds = []*BuildMetadata{}
		err = m.db.Select(&meta.Builds, "SELECT id FROM builds WHERE challenge=?", challenge.Id)
		if err != nil {
			m.log.errorf("failed to select builds for '%s': %s", challenge.Id, err)
			return nil, err
		}

		for j, build := range meta.Builds {
			bMeta, err := m.lookupBuildMetadata(build.Id)
			if err != nil {
				return nil, err
			}

			bMeta.Instances, err = m.lookupBuildInstances(bMeta.Id)
			if err != nil {
				m.log.errorf("failed to select instances for '%s/%d': %s", challenge.Id, bMeta.Id, err)
				return nil, err
			}
			meta.Builds[j] = bMeta
		}
		challenges[i] = meta
	}
	return challenges, nil
}
