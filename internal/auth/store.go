package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides user/session persistence and RBAC checks.
type Store struct {
	pool       *pgxpool.Pool
	sessionTTL time.Duration
}

func NewStore(pool *pgxpool.Pool, sessionTTLHours int) *Store {
	if sessionTTLHours <= 0 {
		sessionTTLHours = 72
	}
	return &Store{pool: pool, sessionTTL: time.Duration(sessionTTLHours) * time.Hour}
}

// InitSchema creates required auth tables if they do not exist.
func (s *Store) InitSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  picture TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS roles (
  id BIGSERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS user_roles (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  PRIMARY KEY(user_id, role_id)
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`)
	return err
}

// EnsureDefaultRoles seeds common roles if missing.
func (s *Store) EnsureDefaultRoles(ctx context.Context) error {
	roles := []string{"admin", "user"}
	for _, name := range roles {
		_, err := s.pool.Exec(ctx, `INSERT INTO roles(name) VALUES($1) ON CONFLICT (name) DO NOTHING`, name)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpsertUser creates or updates a user by provider+subject or email.
func (s *Store) UpsertUser(ctx context.Context, u *User) (*User, error) {
	if u.Email == "" || u.Provider == "" || u.Subject == "" {
		return nil, errors.New("missing required user fields")
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO users(email, name, picture, provider, subject)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (email) DO UPDATE SET
  name=EXCLUDED.name,
  picture=EXCLUDED.picture,
  updated_at=now()
RETURNING id, created_at, updated_at
`, u.Email, u.Name, u.Picture, u.Provider, u.Subject)
	if err := row.Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return u, nil
}

// AddRole assigns a role to a user.
func (s *Store) AddRole(ctx context.Context, userID int64, roleName string) error {
	var roleID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM roles WHERE name=$1`, roleName).Scan(&roleID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, roleID)
	return err
}

// HasRole returns true if the user has the given role.
func (s *Store) HasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM user_roles ur
  JOIN roles r ON r.id=ur.role_id
  WHERE ur.user_id=$1 AND r.name=$2
)
`, userID, roleName).Scan(&exists)
	return exists, err
}

// CreateSession issues a new session for a user.
func (s *Store) CreateSession(ctx context.Context, userID int64) (*Session, error) {
	id, err := randomID(32)
	if err != nil {
		return nil, err
	}
	sess := &Session{ID: id, UserID: userID, ExpiresAt: time.Now().Add(s.sessionTTL)}
	_, err = s.pool.Exec(ctx, `INSERT INTO sessions(id, user_id, expires_at) VALUES($1,$2,$3)`, sess.ID, sess.UserID, sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// GetSession returns the session and associated user if valid.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, *User, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, expires_at, created_at FROM sessions WHERE id=$1`, id).
		Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		// best-effort cleanup
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
		return nil, nil, pgx.ErrNoRows
	}
	var u User
	err = s.pool.QueryRow(ctx, `SELECT id, email, name, picture, provider, subject, created_at, updated_at FROM users WHERE id=$1`, sess.UserID).
		Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Provider, &u.Subject, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, nil, err
	}
	return &sess, &u, nil
}

// DeleteSession removes a session by id.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// URL-safe base64 without padding
	s := base64.RawURLEncoding.EncodeToString(b)
	// trim to requested length (encoding expands; we just ensure reasonable size)
	if len(s) > n*2 {
		s = s[:n*2]
	}
	return s, nil
}

// EmailAllowed checks if email domain is permitted by allowed list; empty list means allow all.
func EmailAllowed(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	dom := strings.ToLower(email[at+1:])
	for _, a := range allowed {
		if strings.EqualFold(dom, strings.TrimSpace(a)) {
			return true
		}
	}
	return false
}
