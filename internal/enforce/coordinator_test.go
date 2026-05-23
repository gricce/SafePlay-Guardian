package enforce_test

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/noop"
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

// fakeBackend declares an arbitrary set of capabilities and records calls.
type fakeBackend struct {
	name     string
	caps     map[device.Capability]bool
	handle   func(*device.Device) bool
	applies  int
	revokes  int
}

func (f *fakeBackend) Name() string                                   { return f.name }
func (f *fakeBackend) Capabilities() map[device.Capability]bool       { return f.caps }
func (f *fakeBackend) CanHandle(d *device.Device) bool {
	if f.handle == nil {
		return true
	}
	return f.handle(d)
}
func (f *fakeBackend) Apply(_ context.Context, deviceID string, slice enforce.Slice) enforce.Result {
	f.applies++
	res := enforce.Result{Backend: f.name, DeviceID: deviceID}
	for _, c := range slice.Caps {
		res.PerRule = append(res.PerRule, enforce.PerRule{Capability: c, Outcome: enforce.RuleApplied, Note: "fake"})
	}
	return res
}
func (f *fakeBackend) Revoke(_ context.Context, _ string) error { f.revokes++; return nil }

// --- helpers ---

func resolverWithPolicy(dev device.Device, p *policy.Policy) (*policy.Resolver, *fakeStore) {
	store := &fakeStore{policies: map[string]*policy.Policy{}, overrides: map[string]*policy.Override{}}
	if dev.PersonID != nil && p != nil {
		store.policies[*dev.PersonID] = p
	}
	return &policy.Resolver{
		Devices: fakeDevices{d: dev},
		Store:   store,
		Usage:   policy.NoUsage{},
	}, store
}

func ruleCaps(r []enforce.RuleOutcome) []device.Capability {
	out := make([]device.Capability, 0, len(r))
	for _, x := range r {
		out = append(out, x.Capability)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func atUTC(h, m int) time.Time {
	return time.Date(2026, 1, 5, h, m, 0, 0, time.UTC)
}

// --- tests ---

func TestCoordinatorRoutesAllCapsToNoop(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	level := policy.MonitoringMetadata
	p := &policy.Policy{
		Timezone: "UTC",
		TimeBudget: &policy.TimeBudget{
			MinutesByWeekday: [7]int{60, 60, 60, 60, 60, 60, 60},
		},
		Schedule: &policy.Schedule{Blocks: []policy.ScheduleBlock{
			{Kind: "blocked", Weekdays: []int{1}, Start: "22:00", End: "07:00"},
		}},
		Content:    &policy.Content{BlockedDomains: []string{"a.com"}, BlockedApps: []string{"roblox"}},
		Monitoring: &level,
	}
	resolver, _ := resolverWithPolicy(dev, p)
	c := &enforce.Coordinator{
		Backends: []enforce.Enforcer{noop.Backend{}},
		Resolver: resolver,
		Devices:  fakeDevices{d: dev},
	}
	got, err := c.Enforce(context.Background(), "d1", atUTC(18, 0))
	if err != nil {
		t.Fatal(err)
	}
	caps := ruleCaps(got.Rules)
	want := []device.Capability{
		device.CapAppBlock, device.CapDomainBlock, device.CapScheduleBlock, device.CapTimeBudget,
	}
	if !reflect.DeepEqual(caps, want) {
		t.Fatalf("rule caps = %v, want %v", caps, want)
	}
	for _, r := range got.Rules {
		if r.Outcome != enforce.RuleApplied {
			t.Errorf("cap %s outcome %s, want applied", r.Capability, r.Outcome)
		}
		if r.Backend != "noop" {
			t.Errorf("cap %s backend %s, want noop", r.Capability, r.Backend)
		}
	}
}

func TestCoordinatorReportsUnenforceableGap(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{
		Timezone: "UTC",
		Content:  &policy.Content{BlockedDomains: []string{"a.com"}},
		Schedule: &policy.Schedule{Blocks: []policy.ScheduleBlock{
			{Kind: "blocked", Weekdays: []int{1}, Start: "22:00", End: "07:00"},
		}},
	}
	resolver, _ := resolverWithPolicy(dev, p)
	// Backend only handles schedule_block, not domain_block.
	partial := &fakeBackend{name: "agent", caps: map[device.Capability]bool{device.CapScheduleBlock: true}}
	c := &enforce.Coordinator{
		Backends: []enforce.Enforcer{partial},
		Resolver: resolver,
		Devices:  fakeDevices{d: dev},
	}
	got, _ := c.Enforce(context.Background(), "d1", atUTC(18, 0))

	var sched, dom *enforce.RuleOutcome
	for i := range got.Rules {
		switch got.Rules[i].Capability {
		case device.CapScheduleBlock:
			sched = &got.Rules[i]
		case device.CapDomainBlock:
			dom = &got.Rules[i]
		}
	}
	if sched == nil || sched.Outcome != enforce.RuleApplied || sched.Backend != "agent" {
		t.Fatalf("schedule_block should be applied by agent, got %+v", sched)
	}
	if dom == nil || dom.Outcome != enforce.RuleUnenforceable {
		t.Fatalf("domain_block should be unenforceable, got %+v", dom)
	}
	if dom.Backend != "" {
		t.Fatalf("unenforceable rule should not record a backend, got %q", dom.Backend)
	}
}

func TestCoordinatorIdempotency(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{Timezone: "UTC", Content: &policy.Content{BlockedDomains: []string{"a.com"}}}
	resolver, _ := resolverWithPolicy(dev, p)
	c := &enforce.Coordinator{
		Backends: []enforce.Enforcer{noop.Backend{}},
		Resolver: resolver,
		Devices:  fakeDevices{d: dev},
	}
	r1, _ := c.Enforce(context.Background(), "d1", atUTC(18, 0))
	r2, _ := c.Enforce(context.Background(), "d1", atUTC(18, 0))
	if !reflect.DeepEqual(ruleCaps(r1.Rules), ruleCaps(r2.Rules)) {
		t.Fatal("re-applying same policy produced different rule set")
	}
	for i := range r1.Rules {
		if r1.Rules[i].Outcome != r2.Rules[i].Outcome {
			t.Fatal("outcome changed on re-apply")
		}
	}
}

func TestCoordinatorSkipsBackendThatCannotHandle(t *testing.T) {
	personID := "alice"
	dev := device.Device{ID: "d1", PersonID: &personID}
	p := &policy.Policy{Timezone: "UTC", Content: &policy.Content{BlockedDomains: []string{"a.com"}}}
	resolver, _ := resolverWithPolicy(dev, p)

	rejecting := &fakeBackend{
		name:   "rejecting",
		caps:   map[device.Capability]bool{device.CapDomainBlock: true},
		handle: func(_ *device.Device) bool { return false },
	}
	accepting := &fakeBackend{
		name: "accepting",
		caps: map[device.Capability]bool{device.CapDomainBlock: true},
	}
	c := &enforce.Coordinator{
		Backends: []enforce.Enforcer{rejecting, accepting},
		Resolver: resolver,
		Devices:  fakeDevices{d: dev},
	}
	got, _ := c.Enforce(context.Background(), "d1", atUTC(18, 0))
	if rejecting.applies != 0 {
		t.Fatalf("rejecting backend should not be called, got %d Apply", rejecting.applies)
	}
	if accepting.applies != 1 {
		t.Fatalf("accepting backend should be called once, got %d", accepting.applies)
	}
	if len(got.Rules) != 1 || got.Rules[0].Backend != "accepting" {
		t.Fatalf("expected one rule routed to accepting, got %+v", got.Rules)
	}
}

func TestCoordinatorOwnerlessDeviceProducesNoRules(t *testing.T) {
	dev := device.Device{ID: "d1"} // no PersonID
	resolver, _ := resolverWithPolicy(dev, nil)
	c := &enforce.Coordinator{
		Backends: []enforce.Enforcer{noop.Backend{}},
		Resolver: resolver,
		Devices:  fakeDevices{d: dev},
	}
	got, err := c.Enforce(context.Background(), "d1", atUTC(18, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 0 {
		t.Fatalf("ownerless device should produce no enforcement rules, got %+v", got.Rules)
	}
}
