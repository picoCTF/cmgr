package cmgr

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// setupProfileTestManager creates a Manager with a temporary on-disk database file for profiling tests
func setupProfileTestManager(t *testing.T) *Manager {
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

	// Set port range to test bitset optimization (10000-63000)
	// This ensures getFreePort uses the bitset logic instead of ephemeral ports
	mgr.portLow = 10000
	mgr.portHigh = 63000

	return mgr
}

// BenchmarkInstanceLaunchWithDBLoad profiles the performance of instance launch
// with varying database sizes (20, 200, 2K, 20K instances).
//
// This benchmark measures time up to the point where Docker commands would be sent,
// excluding actual Docker operations.
func TestProfileInstanceLaunchWithDBLoad(t *testing.T) {
	if os.Getenv("CMGR_TEST_PROFILE") == "" {
		t.Skip("skipping profiling test; set CMGR_TEST_PROFILE=1 to run")
	}

	// Test scenarios with different database loads
	scenarios := []struct {
		name  string
		count int
	}{
		{"20_instances", 20},
		{"200_instances", 200},
		{"20000_instances", 20000},
		{"50000_instances", 50000},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			profileInstanceLaunch(t, scenario.count)
		})
	}
}

// profileInstanceLaunch sets up a manager with the specified number of instances
// in the database, then measures the time to prepare a new instance launch
// (up to the point where Docker commands would be sent).
func profileInstanceLaunch(t *testing.T, instanceCount int) {
	mgr := setupProfileTestManager(t)
	defer mgr.db.Close()

	t.Logf("Setting up test with %d instances in database", instanceCount)
	setupStart := time.Now()

	// Create a challenge
	challenge := &ChallengeMetadata{
		Id:            "test/profile-test",
		Name:          "Profile Test",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "Testing performance",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{"http": {Host: "challenge", Port: 8080}},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {
					Cpus:   "1.0",
					Memory: "512m",
				},
			},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	// Create a build using manual- schema so instances are subject to pruning rules
	build := &BuildMetadata{
		Seed:          42,
		Format:        "flag{%s}",
		Challenge:     "test/profile-test",
		Schema:        "manual-profile-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}

	build.Flag = "flag{profile_test}"
	build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp", "443/tcp"}}}
	build.LookupData = map[string]string{"key1": "value1", "key2": "value2"}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	// Populate database with the specified number of instances
	now := time.Now().UTC()
	t.Logf("Populating database with %d instances...", instanceCount)

	// Use batched inserts for better performance during setup
	batchSize := 1000
	for i := 0; i < instanceCount; i++ {
		instance := &InstanceMetadata{
			Build:      build.Id,
			Ports:      map[string]int{"http": 8000 + (i % 10000)},
			Containers: []string{fmt.Sprintf("container-%d", i)},
		}

		if err := mgr.openInstance(instance); err != nil {
			t.Fatalf("openInstance %d failed: %s", i, err)
		}

		setTimestamp(t, mgr, instance.Id, now, i)

		if err := mgr.finalizeInstance(instance); err != nil {
			t.Fatalf("finalizeInstance %d failed: %s", i, err)
		}

		if (i+1)%batchSize == 0 || i == instanceCount-1 {
			t.Logf("  Created %d/%d instances...", i+1, instanceCount)
		}
	}

	setupDuration := time.Since(setupStart)
	t.Logf("Setup completed in %v", setupDuration)

	// Verify instance count
	var count int
	err := mgr.db.Get(&count, "SELECT COUNT(*) FROM instances")
	if err != nil {
		t.Fatalf("failed to count instances: %s", err)
	}
	t.Logf("Database contains %d instances", count)

	// Now profile the instance launch preparation phase
	// Run multiple iterations to test caching behavior
	t.Logf("\n=== Starting profiling run (with cache warming) ===")

	const numIterations = 10
	var (
		step1Durations  = make([]time.Duration, numIterations)
		step2Durations  = make([]time.Duration, numIterations)
		step3Durations  = make([]time.Duration, numIterations)
		step4Durations  = make([]time.Duration, numIterations)
		step5Durations  = make([]time.Duration, numIterations)
		totalDurations  = make([]time.Duration, numIterations)
		testInstanceIds = make([]InstanceId, numIterations)
	)

	for iter := 0; iter < numIterations; iter++ {
		iterStart := time.Now()

		// Step 1: lookupBuildMetadata (reading from cache/database)
		step1Start := time.Now()
		bMeta, err := mgr.lookupBuildMetadata(build.Id)
		step1Durations[iter] = time.Since(step1Start)
		if err != nil {
			t.Fatalf("lookupBuildMetadata failed: %s", err)
		}

		// Step 2: openInstance (database insert)
		step2Start := time.Now()
		iMeta := &InstanceMetadata{
			Build:      build.Id,
			Ports:      make(map[string]int),
			Containers: []string{},
		}
		err = mgr.openInstance(iMeta)
		step2Durations[iter] = time.Since(step2Start)
		if err != nil {
			t.Fatalf("openInstance failed: %s", err)
		}
		testInstanceIds[iter] = iMeta.Id

		// Step 3: GetChallengeMetadata (reading from cache/database)
		step3Start := time.Now()
		_, err = mgr.GetChallengeMetadata(build.Challenge)
		step3Durations[iter] = time.Since(step3Start)
		if err != nil {
			t.Fatalf("GetChallengeMetadata failed: %s", err)
		}

		// Step 4: getReversePortMap (database query)
		step4Start := time.Now()
		_, err = mgr.getReversePortMap(build.Challenge)
		step4Durations[iter] = time.Since(step4Start)
		if err != nil {
			t.Fatalf("getReversePortMap failed: %s", err)
		}

		// Step 5: reservePort calls (which may involve usedPortBitset)
		step5Start := time.Now()
		var portCount int
		for _, image := range bMeta.Images {
			portCount += len(image.Ports)
		}
		for i := 0; i < portCount; i++ {
			_, err := mgr.reservePort(iMeta.Id, fmt.Sprintf("test-port-%d", i))
			if err != nil {
				t.Fatalf("reservePort failed: %s", err)
			}
		}
		step5Durations[iter] = time.Since(step5Start)

		totalDurations[iter] = time.Since(iterStart)

		if iter == 0 {
			t.Logf("  Iteration %d (cold cache): %v", iter+1, totalDurations[iter])
		} else if iter == numIterations-1 {
			t.Logf("  Iteration %d (warm cache): %v", iter+1, totalDurations[iter])
		}
	}

	// Calculate statistics
	calcStats := func(durations []time.Duration) (min, max, avg time.Duration) {
		min = durations[0]
		max = durations[0]
		var sum time.Duration
		for _, d := range durations {
			if d < min {
				min = d
			}
			if d > max {
				max = d
			}
			sum += d
		}
		avg = sum / time.Duration(len(durations))
		return
	}

	_, _, step1Avg := calcStats(step1Durations)
	_, _, step2Avg := calcStats(step2Durations)
	_, _, step3Avg := calcStats(step3Durations)
	_, _, step4Avg := calcStats(step4Durations)
	_, _, step5Avg := calcStats(step5Durations)
	totalMin, totalMax, totalAvg := calcStats(totalDurations)

	// Use averages for reporting
	step1Duration := step1Avg
	step2Duration := step2Avg
	step3Duration := step3Avg
	step4Duration := step4Avg
	step5Duration := step5Avg
	totalDuration := totalAvg

	t.Logf("\n=== Performance Statistics (over %d iterations) ===", numIterations)
	t.Logf("Total time - Min: %v, Max: %v, Avg: %v", totalMin, totalMax, totalAvg)
	t.Logf("Cache impact: %.2fx faster (cold: %v, warm: %v)",
		float64(totalDurations[0])/float64(totalDurations[numIterations-1]),
		totalDurations[0], totalDurations[numIterations-1])
	t.Logf("\n=== Profile Summary ===")
	t.Logf("Database size: %d instances", count)
	t.Logf("Total time (pre-Docker): %v", totalDuration)
	t.Logf("Breakdown:")
	t.Logf("  - lookupBuildMetadata:    %8v (%5.1f%%)", step1Duration, 100*step1Duration.Seconds()/totalDuration.Seconds())
	t.Logf("  - openInstance:           %8v (%5.1f%%)", step2Duration, 100*step2Duration.Seconds()/totalDuration.Seconds())
	t.Logf("  - GetChallengeMetadata:   %8v (%5.1f%%)", step3Duration, 100*step3Duration.Seconds()/totalDuration.Seconds())
	t.Logf("  - getReversePortMap:      %8v (%5.1f%%)", step4Duration, 100*step4Duration.Seconds()/totalDuration.Seconds())
	t.Logf("  - reservePort operations: %8v (%5.1f%%)", step5Duration, 100*step5Duration.Seconds()/totalDuration.Seconds())

	// Output to CSV for easy comparison
	csvFile := fmt.Sprintf("%s/profile_results_%d.csv", t.TempDir(), instanceCount)
	f, err := os.Create(csvFile)
	if err != nil {
		t.Logf("Warning: could not create CSV file: %s", err)
	} else {
		defer f.Close()
		fmt.Fprintf(f, "metric,duration_ms\n")
		fmt.Fprintf(f, "lookupBuildMetadata,%f\n", step1Duration.Seconds()*1000)
		fmt.Fprintf(f, "openInstance,%f\n", step2Duration.Seconds()*1000)
		fmt.Fprintf(f, "GetChallengeMetadata,%f\n", step3Duration.Seconds()*1000)
		fmt.Fprintf(f, "getReversePortMap,%f\n", step4Duration.Seconds()*1000)
		fmt.Fprintf(f, "reservePort_total,%f\n", step5Duration.Seconds()*1000)
		fmt.Fprintf(f, "total,%f\n", totalDuration.Seconds()*1000)
		t.Logf("\nResults written to %s", csvFile)
	}

	// Clean up all test instances
	for _, iid := range testInstanceIds {
		if err := mgr.removeInstanceMetadata(iid); err != nil {
			t.Logf("Warning: failed to clean up test instance %d: %s", iid, err)
		}
	}
}

// BenchmarkInstanceLaunchDBLoad provides Go benchmark output format
func BenchmarkInstanceLaunchDBLoad(b *testing.B) {
	scenarios := []int{20, 200, 20000, 50000}

	for _, count := range scenarios {
		b.Run(fmt.Sprintf("db_size_%d", count), func(b *testing.B) {
			mgr := setupProfileBenchmarkManager(b)
			defer mgr.db.Close()

			// Setup phase (not measured)
			b.StopTimer()
			build := setupProfileBenchmarkData(b, mgr, count)
			b.StartTimer()

			// Measure the critical path
			for i := 0; i < b.N; i++ {
				profileBenchmarkInstanceLaunchPath(b, mgr, build)
			}
		})
	}
}

func setupProfileBenchmarkManager(b *testing.B) *Manager {
	b.Helper()

	dbFile, err := os.CreateTemp("", "cmgr-bench-*.db")
	if err != nil {
		b.Fatalf("failed to create temp file: %s", err)
	}
	dbFile.Close()
	b.Cleanup(func() {
		os.Remove(dbFile.Name())
	})

	mgr := new(Manager)
	mgr.log = newLogger(DISABLED)
	os.Setenv(DB_ENV, dbFile.Name())
	b.Cleanup(func() {
		os.Unsetenv(DB_ENV)
	})

	err = mgr.initDatabase()
	if err != nil {
		b.Fatalf("initDatabase failed: %s", err)
	}

	// Set port range to test bitset optimization (10000-63000)
	mgr.portLow = 10000
	mgr.portHigh = 63000

	return mgr
}

func setupProfileBenchmarkData(b *testing.B, mgr *Manager, instanceCount int) *BuildMetadata {
	b.Helper()

	challenge := &ChallengeMetadata{
		Id:            "bench/profile",
		Name:          "Bench",
		Namespace:     "bench",
		ChallengeType: "custom",
		Description:   "Benchmarking",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{"http": {Host: "challenge", Port: 8080}},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/bench/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{"": {}},
		},
	}
	errs := mgr.addChallenges([]*ChallengeMetadata{challenge})
	if len(errs) > 0 {
		b.Fatalf("addChallenges failed: %v", errs)
	}

	build := &BuildMetadata{
		Seed:          1,
		Format:        "flag{%s}",
		Challenge:     "bench/profile",
		Schema:        "manual-bench",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		b.Fatalf("openBuild failed: %s", err)
	}

	build.Flag = "flag{bench}"
	build.Images = []Image{{Host: "challenge", Ports: []string{"80/tcp"}}}
	build.LookupData = map[string]string{}
	if err := mgr.finalizeBuild(build); err != nil {
		b.Fatalf("finalizeBuild failed: %s", err)
	}

	now := time.Now().UTC()
	for i := 0; i < instanceCount; i++ {
		instance := &InstanceMetadata{
			Build:      build.Id,
			Ports:      map[string]int{"http": 8000 + (i % 10000)},
			Containers: []string{fmt.Sprintf("container-%d", i)},
		}

		if err := mgr.openInstance(instance); err != nil {
			b.Fatalf("openInstance failed: %s", err)
		}

		setTimestamp(b, mgr, instance.Id, now, i)

		if err := mgr.finalizeInstance(instance); err != nil {
			b.Fatalf("finalizeInstance failed: %s", err)
		}
	}

	return build
}

func profileBenchmarkInstanceLaunchPath(b *testing.B, mgr *Manager, build *BuildMetadata) {
	b.Helper()

	// Simulate the critical path of instance launch
	_, err := mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		b.Fatalf("lookupBuildMetadata failed: %s", err)
	}

	iMeta := &InstanceMetadata{
		Build:      build.Id,
		Ports:      make(map[string]int),
		Containers: []string{},
	}
	err = mgr.openInstance(iMeta)
	if err != nil {
		b.Fatalf("openInstance failed: %s", err)
	}

	_, err = mgr.GetChallengeMetadata(build.Challenge)
	if err != nil {
		b.Fatalf("GetChallengeMetadata failed: %s", err)
	}

	_, err = mgr.getReversePortMap(build.Challenge)
	if err != nil {
		b.Fatalf("getReversePortMap failed: %s", err)
	}

	_, err = mgr.reservePort(iMeta.Id, "test-port")
	if err != nil {
		b.Fatalf("reservePort failed: %s", err)
	}

	// Clean up
	mgr.removeInstanceMetadata(iMeta.Id)
}
