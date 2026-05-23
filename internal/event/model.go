package event

import "time"

// Kind is the closed set of event types per CORE_PLATFORM.md Phase 6. Adding a
// new kind is a code change, not a migration — that constraint keeps the log
// queryable by stable SQL.
type Kind string

const (
	KindDeviceSeen          Kind = "device_seen"
	KindPolicyApplied       Kind = "policy_applied"
	KindPolicyFailed        Kind = "policy_failed"
	KindPolicyUnenforceable Kind = "policy_unenforceable"
	KindBlock               Kind = "block"
	KindSessionStart        Kind = "session_start"
	KindSessionStop         Kind = "session_stop"
	KindScheduleTransition  Kind = "schedule_transition"
)

var validKinds = map[Kind]bool{
	KindDeviceSeen: true, KindPolicyApplied: true, KindPolicyFailed: true,
	KindPolicyUnenforceable: true, KindBlock: true,
	KindSessionStart: true, KindSessionStop: true, KindScheduleTransition: true,
}

func (k Kind) Valid() bool { return validKinds[k] }

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Event is the append-only envelope. `At` is the device clock (less trusted)
// and `RecordedAt` is the core's commit time (trusted ordering).
type Event struct {
	ID         string         `json:"id"`
	At         time.Time      `json:"at"`
	RecordedAt time.Time      `json:"recorded_at"`
	Kind       Kind           `json:"kind"`
	PersonID   *string        `json:"person_id,omitempty"`
	DeviceID   string         `json:"device_id"`
	Severity   Severity       `json:"severity"`
	Attrs      map[string]any `json:"attrs"`
}
