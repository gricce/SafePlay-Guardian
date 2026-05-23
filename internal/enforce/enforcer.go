package enforce

import (
	"context"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

type RuleResult string

const (
	RuleApplied       RuleResult = "applied"
	RuleFailed        RuleResult = "failed"
	RuleUnenforceable RuleResult = "unenforceable"
)

// Slice is the portion of a resolved policy assigned to a single backend.
// Policy carries the full resolved state for context; Caps lists which
// capabilities this backend is responsible for on this device.
type Slice struct {
	Policy policy.ResolvedPolicy
	Caps   []device.Capability
}

type PerRule struct {
	Capability device.Capability `json:"capability"`
	Outcome    RuleResult        `json:"outcome"`
	Error      string            `json:"error,omitempty"`
	Note       string            `json:"note,omitempty"`
}

type Result struct {
	Backend  string    `json:"backend"`
	DeviceID string    `json:"device_id"`
	PerRule  []PerRule `json:"rules"`
}

// Enforcer is one way to make policy take effect on a device.
//
// Apply must be idempotent — re-applying an active policy is a silent no-op.
// Every call returns a PerRule for every cap in slice.Caps, even when the
// backend chose to do nothing.
type Enforcer interface {
	Name() string
	Capabilities() map[device.Capability]bool
	CanHandle(d *device.Device) bool
	Apply(ctx context.Context, deviceID string, slice Slice) Result
	Revoke(ctx context.Context, deviceID string) error
}

// RuleOutcome is the coordinator's per-rule view: which backend handled the
// capability, or empty Backend + Outcome=unenforceable when no backend could.
type RuleOutcome struct {
	Capability device.Capability `json:"capability"`
	Backend    string            `json:"backend,omitempty"`
	Outcome    RuleResult        `json:"outcome"`
	Error      string            `json:"error,omitempty"`
	Note       string            `json:"note,omitempty"`
}

type CoordinatorResult struct {
	DeviceID string        `json:"device_id"`
	At       time.Time     `json:"at"`
	Rules    []RuleOutcome `json:"rules"`
}

// DeviceLookup is what the coordinator needs from the device registry.
type DeviceLookup interface {
	Get(ctx context.Context, id string) (*device.Device, error)
}
