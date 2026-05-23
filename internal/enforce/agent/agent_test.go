package agent_test

import (
	"context"
	"testing"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/enforce"
	"github.com/gricce/SafePlay-Guardian/internal/enforce/agent"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
)

func TestApplyQueuesSliceAndPullPendingConsumes(t *testing.T) {
	b := agent.New()
	slice := enforce.Slice{
		Caps:   []device.Capability{device.CapScreenLock, device.CapDomainBlock},
		Policy: policy.ResolvedPolicy{ScreenLocked: true, LockReason: "schedule"},
	}
	res := b.Apply(context.Background(), "dev-1", slice)
	if len(res.PerRule) != 2 {
		t.Fatalf("Apply must report one PerRule per cap, got %d", len(res.PerRule))
	}
	for _, r := range res.PerRule {
		if r.Outcome != enforce.RuleApplied {
			t.Errorf("cap %s outcome %s, want applied", r.Capability, r.Outcome)
		}
	}
	got := b.PullPending("dev-1")
	if got == nil || len(got.Caps) != 2 || !got.Policy.ScreenLocked {
		t.Fatalf("PullPending must return the queued slice, got %+v", got)
	}
	if again := b.PullPending("dev-1"); again != nil {
		t.Fatalf("PullPending must clear after consume, got %+v", again)
	}
}

func TestPullPendingForUnknownDevice(t *testing.T) {
	b := agent.New()
	if v := b.PullPending("nope"); v != nil {
		t.Fatalf("expected nil for unknown device, got %+v", v)
	}
}

func TestCanHandleGatesOnAgentToken(t *testing.T) {
	b := agent.New()
	if b.CanHandle(nil) {
		t.Fatal("nil device must be unhandled")
	}
	if b.CanHandle(&device.Device{ID: "x"}) {
		t.Fatal("device without agent_token must be unhandled")
	}
	tok := "t"
	if !b.CanHandle(&device.Device{ID: "x", AgentToken: &tok}) {
		t.Fatal("device with agent_token must be handled")
	}
	empty := ""
	if b.CanHandle(&device.Device{ID: "x", AgentToken: &empty}) {
		t.Fatal("empty agent_token must not count as handled")
	}
}

func TestRevokeDropsPending(t *testing.T) {
	b := agent.New()
	b.Apply(context.Background(), "d", enforce.Slice{Caps: []device.Capability{device.CapScreenLock}})
	if err := b.Revoke(context.Background(), "d"); err != nil {
		t.Fatal(err)
	}
	if v := b.PullPending("d"); v != nil {
		t.Fatalf("Revoke must drop pending slice, got %+v", v)
	}
}
