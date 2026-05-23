// Package agent is the Enforcer backend that translates resolved policy into
// the response of the next /api/agent/checkin call. Apply queues the assigned
// slice in memory; the checkin handler pulls it and ships it back to the
// real on-device agent, which is what actually moves bits on the device.
//
// CanHandle gates on the device having an issued agent_token — that's the
// signal that an agent install ever called register and so is theoretically
// reachable. If the token is missing, the coordinator falls through to
// whatever else is registered (today: just noop).
package agent

import (
	"context"
	"sync"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
)

type Backend struct {
	pending sync.Map // deviceID -> *enforce.Slice
}

func New() *Backend { return &Backend{} }

func (*Backend)Name() string { return "agent" }

func (*Backend)Capabilities() map[device.Capability]bool {
	// What an in-device agent can plausibly enforce. The actual reach for a
	// given install is further constrained by what the agent reports in its
	// `caps` field — that lives on Device.Caps and is consulted by the
	// agent when applying the slice on-device.
	return map[device.Capability]bool{
		device.CapScreenLock:    true,
		device.CapTimeBudget:    true,
		device.CapAppBlock:      true,
		device.CapDomainBlock:   true,
		device.CapScheduleBlock: true,
	}
}

func (*Backend)CanHandle(d *device.Device) bool {
	return d != nil && d.AgentToken != nil && *d.AgentToken != ""
}

func (b *Backend) Apply(_ context.Context, deviceID string, slice enforce.Slice) enforce.Result {
	b.pending.Store(deviceID, &slice)
	res := enforce.Result{Backend: "agent", DeviceID: deviceID}
	for _, c := range slice.Caps {
		res.PerRule = append(res.PerRule, enforce.PerRule{
			Capability: c,
			Outcome:    enforce.RuleApplied,
			Note:       "queued for agent pickup",
		})
	}
	return res
}

func (b *Backend) Revoke(_ context.Context, deviceID string) error {
	b.pending.Delete(deviceID)
	return nil
}

// PullPending returns the most recent slice queued for the device and clears
// it. Called from the checkin handler — the agent gets one chance to consume;
// the next coordinator run will re-queue based on the current policy state.
func (b *Backend) PullPending(deviceID string) *enforce.Slice {
	v, ok := b.pending.LoadAndDelete(deviceID)
	if !ok {
		return nil
	}
	return v.(*enforce.Slice)
}
