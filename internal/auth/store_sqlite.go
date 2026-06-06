package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) sqliteInitSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	picture TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL,
	subject TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL,
	description TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS user_roles (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
	PRIMARY KEY(user_id, role_id)
);
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at DATETIME NOT NULL,
	id_token TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL
);
`)
	return err
}

func (s *Store) sqliteEnsureDefaultRoles(ctx context.Context) error {
	for _, name := range []string{"admin", "user"} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO roles(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) sqliteUpsertUser(ctx context.Context, u *User) (*User, error) {
	if u.Email == "" || u.Provider == "" || u.Subject == "" {
		return nil, errors.New("missing required user fields")
	}
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, `
INSERT INTO users(email, name, picture, provider, subject, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(email) DO UPDATE SET
	name = COALESCE(NULLIF(excluded.name, ''), users.name),
	picture = COALESCE(NULLIF(excluded.picture, ''), users.picture),
	updated_at = excluded.updated_at
RETURNING id, created_at, updated_at
`, u.Email, u.Name, u.Picture, u.Provider, u.Subject, now, now)
	if err := row.Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) sqliteAddRole(ctx context.Context, userID int64, roleName string) error {
	var roleID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = ?`, roleName).Scan(&roleID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES(?, ?) ON CONFLICT DO NOTHING`, userID, roleID)
	return err
}

func (s *Store) sqliteHasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM user_roles ur
	JOIN roles r ON r.id = ur.role_id
	WHERE ur.user_id = ? AND r.name = ?
)`, userID, roleName).Scan(&exists)
	return exists, err
}

func (s *Store) sqliteRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.name
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ?
ORDER BY r.name`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *Store) sqliteListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, email, name, picture, provider, subject, created_at, updated_at
FROM users
ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *Store) sqliteGetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
SELECT id, email, name, picture, provider, subject, created_at, updated_at
FROM users
WHERE id = ?`, id).Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Provider, &u.Subject, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) sqliteUpdateUser(ctx context.Context, u *User) error {
	if u == nil || u.ID == 0 {
		return errors.New("invalid user")
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE users
SET email = ?, name = ?, picture = ?, provider = ?, subject = ?, updated_at = ?
WHERE id = ?`, u.Email, u.Name, u.Picture, u.Provider, u.Subject, time.Now().UTC(), u.ID)
	return err
}

func (s *Store) sqliteDeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) sqliteSetUserRoles(ctx context.Context, userID int64, roles []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, name := range roles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var roleID int64
		if err := tx.QueryRowContext(ctx, `
INSERT INTO roles(name)
VALUES(?)
ON CONFLICT(name) DO UPDATE SET name = excluded.name
RETURNING id`, name).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES(?, ?) ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) sqliteCreateSession(ctx context.Context, userID int64) (*Session, error) {
	id, err := randomID(32)
	if err != nil {
		return nil, err
	}
	sess := &Session{ID: id, UserID: userID, ExpiresAt: time.Now().UTC().Add(s.sessionTTL)}
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions(id, user_id, expires_at, id_token, created_at) VALUES(?, ?, ?, '', ?)`, sess.ID, sess.UserID, sess.ExpiresAt, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) sqliteGetSession(ctx context.Context, id string) (*Session, *User, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, expires_at, created_at, id_token FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt, &sess.IDToken)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
		return nil, nil, sql.ErrNoRows
	}
	var u User
	err = s.db.QueryRowContext(ctx, `SELECT id, email, name, picture, provider, subject, created_at, updated_at FROM users WHERE id = ?`, sess.UserID).
		Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Provider, &u.Subject, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, nil, err
	}
	return &sess, &u, nil
}

func (s *Store) sqliteSetSessionIDToken(ctx context.Context, id string, idToken string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET id_token = ? WHERE id = ?`, idToken, id)
	return err
}

func (s *Store) sqliteDeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}
