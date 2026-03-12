package cmgr

import (
	"sync"
	"testing"
)

// TestConcurrentPortReservation verifies that atomic port reservations
// can securely allocate unique ports to simultaneous requests without lock contention or overlaps.
func TestConcurrentPortReservation(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()
	mgr.portLow = 10000
	mgr.portHigh = 20000

	challenge := &ChallengeMetadata{
		Id: "test/concurrent", Name: "Concurrent", Namespace: "t", ChallengeType: "custom", Description: "d", Path: "/t/p",
		ChallengeOptions: ChallengeOptions{Overrides: map[string]ContainerOptions{"": {}}},
	}
	mgr.addChallenges([]*ChallengeMetadata{challenge})
	
	build := &BuildMetadata{
		Seed: 1, Format: "flag{%s}", Challenge: "test/concurrent", Schema: "s", InstanceCount: DYNAMIC_INSTANCES,
	}
	mgr.openBuild(build)
	
	const numInstances = 50
	var instances []InstanceId
	for i := 1; i <= numInstances; i++ {
		instance := &InstanceMetadata{Build: build.Id}
		mgr.openInstance(instance)
		instances = append(instances, instance.Id)
	}

	var wg sync.WaitGroup
	errs := make(chan error, numInstances)
	ports := make(chan int, numInstances)

	for _, instId := range instances {
		wg.Add(1)
		go func(instanceId InstanceId) {
			defer wg.Done()
			port, err := mgr.reservePort(instanceId, "test-port")
			if err != nil {
				errs <- err
			} else {
				ports <- port
			}
		}(instId)
	}

	wg.Wait()
	close(errs)
	close(ports)

	if len(errs) > 0 {
		for err := range errs {
			t.Errorf("Error during concurrent reservation: %s", err)
		}
		t.Fatalf("Failed with %d errors", len(errs))
	}

	uniquePorts := make(map[int]bool)
	for port := range ports {
		if uniquePorts[port] {
			t.Errorf("Collision detected! Port %d was returned multiple times", port)
		}
		uniquePorts[port] = true
	}

	if len(uniquePorts) != numInstances {
		t.Errorf("Expected %d unique ports, got %d", numInstances, len(uniquePorts))
	}
}
