package cmgr

import (
	"testing"

	"github.com/picoCTF/cmgr/cmgr/dockerfiles"
)

// TestDeriveDeliveryType covers the single source of truth for delivery-type
// classification: challenge type wins for flag-only, otherwise the published
// port count decides.
func TestDeriveDeliveryType(t *testing.T) {
	cases := []struct {
		challengeType string
		ports         int
		want          DeliveryType
	}{
		{"custom", 2, DeliveryService},
		{"custom", 1, DeliveryService},
		{"custom", 0, DeliveryArtifactOnly},
		{"static-make", 0, DeliveryArtifactOnly},
		{"remote-make", 1, DeliveryService},
		{"flag-only", 0, DeliveryFlagOnly},
		// Challenge type takes precedence even if ports were somehow present.
		{"flag-only", 3, DeliveryFlagOnly},
		{"", 0, DeliveryArtifactOnly},
	}

	for _, c := range cases {
		got := deriveDeliveryType(c.challengeType, c.ports)
		if got != c.want {
			t.Errorf("deriveDeliveryType(%q, %d) = %q, want %q", c.challengeType, c.ports, got, c.want)
		}
	}
}

// TestFlagOnlyTypeRegistered guards the pairing between the "flag-only"
// literal in deriveDeliveryType and the embedded Dockerfile registering the
// challenge type of the same name: renaming either side without the other
// would silently misclassify flag-only challenges.
func TestFlagOnlyTypeRegistered(t *testing.T) {
	df, err := dockerfiles.Get("flag-only")
	if err != nil || len(df) == 0 {
		t.Fatalf("no embedded Dockerfile registers the 'flag-only' challenge type (err: %v)", err)
	}
	if got := deriveDeliveryType("flag-only", 0); got != DeliveryFlagOnly {
		t.Errorf("deriveDeliveryType(\"flag-only\", 0) = %q, want %q", got, DeliveryFlagOnly)
	}
}

// TestNeedsInstance covers the predicate gating all instance management:
// only service challenges get instances, and an unset delivery type fails
// safe by behaving like service.
func TestNeedsInstance(t *testing.T) {
	cases := []struct {
		delivery DeliveryType
		want     bool
	}{
		{DeliveryService, true},
		{DeliveryArtifactOnly, false},
		{DeliveryFlagOnly, false},
		{"", true},
	}

	for _, c := range cases {
		md := &ChallengeMetadata{DeliveryType: c.delivery}
		if got := md.NeedsInstance(); got != c.want {
			t.Errorf("NeedsInstance() with DeliveryType %q = %t, want %t", c.delivery, got, c.want)
		}
	}
}

// TestLookupChallengeMetadataDerivesDeliveryType verifies the read-time
// derivation site: metadata loaded from the database must carry a valid
// delivery type matching its stored port map.
func TestLookupChallengeMetadataDerivesDeliveryType(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	portless := &ChallengeMetadata{
		Id:            "test/portless",
		Name:          "Portless",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "artifact only",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{},
		Tags:          []string{"quiz"},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}
	ported := &ChallengeMetadata{
		Id:            "test/ported",
		Name:          "Ported",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "service",
		Hosts:         []HostInfo{{Name: "challenge", Target: ""}},
		PortMap:       map[string]PortInfo{"web": {Host: "challenge", Port: 5000}},
		Tags:          []string{},
		Attributes:    map[string]string{},
		Path:          "/tmp/test/problem.md",
		ChallengeOptions: ChallengeOptions{
			Overrides: map[string]ContainerOptions{
				"": {},
			},
		},
	}

	errs := mgr.addChallenges([]*ChallengeMetadata{portless, ported})
	if len(errs) > 0 {
		t.Fatalf("addChallenges failed: %v", errs)
	}

	got, err := mgr.lookupChallengeMetadata("test/portless")
	if err != nil {
		t.Fatalf("lookupChallengeMetadata(portless) failed: %s", err)
	}
	if got.DeliveryType != DeliveryArtifactOnly {
		t.Errorf("portless challenge: expected DeliveryType %q, got %q", DeliveryArtifactOnly, got.DeliveryType)
	}

	got, err = mgr.lookupChallengeMetadata("test/ported")
	if err != nil {
		t.Fatalf("lookupChallengeMetadata(ported) failed: %s", err)
	}
	if got.DeliveryType != DeliveryService {
		t.Errorf("ported challenge: expected DeliveryType %q, got %q", DeliveryService, got.DeliveryType)
	}

	// The light-weight list/search paths must also carry a valid delivery type
	// so no ChallengeMetadata leaving the library has the zero value.
	list, err := mgr.listChallenges()
	if err != nil {
		t.Fatalf("listChallenges failed: %s", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 challenges, got %d", len(list))
	}
	for _, md := range list {
		want := DeliveryArtifactOnly
		if md.Id == "test/ported" {
			want = DeliveryService
		}
		if md.DeliveryType != want {
			t.Errorf("listChallenges %s: expected DeliveryType %q, got %q", md.Id, want, md.DeliveryType)
		}
	}

	search, err := mgr.searchChallenges([]string{"quiz"})
	if err != nil {
		t.Fatalf("searchChallenges failed: %s", err)
	}
	if len(search) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(search))
	}
	if search[0].DeliveryType != DeliveryArtifactOnly {
		t.Errorf("searchChallenges: expected DeliveryType %q, got %q", DeliveryArtifactOnly, search[0].DeliveryType)
	}
}

// TestStartRejectsNonService verifies that starting an instance of a build
// whose challenge needs no instance (artifact-only here) fails before any
// instance state is created.
func TestStartRejectsNonService(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := &ChallengeMetadata{
		Id:            "test/no-instance",
		Name:          "No Instance",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "artifact only",
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
		Challenge:     "test/no-instance",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{static}"
	build.Images = []Image{{Host: "challenge", Ports: []string{}}}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	if _, err := mgr.Start(build.Id, nil); err == nil {
		t.Fatal("Start succeeded for an artifact-only build; expected an error")
	}

	instances, err := mgr.getBuildInstances(build.Id)
	if err != nil {
		t.Fatalf("getBuildInstances failed: %s", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected no instance rows after rejected Start, found %d", len(instances))
	}
}

// TestRecordBuildSolve verifies build-level solve recording (used by CheckBuild
// for builds with no instance): the timestamp is stored and never moves
// backwards.
func TestRecordBuildSolve(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.db.Close()

	challenge := &ChallengeMetadata{
		Id:            "test/solve-record",
		Name:          "Solve Record",
		Namespace:     "test",
		ChallengeType: "custom",
		Description:   "solve recording",
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
		Challenge:     "test/solve-record",
		Schema:        "manual-test",
		InstanceCount: DYNAMIC_INSTANCES,
	}
	if err := mgr.openBuild(build); err != nil {
		t.Fatalf("openBuild failed: %s", err)
	}
	build.Flag = "flag{solved}"
	build.Images = []Image{{Host: "challenge", Ports: []string{}}}
	if err := mgr.finalizeBuild(build); err != nil {
		t.Fatalf("finalizeBuild failed: %s", err)
	}

	build.LastSolved = 1000
	if err := mgr.recordBuildSolve(build); err != nil {
		t.Fatalf("recordBuildSolve failed: %s", err)
	}

	got, err := mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}
	if got.LastSolved != 1000 {
		t.Errorf("expected LastSolved 1000, got %d", got.LastSolved)
	}

	// An older solve timestamp must not move lastsolved backwards.
	build.LastSolved = 500
	if err := mgr.recordBuildSolve(build); err != nil {
		t.Fatalf("recordBuildSolve (older) failed: %s", err)
	}
	got, err = mgr.lookupBuildMetadata(build.Id)
	if err != nil {
		t.Fatalf("lookupBuildMetadata failed: %s", err)
	}
	if got.LastSolved != 1000 {
		t.Errorf("older timestamp overwrote LastSolved: expected 1000, got %d", got.LastSolved)
	}
}
