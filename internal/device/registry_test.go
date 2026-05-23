package device_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/gricce/SafePlay-Guardian/internal/device"
	"github.com/gricce/SafePlay-Guardian/internal/store"
	"github.com/gricce/SafePlay-Guardian/migrations"
	_ "modernc.org/sqlite"
)

func newTestRegistry(t *testing.T) *device.SQLiteRegistry {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return device.NewSQLiteRegistry(db)
}

func TestUpsertGetRoundTrip(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	got, err := r.Upsert(ctx, device.Device{
		Label: "Alice iPad",
		Kind:  "tablet",
		State: device.StateActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Fatal("expected generated ID")
	}
	if got.Label != "Alice iPad" || got.Kind != "tablet" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.State != device.StateActive {
		t.Fatalf("state mismatch: %s", got.State)
	}

	again, err := r.Get(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != got.ID {
		t.Fatal("Get returned different device")
	}
}

func TestUpsertIdempotent(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	first, err := r.Upsert(ctx, device.Device{ID: "fixed-id", Label: "x", Kind: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Upsert(ctx, device.Device{ID: "fixed-id", Label: "x", Kind: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("idempotent upsert changed id")
	}

	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 device, got %d", len(all))
	}
}

func TestListFilterByState(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	if _, err := r.Upsert(ctx, device.Device{Label: "a", Kind: "phone", State: device.StateActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Upsert(ctx, device.Device{Label: "b", Kind: "phone", State: device.StatePaused}); err != nil {
		t.Fatal(err)
	}

	active, err := r.List(ctx, device.Filter{State: device.StateActive})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Label != "a" {
		t.Fatalf("expected only active=a, got %+v", active)
	}
}

func TestReconcileByAgentID(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	id1, err := r.Reconcile(ctx, device.Observation{
		Source: "agent", SeenAt: time.Now(), AgentID: "agent-xyz",
		Caps: []device.Capability{device.CapScreenLock},
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := r.Reconcile(ctx, device.Observation{
		Source: "agent", SeenAt: time.Now(), AgentID: "agent-xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("same agent_id produced two devices: %s vs %s", id1, id2)
	}

	d, err := r.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Caps) != 1 || d.Caps[0] != device.CapScreenLock {
		t.Fatalf("expected screen_lock cap recorded, got %+v", d.Caps)
	}
}

func TestReconcileByNonRandomizedMAC(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	// 00:1a:2b:... — first byte 0x00, locally-administered bit clear → universal.
	id1, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: time.Now(), MAC: "00:1a:2b:3c:4d:5e",
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: time.Now(), MAC: "00:1a:2b:3c:4d:5e",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("non-randomized MAC should match: %s vs %s", id1, id2)
	}
}

func TestReconcileRandomizedMACDoesNotMergeWithoutAgent(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	// 02:... — locally-administered bit set → randomized.
	id1, err := r.Reconcile(ctx, device.Observation{Source: "scan", SeenAt: time.Now(), MAC: "02:11:22:33:44:55"})
	if err != nil {
		t.Fatal(err)
	}
	// Same randomized MAC seen again. Phase 1 minimal reconciler creates a new
	// device because randomized MAC is too weak to merge on alone, and the
	// previous insertion never recorded that MAC as an identity.
	id2, err := r.Reconcile(ctx, device.Observation{Source: "scan", SeenAt: time.Now(), MAC: "02:11:22:33:44:55"})
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatal("randomized MAC should not auto-merge in minimal reconciler")
	}
}

// --- Phase 2 reconciliation tests (the five from CORE_PLATFORM.md) ---

// 1. scan-then-agent for one device → one device, not two.
func TestReconcileScanThenAgent(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()
	now := time.Now()

	scanID, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now,
		MAC: "00:1a:2b:3c:4d:5e", Hostname: "alice-laptop",
	})
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := r.Reconcile(ctx, device.Observation{
		Source: "agent", SeenAt: now.Add(time.Minute),
		AgentID: "agent-fresh", MAC: "00:1a:2b:3c:4d:5e", Hostname: "alice-laptop",
		Caps: []device.Capability{device.CapScreenLock},
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanID != agentID {
		t.Fatalf("scan-then-agent produced two devices: %s vs %s", scanID, agentID)
	}
	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 device after scan-then-agent merge, got %d", len(all))
	}
}

// 2. randomized MAC rotates mid-session → no duplicate, no wrong merge.
func TestReconcileRandomizedMACRotation(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()
	now := time.Now()

	id1, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now,
		MAC: "02:aa:aa:aa:aa:01", IP: "192.168.1.10", Hostname: "alice-phone",
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now.Add(30 * time.Second),
		MAC: "02:aa:aa:aa:aa:02", IP: "192.168.1.10", Hostname: "alice-phone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("randomized-MAC rotation should corroborate via IP+hostname: %s vs %s", id1, id2)
	}
	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 device across MAC rotation, got %d", len(all))
	}
}

// 3. two devices briefly share an IP across a DHCP lease → must NOT merge.
func TestReconcileDHCPSharedIPDoesNotMerge(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()
	now := time.Now()

	idA, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now,
		MAC: "00:11:22:33:44:55", IP: "192.168.1.50", Hostname: "laptop-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	idB, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now.Add(time.Hour),
		MAC: "00:66:77:88:99:aa", IP: "192.168.1.50", Hostname: "phone-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if idA == idB {
		t.Fatal("DHCP-shared IP must not merge distinct devices (only 1 weak signal in common)")
	}
}

// 4. agent check-in for a manually-added device → claims it, no second device.
func TestReconcileAgentClaimsManualDevice(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	token := "pre-provisioned-secret"
	manual, err := r.Upsert(ctx, device.Device{
		Label: "Alice iPad", Kind: "tablet", State: device.StateActive,
		AgentToken: &token,
	})
	if err != nil {
		t.Fatal(err)
	}

	claimedID, err := r.Reconcile(ctx, device.Observation{
		Source: "agent", SeenAt: time.Now(),
		AgentToken: token, AgentID: "agent-xyz",
		MAC: "00:1a:2b:3c:4d:5e", Hostname: "alice-ipad",
		Caps: []device.Capability{device.CapAppBlock},
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimedID != manual.ID {
		t.Fatalf("agent did not claim manual device: %s vs %s", claimedID, manual.ID)
	}
	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected only the manually-added device, got %d", len(all))
	}
	d, err := r.Get(ctx, claimedID)
	if err != nil {
		t.Fatal(err)
	}
	hasAgentIdent := false
	for _, i := range d.Identities {
		if i.Kind == device.IdentityAgent && i.Value == "agent-xyz" {
			hasAgentIdent = true
		}
	}
	if !hasAgentIdent {
		t.Fatal("claim did not record agent_id identity on the existing device")
	}
}

// 5. replaying an identical observation → no-op.
func TestReconcileReplayIdentical(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()

	obs := device.Observation{
		Source: "agent", SeenAt: time.Now(),
		AgentID: "agent-replay", MAC: "00:1a:2b:3c:4d:5e",
		IP: "192.168.1.20", Hostname: "alice-phone",
		Caps: []device.Capability{device.CapScreenLock},
	}
	id1, err := r.Reconcile(ctx, obs)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := r.Reconcile(ctx, obs)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("replay created a second device: %s vs %s", id1, id2)
	}
	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 device after replay, got %d", len(all))
	}
	d, err := r.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, i := range d.Identities {
		seen[string(i.Kind)+":"+i.Value]++
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("duplicate identity after replay: %s appears %d times", k, n)
		}
	}
	if len(d.Caps) != 1 || d.Caps[0] != device.CapScreenLock {
		t.Fatalf("replay altered caps: %+v", d.Caps)
	}
}

// Bonus: weak signals that match 2+ existing devices yield a pending merge
// rather than a silent (wrong) auto-merge.
func TestReconcileAmbiguousFlagsPending(t *testing.T) {
	r := newTestRegistry(t)
	ctx := context.Background()
	now := time.Now()

	sharedIP := "192.168.1.99"
	sharedHost := "shared-name"
	mkSeen := func(t time.Time) *time.Time { return &t }
	_, err := r.Upsert(ctx, device.Device{
		Label: "A", Kind: "phone", State: device.StateActive,
		Identities: []device.NetworkIdentity{
			{Kind: device.IdentityIP, Value: sharedIP, LastSeen: mkSeen(now)},
			{Kind: device.IdentityHostname, Value: sharedHost, LastSeen: mkSeen(now)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Upsert(ctx, device.Device{
		Label: "B", Kind: "phone", State: device.StateActive,
		Identities: []device.NetworkIdentity{
			{Kind: device.IdentityIP, Value: sharedIP, LastSeen: mkSeen(now)},
			{Kind: device.IdentityHostname, Value: sharedHost, LastSeen: mkSeen(now)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	newID, err := r.Reconcile(ctx, device.Observation{
		Source: "scan", SeenAt: now.Add(time.Minute),
		IP: sharedIP, Hostname: sharedHost,
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := r.List(ctx, device.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ambiguous obs should create a third (pending) device, got %d total", len(all))
	}

	var pendingCount int
	if err := r.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_merges WHERE source_device_id = ?`, newID).Scan(&pendingCount); err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending_merge row for source device %s, got %d", newID, pendingCount)
	}
}

func TestIsRandomizedMAC(t *testing.T) {
	cases := []struct {
		mac      string
		expected bool
	}{
		{"00:1a:2b:3c:4d:5e", false}, // universal
		{"02:11:22:33:44:55", true},  // locally administered
		{"AA-BB-CC-DD-EE-FF", true},  // dash separator, 0xAA → 0x02 bit set
		{"04:00:00:00:00:00", false}, // 0x04 → 0x02 bit clear
		{"06:00:00:00:00:00", true},  // 0x06 → 0x02 bit set
		{"", false},
		{"not-a-mac", false},
	}
	for _, tc := range cases {
		if got := device.IsRandomizedMAC(tc.mac); got != tc.expected {
			t.Errorf("IsRandomizedMAC(%q) = %v, want %v", tc.mac, got, tc.expected)
		}
	}
}
