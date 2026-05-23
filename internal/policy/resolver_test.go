package policy_test

import (
	"context"
	"testing"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

// --- test doubles ---

type fakeDevices struct{ d device.Device }

func (f fakeDevices) Get(_ context.Context, id string) (*device.Device, error) {
	if id != f.d.ID {
		return nil, device.ErrNotFound
	}
	c := f.d
	return &c, nil
}

type fakeStore struct {
	policies  map[string]*policy.Policy
	overrides map[string]*policy.Override
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		policies:  map[string]*policy.Policy{},
		overrides: map[string]*policy.Override{},
	}
}

func (f *fakeStore) GetPersonPolicy(_ context.Context, id string) (*policy.Policy, error) {
	return f.policies[id], nil
}
func (f *fakeStore) SetPersonPolicy(_ context.Context, id string, p *policy.Policy) error {
	f.policies[id] = p
	return nil
}
func (f *fakeStore) GetDeviceOverride(_ context.Context, id string) (*policy.Override, error) {
	return f.overrides[id], nil
}
func (f *fakeStore) SetDeviceOverride(_ context.Context, id string, o *policy.Override) error {
	f.overrides[id] = o
	return nil
}

type fixedUsage struct{ d time.Duration }

func (f fixedUsage) UsedSince(_ context.Context, _ string, _ time.Time) (time.Duration, error) {
	return f.d, nil
}

// --- setup helpers ---

func ptr[T any](v T) *T { return &v }

func newResolver(t *testing.T, dev device.Device, p *policy.Policy, o *policy.Override, used time.Duration) *policy.Resolver {
	t.Helper()
	store := newFakeStore()
	if dev.PersonID != nil && p != nil {
		store.policies[*dev.PersonID] = p
	}
	if o != nil {
		store.overrides[dev.ID] = o
	}
	return &policy.Resolver{
		Devices: fakeDevices{d: dev},
		Store:   store,
		Usage:   fixedUsage{d: used},
	}
}

func mondayAt(hour, min int) time.Time {
	// 2026-01-05 is a Monday in UTC; we use UTC here and let the Timezone
	// field control localization.
	return time.Date(2026, 1, 5, hour, min, 0, 0, time.UTC)
}

// --- tests ---

// The headline test from CORE_PLATFORM.md:
// "at 21:05 with 2h10m used, is the device locked?"
func TestResolveLockedByBudget_2105_2h10m(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "UTC",
		TimeBudget: &policy.TimeBudget{
			MinutesByWeekday: [7]int{120, 120, 120, 120, 120, 120, 120}, // 2h every day
		},
	}
	r := newResolver(t, dev, p, nil, 2*time.Hour+10*time.Minute)

	got, err := r.Resolve(context.Background(), "d1", mondayAt(21, 5))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScreenLocked {
		t.Fatal("expected ScreenLocked=true at 21:05 with 130 min used and 120 min budget")
	}
	if got.LockReason != "time_budget_exceeded" {
		t.Fatalf("LockReason = %q, want time_budget_exceeded", got.LockReason)
	}
	if got.TimeBudgetMinutes != 120 || got.TimeBudgetUsedMins != 130 {
		t.Fatalf("budget reporting wrong: %+v", got)
	}
}

func TestResolveBudgetNotExceeded(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "UTC",
		TimeBudget: &policy.TimeBudget{
			MinutesByWeekday: [7]int{120, 120, 120, 120, 120, 120, 120},
		},
	}
	r := newResolver(t, dev, p, nil, 1*time.Hour)
	got, _ := r.Resolve(context.Background(), "d1", mondayAt(18, 0))
	if got.ScreenLocked {
		t.Fatalf("not exceeded but ScreenLocked=true: %+v", got)
	}
}

// Schedule windows: bedtime block 22:00-07:00 (wraps midnight) on every weekday.
func TestResolveScheduleBedtime(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "UTC",
		Schedule: &policy.Schedule{
			Blocks: []policy.ScheduleBlock{
				{Kind: "blocked", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Start: "22:00", End: "07:00"},
			},
		},
	}
	r := newResolver(t, dev, p, nil, 0)

	// 22:30 Monday: inside the evening half of the window → locked
	if got, _ := r.Resolve(context.Background(), "d1", mondayAt(22, 30)); !got.ScreenLocked || got.LockReason != "schedule" {
		t.Fatalf("22:30 Monday should be locked by schedule, got %+v", got)
	}
	// 21:30 Monday: outside the window → not locked
	if got, _ := r.Resolve(context.Background(), "d1", mondayAt(21, 30)); got.ScreenLocked {
		t.Fatalf("21:30 Monday should NOT be locked, got %+v", got)
	}
	// 03:00 Tuesday: wraps from Monday night → locked (Monday is listed,
	// previous weekday = Monday and time < 07:00).
	tuesday := mondayAt(0, 0).AddDate(0, 0, 1).Add(3 * time.Hour)
	if got, _ := r.Resolve(context.Background(), "d1", tuesday); !got.ScreenLocked || got.LockReason != "schedule" {
		t.Fatalf("Tue 03:00 should be locked by schedule wrap, got %+v", got)
	}
}

// Override wins field-by-field: person policy says blocked at 22:00, device
// override pushes bedtime back to 23:00 — at 22:30 the device must NOT be locked.
func TestResolveOverrideWinsFieldByField(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "UTC",
		Schedule: &policy.Schedule{
			Blocks: []policy.ScheduleBlock{
				{Kind: "blocked", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Start: "22:00", End: "07:00"},
			},
		},
		Content: &policy.Content{
			BlockedDomains: []string{"example.com"},
		},
	}
	o := &policy.Override{
		Schedule: &policy.Schedule{
			Blocks: []policy.ScheduleBlock{
				{Kind: "blocked", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Start: "23:00", End: "07:00"},
			},
		},
	}
	r := newResolver(t, dev, p, o, 0)

	// 22:30 Monday: person's schedule locks, but override schedule starts at 23:00 → not locked.
	got, _ := r.Resolve(context.Background(), "d1", mondayAt(22, 30))
	if got.ScreenLocked {
		t.Fatalf("override should push bedtime to 23:00; 22:30 should be free, got %+v", got)
	}
	// Content was NOT overridden, so the original blocked_domains must still be there.
	if len(got.BlockedDomains) != 1 || got.BlockedDomains[0] != "example.com" {
		t.Fatalf("override clobbered Content field that should have fallen through: %+v", got.BlockedDomains)
	}
}

// Device with no owner gets a default ResolvedPolicy (unlocked, default log level).
func TestResolveNoOwnerDevice(t *testing.T) {
	dev := device.Device{ID: "d1"} // PersonID nil
	r := newResolver(t, dev, nil, nil, 0)
	got, err := r.Resolve(context.Background(), "d1", mondayAt(22, 30))
	if err != nil {
		t.Fatal(err)
	}
	if got.ScreenLocked {
		t.Fatalf("ownerless device should not be locked: %+v", got)
	}
	if got.LogLevel != policy.MonitoringEnforcementOnly {
		t.Fatalf("LogLevel default should be enforcement_only, got %s", got.LogLevel)
	}
}

// Resolver respects the policy's IANA timezone: 22:05 UTC is 14:05 in
// America/Los_Angeles, well before bedtime defined in local time.
func TestResolveAppliesTimezone(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "America/Los_Angeles",
		Schedule: &policy.Schedule{
			Blocks: []policy.ScheduleBlock{
				{Kind: "blocked", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Start: "22:00", End: "07:00"},
			},
		},
	}
	r := newResolver(t, dev, p, nil, 0)

	utc22 := time.Date(2026, 1, 5, 22, 5, 0, 0, time.UTC) // 14:05 in LA → not locked
	got, _ := r.Resolve(context.Background(), "d1", utc22)
	if got.ScreenLocked {
		t.Fatalf("22:05 UTC = 14:05 LA should not be locked: %+v", got)
	}

	utc06 := time.Date(2026, 1, 6, 6, 5, 0, 0, time.UTC) // 22:05 LA on Mon → locked
	got, _ = r.Resolve(context.Background(), "d1", utc06)
	if !got.ScreenLocked {
		t.Fatalf("06:05 UTC = 22:05 LA Mon should be locked: %+v", got)
	}
}

// Schedule lock wins over budget when both fire, but budget is reported anyway.
func TestResolveScheduleBeatsBudget(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	level := policy.MonitoringMetadata
	p := &policy.Policy{
		Timezone: "UTC",
		Schedule: &policy.Schedule{
			Blocks: []policy.ScheduleBlock{
				{Kind: "blocked", Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, Start: "22:00", End: "07:00"},
			},
		},
		TimeBudget: &policy.TimeBudget{
			MinutesByWeekday: [7]int{60, 60, 60, 60, 60, 60, 60},
		},
		Monitoring: &level,
	}
	r := newResolver(t, dev, p, nil, 90*time.Minute)
	got, _ := r.Resolve(context.Background(), "d1", mondayAt(22, 30))
	if got.LockReason != "schedule" {
		t.Fatalf("schedule should win over budget when both fire, got reason %q", got.LockReason)
	}
	if got.TimeBudgetUsedMins != 90 {
		t.Fatalf("budget usage should still be reported: %+v", got)
	}
	if got.LogLevel != policy.MonitoringMetadata {
		t.Fatalf("LogLevel = %s, want metadata", got.LogLevel)
	}
}

// Defensive: pinned ensures Monday in our test fixture.
func TestMondayFixture(t *testing.T) {
	if mondayAt(0, 0).Weekday() != time.Monday {
		t.Fatal("fixture date is not a Monday")
	}
}
