package policy

import (
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
)

type MonitoringLevel string

const (
	MonitoringEnforcementOnly MonitoringLevel = "enforcement_only"
	MonitoringMetadata        MonitoringLevel = "metadata"
	MonitoringGranular        MonitoringLevel = "granular"
)

// TimeBudget caps daily screen time per weekday. Index = time.Weekday()
// (Sunday=0..Saturday=6). Zero means no time allowed that day; absence of
// the whole TimeBudget rule means uncapped.
type TimeBudget struct {
	MinutesByWeekday [7]int `json:"minutes_by_weekday"`
}

type ScheduleBlock struct {
	Kind     string `json:"kind"`     // currently only "blocked" is acted on
	Weekdays []int  `json:"weekdays"` // 0..6, time.Weekday-compatible
	Start    string `json:"start"`    // "HH:MM"
	End      string `json:"end"`      // "HH:MM"; if End < Start the window wraps midnight onto the next day
}

type Schedule struct {
	Blocks []ScheduleBlock `json:"blocks"`
}

type Content struct {
	BlockedDomains    []string `json:"blocked_domains"`
	BlockedApps       []string `json:"blocked_apps"`
	BlockedCategories []string `json:"blocked_categories"`
}

// Policy is the person-level intent: every field is optional except Timezone.
type Policy struct {
	Timezone   string           `json:"timezone"`
	TimeBudget *TimeBudget      `json:"time_budget,omitempty"`
	Schedule   *Schedule        `json:"schedule,omitempty"`
	Content    *Content         `json:"content,omitempty"`
	Monitoring *MonitoringLevel `json:"monitoring,omitempty"`
}

// Override is a per-device partial. Pointer fields distinguish absent (=fall
// through to the person policy) from set (=replace this field).
type Override struct {
	Timezone   *string          `json:"timezone,omitempty"`
	TimeBudget *TimeBudget      `json:"time_budget,omitempty"`
	Schedule   *Schedule        `json:"schedule,omitempty"`
	Content    *Content         `json:"content,omitempty"`
	Monitoring *MonitoringLevel `json:"monitoring,omitempty"`
}

type ResolvedPolicy struct {
	DeviceID string    `json:"device_id"`
	At       time.Time `json:"at"`
	PersonID *string   `json:"person_id,omitempty"`

	ScreenLocked bool   `json:"screen_locked"`
	LockReason   string `json:"lock_reason,omitempty"`

	BlockedDomains    []string        `json:"blocked_domains"`
	BlockedApps       []string        `json:"blocked_apps"`
	BlockedCategories []string        `json:"blocked_categories"`
	LogLevel          MonitoringLevel `json:"log_level"`

	TimeBudgetMinutes  int `json:"time_budget_minutes,omitempty"`
	TimeBudgetUsedMins int `json:"time_budget_used_mins,omitempty"`

	// RequiredCaps is the set of enforcement capabilities a backend must offer
	// to fully realize this policy. Derived from rule presence (not "is the
	// rule firing right now"), so the coordinator can flag unenforceable gaps
	// proactively rather than only when a rule's window happens to fire.
	RequiredCaps []device.Capability `json:"required_caps"`
}

// RequiredCaps inspects a (merged) policy and returns the enforcement
// capabilities a backend must implement to enforce it.
func RequiredCaps(p *Policy) []device.Capability {
	if p == nil {
		return nil
	}
	var caps []device.Capability
	if p.Schedule != nil && len(p.Schedule.Blocks) > 0 {
		caps = append(caps, device.CapScheduleBlock)
	}
	if p.TimeBudget != nil {
		for _, m := range p.TimeBudget.MinutesByWeekday {
			if m > 0 {
				caps = append(caps, device.CapTimeBudget)
				break
			}
		}
	}
	if p.Content != nil {
		if len(p.Content.BlockedDomains) > 0 {
			caps = append(caps, device.CapDomainBlock)
		}
		if len(p.Content.BlockedApps) > 0 {
			caps = append(caps, device.CapAppBlock)
		}
	}
	return caps
}
