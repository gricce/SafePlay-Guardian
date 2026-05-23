package event

import (
	"context"
	"log/slog"

	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

// PolicyLookup is the subset of policy.Store the Logger needs to resolve a
// device's effective monitoring level. Both phase4 SQLiteStore and the test
// fakes already satisfy it.
type PolicyLookup interface {
	GetPersonPolicy(ctx context.Context, personID string) (*policy.Policy, error)
	GetDeviceOverride(ctx context.Context, deviceID string) (*policy.Override, error)
}

// Logger appends events with monitoring-level redaction. Pure-audit kinds
// (policy_*, device_seen, schedule_transition) always pass through; block
// events get their specifics stripped unless the effective level allows them.
type Logger struct {
	Store  *Store
	Lookup PolicyLookup
	Slog   *slog.Logger
}

func (l *Logger) Append(ctx context.Context, ev Event) error {
	level := l.effectiveLevel(ctx, ev)
	Redact(&ev, level)
	if err := l.Store.Insert(ctx, ev); err != nil {
		if l.Slog != nil {
			l.Slog.Warn("event append failed", "kind", ev.Kind, "device", ev.DeviceID, "err", err)
		}
		return err
	}
	return nil
}

func (l *Logger) effectiveLevel(ctx context.Context, ev Event) policy.MonitoringLevel {
	if l.Lookup == nil || ev.PersonID == nil {
		return policy.MonitoringEnforcementOnly
	}
	p, err := l.Lookup.GetPersonPolicy(ctx, *ev.PersonID)
	if err != nil {
		return policy.MonitoringEnforcementOnly
	}
	o, _ := l.Lookup.GetDeviceOverride(ctx, ev.DeviceID)
	merged := policy.Merge(p, o)
	if merged != nil && merged.Monitoring != nil {
		return *merged.Monitoring
	}
	return policy.MonitoringEnforcementOnly
}

// Redact strips fields from `ev` that the given monitoring level should not
// persist. The redaction is *write-time* — it shapes what ever reaches disk,
// not what we serve from disk. That's the right place for privacy guarantees.
//
//   - enforcement_only: block events keep only `capability`; nothing about
//     which domain/app was blocked.
//   - metadata: block events keep `capability` plus aggregate hints (category,
//     count) but drop the per-URL `value` field.
//   - granular: nothing is stripped.
func Redact(ev *Event, level policy.MonitoringLevel) {
	if ev.Kind != KindBlock {
		return
	}
	switch level {
	case policy.MonitoringEnforcementOnly:
		cap, hasCap := ev.Attrs["capability"]
		ev.Attrs = map[string]any{}
		if hasCap {
			ev.Attrs["capability"] = cap
		}
	case policy.MonitoringMetadata:
		delete(ev.Attrs, "value")
		delete(ev.Attrs, "url")
		delete(ev.Attrs, "host")
	case policy.MonitoringGranular:
		// keep everything
	}
}
