package policy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

// DeviceLookup is what the resolver needs from the device registry: just the
// owner-person link.
type DeviceLookup interface {
	Get(ctx context.Context, id string) (*device.Device, error)
}

// UsageQuery returns how much screen time a device has consumed since the
// start of the local day. Phase 6 will wire this to the event log; Phase 4
// uses NoUsage so the resolver compiles and runs end-to-end.
type UsageQuery interface {
	UsedSince(ctx context.Context, deviceID string, start time.Time) (time.Duration, error)
}

type NoUsage struct{}

func (NoUsage) UsedSince(_ context.Context, _ string, _ time.Time) (time.Duration, error) {
	return 0, nil
}

type Resolver struct {
	Devices DeviceLookup
	Store   Store
	Usage   UsageQuery
}

// Resolve is pure with respect to `at`: it never calls time.Now() internally,
// which is what makes bedtime enforcement defeat-proof (a child fast-forwarding
// the device clock can't bypass it) and tests trivial.
func (r *Resolver) Resolve(ctx context.Context, deviceID string, at time.Time) (ResolvedPolicy, error) {
	result := ResolvedPolicy{
		DeviceID:          deviceID,
		At:                at,
		LogLevel:          MonitoringEnforcementOnly,
		BlockedDomains:    []string{},
		BlockedApps:       []string{},
		BlockedCategories: []string{},
	}

	dev, err := r.Devices.Get(ctx, deviceID)
	if err != nil {
		return result, fmt.Errorf("resolve get device: %w", err)
	}
	if dev.PersonID == nil {
		return result, nil
	}
	result.PersonID = dev.PersonID

	p, err := r.Store.GetPersonPolicy(ctx, *dev.PersonID)
	if err != nil {
		return result, fmt.Errorf("resolve get policy: %w", err)
	}
	o, err := r.Store.GetDeviceOverride(ctx, deviceID)
	if err != nil {
		return result, fmt.Errorf("resolve get override: %w", err)
	}
	merged := Merge(p, o)
	if merged == nil {
		return result, nil
	}
	result.RequiredCaps = RequiredCaps(merged)

	loc := time.UTC
	if merged.Timezone != "" {
		if l, err := time.LoadLocation(merged.Timezone); err == nil {
			loc = l
		}
	}
	local := at.In(loc)

	if locked, reason := evalSchedule(merged.Schedule, local); locked {
		result.ScreenLocked = true
		result.LockReason = reason
	}

	if merged.TimeBudget != nil {
		wd := int(local.Weekday())
		allowed := merged.TimeBudget.MinutesByWeekday[wd]
		if allowed > 0 {
			startOfDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
			var used time.Duration
			if r.Usage != nil {
				used, _ = r.Usage.UsedSince(ctx, deviceID, startOfDay)
			}
			usedMins := int(used.Minutes())
			result.TimeBudgetMinutes = allowed
			result.TimeBudgetUsedMins = usedMins
			if usedMins >= allowed && !result.ScreenLocked {
				result.ScreenLocked = true
				result.LockReason = "time_budget_exceeded"
			}
		}
	}

	if merged.Content != nil {
		if merged.Content.BlockedDomains != nil {
			result.BlockedDomains = merged.Content.BlockedDomains
		}
		if merged.Content.BlockedApps != nil {
			result.BlockedApps = merged.Content.BlockedApps
		}
		if merged.Content.BlockedCategories != nil {
			result.BlockedCategories = merged.Content.BlockedCategories
		}
	}
	if merged.Monitoring != nil {
		result.LogLevel = *merged.Monitoring
	}
	return result, nil
}

// Merge applies override on top of policy, field-by-field. Any non-nil field
// on Override replaces the corresponding field on Policy.
func Merge(p *Policy, o *Override) *Policy {
	if p == nil && o == nil {
		return nil
	}
	var out Policy
	if p != nil {
		out = *p
	}
	if o == nil {
		return &out
	}
	if o.Timezone != nil {
		out.Timezone = *o.Timezone
	}
	if o.TimeBudget != nil {
		out.TimeBudget = o.TimeBudget
	}
	if o.Schedule != nil {
		out.Schedule = o.Schedule
	}
	if o.Content != nil {
		out.Content = o.Content
	}
	if o.Monitoring != nil {
		out.Monitoring = o.Monitoring
	}
	return &out
}

func evalSchedule(s *Schedule, at time.Time) (bool, string) {
	if s == nil {
		return false, ""
	}
	wd := int(at.Weekday())
	prevWd := (wd + 6) % 7
	cur := at.Hour()*60 + at.Minute()

	for _, b := range s.Blocks {
		// "bedtime" is semantically a subtype of "blocked" — separated in the
		// schedule editor for paint clarity but treated identically here.
		// "school" stays free for now; it's a content-policy hook for later.
		if b.Kind != "blocked" && b.Kind != "bedtime" {
			continue
		}
		start, ok1 := parseHM(b.Start)
		end, ok2 := parseHM(b.End)
		if !ok1 || !ok2 || start == end {
			continue
		}
		if start < end {
			if intsContain(b.Weekdays, wd) && cur >= start && cur < end {
				return true, "schedule"
			}
			continue
		}
		// Wraps midnight: [start, 24:00) on the listed weekday and
		// [00:00, end) on the next weekday.
		if intsContain(b.Weekdays, wd) && cur >= start {
			return true, "schedule"
		}
		if intsContain(b.Weekdays, prevWd) && cur < end {
			return true, "schedule"
		}
	}
	return false, ""
}

func parseHM(s string) (int, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func intsContain(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
