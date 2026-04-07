package cmgr

import (
	"testing"
	"time"
)

// setupPruneTestFixture creates a Manager, a challenge, a build with the
// given schema, and one instance.  It returns the Manager, BuildId, and
// InstanceId so callers can manipulate them directly.
func setupPruneTestFixture(t *testing.T, schema string) (*Manager, BuildId, InstanceId) {
	t.Helper()

	mgr := setupTestManager(t)
	t.Cleanup(func() { mgr.db.Close() })

	challenge := &ChallengeMetadata{
		Id:            "test/prune-test",
		Name:          "Prune Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing pruning",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{"": {}},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          1,
		Format:        "flag{%s}",
		Challenge:     "test/prune-test",
		Schema:        schema,
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{prune_test}"
	build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp"}}}
	build.LookupData = map[string]string{}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	instance := &InstanceMetadata{
		Build:      build.Id,
		Ports:      map[string]int{},
		Containers: []string{},
	}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}
	if err := mgr.finalizeInstance(instance); err != nil {
		t.Fatalf("finalizeInstance failed: %s", err)
	}

	return mgr, build.Id, instance.Id
}

// backdateInstance sets the created_at of the given instance to a timestamp
// older than the specified age relative to now.
func backdateInstance(t *testing.T, mgr *Manager, id InstanceId, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age).UTC().Format("2006-01-02 15:04:05")
	_, err := mgr.db.Exec("UPDATE instances SET created_at = ? WHERE id = ?", ts, id)
	if err != nil {
		t.Fatalf("backdateInstance failed: %s", err)
	}
}

// TestPruneRemovesOldManualInstances verifies that instances belonging to
// manual-schema builds whose created_at is older than pruneAge are deleted.
func TestPruneRemovesOldManualInstances(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "manual-prune-test")
	mgr.pruneAge = 1 * time.Hour

	// Make the instance appear 2 hours old.
	backdateInstance(t, mgr, iid, 2*time.Hour)

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	_, err := mgr.lookupInstanceMetadata(iid)
	if err == nil {
		t.Error("expected instance to be pruned, but it still exists")
	}
}

// TestPruneRetainsRecentManualInstances verifies that instances younger than
// pruneAge are left untouched.
func TestPruneRetainsRecentManualInstances(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "manual-prune-test")
	mgr.pruneAge = 1 * time.Hour

	// The instance was just created (seconds ago) — well within the 1 h window.
	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	if _, err := mgr.lookupInstanceMetadata(iid); err != nil {
		t.Errorf("expected recent instance to be retained, got error: %s", err)
	}
}

// TestPruneDoesNotAffectNonManualInstances verifies that instances belonging
// to builds whose schema does NOT start with "manual-" are never pruned, even
// when they are older than pruneAge.
func TestPruneDoesNotAffectNonManualInstances(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "schema-abc123")
	mgr.pruneAge = 1 * time.Hour

	// Backdate the instance to be older than pruneAge.
	backdateInstance(t, mgr, iid, 2*time.Hour)

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	if _, err := mgr.lookupInstanceMetadata(iid); err != nil {
		t.Errorf("expected non-manual instance to be retained, got error: %s", err)
	}
}

// TestPruneNullCreatedAt verifies that instances with a NULL created_at field
// (legacy rows) are treated as expired and pruned.
func TestPruneNullCreatedAt(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "manual-prune-test")
	mgr.pruneAge = 1 * time.Hour

	// Simulate a legacy row that has no created_at value.
	if _, err := mgr.db.Exec("UPDATE instances SET created_at = NULL WHERE id = ?", iid); err != nil {
		t.Fatalf("failed to NULL out created_at: %s", err)
	}

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	_, err := mgr.lookupInstanceMetadata(iid)
	if err == nil {
		t.Error("expected NULL-created_at instance to be pruned, but it still exists")
	}
}

// TestCheckPruneSkipsWhenDisabled verifies that checkPrune is a no-op when
// pruneAge is zero (i.e., pruning is disabled).
func TestCheckPruneSkipsWhenDisabled(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "manual-prune-test")
	mgr.pruneAge = 0 // disabled
	mgr.pruneInterval = 0

	// Backdate to ensure the instance would be pruned if pruning were active.
	backdateInstance(t, mgr, iid, 24*time.Hour)

	mgr.checkPrune()

	// Give the goroutine a moment in case it fired unexpectedly.
	time.Sleep(50 * time.Millisecond)

	if _, err := mgr.lookupInstanceMetadata(iid); err != nil {
		t.Errorf("pruning should be disabled (pruneAge=0), but instance was removed: %s", err)
	}
}

// TestCheckPruneSkipsWithinInterval verifies that checkPrune does not trigger
// a prune when the prune interval has not yet elapsed.
func TestCheckPruneSkipsWithinInterval(t *testing.T) {
	mgr, _, iid := setupPruneTestFixture(t, "manual-prune-test")
	mgr.pruneAge = 1 * time.Hour
	mgr.pruneInterval = 10 * time.Minute

	// Backdate so the instance would be prunable if the interval had elapsed.
	backdateInstance(t, mgr, iid, 2*time.Hour)

	// Record "now" as the last prune time so the interval hasn't elapsed.
	mgr.lastPruneUnix.Store(time.Now().UnixNano())

	mgr.checkPrune()

	// Allow a brief window for any unexpected goroutine to finish.
	time.Sleep(50 * time.Millisecond)

	if _, err := mgr.lookupInstanceMetadata(iid); err != nil {
		t.Errorf("prune should not have fired (interval not elapsed), but instance was removed: %s", err)
	}
}

// TestPruneGarbageCollectsStaleUnfinalizedInstances verifies that unfinalized
// instances (simulating a crashed launch) older than 5 minutes are deleted.
func TestPruneGarbageCollectsStaleUnfinalizedInstances(t *testing.T) {
	mgr, bid, _ := setupPruneTestFixture(t, "schema-gc-test")
	mgr.pruneAge = 1 * time.Hour

	instance := &InstanceMetadata{Build: bid}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}
	// intentionally not finalized — simulates a crashed launch

	backdateInstance(t, mgr, instance.Id, 6*time.Minute)

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	if _, err := mgr.lookupInstanceMetadata(instance.Id); err == nil {
		t.Error("expected stale unfinalized instance to be GC'd, but it still exists")
	}
}

// TestPruneRetainsRecentUnfinalizedInstances verifies that an unfinalized
// instance newer than 5 minutes is left alone (launch may still be in progress).
func TestPruneRetainsRecentUnfinalizedInstances(t *testing.T) {
	mgr, bid, _ := setupPruneTestFixture(t, "schema-gc-test")
	mgr.pruneAge = 1 * time.Hour

	instance := &InstanceMetadata{Build: bid}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}
	// not finalized, but just created — within the 5-minute window

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	if _, err := mgr.lookupInstanceMetadata(instance.Id); err != nil {
		t.Errorf("expected recent unfinalized instance to be retained, got error: %s", err)
	}
}

// TestPruneGCReleasesReservedPorts verifies that ports reserved before a
// crashed launch are freed when the unfinalized instance is GC'd.
func TestPruneGCReleasesReservedPorts(t *testing.T) {
	mgr, bid, _ := setupPruneTestFixture(t, "schema-gc-ports-test")
	mgr.pruneAge = 1 * time.Hour
	mgr.portLow = 10000
	mgr.portHigh = 20000

	instance := &InstanceMetadata{Build: bid}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}

	port, err := mgr.reservePort(instance.Id, "http")
	if err != nil {
		t.Fatalf("reservePort failed: %s", err)
	}

	backdateInstance(t, mgr, instance.Id, 6*time.Minute)

	if err := mgr.Prune(); err != nil {
		t.Fatalf("Prune() returned error: %s", err)
	}

	var count int
	if err := mgr.db.QueryRow("SELECT COUNT(*) FROM portAssignments WHERE port = ?", port).Scan(&count); err != nil {
		t.Fatalf("portAssignments query failed: %s", err)
	}
	if count != 0 {
		t.Errorf("expected port %d to be released after GC, but it still exists in portAssignments", port)
	}
}
