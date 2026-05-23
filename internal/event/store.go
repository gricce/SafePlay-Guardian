package event

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// DB exposes the underlying handle for tests and ad-hoc reporting queries.
func (s *Store) DB() *sql.DB { return s.db }

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// Insert writes one event. The caller is responsible for filtering attrs
// according to the effective monitoring level — use Logger.Append for that.
func (s *Store) Insert(ctx context.Context, ev Event) error {
	if !ev.Kind.Valid() {
		return fmt.Errorf("invalid event kind: %q", ev.Kind)
	}
	if ev.DeviceID == "" {
		return fmt.Errorf("event missing device_id")
	}
	if ev.ID == "" {
		ev.ID = newID()
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	if ev.Severity == "" {
		ev.Severity = SeverityInfo
	}
	if ev.Attrs == nil {
		ev.Attrs = map[string]any{}
	}
	body, err := json.Marshal(ev.Attrs)
	if err != nil {
		return fmt.Errorf("marshal attrs: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO events (id, at, kind, person_id, device_id, severity, attrs)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.At, string(ev.Kind), ev.PersonID, ev.DeviceID, string(ev.Severity), string(body))
	return err
}

type RecentFilter struct {
	DeviceID string
	PersonID string
	Kinds    []Kind
	Since    time.Time
	Limit    int
}

func (s *Store) Recent(ctx context.Context, f RecentFilter) ([]Event, error) {
	q := `SELECT id, at, recorded_at, kind, person_id, device_id, severity, attrs FROM events WHERE 1=1`
	args := []any{}
	if f.DeviceID != "" {
		q += ` AND device_id = ?`
		args = append(args, f.DeviceID)
	}
	if f.PersonID != "" {
		q += ` AND person_id = ?`
		args = append(args, f.PersonID)
	}
	if len(f.Kinds) > 0 {
		placeholders := strings.Repeat("?,", len(f.Kinds))
		placeholders = placeholders[:len(placeholders)-1]
		q += ` AND kind IN (` + placeholders + `)`
		for _, k := range f.Kinds {
			args = append(args, string(k))
		}
	}
	if !f.Since.IsZero() {
		q += ` AND at >= ?`
		args = append(args, f.Since)
	}
	q += ` ORDER BY at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// DeleteOlderThan deletes events whose recorded_at is before cutoff. Returns
// the count deleted so the retention job can log progress.
func (s *Store) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE recorded_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UsageSince approximates active-use minutes for a device since `start` by
// chaining consecutive device_seen events whose gap is under 5 minutes.
// This is honest about scanner-driven sightings being a proxy for activity;
// Phase 7 replaces this with agent-reported session_start/stop pairs.
func (s *Store) UsageSince(ctx context.Context, deviceID string, start time.Time) (time.Duration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT at FROM events WHERE device_id = ? AND kind = ? AND at >= ? ORDER BY at`,
		deviceID, string(KindDeviceSeen), start)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	const gap = 5 * time.Minute
	var total time.Duration
	var prev time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return 0, err
		}
		if !prev.IsZero() {
			d := at.Sub(prev)
			if d > 0 && d < gap {
				total += d
			}
		}
		prev = at
	}
	return total, rows.Err()
}

type ScreenTimeRow struct {
	PersonID string `json:"person_id"`
	Day      string `json:"day"`
	Minutes  int    `json:"minutes"`
}

// ScreenTimePerDay returns approximate minutes of activity per UTC day for
// a person over the last `days` calendar days. Same gap-chained heuristic
// as UsageSince, but bucketed by day. Empty personID aggregates every device
// in the home — used by the "All children" view on /reports.
func (s *Store) ScreenTimePerDay(ctx context.Context, personID string, days int) ([]ScreenTimeRow, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	where := `kind = ? AND at >= ?`
	args := []any{string(KindDeviceSeen), since}
	if personID != "" {
		where += ` AND person_id = ?`
		args = append(args, personID)
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT date(at) AS day,
               SUM(CASE WHEN gap_minutes IS NOT NULL AND gap_minutes < 5 THEN gap_minutes ELSE 0 END) AS minutes
        FROM (
            SELECT at,
                   (julianday(at) - julianday(LAG(at) OVER (PARTITION BY device_id ORDER BY at))) * 1440 AS gap_minutes
            FROM events
            WHERE `+where+`
        )
        GROUP BY day
        ORDER BY day`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScreenTimeRow
	for rows.Next() {
		var r ScreenTimeRow
		r.PersonID = personID
		var minutes sql.NullFloat64
		if err := rows.Scan(&r.Day, &minutes); err != nil {
			return nil, err
		}
		if minutes.Valid {
			r.Minutes = int(minutes.Float64)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type DeviceUsageRow struct {
	DeviceID string `json:"device_id"`
	Minutes  int    `json:"minutes"`
}

// UsageByDevice returns approximate active minutes per device over the last
// `days` calendar days, biggest first. Same gap-chained heuristic. An empty
// personID returns every device; a non-empty one filters to a single child.
func (s *Store) UsageByDevice(ctx context.Context, personID string, days int) ([]DeviceUsageRow, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	where := `kind = ? AND at >= ?`
	args := []any{string(KindDeviceSeen), since}
	if personID != "" {
		where += ` AND person_id = ?`
		args = append(args, personID)
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT device_id,
               SUM(CASE WHEN gap_minutes IS NOT NULL AND gap_minutes < 5 THEN gap_minutes ELSE 0 END) AS minutes
        FROM (
            SELECT device_id, at,
                   (julianday(at) - julianday(LAG(at) OVER (PARTITION BY device_id ORDER BY at))) * 1440 AS gap_minutes
            FROM events
            WHERE `+where+`
        )
        GROUP BY device_id
        HAVING minutes > 0
        ORDER BY minutes DESC`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceUsageRow
	for rows.Next() {
		var r DeviceUsageRow
		var minutes sql.NullFloat64
		if err := rows.Scan(&r.DeviceID, &minutes); err != nil {
			return nil, err
		}
		if minutes.Valid {
			r.Minutes = int(minutes.Float64)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type BlockRow struct {
	Capability string `json:"capability"`
	Value      string `json:"value,omitempty"`
	Count      int    `json:"count"`
}

// TopBlocks aggregates block events over the last `days` days. The `value`
// dimension is only populated for events stored at the granular monitoring
// level (enforcement_only/metadata redact it before insert).
func (s *Store) TopBlocks(ctx context.Context, days int, limit int) ([]BlockRow, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 {
		limit = 20
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
        SELECT COALESCE(json_extract(attrs, '$.capability'), '') AS capability,
               COALESCE(json_extract(attrs, '$.value'), '')      AS value,
               COUNT(*) AS n
        FROM events
        WHERE kind = ? AND at >= ?
        GROUP BY capability, value
        ORDER BY n DESC
        LIMIT ?`,
		string(KindBlock), since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockRow
	for rows.Next() {
		var r BlockRow
		if err := rows.Scan(&r.Capability, &r.Value, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dst ...any) error
}

func scanEvent(sc scanner) (Event, error) {
	var (
		ev        Event
		kind      string
		severity  string
		personID  sql.NullString
		attrsJSON string
	)
	if err := sc.Scan(&ev.ID, &ev.At, &ev.RecordedAt, &kind, &personID, &ev.DeviceID, &severity, &attrsJSON); err != nil {
		return Event{}, err
	}
	ev.Kind = Kind(kind)
	ev.Severity = Severity(severity)
	if personID.Valid {
		v := personID.String
		ev.PersonID = &v
	}
	ev.Attrs = map[string]any{}
	if attrsJSON != "" {
		_ = json.Unmarshal([]byte(attrsJSON), &ev.Attrs)
	}
	return ev, nil
}
