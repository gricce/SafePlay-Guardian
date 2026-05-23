// Package noop is the permanent dry-run / fallback enforcer. It declares every
// capability so the coordinator can always route somewhere, and its Apply just
// logs what it WOULD have done. Used for testing, dry-runs, and as the
// "we received the intent but have no real backend yet" backstop.
package noop

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

type Backend struct {
	Logger *slog.Logger
}

func (Backend) Name() string { return "noop" }

func (Backend) Capabilities() map[device.Capability]bool {
	return map[device.Capability]bool{
		device.CapScreenLock:    true,
		device.CapTimeBudget:    true,
		device.CapAppBlock:      true,
		device.CapDomainBlock:   true,
		device.CapScheduleBlock: true,
	}
}

// CanHandle always returns true: the noop backend is the universal fallback so
// that no policy is ever silently dropped. Real backends (agent, network) will
// gate this by device-specific reachability.
func (Backend) CanHandle(_ *device.Device) bool { return true }

func (b Backend) Apply(_ context.Context, deviceID string, slice enforce.Slice) enforce.Result {
	res := enforce.Result{Backend: b.Name(), DeviceID: deviceID}
	for _, cap := range slice.Caps {
		note := describe(cap, slice.Policy)
		if b.Logger != nil {
			b.Logger.Info("noop enforce", "device", deviceID, "cap", cap, "note", note)
		}
		res.PerRule = append(res.PerRule, enforce.PerRule{
			Capability: cap,
			Outcome:    enforce.RuleApplied,
			Note:       note,
		})
	}
	return res
}

func (Backend) Revoke(_ context.Context, _ string) error { return nil }

func describe(cap device.Capability, rp policy.ResolvedPolicy) string {
	switch cap {
	case device.CapScreenLock:
		if rp.ScreenLocked {
			return fmt.Sprintf("would lock screen (reason=%s)", rp.LockReason)
		}
		return "would keep screen unlocked"
	case device.CapTimeBudget:
		return fmt.Sprintf("would enforce %d min/day budget (used %d)", rp.TimeBudgetMinutes, rp.TimeBudgetUsedMins)
	case device.CapScheduleBlock:
		return "would honor schedule rule"
	case device.CapDomainBlock:
		return fmt.Sprintf("would block %d domain(s)", len(rp.BlockedDomains))
	case device.CapAppBlock:
		return fmt.Sprintf("would block %d app(s)", len(rp.BlockedApps))
	}
	return "noop"
}
