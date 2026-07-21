package cmgr

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// TestDatabaseChallengeRoundTrip tests adding, looking up, and removing challenges
func TestDatabaseChallengeRoundTrip(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Create a challenge metadata
	challenge := &ChallengeMetadata{
		Id:            "test/my-challenge",
		Name:          "My Challenge",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "A test challenge",
		Details:       "Some details",
		Hints:         []string{"hint1", "hint2"},
		Tags:          []string{"web", "easy"},
		Attributes:    map[string]string{"author": "tester"},
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Path:          "/tmp/test/problem.md",
		SolveScript:   true,
		Templatable:   false,
		MaxUsers:      0,
		Category:      "Web",
		Points:        100,
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}

	// Add challenge
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Look up challenge metadata
	got, err := mgr.lookupChallengeMetadata("test/my-challenge")
	if err != nil {
		t.Fatalf("lookupChallengeMetadata failed: %s", err)
	}

	if got.Id != "test/my-challenge" {
		t.Errorf("expected Id 'test/my-challenge', got %q", got.Id)
	}
	if got.Name != "My Challenge" {
		t.Errorf("expected Name 'My Challenge', got %q", got.Name)
	}
	if got.ChallengeType != "custom" {
		t.Errorf("expected ChallengeType 'custom', got %q", got.ChallengeType)
	}
	if got.Description != "A test challenge" {
		t.Errorf("expected Description 'A test challenge', got %q", got.Description)
	}
	if got.Category != "Web" {
		t.Errorf("expected Category 'Web', got %q", got.Category)
	}
	if got.Points != 100 {
		t.Errorf("expected Points 100, got %d", got.Points)
	}
	if len(got.Hints) != 2 || got.Hints[0] != "hint1" || got.Hints[1] != "hint2" {
		t.Errorf("unexpected Hints: %v", got.Hints)
	}
	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
	if got.Attributes["author"] != "tester" {
		t.Errorf("expected attribute author=tester, got %q", got.Attributes["author"])
	}

	// List challenges
	list, err := mgr.listChallenges()
	if err != nil {
		t.Fatalf("listChallenges failed: %s", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 challenge, got %d", len(list))
	}
	if list[0].Id != "test/my-challenge" {
		t.Errorf("expected Id 'test/my-challenge', got %q", list[0].Id)
	}

	// Search challenges
	searchResult, err := mgr.searchChallenges([]string{"web"})
	if err != nil {
		t.Fatalf("searchChallenges failed: %s", err)
	}
	if len(searchResult) != 1 {
		t.Fatalf("expected 1 result for tag 'web', got %d", len(searchResult))
	}

	searchResult, err = mgr.searchChallenges([]string{"nonexistent"})
	if err != nil {
		t.Fatalf("searchChallenges failed: %s", err)
	}
	if len(searchResult) != 0 {
		t.Errorf("expected 0 results for tag 'nonexistent', got %d", len(searchResult))
	}

	// Search with empty tags (should return all)
	searchResult, err = mgr.searchChallenges([]string{})
	if err != nil {
		t.Fatalf("searchChallenges(empty) failed: %s", err)
	}
	if len(searchResult) != 1 {
		t.Errorf("expected 1 result for empty tags, got %d", len(searchResult))
	}

	// Remove challenge
	err = mgr.removeChallenges([]*ChallengeMetadata{challenge})
	if err != nil {
		t.Fatalf("removeChallenges failed: %s", err)
	}

	list, err = mgr.listChallenges()
	if err != nil {
		t.Fatalf("listChallenges after removal failed: %s", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 challenges after removal, got %d", len(list))
	}
}

// TestDatabaseUpdateChallenge tests updating challenge metadata
func TestDatabaseUpdateChallenge(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := &ChallengeMetadata{
		Id:            "test/update-test",
		Name:          "Update Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Original description",
		Details:       "",
		Hints:         []string{"original hint"},
		Tags:          []string{"original"},
		Attributes:    map[string]string{"version": "1"},
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}

	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Update the challenge
	challenge.Description = "Updated description"
	challenge.Hints = []string{"new hint 1", "new hint 2", "new hint 3"}
	challenge.Tags = []string{"updated", "modified"}
	challenge.Attributes = map[string]string{"version": "2", "status": "active"}

	errs = mgr.updateChallenges([]*ChallengeMetadata{challenge}, false, false)
	if len(errs) > 0 {
		t.Fatalf("updateChallenges failed: %v", errs)
	}

	got, err := mgr.lookupChallengeMetadata("test/update-test")
	if err != nil {
		t.Fatalf("lookupChallengeMetadata failed: %s", err)
	}

	if got.Description != "Updated description" {
		t.Errorf("expected updated description, got %q", got.Description)
	}
	if len(got.Hints) != 3 {
		t.Errorf("expected 3 hints, got %d", len(got.Hints))
	}
	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
	if got.Attributes["version"] != "2" {
		t.Errorf("expected version=2, got %q", got.Attributes["version"])
	}
	if got.Attributes["status"] != "active" {
		t.Errorf("expected status=active, got %q", got.Attributes["status"])
	}
}

// TestDatabaseBuildLifecycle tests the build database operations
func TestDatabaseBuildLifecycle(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// First create a challenge
	challenge := &ChallengeMetadata{
		Id:            "test/build-test",
		Name:          "Build Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing builds",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}

	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Open a build
	build := &BuildMetadata{
		Flag:          "",
		Seed:          12345,
		Format:        "flag{%s}",
		Challenge:     "test/build-test",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}

	err := mgr.openBuild(build)
	if err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	if build.Id == 0 {
		t.Error("expected non-zero build ID")
	}

	// Finalize the build
	build.Flag = "flag{test_flag_123}"
	build.HasArtifacts = false
	build.LookupData = map[string]string{"key1": "val1"}
	build.Images = []Image{
		{Host: "challenge", Ports: []string{"8080/tcp"}},
	}

	err = mgr.finalizeBuild(build)
	if err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	// Look up the build
	got, err := mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}

	if got.Flag != "flag{test_flag_123}" {
		t.Errorf("expected flag 'flag{test_flag_123}', got %q", got.Flag)
	}
	if got.Seed != 12345 {
		t.Errorf("expected seed 12345, got %d", got.Seed)
	}
	if got.Challenge != "test/build-test" {
		t.Errorf("expected challenge 'test/build-test', got %q", got.Challenge)
	}
	if got.LookupData["key1"] != "val1" {
		t.Errorf("expected lookup key1=val1, got %q", got.LookupData["key1"])
	}
	if len(got.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(got.Images))
	}
	if got.Images[0].Host != "challenge" {
		t.Errorf("expected image host 'challenge', got %q", got.Images[0].Host)
	}

	// Look up a non-existent build
	_, err = mgr.lookupBuildMetadata(BuildId(99999))
	if err == nil {
		t.Error("expected error for non-existent build")
	}

	// Remove the build
	err = mgr.removeBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("removeBuildMetadata failed: %s", err)
	}

	_, err = mgr.lookupBuildMetadata(build.Id)
	if err == nil {
		t.Error("expected error after removing build")
	}
}

// TestDatabaseInstanceLifecycle tests instance database operations
func TestDatabaseInstanceLifecycle(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Create challenge and build first
	challenge := &ChallengeMetadata{
		Id:            "test/instance-test",
		Name:          "Instance Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing instances",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          1,
		Format:        "flag{%s}",
		Challenge:     "test/instance-test",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	err := mgr.openBuild(build)
	if err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}

	build.Flag = "flag{instance_test}"
	build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp"}}}
	build.LookupData = map[string]string{}
	err = mgr.finalizeBuild(build)
	if err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	// Open an instance
	instance := &InstanceMetadata{
		Build:      build.Id,
		Ports:      map[string]int{},
		Containers: []string{},
	}
	err = mgr.openInstance(instance)
	if err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}
	if instance.Id == 0 {
		t.Error("expected non-zero instance ID")
	}

	// Finalize the instance
	instance.Ports = map[string]int{"http": 8080}
	instance.Containers = []string{"container-abc123"}
	err = mgr.finalizeInstance(instance)
	if err != nil {
		t.Fatalf("finalizeInstance failed: %s", err)
	}

	// Look up the instance
	got, err := mgr.lookupInstanceMetadata(instance.Id)
	if err != nil {
		t.Fatalf("lookupInstanceMetadata failed: %s", err)
	}
	if got.Build != build.Id {
		t.Errorf("expected build ID %d, got %d", build.Id, got.Build)
	}
	if got.Ports["http"] != 8080 {
		t.Errorf("expected port http=8080, got %d", got.Ports["http"])
	}
	if len(got.Containers) != 1 || got.Containers[0] != "container-abc123" {
		t.Errorf("unexpected containers: %v", got.Containers)
	}

	// Verify created_at is populated as a non-nil *time.Time
	if got.CreatedAt == nil {
		t.Error("expected non-nil CreatedAt for new instance")
	} else {
		// Verify it is a recent time (within the last minute)
		elapsed := time.Since(*got.CreatedAt)
		if elapsed < 0 || elapsed > time.Minute {
			t.Errorf("CreatedAt %v is not recent (elapsed %v)", *got.CreatedAt, elapsed)
		}

		// Verify JSON serialization produces RFC3339 format
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("json.Marshal failed: %s", err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %s", err)
		}
		createdAtStr, ok := decoded["created_at"].(string)
		if !ok {
			t.Errorf("expected created_at to be a JSON string, got %T", decoded["created_at"])
		} else if _, err := time.Parse(time.RFC3339, createdAtStr); err != nil {
			t.Errorf("created_at %q is not RFC3339: %s", createdAtStr, err)
		}
	}

	// Get build instances
	instances, err := mgr.getBuildInstances(build.Id)
	if err != nil {
		t.Fatalf("getBuildInstances failed: %s", err)
	}
	if len(instances) != 1 || instances[0] != instance.Id {
		t.Errorf("unexpected build instances: %v", instances)
	}

	// Look up a non-existent instance
	_, err = mgr.lookupInstanceMetadata(InstanceId(99999))
	if err == nil {
		t.Error("expected error for non-existent instance")
	}

	// Remove container metadata
	err = mgr.removeContainersMetadata(instance)
	if err != nil {
		t.Fatalf("removeContainersMetadata failed: %s", err)
	}
	if len(instance.Containers) != 0 {
		t.Errorf("expected empty containers after removal, got %v", instance.Containers)
	}

	// Remove instance
	err = mgr.removeInstanceMetadata(instance.Id)
	if err != nil {
		t.Fatalf("removeInstanceMetadata failed: %s", err)
	}
}

// TestInstanceCreatedAtNullLegacy verifies that legacy instances with NULL created_at
// deserialize to nil *time.Time and serialize as JSON null.
func TestInstanceCreatedAtNullLegacy(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Set up a challenge and build as required by the instances foreign key
	challenge := &ChallengeMetadata{
		Id:            "test/null-created-at",
		Name:          "Null CreatedAt Challenge",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Test",
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
		Seed:          42,
		Format:        "flag{%s}",
		Challenge:     challenge.Id,
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{null_created_at}"
	build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp"}}}
	build.LookupData = map[string]string{}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	// Insert a legacy instance with NULL created_at directly
	res, err := mgr.db.Exec(
		"INSERT INTO instances(build, lastsolved, created_at) VALUES (?, 0, NULL)",
		build.Id,
	)
	if err != nil {
		t.Fatalf("failed to insert legacy instance: %s", err)
	}
	id, _ := res.LastInsertId()

	got, err := mgr.lookupInstanceMetadata(InstanceId(id))
	if err != nil {
		t.Fatalf("lookupInstanceMetadata failed: %s", err)
	}

	if got.CreatedAt != nil {
		t.Errorf("expected nil CreatedAt for legacy instance, got %v", got.CreatedAt)
	}

	// Verify JSON serialization produces null for a nil *time.Time
	data, jsonErr := json.Marshal(got)
	if jsonErr != nil {
		t.Fatalf("json.Marshal failed: %s", jsonErr)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %s", err)
	}
	if decoded["created_at"] != nil {
		t.Errorf("expected JSON null for nil CreatedAt, got %v", decoded["created_at"])
	}
}

// TestContainerOptionsDbRoundTrip tests serialization of ContainerOptions to/from database format
func TestContainerOptionsDbRoundTrip(t *testing.T) {
	opts := ContainerOptions{
		Init:            true,
		Cpus:            "0.5",
		Memory:          "256m",
		Ulimits:         []string{"nofile=1024:2048"},
		PidsLimit:       100,
		ReadonlyRootfs:  true,
		DroppedCaps:     []string{"CAP_NET_RAW", "CAP_SYS_CHROOT"},
		NoNewPrivileges: true,
		DiskQuota:       "1G",
		CgroupParent:    "/docker",
	}

	dbOpts, err := opts.toDbContainerOptions()
	if err != nil {
		t.Fatalf("toDbContainerOptions failed: %s", err)
	}

	if dbOpts.Init != true {
		t.Error("expected Init=true")
	}
	if dbOpts.Cpus != "0.5" {
		t.Errorf("expected Cpus='0.5', got %q", dbOpts.Cpus)
	}
	if dbOpts.Memory != "256m" {
		t.Errorf("expected Memory='256m', got %q", dbOpts.Memory)
	}
	if dbOpts.PidsLimit != 100 {
		t.Errorf("expected PidsLimit=100, got %d", dbOpts.PidsLimit)
	}

	// Verify JSON serialization of slices
	var ulimits []string
	if err := json.Unmarshal([]byte(dbOpts.Ulimits), &ulimits); err != nil {
		t.Fatalf("failed to unmarshal ulimits: %s", err)
	}
	if len(ulimits) != 1 || ulimits[0] != "nofile=1024:2048" {
		t.Errorf("unexpected ulimits: %v", ulimits)
	}

	var caps []string
	if err := json.Unmarshal([]byte(dbOpts.DroppedCaps), &caps); err != nil {
		t.Fatalf("failed to unmarshal dropped caps: %s", err)
	}
	if len(caps) != 2 {
		t.Errorf("expected 2 dropped caps, got %d", len(caps))
	}

	// Round-trip back
	roundTripped, err := newFromDbContainerOptions(dbOpts)
	if err != nil {
		t.Fatalf("newFromDbContainerOptions failed: %s", err)
	}

	if roundTripped.Init != opts.Init {
		t.Errorf("Init mismatch: %v vs %v", roundTripped.Init, opts.Init)
	}
	if roundTripped.Cpus != opts.Cpus {
		t.Errorf("Cpus mismatch: %q vs %q", roundTripped.Cpus, opts.Cpus)
	}
	if roundTripped.Memory != opts.Memory {
		t.Errorf("Memory mismatch: %q vs %q", roundTripped.Memory, opts.Memory)
	}
	if roundTripped.PidsLimit != opts.PidsLimit {
		t.Errorf("PidsLimit mismatch: %d vs %d", roundTripped.PidsLimit, opts.PidsLimit)
	}
	if roundTripped.ReadonlyRootfs != opts.ReadonlyRootfs {
		t.Errorf("ReadonlyRootfs mismatch: %v vs %v", roundTripped.ReadonlyRootfs, opts.ReadonlyRootfs)
	}
	if roundTripped.NoNewPrivileges != opts.NoNewPrivileges {
		t.Errorf("NoNewPrivileges mismatch: %v vs %v", roundTripped.NoNewPrivileges, opts.NoNewPrivileges)
	}
	if roundTripped.DiskQuota != opts.DiskQuota {
		t.Errorf("DiskQuota mismatch: %q vs %q", roundTripped.DiskQuota, opts.DiskQuota)
	}
	if roundTripped.CgroupParent != opts.CgroupParent {
		t.Errorf("CgroupParent mismatch: %q vs %q", roundTripped.CgroupParent, opts.CgroupParent)
	}
	if len(roundTripped.Ulimits) != len(opts.Ulimits) {
		t.Errorf("Ulimits length mismatch: %d vs %d", len(roundTripped.Ulimits), len(opts.Ulimits))
	}
	if len(roundTripped.DroppedCaps) != len(opts.DroppedCaps) {
		t.Errorf("DroppedCaps length mismatch: %d vs %d", len(roundTripped.DroppedCaps), len(opts.DroppedCaps))
	}
}

// TestContainerOptionsDbRoundTripEmptySlices tests that empty slices are handled correctly
func TestContainerOptionsDbRoundTripEmptySlices(t *testing.T) {
	opts := ContainerOptions{
		Ulimits:     []string{},
		DroppedCaps: []string{},
	}

	dbOpts, err := opts.toDbContainerOptions()
	if err != nil {
		t.Fatalf("toDbContainerOptions failed: %s", err)
	}

	roundTripped, err := newFromDbContainerOptions(dbOpts)
	if err != nil {
		t.Fatalf("newFromDbContainerOptions failed: %s", err)
	}

	if roundTripped.Ulimits == nil {
		t.Error("expected non-nil Ulimits")
	}
	if len(roundTripped.Ulimits) != 0 {
		t.Errorf("expected empty Ulimits, got %v", roundTripped.Ulimits)
	}
	if roundTripped.DroppedCaps == nil {
		t.Error("expected non-nil DroppedCaps")
	}
	if len(roundTripped.DroppedCaps) != 0 {
		t.Errorf("expected empty DroppedCaps, got %v", roundTripped.DroppedCaps)
	}
}

// TestDatabaseSchemaOperations tests schema-related database operations
func TestDatabaseSchemaOperations(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Create challenge
	challenge := &ChallengeMetadata{
		Id:            "test/schema-test",
		Name:          "Schema Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing schemas",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Schema should not exist yet
	exists, err := mgr.schemaExists("test-schema")
	if err != nil {
		t.Fatalf("schemaExists failed: %s", err)
	}
	if exists {
		t.Error("schema should not exist yet")
	}

	// Open a build with the schema
	build := &BuildMetadata{
		Seed:          42,
		Format:        "flag{%s}",
		Challenge:     "test/schema-test",
		Schema:        "test-schema",
		InstanceCount: 1,
	}
	err = mgr.openBuild(build)
	if err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}

	// Schema should exist now
	exists, err = mgr.schemaExists("test-schema")
	if err != nil {
		t.Fatalf("schemaExists failed: %s", err)
	}
	if !exists {
		t.Error("schema should exist after adding build")
	}

	// List schemas
	schemas, err := mgr.queryForSchemas()
	if err != nil {
		t.Fatalf("queryForSchemas failed: %s", err)
	}
	found := false
	for _, s := range schemas {
		if s == "test-schema" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'test-schema' in schema list, got %v", schemas)
	}

	// Get schema builds
	builds, err := mgr.getSchemaBuilds("test-schema")
	if err != nil {
		t.Fatalf("getSchemaBuilds failed: %s", err)
	}
	if len(builds) != 1 {
		t.Errorf("expected 1 build in schema, got %d", len(builds))
	}

	// Lock the schema
	err = mgr.lockSchema("test-schema")
	if err != nil {
		t.Fatalf("lockSchema failed: %s", err)
	}

	// Verify build is locked
	lockedBuild, err := mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}
	if lockedBuild.InstanceCount != LOCKED {
		t.Errorf("expected InstanceCount=%d (LOCKED), got %d", LOCKED, lockedBuild.InstanceCount)
	}
}

// TestDatabaseDumpState tests the state dump functionality
func TestDatabaseDumpState(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Empty state
	state, err := mgr.dumpState()
	if err != nil {
		t.Fatalf("dumpState failed: %s", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state, got %d challenges", len(state))
	}

	// Add a challenge
	challenge := &ChallengeMetadata{
		Id:            "test/dump-test",
		Name:          "Dump Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing dump",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	state, err = mgr.dumpState()
	if err != nil {
		t.Fatalf("dumpState failed: %s", err)
	}
	if len(state) != 1 {
		t.Errorf("expected 1 challenge in dump, got %d", len(state))
	}
	if state[0].Id != "test/dump-test" {
		t.Errorf("expected challenge Id 'test/dump-test', got %q", state[0].Id)
	}
}

// TestDatabasePortOperations tests port-related database operations
func TestDatabasePortOperations(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := &ChallengeMetadata{
		Id:            "test/port-test",
		Name:          "Port Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing ports",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{"http": {Host: "challenge", Port: 8080}},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Test reverse port map
	rpm, err := mgr.getReversePortMap("test/port-test")
	if err != nil {
		t.Fatalf("getReversePortMap failed: %s", err)
	}
	if rpm["8080/tcp"] != "http" {
		t.Errorf("expected reverse port map '8080/tcp' -> 'http', got %q", rpm["8080/tcp"])
	}

	// Test used port set (empty since no instances)
	portSet, err := mgr.usedPortSet()
	if err != nil {
		t.Fatalf("usedPortSet failed: %s", err)
	}
	if len(portSet) != 0 {
		t.Errorf("expected empty port set, got %v", portSet)
	}
}

// TestExplicitPortReadbackRedundant verifies the invariant that makes the
// post-start port read-back loop in startContainers safe to skip when CMGR_PORTS
// is set (m.portLow != 0). By the time startContainers runs, the explicit-port
// reservation loop in newInstance has already populated instance.Ports — for
// every exposed image port — with exactly the host port that Docker will bind.
// The read-back loop's only effect is
//
//	instance.Ports[revPortMap[cPort.String()]] = hPort
//
// which writes the SAME key (cPort.String() yields the same "<port>/tcp" form
// that getReversePortMap keys on and that startContainers builds its image port
// list with) and the SAME value (an explicit bind binds exactly the reserved
// port). So skipping it loses nothing. This mirrors the reservation flow rather
// than driving Docker, in keeping with the launch-path tests in
// concurrency_test.go.
func TestExplicitPortReadbackRedundant(t *testing.T) {
	const portLow = 41000
	const portHigh = 41010

	mgr := setupTestManager(t)
	defer mgr.db.Close()
	mgr.portLow = portLow
	mgr.portHigh = portHigh

	challenge := &ChallengeMetadata{
		Id:            "test/readback",
		Name:          "Readback",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing explicit-port read-back invariant",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap: map[string]PortInfo{
			"http": {Host: "challenge", Port: 8080},
			"ssh":  {Host: "challenge", Port: 22},
		},
		Tags:       []string{},
		Attributes: map[string]string{},
		Path:       "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{"": {}},
		},
	}
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          1,
		Format:        "flag{%s}",
		Challenge:     "test/readback",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{readback}"
	// "<port>/tcp" is exactly the form startContainers builds its image port
	// list with, and that cPort.String() would yield in the skipped read-back loop.
	build.Images = []Image{{Host: "challenge", Ports: []string{"8080/tcp", "22/tcp"}}}
	build.LookupData = map[string]string{}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	revPortMap, err := mgr.getReversePortMap(build.Challenge)
	if err != nil {
		t.Fatalf("getReversePortMap failed: %s", err)
	}

	instance := &InstanceMetadata{Build: build.Id, Ports: map[string]int{}, Containers: []string{}}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %s", err)
	}

	// Mirror the explicit-port reservation loop in newInstance, which runs
	// BEFORE startContainers.
	for _, image := range build.Images {
		if image.Host == "builder" {
			continue
		}
		for _, portStr := range image.Ports {
			portName := revPortMap[portStr]
			hostPort, err := mgr.reservePort(instance.Id, portName)
			if err != nil {
				t.Fatalf("reservePort(%q) failed: %s", portName, err)
			}
			instance.Ports[portName] = hostPort
		}
	}

	// Invariant 1: every exposed port is bound to a valid in-range reservation
	// that startContainers will publish (it reads instance.Ports[revPortMap[portStr]]).
	// If any were missing, the read-back would be needed and the skip unsafe.
	for _, portStr := range build.Images[0].Ports {
		name := revPortMap[portStr]
		if name == "" {
			t.Fatalf("no reverse-port-map entry for exposed port %q", portStr)
		}
		got := instance.Ports[name]
		if got < portLow || got > portHigh {
			t.Errorf("port %s (%s): reserved host port %d outside range [%d,%d]", portStr, name, got, portLow, portHigh)
		}
	}

	// Sanity: both ports present and distinct.
	if len(instance.Ports) != 2 {
		t.Errorf("expected 2 reserved ports, got %d: %v", len(instance.Ports), instance.Ports)
	}
	if instance.Ports["http"] == instance.Ports["ssh"] {
		t.Errorf("expected distinct ports for http and ssh, both = %d", instance.Ports["http"])
	}

	// Invariant 2: the gate startContainers uses agrees that this real
	// reservation-flow state is "known", so the read-back is skipped. (The gate's
	// edge cases — ephemeral ports, cleared/rebuild, missing entries — are covered
	// by TestPortsAlreadyKnown.)
	if !portsAlreadyKnown(mgr.portLow, build.Images[0].Ports, instance.Ports, revPortMap) {
		t.Error("portsAlreadyKnown = false with all ports reserved; read-back would not be skipped")
	}
}

// TestPortsAlreadyKnown covers the gate that decides whether startContainers can
// skip the post-start port read-back, including the edge cases that the
// reservation-flow test above cannot exercise directly.
func TestPortsAlreadyKnown(t *testing.T) {
	revPortMap := map[string]string{"8080/tcp": "http", "22/tcp": "ssh"}
	imagePorts := []string{"8080/tcp", "22/tcp"}

	tests := []struct {
		name      string
		portLow   int
		imgPorts  []string
		ports     map[string]int
		wantKnown bool
	}{
		{"explicit all reserved", 30000, imagePorts, map[string]int{"http": 30001, "ssh": 30002}, true},
		{"ephemeral ports (portLow==0)", 0, imagePorts, map[string]int{"http": 30001, "ssh": 30002}, false},
		{"explicit but ports cleared (rebuild)", 30000, imagePorts, map[string]int{}, false},
		{"explicit but one port missing", 30000, imagePorts, map[string]int{"http": 30001}, false},
		{"explicit but a port is zero", 30000, imagePorts, map[string]int{"http": 30001, "ssh": 0}, false},
		{"no published ports", 30000, []string{}, map[string]int{}, true},
		// A port missing from the reverse map resolves to name "". Even if a
		// non-zero port happens to sit under the "" key, the mapping is not known,
		// so the read-back must not be skipped.
		{"port missing from reverse map", 30000, []string{"9999/tcp"}, map[string]int{"": 30005}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := portsAlreadyKnown(tc.portLow, tc.imgPorts, tc.ports, revPortMap)
			if got != tc.wantKnown {
				t.Errorf("portsAlreadyKnown() = %v, want %v", got, tc.wantKnown)
			}
		})
	}
}

// setupPortAssignments is a helper that creates a challenge, build, and instance
// with the given ports assigned in the database, and returns the manager.
// portLow and portHigh configure the manager's port range for bitset tests.
func setupPortAssignments(t *testing.T, portLow, portHigh int, ports map[string]int) *Manager {
	t.Helper()
	mgr := setupTestManager(t)
	mgr.portLow = portLow
	mgr.portHigh = portHigh

	challenge := &ChallengeMetadata{
		Id:            "test/port-assign-test",
		Name:          "Port Assign Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing port assignments",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          1,
		Format:        "flag{%s}",
		Challenge:     "test/port-assign-test",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{port_assign}"
	build.Images = []Image{{Host: "challenge", Ports: []string{}}}
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
	instance.Ports = ports
	instance.Containers = []string{}

	savedPortLow := mgr.portLow
	mgr.portLow = 0
	if err := mgr.finalizeInstance(instance); err != nil {
		t.Fatalf("finalizeInstance failed: %s", err)
	}
	mgr.portLow = savedPortLow

	return mgr
}

// TestUsedPortBitset verifies that usedPortBitset returns a bitset that marks
// exactly the ports recorded in portAssignments within [portLow, portHigh].
func TestUsedPortBitset(t *testing.T) {
	// Use a small range so we can reason about exact bit positions.
	const portLow = 30000
	const portHigh = 30063 // exactly 64 ports → one uint64 word

	assignedPorts := map[string]int{
		"http":  30000, // bit 0
		"https": 30001, // bit 1
		"ssh":   30063, // bit 63
	}

	mgr := setupPortAssignments(t, portLow, portHigh, assignedPorts)
	defer mgr.db.Close()

	bitset, err := mgr.usedPortBitset()
	if err != nil {
		t.Fatalf("usedPortBitset failed: %s", err)
	}
	if len(bitset) == 0 {
		t.Fatal("expected non-empty bitset")
	}

	for name, port := range assignedPorts {
		p := port - portLow
		word := p / 64
		bit := uint(p) % 64
		if bitset[word]&(1<<bit) == 0 {
			t.Errorf("port %d (%s) should be marked in bitset but is not", port, name)
		}
	}

	// Verify that an unassigned port in the range is NOT marked.
	unassigned := 30002
	p := unassigned - portLow
	word := p / 64
	bit := uint(p) % 64
	if bitset[word]&(1<<bit) != 0 {
		t.Errorf("port %d should not be marked in bitset", unassigned)
	}
}

// TestUsedPortBitsetNoRange verifies that usedPortBitset returns nil when no
// port range is configured (portLow == 0).
func TestUsedPortBitsetNoRange(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()
	// portLow defaults to 0 from setupTestManager

	bitset, err := mgr.usedPortBitset()
	if err != nil {
		t.Fatalf("usedPortBitset failed: %s", err)
	}
	if bitset != nil {
		t.Errorf("expected nil bitset when portLow==0, got %v", bitset)
	}
}

// TestReservePortNoRange verifies that reservePort returns an error
// when no port range is configured.
func TestReservePortNoRange(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()
	// portLow defaults to 0

	_, err := mgr.reservePort(1, "test")
	if err == nil {
		t.Errorf("expected error when port reservation disabled, got nil")
	}
}

// TestReservePortWithRange verifies that reservePort returns a valid port
// within [portLow, portHigh] when the range is configured and ports are free.
func TestReservePortWithRange(t *testing.T) {
	const portLow = 40000
	const portHigh = 40010

	mgr := setupTestManager(t)
	defer mgr.db.Close()
	mgr.portLow = portLow
	mgr.portHigh = portHigh

	// Create challenge and build properly
	challenge := &ChallengeMetadata{
		Id: "test/range", Name: "Range", Namespace: "t", ChallengeType: "custom", Description: "d", Path: "/t/p",
		ChallengeOptions: ChallengeOptions{Overrides: map[string]ContainerOptions{"": {}}},
	}
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed: 1, Format: "flag{%s}", Challenge: "test/range", Schema: "s", InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %v", err)
	}

	instance := &InstanceMetadata{Build: build.Id}
	if err := mgr.openInstance(instance); err != nil {
		t.Fatalf("openInstance failed: %v", err)
	}

	port, err := mgr.reservePort(instance.Id, "test")
	if err != nil {
		t.Fatalf("reservePort failed: %s", err)
	}

	if port < portLow || port > portHigh {
		t.Errorf("returned port %d is outside range [%d, %d]", port, portLow, portHigh)
	}
}

// TestReservePortSkipsUsed verifies that reservePort does not return a port
// that is already recorded in portAssignments.
func TestReservePortSkipsUsed(t *testing.T) {
	const portLow = 50000
	const portHigh = 50002 // only 3 ports: 50000, 50001, 50002

	// Assign two of the three ports, leaving only 50002 free.
	assignedPorts := map[string]int{
		"svc1": 50000,
		"svc2": 50001,
	}

	mgr := setupPortAssignments(t, portLow, portHigh, assignedPorts)
	defer mgr.db.Close()

	// Need a dummy instance ID from setup
	var instId InstanceId
	if err := mgr.db.QueryRow("SELECT id FROM instances LIMIT 1").Scan(&instId); err != nil {
		t.Fatalf("failed to get instance id: %v", err)
	}

	port, err := mgr.reservePort(instId, "test")
	if err != nil {
		t.Fatalf("reservePort failed: %s", err)
	}

	if port < portLow || port > portHigh {
		t.Errorf("returned port %d is outside range [%d, %d]", port, portLow, portHigh)
	}
	for name, assigned := range assignedPorts {
		if port == assigned {
			t.Errorf("returned port %d (%s) is already assigned", port, name)
		}
	}
}

// TestReservePortAllUsed verifies that reservePort returns an error when all
// ports in the configured range are already assigned.
func TestReservePortAllUsed(t *testing.T) {
	const portLow = 60000
	const portHigh = 60001 // only 2 ports

	assignedPorts := map[string]int{
		"svc1": 60000,
		"svc2": 60001,
	}

	mgr := setupPortAssignments(t, portLow, portHigh, assignedPorts)
	defer mgr.db.Close()

	var instId InstanceId
	if err := mgr.db.QueryRow("SELECT id FROM instances LIMIT 1").Scan(&instId); err != nil {
		t.Fatalf("failed to get instance id: %v", err)
	}

	_, err := mgr.reservePort(instId, "test")
	if err == nil {
		t.Error("expected an error when all ports are in use, got nil")
	}
}

// TestLookupBuildInstances verifies that lookupBuildInstances returns instances with
// Ports and Containers correctly populated, and handles multiple instances per build.
// It also confirms equivalence with the getBuildInstances()+lookupInstanceMetadata loop
// it replaced.
func TestLookupBuildInstances(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	// Helper to set up a challenge and build
	setupBuild := func(challengeId ChallengeId, schema string) *BuildMetadata {
		t.Helper()
		challenge := &ChallengeMetadata{
			Id:            challengeId,
			Name:          string(challengeId),
			Namespace:     "test",
			ChallengeType: "custom",
			Description:   "lookup build instances test",
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
			Challenge:     challengeId,
			Schema:        schema,
			InstanceCount: DYNAMIC_INSTANCES,
		}
		if err := mgr.openBuild(build); err != nil {
			t.Fatalf("openBuild failed: %s", err)
		}
		build.Flag = "flag{test}"
		build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp"}}}
		build.LookupData = map[string]string{}
		if err := mgr.finalizeBuild(build); err != nil {
			t.Fatalf("finalizeBuild failed: %s", err)
		}
		return build
	}

	// Helper to open and finalize an instance with given ports and containers
	addInstance := func(build *BuildMetadata, ports map[string]int, containers []string) *InstanceMetadata {
		t.Helper()
		inst := &InstanceMetadata{Build: build.Id}
		if err := mgr.openInstance(inst); err != nil {
			t.Fatalf("openInstance failed: %s", err)
		}
		inst.Ports = ports
		inst.Containers = containers
		if err := mgr.finalizeInstance(inst); err != nil {
			t.Fatalf("finalizeInstance failed: %s", err)
		}
		return inst
	}

	t.Run("empty build returns no instances", func(t *testing.T) {
		build := setupBuild("test/lbi-empty", "lbi-empty")
		instances, err := mgr.lookupBuildInstances(build.Id)
		if err != nil {
			t.Fatalf("lookupBuildInstances failed: %s", err)
		}
		if len(instances) != 0 {
			t.Errorf("expected 0 instances, got %d", len(instances))
		}
	})

	t.Run("single instance with ports and containers", func(t *testing.T) {
		build := setupBuild("test/lbi-single", "lbi-single")
		inst := addInstance(build,
			map[string]int{"http": 8080, "https": 8443},
			[]string{"container-aaa", "container-bbb"},
		)

		instances, err := mgr.lookupBuildInstances(build.Id)
		if err != nil {
			t.Fatalf("lookupBuildInstances failed: %s", err)
		}
		if len(instances) != 1 {
			t.Fatalf("expected 1 instance, got %d", len(instances))
		}

		got := instances[0]
		if got.Id != inst.Id {
			t.Errorf("expected instance ID %d, got %d", inst.Id, got.Id)
		}
		if got.Build != build.Id {
			t.Errorf("expected build ID %d, got %d", build.Id, got.Build)
		}
		if got.Ports["http"] != 8080 {
			t.Errorf("expected Ports[http]=8080, got %d", got.Ports["http"])
		}
		if got.Ports["https"] != 8443 {
			t.Errorf("expected Ports[https]=8443, got %d", got.Ports["https"])
		}
		if len(got.Containers) != 2 {
			t.Errorf("expected 2 containers, got %d: %v", len(got.Containers), got.Containers)
		} else {
			containerSet := make(map[string]bool)
			for _, c := range got.Containers {
				containerSet[c] = true
			}
			if !containerSet["container-aaa"] || !containerSet["container-bbb"] {
				t.Errorf("expected containers [container-aaa, container-bbb], got %v", got.Containers)
			}
		}
	})

	t.Run("multiple instances each with distinct ports and containers", func(t *testing.T) {
		build := setupBuild("test/lbi-multi", "lbi-multi")

		inst1 := addInstance(build,
			map[string]int{"http": 9001},
			[]string{"c-alpha"},
		)
		inst2 := addInstance(build,
			map[string]int{"http": 9002, "debug": 9003},
			[]string{"c-beta", "c-gamma"},
		)
		inst3 := addInstance(build,
			map[string]int{},
			[]string{},
		)

		instances, err := mgr.lookupBuildInstances(build.Id)
		if err != nil {
			t.Fatalf("lookupBuildInstances failed: %s", err)
		}
		if len(instances) != 3 {
			t.Fatalf("expected 3 instances, got %d", len(instances))
		}

		// Build a map by ID for order-independent checking
		byID := make(map[InstanceId]*InstanceMetadata, len(instances))
		for _, inst := range instances {
			byID[inst.Id] = inst
		}

		// Instance 1
		got1, ok := byID[inst1.Id]
		if !ok {
			t.Fatalf("instance %d not found in results", inst1.Id)
		}
		if got1.Ports["http"] != 9001 {
			t.Errorf("inst1: expected Ports[http]=9001, got %d", got1.Ports["http"])
		}
		if len(got1.Containers) != 1 || got1.Containers[0] != "c-alpha" {
			t.Errorf("inst1: unexpected containers: %v", got1.Containers)
		}

		// Instance 2
		got2, ok := byID[inst2.Id]
		if !ok {
			t.Fatalf("instance %d not found in results", inst2.Id)
		}
		if got2.Ports["http"] != 9002 {
			t.Errorf("inst2: expected Ports[http]=9002, got %d", got2.Ports["http"])
		}
		if got2.Ports["debug"] != 9003 {
			t.Errorf("inst2: expected Ports[debug]=9003, got %d", got2.Ports["debug"])
		}
		if len(got2.Containers) != 2 {
			t.Errorf("inst2: expected 2 containers, got %d: %v", len(got2.Containers), got2.Containers)
		} else {
			containerSet := make(map[string]bool)
			for _, c := range got2.Containers {
				containerSet[c] = true
			}
			if !containerSet["c-beta"] || !containerSet["c-gamma"] {
				t.Errorf("inst2: expected containers [c-beta, c-gamma], got %v", got2.Containers)
			}
		}

		// Instance 3 (no ports, no containers)
		got3, ok := byID[inst3.Id]
		if !ok {
			t.Fatalf("instance %d not found in results", inst3.Id)
		}
		if len(got3.Ports) != 0 {
			t.Errorf("inst3: expected empty ports, got %v", got3.Ports)
		}
		if len(got3.Containers) != 0 {
			t.Errorf("inst3: expected empty containers, got %v", got3.Containers)
		}
	})

	t.Run("equivalence with getBuildInstances+lookupInstanceMetadata loop", func(t *testing.T) {
		build := setupBuild("test/lbi-equiv", "lbi-equiv")
		addInstance(build, map[string]int{"svc": 7001}, []string{"c-one"})
		addInstance(build, map[string]int{"svc": 7002}, []string{"c-two", "c-three"})

		// New batch approach
		batchInstances, err := mgr.lookupBuildInstances(build.Id)
		if err != nil {
			t.Fatalf("lookupBuildInstances failed: %s", err)
		}

		// Legacy loop approach: getBuildInstances + lookupInstanceMetadata per ID
		ids, err := mgr.getBuildInstances(build.Id)
		if err != nil {
			t.Fatalf("getBuildInstances failed: %s", err)
		}
		loopInstances := make([]*InstanceMetadata, 0, len(ids))
		for _, id := range ids {
			meta, err := mgr.lookupInstanceMetadata(id)
			if err != nil {
				t.Fatalf("lookupInstanceMetadata(%d) failed: %s", id, err)
			}
			loopInstances = append(loopInstances, meta)
		}

		if len(batchInstances) != len(loopInstances) {
			t.Fatalf("length mismatch: batch=%d loop=%d", len(batchInstances), len(loopInstances))
		}

		// Index batch results by ID
		batchByID := make(map[InstanceId]*InstanceMetadata, len(batchInstances))
		for _, inst := range batchInstances {
			batchByID[inst.Id] = inst
		}

		for _, loopInst := range loopInstances {
			batchInst, ok := batchByID[loopInst.Id]
			if !ok {
				t.Errorf("instance %d from loop not found in batch results", loopInst.Id)
				continue
			}
			if batchInst.Build != loopInst.Build {
				t.Errorf("instance %d: Build mismatch batch=%d loop=%d", loopInst.Id, batchInst.Build, loopInst.Build)
			}
			if len(batchInst.Ports) != len(loopInst.Ports) {
				t.Errorf("instance %d: Ports length mismatch batch=%d loop=%d", loopInst.Id, len(batchInst.Ports), len(loopInst.Ports))
			}
			for name, port := range loopInst.Ports {
				if batchInst.Ports[name] != port {
					t.Errorf("instance %d: Ports[%s] mismatch batch=%d loop=%d", loopInst.Id, name, batchInst.Ports[name], port)
				}
			}
			if len(batchInst.Containers) != len(loopInst.Containers) {
				t.Errorf("instance %d: Containers length mismatch batch=%d loop=%d", loopInst.Id, len(batchInst.Containers), len(loopInst.Containers))
			} else {
				loopContainerSet := make(map[string]bool, len(loopInst.Containers))
				for _, c := range loopInst.Containers {
					loopContainerSet[c] = true
				}
				for _, c := range batchInst.Containers {
					if !loopContainerSet[c] {
						t.Errorf("instance %d: batch container %q not found in loop result %v", loopInst.Id, c, loopInst.Containers)
					}
				}
			}
		}
	})
}

// removeDBFiles deletes a SQLite database file along with the -wal and -shm
// sidecar files that WAL mode can leave behind, so tests don't leak temp files.
func removeDBFiles(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
}

// setupTestManager creates a Manager with a temporary on-disk database file for testing
func setupTestManager(t *testing.T) *Manager {
	t.Helper()

	dbFile, err := os.CreateTemp("", "cmgr-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	t.Cleanup(func() { removeDBFiles(dbFile.Name()) })

	mgr := new(Manager)
	mgr.log = newLogger(DISABLED)
	os.Setenv(DB_ENV, dbFile.Name())
	defer os.Unsetenv(DB_ENV)

	err = mgr.initDatabase()
	if err != nil {
		t.Fatalf("initDatabase failed: %s", err)
	}

	return mgr
}

// openLegacyInstancesDB creates a temp database whose `instances` table omits
// the created_at and/or is_finalized columns, simulating a schema from before
// those columns existed. It also creates the portAssignments and containers
// tables (which hold ON DELETE CASCADE foreign keys into instances) and seeds a
// build, an instance, and one dependent row in each, so a rebuild's foreign-key
// safety can be verified.
func openLegacyInstancesDB(t *testing.T, withCreatedAt, withIsFinalized bool) *sqlx.DB {
	t.Helper()

	dbFile, err := os.CreateTemp("", "cmgr-migrate-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	t.Cleanup(func() { removeDBFiles(dbFile.Name()) })

	dsn := dbFile.Name() + "?_fk=true&_journal_mode=WAL&_busy_timeout=50&_synchronous=NORMAL"
	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}
	t.Cleanup(func() { db.Close() })

	instanceCols := "id INTEGER PRIMARY KEY, lastsolved INTEGER, build INTEGER NOT NULL"
	if withCreatedAt {
		instanceCols += ", created_at DATETIME DEFAULT CURRENT_TIMESTAMP"
	}
	if withIsFinalized {
		instanceCols += ", is_finalized INTEGER NOT NULL DEFAULT 0 CHECK(is_finalized IN (0,1))"
	}

	schema := `
	CREATE TABLE builds (id INTEGER PRIMARY KEY);
	CREATE TABLE instances (
		` + instanceCols + `,
		FOREIGN KEY (build) REFERENCES builds (id) ON UPDATE RESTRICT ON DELETE RESTRICT
	);
	CREATE INDEX instanceBuildIndex ON instances(build);
	CREATE TABLE portAssignments (
		instance INTEGER NOT NULL,
		name TEXT NOT NULL,
		port INTEGER NOT NULL,
		FOREIGN KEY (instance) REFERENCES instances (id) ON UPDATE RESTRICT ON DELETE CASCADE
	);
	CREATE TABLE containers (
		instance INTEGER NOT NULL,
		id TEXT NOT NULL PRIMARY KEY,
		FOREIGN KEY (instance) REFERENCES instances (id) ON UPDATE RESTRICT ON DELETE CASCADE
	);`
	if withCreatedAt {
		schema += "\n\tCREATE INDEX instanceCreatedAtIndex ON instances(created_at);"
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create legacy schema: %s", err)
	}

	seed := []string{
		"INSERT INTO builds(id) VALUES (1);",
		// created_at/is_finalized fall back to column defaults where present.
		"INSERT INTO instances(id, lastsolved, build) VALUES (1, 0, 1);",
		"INSERT INTO portAssignments(instance, name, port) VALUES (1, 'challenge', 12345);",
		"INSERT INTO containers(instance, id) VALUES (1, 'deadbeefcafe');",
	}
	for _, stmt := range seed {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed failed (%q): %s", stmt, err)
		}
	}

	return db
}

// TestRebuildInstancesTableMigration exercises the legacy-database migration
// path directly: it rebuilds an old `instances` table to the current schema and
// verifies the rows survive (including FK-dependent rows that ON DELETE CASCADE
// would otherwise destroy), the new columns are populated with the correct
// legacy semantics, and the restored DEFAULT CURRENT_TIMESTAMP works.
func TestRebuildInstancesTableMigration(t *testing.T) {
	cases := []struct {
		name           string
		hasCreatedAt   bool
		hasIsFinalized bool
	}{
		{"legacy v1.2.0 (neither column)", false, false},
		{"created_at only (pre-is_finalized)", true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openLegacyInstancesDB(t, tc.hasCreatedAt, tc.hasIsFinalized)

			// When created_at already exists, a row may legitimately hold NULL
			// (an "unknown" timestamp). The rebuild must copy it verbatim, not
			// rewrite it to now.
			if tc.hasCreatedAt {
				if _, err := db.Exec("INSERT INTO instances(id, lastsolved, build, created_at) VALUES (3, 0, 1, NULL);"); err != nil {
					t.Fatalf("seed NULL created_at instance: %s", err)
				}
			}

			if err := rebuildInstancesTable(db, tc.hasCreatedAt, tc.hasIsFinalized); err != nil {
				t.Fatalf("rebuildInstancesTable failed: %s", err)
			}

			// Both columns must exist after the rebuild.
			for _, col := range []string{"created_at", "is_finalized"} {
				var n int
				if err := db.Get(&n, "SELECT COUNT(*) FROM pragma_table_info('instances') WHERE name = ?;", col); err != nil {
					t.Fatalf("schema check for %s: %s", col, err)
				}
				if n != 1 {
					t.Errorf("expected column %s to exist after migration", col)
				}
			}

			// The instance row, and crucially its FK-dependent rows, must survive.
			// If the rebuild ran with foreign keys enforced, DROP TABLE's implicit
			// DELETE would have cascaded and wiped these.
			for _, check := range []struct {
				what  string
				query string
			}{
				{"instance", "SELECT COUNT(*) FROM instances WHERE id = 1;"},
				{"portAssignment", "SELECT COUNT(*) FROM portAssignments WHERE instance = 1;"},
				{"container", "SELECT COUNT(*) FROM containers WHERE instance = 1;"},
			} {
				var n int
				if err := db.Get(&n, check.query); err != nil {
					t.Fatalf("%s count: %s", check.what, err)
				}
				if n != 1 {
					t.Errorf("expected %s row to survive rebuild, found %d", check.what, n)
				}
			}

			// Legacy rows are treated as finalized and get a backfilled timestamp.
			var isFinalized int
			var createdAt *time.Time
			if err := db.QueryRow("SELECT is_finalized, created_at FROM instances WHERE id = 1;").Scan(&isFinalized, &createdAt); err != nil {
				t.Fatalf("read migrated row: %s", err)
			}
			if isFinalized != 1 {
				t.Errorf("expected legacy instance is_finalized=1, got %d", isFinalized)
			}
			if createdAt == nil {
				t.Error("expected backfilled created_at, got NULL")
			}

			// The DEFAULT CURRENT_TIMESTAMP must be restored: an insert omitting
			// created_at should still populate it.
			if _, err := db.Exec("INSERT INTO instances(id, lastsolved, build) VALUES (2, 0, 1);"); err != nil {
				t.Fatalf("insert new instance: %s", err)
			}
			var newCreatedAt *time.Time
			if err := db.QueryRow("SELECT created_at FROM instances WHERE id = 2;").Scan(&newCreatedAt); err != nil {
				t.Fatalf("read new row: %s", err)
			}
			if newCreatedAt == nil {
				t.Error("expected restored DEFAULT CURRENT_TIMESTAMP to populate created_at on insert")
			}

			// A pre-existing NULL created_at must be preserved, not backfilled.
			if tc.hasCreatedAt {
				var preserved *time.Time
				if err := db.QueryRow("SELECT created_at FROM instances WHERE id = 3;").Scan(&preserved); err != nil {
					t.Fatalf("read preserved-NULL row: %s", err)
				}
				if preserved != nil {
					t.Errorf("expected NULL created_at to survive rebuild as NULL, got %v", preserved)
				}
			}

			// Foreign-key enforcement must still hold after the rebuild: a
			// reference to a nonexistent instance should be rejected.
			if _, err := db.Exec("INSERT INTO portAssignments(instance, name, port) VALUES (999, 'ghost', 23456);"); err == nil {
				t.Error("expected foreign key violation after rebuild, insert succeeded")
			}
		})
	}
}

// TestInitDatabaseRepairsStaleIsFinalizedDefault drives initDatabase against a
// database in the buggy intermediate state: created_at and is_finalized both
// exist, but is_finalized was added by the old ALTER ... DEFAULT 1 migration.
// Because both columns are present, a column-existence-only trigger would skip
// the rebuild and leave new instances born finalized (so the unfinalized-launch
// GC never reclaims a crash). The migration must detect the non-canonical
// default, rebuild to DEFAULT 0, preserve existing rows, and make newly opened
// instances unfinalized.
func TestInitDatabaseRepairsStaleIsFinalizedDefault(t *testing.T) {
	dbFile, err := os.CreateTemp("", "cmgr-finalized-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	t.Cleanup(func() { removeDBFiles(dbFile.Name()) })

	dsn := dbFile.Name() + "?_fk=true&_journal_mode=WAL&_busy_timeout=50&_synchronous=NORMAL"
	seed, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("failed to open seed db: %s", err)
	}
	// Mirror the pre-rebuild schema: created_at present, is_finalized added with
	// the legacy DEFAULT 1. Seed one already-launched instance (default 1).
	// The builds stub carries the seed/format/challenge columns every real
	// pre-checksum database has, so the builds.checksum migration's backfill
	// query can run against it (its challenges join simply matches nothing).
	legacy := `
	CREATE TABLE builds (id INTEGER PRIMARY KEY, schema TEXT, seed INTEGER, format TEXT, challenge TEXT);
	CREATE TABLE instances (
		id INTEGER PRIMARY KEY,
		lastsolved INTEGER,
		build INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_finalized INTEGER NOT NULL DEFAULT 1 CHECK(is_finalized IN (0,1)),
		FOREIGN KEY (build) REFERENCES builds (id) ON UPDATE RESTRICT ON DELETE RESTRICT
	);
	INSERT INTO builds(id) VALUES (1);
	INSERT INTO instances(id, lastsolved, build) VALUES (1, 0, 1);`
	if _, err := seed.Exec(legacy); err != nil {
		t.Fatalf("failed to seed legacy schema: %s", err)
	}
	seed.Close()

	mgr := new(Manager)
	mgr.log = newLogger(DISABLED)
	os.Setenv(DB_ENV, dbFile.Name())
	defer os.Unsetenv(DB_ENV)
	if err := mgr.initDatabase(); err != nil {
		t.Fatalf("initDatabase failed: %s", err)
	}
	defer mgr.db.Close()

	// The default must have been normalized to the canonical 0.
	var dflt string
	if err := mgr.db.QueryRow("SELECT dflt_value FROM pragma_table_info('instances') WHERE name = 'is_finalized';").Scan(&dflt); err != nil {
		t.Fatalf("read is_finalized default: %s", err)
	}
	if dflt != "0" {
		t.Errorf("expected is_finalized DEFAULT 0 after migration, got %q", dflt)
	}

	// The pre-existing launched instance must survive with is_finalized=1.
	var existing int
	if err := mgr.db.QueryRow("SELECT is_finalized FROM instances WHERE id = 1;").Scan(&existing); err != nil {
		t.Fatalf("read existing instance: %s", err)
	}
	if existing != 1 {
		t.Errorf("expected existing instance to keep is_finalized=1, got %d", existing)
	}

	// A newly opened instance (is_finalized omitted, as openInstance does) must
	// now be born unfinalized so the GC can reclaim it if the launch crashes.
	if _, err := mgr.db.Exec("INSERT INTO instances(id, lastsolved, build) VALUES (2, 0, 1);"); err != nil {
		t.Fatalf("insert new instance: %s", err)
	}
	var fresh int
	if err := mgr.db.QueryRow("SELECT is_finalized FROM instances WHERE id = 2;").Scan(&fresh); err != nil {
		t.Fatalf("read new instance: %s", err)
	}
	if fresh != 0 {
		t.Errorf("expected new instance to default to is_finalized=0, got %d", fresh)
	}
}
