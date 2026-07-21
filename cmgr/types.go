package cmgr

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/moby/moby/client"
)

const (
	DB_ENV             string = "CMGR_DB"
	DIR_ENV            string = "CMGR_DIR"
	ARTIFACT_DIR_ENV   string = "CMGR_ARTIFACT_DIR"
	REGISTRY_ENV       string = "CMGR_REGISTRY"
	REGISTRY_USER_ENV  string = "CMGR_REGISTRY_USER"
	REGISTRY_TOKEN_ENV string = "CMGR_REGISTRY_TOKEN"
	LOGGING_ENV        string = "CMGR_LOGGING"
	IFACE_ENV          string = "CMGR_INTERFACE"
	PORTS_ENV          string = "CMGR_PORTS"
	DISK_QUOTA_ENV     string = "CMGR_ENABLE_DISK_QUOTAS"
	PRUNE_AGE_ENV      string = "CMGR_PRUNE_AGE"
	DB_WAL_ENV         string = "CMGR_DB_WAL"

	DYNAMIC_INSTANCES int = -1
	LOCKED            int = -2
)

type UnknownIdentifierError struct {
	Type string
	Name string
}

type Manager struct {
	cli                  *client.Client
	ctx                  context.Context
	log                  *logger
	chalDir              string
	artifactsDir         string
	db                   *sqlx.DB
	dbPath               string
	challengeDockerfiles map[string][]byte
	rand                 *rand.Rand
	randMu               sync.Mutex
	// imageMu serializes the "is this content still referenced? if not, untag"
	// critical sections (executeBuild cleanup, pruneReplacedImages,
	// destroyImages) so a concurrent remover cannot delete a tag between
	// another's reference check and its ImageRemove. Content-addressed tags are
	// shared across build rows, so these checks race under cmgrd's concurrent
	// request handling.
	imageMu            sync.Mutex
	challengeInterface string
	challengeRegistry  string
	authString         string
	hostOSType         string // docker daemon OSType, cached once at initDocker (immutable for the daemon)
	portLow            int
	portHigh           int
	lastPruneUnix      atomic.Int64 // atomic UnixNano timestamp used as CAS gate for prune interval
	pruneInterval      time.Duration
	pruneAge           time.Duration
	launchSemaphore    chan struct{}
}

type PortInfo struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type HostInfo struct {
	Name   string `json:"name"`
	Target string `json:"target,omitempty"`
}

type NetworkOptions struct{}

type ContainerOptions struct {
	Init            bool     `json:"init,omitempty"            yaml:"init"`
	Cpus            string   `json:"cpus,omitempty"            yaml:"cpus"`
	Memory          string   `json:"memory,omitempty"          yaml:"memory"`
	Ulimits         []string `json:"ulimits,omitempty"         yaml:"ulimits"`
	PidsLimit       int64    `json:"pidslimit,omitempty"       yaml:"pidslimit"`
	ReadonlyRootfs  bool     `json:"readonlyrootfs,omitempty"  yaml:"readonlyrootfs"`
	DroppedCaps     []string `json:"droppedcaps,omitempty"     yaml:"droppedcaps"`
	NoNewPrivileges bool     `json:"nonewprivileges,omitempty" yaml:"nonewprivileges"`
	DiskQuota       string   `json:"diskquota,omitempty"       yaml:"diskquota"`
	CgroupParent    string   `json:"cgroupparent,omitempty"    yaml:"cgroupparent"`
	CapImmutable    bool     `json:"capimmutable,omitempty"    yaml:"cap_immutable"`
}

type ChallengeOptions struct {
	NetworkOptions   `yaml:",inline"`
	ContainerOptions `yaml:",inline"`
	Overrides        map[string]ContainerOptions `json:"overrides,omitempty" yaml:"overrides"`
}

type ChallengeId string
type ChallengeMetadata struct {
	Id               ChallengeId         `json:"id"`
	Name             string              `json:"name,omitempty"`
	Namespace        string              `json:"namespace"`
	ChallengeType    string              `json:"challenge_type"`
	Description      string              `json:"description,omitempty"`
	Details          string              `json:"details,omitempty"`
	Hints            []string            `json:"hints,omitempty"`
	SourceChecksum   uint32              `json:"source_checksum"`
	MetadataChecksum uint32              `json:"metadata_checksum"`
	Path             string              `json:"path"`
	Templatable      bool                `json:"templatable,omitempty"`
	PortMap          map[string]PortInfo `json:"port_map,omitempty"`
	Hosts            []HostInfo          `json:"hosts"`
	MaxUsers         int                 `json:"max_users,omitempty"`
	Category         string              `json:"category,omitempty"`
	Points           int                 `json:"points,omitempty"`
	Tags             []string            `json:"tags,omitempty"`
	Attributes       map[string]string   `json:"attributes,omitempty"`
	ChallengeOptions ChallengeOptions    `json:"challenge_options,omitempty"`

	SolveScript bool `json:"solve_script,omitempty"`

	// DeliveryType classifies what a player receives and therefore what cmgr
	// must stand up at runtime. It is derived (deriveDeliveryType), never stored
	// or author-written, so it cannot disagree with the port map or challenge
	// type it is computed from. Always emitted (no omitempty) so consumers can
	// distinguish "service" from absent-on-old-cmgr. Instances are launched only
	// for "service" challenges; consumers should treat an empty instance pool as
	// expected exactly when this is not "service".
	DeliveryType DeliveryType `json:"delivery_type" db:"-"`

	Builds []*BuildMetadata `json:"builds,omitempty"`
}

// DeliveryType is the intrinsic runtime shape of a challenge: what the player
// receives and what infrastructure must exist for it. It is orthogonal to the
// schema-level deployment policy (instance_count: persistent vs on-demand),
// which only applies to "service" challenges.
type DeliveryType string

const (
	// DeliveryService: the challenge publishes at least one port; a running
	// instance is required for players to interact with it.
	DeliveryService DeliveryType = "service"
	// DeliveryArtifactOnly: no published ports; the artifacts produced at build
	// time are the entire challenge and no instance is ever launched.
	DeliveryArtifactOnly DeliveryType = "artifact_only"
	// DeliveryFlagOnly: intentionally no ports and no artifacts; the build runs
	// only to generate the flag and lookup data (e.g. multiple-choice options)
	// for a bare submission prompt. No instance is ever launched. Declared via
	// the "flag-only" challenge type (that literal and the embedded
	// flag-only.Dockerfile name must stay in sync with deriveDeliveryType).
	DeliveryFlagOnly DeliveryType = "flag_only"
)

// deriveDeliveryType is the single source of truth for DeliveryType, used by
// both the loader (parse time) and metadata reads (query time). Intentional
// inertness cannot be derived (it is indistinguishable from a forgotten
// '# PUBLISH' directive), so flag-only must be declared via the challenge type.
func deriveDeliveryType(challengeType string, publishedPorts int) DeliveryType {
	if challengeType == "flag-only" {
		return DeliveryFlagOnly
	}
	if publishedPorts == 0 {
		return DeliveryArtifactOnly
	}
	return DeliveryService
}

// NeedsInstance reports whether running instances are part of delivering this
// challenge. Only "service" challenges need them; artifact-only and flag-only
// challenges are fully delivered by their builds. An unset DeliveryType
// (metadata that never passed through the loader or a database read) counts as
// needing instances, so a missed derivation can never silently skip instance
// management.
func (cm *ChallengeMetadata) NeedsInstance() bool {
	return cm.DeliveryType == "" || cm.DeliveryType == DeliveryService
}

type ChallengeUpdates struct {
	Added      []*ChallengeMetadata `json:"added"`
	Refreshed  []*ChallengeMetadata `json:"refreshed"`
	Updated    []*ChallengeMetadata `json:"updated"`
	Removed    []*ChallengeMetadata `json:"removed"`
	Unmodified []*ChallengeMetadata `json:"unmodified"`
	Errors     []error              `json:"errors"`
}

type BuildId int64
type BuildMetadata struct {
	Id BuildId `json:"id"`

	Flag       string            `json:"flag"`
	LookupData map[string]string `json:"lookup_data,omitempty"`

	Seed   int    `json:"seed"`
	Format string `json:"format"`
	// Checksum identifies the content this build's images were produced from:
	// a CRC-32 over the challenge's source checksum and the flag format (see
	// contentChecksum). It is set when the images are built, so after a source
	// change it intentionally differs from the value derived from the
	// challenge's current metadata until the build is rebuilt.
	Checksum uint32 `json:"checksum,omitempty"`
	// PrevChecksum is the generation Checksum displaced on the last rebuild
	// (0 = none): its images are retained as the rollback target. A future
	// `rollback` operation would swap it with Checksum, re-extract /challenge
	// from that image (see executeBuild's extraction step), and restart
	// instances. Same-row rollback is format- and seed-stable by construction.
	PrevChecksum uint32              `json:"prev_checksum,omitempty" db:"prevchecksum"`
	Images       []Image             `json:"images"`
	HasArtifacts bool                `json:"has_artifacts"`
	LastSolved   int64               `json:"last_solved"`
	Challenge    ChallengeId         `json:"challenge_id"`
	Instances    []*InstanceMetadata `json:"instances,omitempty"`

	Schema        string `json:"schema"`
	InstanceCount int    `json:"instance_count"`
}

type ImageId int64
type Image struct {
	Id    ImageId  `json:"id"`
	Host  string   `json:"host"`
	Ports []string `json:"exposed_ports"`
	Build BuildId  `json:"build"`
}

type InstanceId int64
type InstanceMetadata struct {
	Id          InstanceId     `json:"id"`
	IsFinalized bool           `json:"-" db:"is_finalized"`
	Ports       map[string]int `json:"ports,omitempty"`
	Containers  []string       `json:"containers"`
	LastSolved  int64          `json:"last_solved"`
	CreatedAt   *time.Time     `json:"created_at" db:"created_at"`
	Build       BuildId        `json:"build_id"`
}

type Schema struct {
	Name       string                             `json:"name"        yaml:"name"`
	FlagFormat string                             `json:"flag_format" yaml:"flag_format"`
	Challenges map[ChallengeId]BuildSpecification `json:"challenges"  yaml:"challenges"`
}
type BuildSpecification struct {
	Seeds         []int `json:"seeds"          yaml:"seeds"`
	InstanceCount int   `json:"instance_count" yaml:"instance_count"`
}
