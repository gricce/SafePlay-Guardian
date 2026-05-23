package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	GetPersonPolicy(ctx context.Context, personID string) (*Policy, error)
	SetPersonPolicy(ctx context.Context, personID string, p *Policy) error
	GetDeviceOverride(ctx context.Context, deviceID string) (*Override, error)
	SetDeviceOverride(ctx context.Context, deviceID string, o *Override) error
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore { return &SQLiteStore{db: db} }

func (s *SQLiteStore) GetPersonPolicy(ctx context.Context, personID string) (*Policy, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM policies WHERE person_id = ?`, personID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}
	var p Policy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	return &p, nil
}

func (s *SQLiteStore) SetPersonPolicy(ctx context.Context, personID string, p *Policy) error {
	if p == nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE person_id = ?`, personID)
		return err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO policies (person_id, data, updated_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(person_id) DO UPDATE SET data = excluded.data, updated_at = CURRENT_TIMESTAMP`,
		personID, string(body))
	return err
}

func (s *SQLiteStore) GetDeviceOverride(ctx context.Context, deviceID string) (*Override, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM device_overrides WHERE device_id = ?`, deviceID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read override: %w", err)
	}
	var o Override
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		return nil, fmt.Errorf("decode override: %w", err)
	}
	return &o, nil
}

func (s *SQLiteStore) SetDeviceOverride(ctx context.Context, deviceID string, o *Override) error {
	if o == nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM device_overrides WHERE device_id = ?`, deviceID)
		return err
	}
	body, err := json.Marshal(o)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO device_overrides (device_id, data, updated_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(device_id) DO UPDATE SET data = excluded.data, updated_at = CURRENT_TIMESTAMP`,
		deviceID, string(body))
	return err
}
