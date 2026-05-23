package event_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/event"
	"github.com/gricce/SafePlay-Guardian/internal/policy"
	"github.com/gricce/SafePlay-Guardian/internal/store"
	"github.com/gricce/SafePlay-Guardian/migrations"
	_ "modernc.org/sqlite"
)

type fakePolicies struct {
	level *policy.MonitoringLevel
}

func (f fakePolicies) GetPersonPolicy(_ context.Context, _ string) (*policy.Policy, error) {
	if f.level == nil {
		return nil, nil
	}
	return &policy.Policy{Monitoring: f.level}, nil
}
func (f fakePolicies) GetDeviceOverride(_ context.Context, _ string) (*policy.Override, error) {
	return nil, nil
}

func newTestStore(t *testing.T) *event.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "t.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return event.NewStore(db)
}

func TestStoreInsertAndRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "p"
	for i := 0; i < 3; i++ {
		err := s.Insert(ctx, event.Event{
			DeviceID: "d1", PersonID: &pid,
			Kind:  event.KindDeviceSeen,
			At:    time.Now().UTC().Add(-time.Duration(i) * time.Minute),
			Attrs: map[string]any{"source": "scan"},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Recent(ctx, event.RecentFilter{DeviceID: "d1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
}

func TestStoreRejectsBadKind(t *testing.T) {
	s := newTestStore(t)
	err := s.Insert(context.Background(), event.Event{DeviceID: "d", Kind: "bogus"})
	if err == nil {
		t.Fatal("invalid kind must be rejected")
	}
}

func TestRedactByMonitoringLevel(t *testing.T) {
	ev := event.Event{
		Kind:     event.KindBlock,
		DeviceID: "d",
		Attrs: map[string]any{
			"capability": "domain_block",
			"value":      "facebook.com",
			"category":   "social",
		},
	}

	enf := ev
	event.Redact(&enf, policy.MonitoringEnforcementOnly)
	if _, ok := enf.Attrs["value"]; ok {
		t.Fatalf("enforcement_only must strip value: %+v", enf.Attrs)
	}
	if _, ok := enf.Attrs["category"]; ok {
		t.Fatalf("enforcement_only must strip category: %+v", enf.Attrs)
	}
	if enf.Attrs["capability"] != "domain_block" {
		t.Fatalf("enforcement_only must keep capability: %+v", enf.Attrs)
	}

	meta := ev
	meta.Attrs = map[string]any{"capability": "domain_block", "value": "facebook.com", "category": "social"}
	event.Redact(&meta, policy.MonitoringMetadata)
	if _, ok := meta.Attrs["value"]; ok {
		t.Fatalf("metadata level must strip value: %+v", meta.Attrs)
	}
	if meta.Attrs["category"] != "social" {
		t.Fatalf("metadata level must keep category: %+v", meta.Attrs)
	}

	gran := ev
	gran.Attrs = map[string]any{"capability": "domain_block", "value": "facebook.com", "category": "social"}
	event.Redact(&gran, policy.MonitoringGranular)
	if gran.Attrs["value"] != "facebook.com" {
		t.Fatalf("granular must keep value: %+v", gran.Attrs)
	}
}

func TestRedactLeavesNonBlockEvents(t *testing.T) {
	ev := event.Event{Kind: event.KindPolicyApplied, Attrs: map[string]any{"capability": "domain_block", "value": "anything"}}
	event.Redact(&ev, policy.MonitoringEnforcementOnly)
	if ev.Attrs["value"] != "anything" {
		t.Fatalf("non-block events must not be redacted: %+v", ev.Attrs)
	}
}

func TestLoggerAppliesEffectiveLevel(t *testing.T) {
	s := newTestStore(t)
	enf := policy.MonitoringEnforcementOnly
	logger := &event.Logger{Store: s, Lookup: fakePolicies{level: &enf}}

	pid := "p1"
	err := logger.Append(context.Background(), event.Event{
		DeviceID: "d1", PersonID: &pid,
		Kind: event.KindBlock,
		Attrs: map[string]any{
			"capability": "domain_block",
			"value":      "facebook.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Recent(context.Background(), event.RecentFilter{DeviceID: "d1", Limit: 10})
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if _, ok := got[0].Attrs["value"]; ok {
		t.Fatalf("stored event still has value at enforcement_only: %+v", got[0].Attrs)
	}
}

func TestRetentionDeletesOldRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Insert with backdated `at`. recorded_at gets CURRENT_TIMESTAMP, so we
	// can't backdate it without raw SQL. The retention job uses recorded_at,
	// so we use raw SQL here to set an old recorded_at for the test.
	if err := s.Insert(ctx, event.Event{DeviceID: "d1", Kind: event.KindDeviceSeen}); err != nil {
		t.Fatal(err)
	}
	// Build a deliberately ancient row.
	db := storeDB(t, s)
	old := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events (id, at, recorded_at, kind, device_id, severity, attrs)
         VALUES ('old', ?, ?, ?, 'd1', 'info', '{}')`,
		old, old, string(event.KindDeviceSeen)); err != nil {
		t.Fatal(err)
	}
	n, err := s.DeleteOlderThan(ctx, time.Now().UTC().Add(-14*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row deleted, got %d", n)
	}
	left, _ := s.Recent(ctx, event.RecentFilter{DeviceID: "d1", Limit: 10})
	if len(left) != 1 {
		t.Fatalf("expected 1 row left, got %d", len(left))
	}
}

func TestUsageSinceChainsSightings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Minute)
	// 5 sightings 30s apart → 4 gaps of 30s = 120s = 2 min.
	for i := 0; i < 5; i++ {
		if err := s.Insert(ctx, event.Event{
			DeviceID: "d1", Kind: event.KindDeviceSeen,
			At: base.Add(time.Duration(i) * 30 * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Lone sighting later, gap > 5 min — should NOT count.
	if err := s.Insert(ctx, event.Event{
		DeviceID: "d1", Kind: event.KindDeviceSeen,
		At: base.Add(20 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.UsageSince(ctx, "d1", base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got != 2*time.Minute {
		t.Fatalf("UsageSince = %s, want 2m", got)
	}
}

// storeDB is a tiny escape hatch for tests that need raw SQL access.
func storeDB(t *testing.T, s *event.Store) *sql.DB {
	t.Helper()
	type dbHolder interface{ DB() *sql.DB }
	if h, ok := any(s).(dbHolder); ok {
		return h.DB()
	}
	t.Fatal("event.Store does not expose DB() — add it for tests")
	return nil
}
