package device

import "time"

type DeviceState string

const (
	StateDiscovered DeviceState = "discovered"
	StateActive     DeviceState = "active"
	StatePaused     DeviceState = "paused"
	StateRetired    DeviceState = "retired"
)

type IdentityKind string

const (
	IdentityMAC      IdentityKind = "mac"
	IdentityIP       IdentityKind = "ip"
	IdentityHostname IdentityKind = "hostname"
	IdentityAgent    IdentityKind = "agent_id"
)

// Capability constants align with the enforcement capability set defined in
// Phase 5 of CORE_PLATFORM.md.
type Capability string

const (
	CapScreenLock    Capability = "screen_lock"
	CapTimeBudget    Capability = "time_budget"
	CapAppBlock      Capability = "app_block"
	CapDomainBlock   Capability = "domain_block"
	CapScheduleBlock Capability = "schedule_block"
)

type NetworkIdentity struct {
	Kind     IdentityKind `json:"kind"`
	Value    string       `json:"value"`
	LastSeen *time.Time   `json:"last_seen,omitempty"`
}

type Device struct {
	ID         string            `json:"id"`
	PersonID   *string           `json:"person_id,omitempty"`
	Label      string            `json:"label"`
	Kind       string            `json:"kind"`
	AgentToken *string           `json:"agent_token,omitempty"`
	State      DeviceState       `json:"state"`
	LastSeen   *time.Time        `json:"last_seen,omitempty"`
	Identities []NetworkIdentity `json:"identities"`
	Caps       []Capability      `json:"caps"`
}

// Observation is the input to Reconcile: a single sighting of a device from
// some source (agent check-in, network scan, manual add).
type Observation struct {
	Source     string
	SeenAt     time.Time
	AgentToken string // pre-provisioned token claiming a manually-added device
	AgentID    string
	MAC        string
	IP         string
	Hostname   string
	Caps       []Capability

	// Optional hints used when creating a new device from this observation.
	Label string
	Kind  string
}

type Filter struct {
	State DeviceState // empty = no filter
}
