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
	id_token TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`)
	if err != nil {
		return err
	}
	// Ensure id_token column exists on sessions for RP-initiated logout
	_, _ = s.pool.Exec(ctx, `ALTER TABLE sessions ADD COLUMN IF NOT EXISTS id_token TEXT NOT NULL DEFAULT ''`)
	return nil
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

// RolesForUser returns a list of role names for the given user.
func (s *Store) RolesForUser(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT r.name
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = $1
ORDER BY r.name
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, email, name, picture, provider, subject, created_at, updated_at
FROM users
ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]User, 0, 128)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Provider, &u.Subject, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUserByID fetches a user by ID.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
SELECT id, email, name, picture, provider, subject, created_at, updated_at
FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Provider, &u.Subject, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser updates mutable fields for a user (email, name, picture, provider, subject).
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	if u == nil || u.ID == 0 {
		return errors.New("invalid user")
	}
	// Email, provider, and subject are treated as identifiers for OIDC; allow updating with care.
	_, err := s.pool.Exec(ctx, `
UPDATE users
SET email=$1, name=$2, picture=$3, provider=$4, subject=$5, updated_at=now()
WHERE id=$6
`, u.Email, u.Name, u.Picture, u.Provider, u.Subject, u.ID)
	return err
}

// DeleteUser deletes a user by ID (cascades to sessions and user_roles).
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

// SetUserRoles replaces the set of roles for a user.
func (s *Store) SetUserRoles(ctx context.Context, userID int64, roles []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// rollback if still in progress
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, name := range roles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var roleID int64
		// Ensure role exists
		err := tx.QueryRow(ctx, `INSERT INTO roles(name) VALUES($1)
ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name
RETURNING id`, name).Scan(&roleID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CreateSession issues a new session for a user.
func (s *Store) CreateSession(ctx context.Context, userID int64) (*Session, error) {
	id, err := randomID(32)
	if err != nil {
		return nil, err
	}
	sess := &Session{ID: id, UserID: userID, ExpiresAt: time.Now().Add(s.sessionTTL)}
	_, err = s.pool.Exec(ctx, `INSERT INTO sessions(id, user_id, expires_at, id_token) VALUES($1,$2,$3,'')`, sess.ID, sess.UserID, sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// GetSession returns the session and associated user if valid.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, *User, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, expires_at, created_at, id_token FROM sessions WHERE id=$1`, id).
		Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt, &sess.IDToken)
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

// SetSessionIDToken stores the OIDC ID token for a session (used for RP-initiated logout).
func (s *Store) SetSessionIDToken(ctx context.Context, id string, idToken string) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET id_token=$2 WHERE id=$1`, id, idToken)
	return err
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
