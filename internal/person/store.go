package person

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("person not found")

type Person struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return hex.EncodeToString(b[:])
}

func (s *Store) Create(ctx context.Context, name string) (*Person, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	p := Person{ID: newID(), Name: name}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO people (id, name) VALUES (?, ?)`, p.ID, p.Name)
	if err != nil {
		return nil, fmt.Errorf("insert person: %w", err)
	}
	return s.Get(ctx, p.ID)
}

func (s *Store) Get(ctx context.Context, id string) (*Person, error) {
	var p Person
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM people WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) List(ctx context.Context) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM people ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
