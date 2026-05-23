package device

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("device not found")

// ObserveHook is invoked after a successful Reconcile, with the device ID the
// observation was bound to. The hook is the event log's plug point; the device
// package itself stays free of any event dependency to avoid an import cycle.
type ObserveHook func(ctx context.Context, deviceID string, obs Observation)

type SQLiteRegistry struct {
	db      *sql.DB
	observe ObserveHook
}

func NewSQLiteRegistry(db *sql.DB) *SQLiteRegistry {
	return &SQLiteRegistry{db: db}
}

// SetObserveHook installs a callback that fires once per successful Reconcile.
func (r *SQLiteRegistry) SetObserveHook(h ObserveHook) { r.observe = h }

// DB exposes the underlying handle for tests and ad-hoc reporting queries
// (e.g. pending_merges, which has no dedicated method yet).
func (r *SQLiteRegistry) DB() *sql.DB { return r.db }

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand failure: %w", err))
	}
	return hex.EncodeToString(b[:])
}

func (r *SQLiteRegistry) Get(ctx context.Context, id string) (*Device, error) {
	row := r.db.QueryRowContext(ctx, `
        SELECT id, person_id, label, kind, agent_token, state, last_seen
        FROM devices WHERE id = ?`, id)
	d, err := scanDevice(row)
	if err != nil {
		return nil, err
	}
	if err := r.attachChildren(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (r *SQLiteRegistry) List(ctx context.Context, f Filter) ([]Device, error) {
	q := `SELECT id, person_id, label, kind, agent_token, state, last_seen FROM devices`
	args := []any{}
	if f.State != "" {
		q += ` WHERE state = ?`
		args = append(args, string(f.State))
	}
	q += ` ORDER BY label`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		if err := r.attachChildren(ctx, d); err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// Upsert inserts a new device or updates an existing one by ID. Idempotent:
// calling twice with the same input produces the same end state.
func (r *SQLiteRegistry) Upsert(ctx context.Context, d Device) (*Device, error) {
	if d.ID == "" {
		d.ID = newID()
	}
	if d.State == "" {
		d.State = StateActive
	}
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO devices (id, person_id, label, kind, agent_token, state, last_seen, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(id) DO UPDATE SET
            person_id   = excluded.person_id,
            label       = excluded.label,
            kind        = excluded.kind,
            agent_token = excluded.agent_token,
            state       = excluded.state,
            last_seen   = excluded.last_seen,
            updated_at  = CURRENT_TIMESTAMP`,
		d.ID, d.PersonID, d.Label, d.Kind, d.AgentToken, string(d.State), d.LastSeen)
	if err != nil {
		return nil, fmt.Errorf("upsert device: %w", err)
	}
	for _, ident := range d.Identities {
		if err := r.addIdentity(ctx, d.ID, ident); err != nil {
			return nil, err
		}
	}
	for _, cap := range d.Caps {
		if err := r.addCapability(ctx, d.ID, cap); err != nil {
			return nil, err
		}
	}
	return r.Get(ctx, d.ID)
}

// Reconcile applies the layered matching rules from Phase 2 of CORE_PLATFORM.md:
//
//  1. Strong: pre-provisioned agent_token claims a manually-added device.
//  2. Strong: agent_id identity match.
//  3. Strong: non-randomized MAC identity match.
//  4. Weak corroboration: at least two of {IP, hostname, randomized MAC} must
//     point to the same existing device; one weak signal is never enough.
//  5. Multiple confident candidates → create new device + record a
//     pending_merge for the parent to resolve. No confident candidate →
//     create new.
func (r *SQLiteRegistry) Reconcile(ctx context.Context, obs Observation) (string, error) {
	id, err := r.reconcileImpl(ctx, obs)
	if err != nil {
		return "", err
	}
	if r.observe != nil {
		r.observe(ctx, id, obs)
	}
	return id, nil
}

func (r *SQLiteRegistry) reconcileImpl(ctx context.Context, obs Observation) (string, error) {
	if obs.AgentToken != "" {
		if id, ok, err := r.findDeviceByAgentToken(ctx, obs.AgentToken); err != nil {
			return "", err
		} else if ok {
			return id, r.mergeObservation(ctx, id, obs)
		}
	}
	if obs.AgentID != "" {
		if id, ok, err := r.findByIdentity(ctx, IdentityAgent, obs.AgentID); err != nil {
			return "", err
		} else if ok {
			return id, r.mergeObservation(ctx, id, obs)
		}
	}
	if obs.MAC != "" && !IsRandomizedMAC(obs.MAC) {
		if id, ok, err := r.findByIdentity(ctx, IdentityMAC, obs.MAC); err != nil {
			return "", err
		} else if ok {
			return id, r.mergeObservation(ctx, id, obs)
		}
	}

	hits, err := r.findWeakHits(ctx, obs)
	if err != nil {
		return "", err
	}
	var confident []string
	for id, n := range hits {
		if n >= 2 {
			confident = append(confident, id)
		}
	}
	if len(confident) == 1 {
		return confident[0], r.mergeObservation(ctx, confident[0], obs)
	}

	newID, err := r.createFromObservation(ctx, obs)
	if err != nil {
		return "", err
	}
	if len(confident) >= 2 {
		if err := r.createPendingMerge(ctx, newID, confident); err != nil {
			return "", err
		}
	}
	return newID, nil
}

// findWeakHits returns a count of corroborating weak signals per existing
// device. A weak signal is IP, hostname, or a randomized MAC — each on its own
// can collide across devices (DHCP, generic names, MAC rotation), so a single
// hit is never enough to merge.
func (r *SQLiteRegistry) findWeakHits(ctx context.Context, obs Observation) (map[string]int, error) {
	type kv struct{ kind, value string }
	var pairs []kv
	if obs.IP != "" {
		pairs = append(pairs, kv{string(IdentityIP), obs.IP})
	}
	if obs.Hostname != "" {
		pairs = append(pairs, kv{string(IdentityHostname), obs.Hostname})
	}
	if obs.MAC != "" && IsRandomizedMAC(obs.MAC) {
		pairs = append(pairs, kv{string(IdentityMAC), obs.MAC})
	}
	hits := map[string]int{}
	for _, p := range pairs {
		rows, err := r.db.QueryContext(ctx,
			`SELECT DISTINCT device_id FROM network_identities WHERE kind = ? AND value = ?`,
			p.kind, p.value)
		if err != nil {
			return nil, fmt.Errorf("weak hit lookup: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			hits[id]++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return hits, nil
}

func (r *SQLiteRegistry) findDeviceByAgentToken(ctx context.Context, token string) (string, bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM devices WHERE agent_token = ?`, token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find by agent_token: %w", err)
	}
	return id, true, nil
}

func (r *SQLiteRegistry) createPendingMerge(ctx context.Context, sourceID string, candidates []string) error {
	body, err := json.Marshal(candidates)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO pending_merges (source_device_id, candidate_ids) VALUES (?, ?)`,
		sourceID, string(body))
	return err
}

func (r *SQLiteRegistry) findByIdentity(ctx context.Context, kind IdentityKind, value string) (string, bool, error) {
	var deviceID string
	err := r.db.QueryRowContext(ctx,
		`SELECT device_id FROM network_identities WHERE kind = ? AND value = ?`,
		string(kind), value).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find identity: %w", err)
	}
	return deviceID, true, nil
}

func (r *SQLiteRegistry) mergeObservation(ctx context.Context, deviceID string, obs Observation) error {
	seen := obs.SeenAt
	if seen.IsZero() {
		seen = time.Now().UTC()
	}
	if obs.AgentID != "" {
		if err := r.touchIdentity(ctx, deviceID, IdentityAgent, obs.AgentID, seen); err != nil {
			return err
		}
	}
	if obs.MAC != "" {
		// Record every MAC, including randomized ones — they still serve as
		// weak corroboration when paired with other signals.
		if err := r.touchIdentity(ctx, deviceID, IdentityMAC, obs.MAC, seen); err != nil {
			return err
		}
	}
	if obs.IP != "" {
		if err := r.touchIdentity(ctx, deviceID, IdentityIP, obs.IP, seen); err != nil {
			return err
		}
	}
	if obs.Hostname != "" {
		if err := r.touchIdentity(ctx, deviceID, IdentityHostname, obs.Hostname, seen); err != nil {
			return err
		}
	}
	if obs.Source == "agent" {
		for _, c := range obs.Caps {
			if err := r.addCapability(ctx, deviceID, c); err != nil {
				return err
			}
		}
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE devices SET last_seen = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		seen, deviceID)
	return err
}

func (r *SQLiteRegistry) createFromObservation(ctx context.Context, obs Observation) (string, error) {
	seen := obs.SeenAt
	if seen.IsZero() {
		seen = time.Now().UTC()
	}
	label := obs.Label
	if label == "" {
		switch {
		case obs.Hostname != "":
			label = obs.Hostname
		case obs.AgentID != "":
			label = "agent:" + obs.AgentID
		case obs.MAC != "":
			label = "mac:" + obs.MAC
		default:
			label = "unknown device"
		}
	}
	kind := obs.Kind
	if kind == "" {
		kind = "unknown"
	}
	state := StateActive
	if obs.Source == "scan" {
		state = StateDiscovered
	}
	d := Device{
		Label:    label,
		Kind:     kind,
		State:    state,
		LastSeen: &seen,
	}
	created, err := r.Upsert(ctx, d)
	if err != nil {
		return "", err
	}
	if err := r.mergeObservation(ctx, created.ID, obs); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (r *SQLiteRegistry) touchIdentity(ctx context.Context, deviceID string, kind IdentityKind, value string, seen time.Time) error {
	// Insert if absent on this device, refresh last_seen if already present.
	// The same (kind, value) may also appear on other devices — that's
	// expected for weak identities (IP via DHCP, generic hostnames).
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO network_identities (device_id, kind, value, last_seen)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(device_id, kind, value) DO UPDATE SET last_seen = excluded.last_seen`,
		deviceID, string(kind), value, seen)
	if err != nil {
		return fmt.Errorf("touch identity: %w", err)
	}
	return nil
}

func (r *SQLiteRegistry) addIdentity(ctx context.Context, deviceID string, ident NetworkIdentity) error {
	seen := time.Now().UTC()
	if ident.LastSeen != nil {
		seen = *ident.LastSeen
	}
	return r.touchIdentity(ctx, deviceID, ident.Kind, ident.Value, seen)
}

func (r *SQLiteRegistry) addCapability(ctx context.Context, deviceID string, cap Capability) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO device_capabilities (device_id, capability) VALUES (?, ?)`,
		deviceID, string(cap))
	return err
}

func (r *SQLiteRegistry) attachChildren(ctx context.Context, d *Device) error {
	identRows, err := r.db.QueryContext(ctx,
		`SELECT kind, value, last_seen FROM network_identities WHERE device_id = ? ORDER BY kind, value`,
		d.ID)
	if err != nil {
		return err
	}
	defer identRows.Close()
	d.Identities = []NetworkIdentity{}
	for identRows.Next() {
		var ni NetworkIdentity
		var kind string
		var lastSeen sql.NullTime
		if err := identRows.Scan(&kind, &ni.Value, &lastSeen); err != nil {
			return err
		}
		ni.Kind = IdentityKind(kind)
		if lastSeen.Valid {
			t := lastSeen.Time
			ni.LastSeen = &t
		}
		d.Identities = append(d.Identities, ni)
	}
	if err := identRows.Err(); err != nil {
		return err
	}

	capRows, err := r.db.QueryContext(ctx,
		`SELECT capability FROM device_capabilities WHERE device_id = ? ORDER BY capability`, d.ID)
	if err != nil {
		return err
	}
	defer capRows.Close()
	d.Caps = []Capability{}
	for capRows.Next() {
		var c string
		if err := capRows.Scan(&c); err != nil {
			return err
		}
		d.Caps = append(d.Caps, Capability(c))
	}
	return capRows.Err()
}

type scanner interface {
	Scan(dst ...any) error
}

func scanDevice(s scanner) (*Device, error) {
	var (
		d         Device
		state     string
		personID  sql.NullString
		agentTok  sql.NullString
		lastSeen  sql.NullTime
	)
	if err := s.Scan(&d.ID, &personID, &d.Label, &d.Kind, &agentTok, &state, &lastSeen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.State = DeviceState(state)
	if personID.Valid {
		v := personID.String
		d.PersonID = &v
	}
	if agentTok.Valid {
		v := agentTok.String
		d.AgentToken = &v
	}
	if lastSeen.Valid {
		t := lastSeen.Time
		d.LastSeen = &t
	}
	return &d, nil
}
