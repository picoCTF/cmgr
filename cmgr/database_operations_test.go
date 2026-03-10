package cmgr

import (
	"encoding/json"
	"os"
	"testing"
	"time"
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

	errs = mgr.updateChallenges([]*ChallengeMetadata{challenge}, false)
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

// setupTestManager creates a Manager with a temporary on-disk database file for testing
func setupTestManager(t *testing.T) *Manager {
	t.Helper()

	dbFile, err := os.CreateTemp("", "cmgr-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	t.Cleanup(func() {
		os.Remove(dbFile.Name())
	})

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
