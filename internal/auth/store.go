package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrUserExists     = errors.New("user already exists")
	ErrBadCredentials = errors.New("invalid credentials")
	ErrNoSession      = errors.New("no session")
)

const SessionLifetime = 7 * 24 * time.Hour

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_users`).Scan(&n)
	return n > 0, err
}

func (s *Store) Create(ctx context.Context, username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username required")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	h, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	id := newRandID()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO auth_users (id, username, pw_hash) VALUES (?, ?, ?)`, id, username, h)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &User{ID: id, Username: username}, nil
}

// Login verifies the credentials and issues a new session token. The cost of
// the argon2id verify also covers a constant-time path: enumerating users via
// timing of bad-username vs. bad-password is non-trivial here.
func (s *Store) Login(ctx context.Context, username, password string) (string, error) {
	var id, hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, pw_hash FROM auth_users WHERE username = ?`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Run a verify against a dummy hash so timing matches the valid path.
		_ = VerifyPassword(password, "$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		return "", ErrBadCredentials
	}
	if err != nil {
		return "", err
	}
	if !VerifyPassword(password, hash) {
		return "", ErrBadCredentials
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(SessionLifetime)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, id, expires); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Verify(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	var u User
	err := s.db.QueryRowContext(ctx, `
        SELECT u.id, u.username
        FROM auth_sessions s
        JOIN auth_users u ON u.id = s.user_id
        WHERE s.token = ? AND s.expires_at > CURRENT_TIMESTAMP`, token).
		Scan(&u.ID, &u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token = ?`, token)
	return err
}

// PurgeExpired runs once at startup and periodically; safe to call any time.
func (s *Store) PurgeExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	return err
}

func newRandID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
