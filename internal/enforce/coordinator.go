package enforce

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

// RuleHook fires once per rule outcome the coordinator produces. The event
// log plugs in via main.go without enforce depending on the event package.
type RuleHook func(ctx context.Context, deviceID string, outcome RuleOutcome)

type Coordinator struct {
	Backends []Enforcer
	Resolver *policy.Resolver
	Devices  DeviceLookup
	Logger   *slog.Logger
	OnRule   RuleHook
}

// Enforce resolves the policy for the device at `at`, splits the required
// capabilities across registered backends, calls Apply on each, and aggregates
// the per-rule results. Capabilities no backend can satisfy come back as
// outcome=unenforceable so the parent learns about gaps instead of silent drops.
func (c *Coordinator) Enforce(ctx context.Context, deviceID string, at time.Time) (CoordinatorResult, error) {
	res := CoordinatorResult{DeviceID: deviceID, At: at, Rules: []RuleOutcome{}}
	dev, err := c.Devices.Get(ctx, deviceID)
	if err != nil {
		return res, err
	}
	resolved, err := c.Resolver.Resolve(ctx, deviceID, at)
	if err != nil {
		return res, err
	}

	needed := resolved.RequiredCaps
	sortCaps(needed)

	assignments := map[string][]device.Capability{}
	backendOrder := []string{}
	var unsatisfied []device.Capability
	for _, cap := range needed {
		b := c.pick(cap, dev)
		if b == nil {
			unsatisfied = append(unsatisfied, cap)
			continue
		}
		if _, seen := assignments[b.Name()]; !seen {
			backendOrder = append(backendOrder, b.Name())
		}
		assignments[b.Name()] = append(assignments[b.Name()], cap)
	}

	byName := map[string]Enforcer{}
	for _, b := range c.Backends {
		byName[b.Name()] = b
	}
	for _, name := range backendOrder {
		b := byName[name]
		caps := assignments[name]
		slice := Slice{Policy: resolved, Caps: caps}
		out := b.Apply(ctx, deviceID, slice)
		for _, r := range out.PerRule {
			ro := RuleOutcome{
				Capability: r.Capability,
				Backend:    b.Name(),
				Outcome:    r.Outcome,
				Error:      r.Error,
				Note:       r.Note,
			}
			res.Rules = append(res.Rules, ro)
			if c.OnRule != nil {
				c.OnRule(ctx, deviceID, ro)
			}
		}
		if c.Logger != nil && len(out.PerRule) > 0 {
			c.Logger.Info("enforce dispatched", "backend", b.Name(), "device", deviceID, "caps", caps)
		}
	}
	for _, cap := range unsatisfied {
		ro := RuleOutcome{
			Capability: cap,
			Outcome:    RuleUnenforceable,
			Note:       "no registered backend can apply this capability to this device",
		}
		res.Rules = append(res.Rules, ro)
		if c.OnRule != nil {
			c.OnRule(ctx, deviceID, ro)
		}
	}
	return res, nil
}

func (c *Coordinator) pick(cap device.Capability, dev *device.Device) Enforcer {
	for _, b := range c.Backends {
		if !b.Capabilities()[cap] {
			continue
		}
		if !b.CanHandle(dev) {
			continue
		}
		return b
	}
	return nil
}

func sortCaps(caps []device.Capability) {
	sort.Slice(caps, func(i, j int) bool { return string(caps[i]) < string(caps[j]) })
}
