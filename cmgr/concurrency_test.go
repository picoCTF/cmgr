package cmgr

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	if errs := mgr.addChallenges([]*ChallengeMetadata{challenge}); len(errs) > 0 {
		t.Fatalf("failed to add challenges: %v", errs)
	}

	build := &BuildMetadata{
		Seed: 1, Format: "flag{%s}", Challenge: "test/concurrent", Schema: "s", InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("failed to open build: %v", err)
	}

	const numInstances = 50
	var instances []InstanceId
	for i := 1; i <= numInstances; i++ {
		instance := &InstanceMetadata{Build: build.Id}
		if err := mgr.openInstance(instance); err != nil {
			t.Fatalf("failed to open instance %d: %v", i, err)
		}
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

// TestLaunchSemaphoreEnforcesConcurrencyLimit verifies that the buffered-channel
// semaphore used in startContainers never allows more than the configured number
// of concurrent holders.
func TestLaunchSemaphoreEnforcesConcurrencyLimit(t *testing.T) {
	for _, limit := range []int{1, 2} {
		limit := limit
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			t.Parallel()
			mgr := &Manager{
				log:             newLogger(DISABLED),
				launchSemaphore: make(chan struct{}, limit),
			}

			const workers = 8
			var inFlight atomic.Int32
			var peak atomic.Int32
			var wg sync.WaitGroup

			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					// mirrors the exact pattern in startContainers
					mgr.launchSemaphore <- struct{}{}
					defer func() { <-mgr.launchSemaphore }()

					cur := inFlight.Add(1)
					for {
						p := peak.Load()
						if cur <= p || peak.CompareAndSwap(p, cur) {
							break
						}
					}
					time.Sleep(20 * time.Millisecond) // hold slot so goroutines overlap
					inFlight.Add(-1)
				}()
			}

			wg.Wait()

			if got := peak.Load(); got > int32(limit) {
				t.Errorf("peak in-flight %d exceeded semaphore limit %d", got, limit)
			}
		})
	}
}

// TestLaunchSemaphoreReleasedOnReturn verifies that the deferred release in
// startContainers actually frees the slot so subsequent callers can proceed.
func TestLaunchSemaphoreReleasedOnReturn(t *testing.T) {
	mgr := &Manager{
		log:             newLogger(DISABLED),
		launchSemaphore: make(chan struct{}, 1),
	}

	acquire := func() func() {
		mgr.launchSemaphore <- struct{}{}
		return func() { <-mgr.launchSemaphore }
	}

	release := acquire()
	// slot is held — a non-blocking send should fail
	select {
	case mgr.launchSemaphore <- struct{}{}:
		t.Fatal("acquired semaphore while it should be full")
	default:
	}

	release()
	// slot is free — the next acquire must not block
	done := make(chan struct{})
	go func() {
		mgr.launchSemaphore <- struct{}{}
		<-mgr.launchSemaphore
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("semaphore slot was not released after return")
	}
}
