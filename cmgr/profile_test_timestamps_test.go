package cmgr

import (
	"testing"
	"time"
)

// setTimestamp is a shared helper to update the created_at field for an instance.
func setTimestamp(tb testing.TB, mgr *Manager, id InstanceId, base time.Time, offset int) {
	tb.Helper()
	// Add offset seconds to base time and format for SQLite
	ts := base.Add(time.Duration(offset) * time.Second).UTC().Format("2006-01-02 15:04:05")
	_, err := mgr.db.Exec("UPDATE instances SET created_at = ? WHERE id = ?", ts, id)
	if err != nil {
		tb.Fatalf("failed to set timestamp for instance %d: %s", id, err)
	}
}
