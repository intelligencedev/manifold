package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"manifold/internal/persistence"
	"manifold/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewMCPStore returns a Postgres-backed store if a pool is provided, otherwise an in-memory store.
func NewMCPStore(pool *pgxpool.Pool) persistence.MCPStore {
	if pool == nil {
		return &memMCPStore{m: map[int64]map[string]persistence.MCPServer{}}
	}
	return &pgMCPStore{pool: pool}
}

func NewMCPStoreWithCodec(pool *pgxpool.Pool, codec secrets.Codec) persistence.MCPStore {
	if pool == nil {
		return &memMCPStore{m: map[int64]map[string]persistence.MCPServer{}}
	}
	return &pgMCPStore{pool: pool, codec: codec}
}

type memMCPStore struct {
	mu     sync.RWMutex
	m      map[int64]map[string]persistence.MCPServer
	nextID int64
}

func (s *memMCPStore) Init(ctx context.Context) error { return nil }

func (s *memMCPStore) List(ctx context.Context, userID int64) ([]persistence.MCPServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userMap := s.m[userID]
	if userMap == nil {
		return []persistence.MCPServer{}, nil
	}
	out := make([]persistence.MCPServer, 0, len(userMap))
	for _, v := range userMap {
		out = append(out, v)
	}
	// Sort by name
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *memMCPStore) GetByName(ctx context.Context, userID int64, name string) (persistence.MCPServer, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if userMap := s.m[userID]; userMap != nil {
		v, ok := userMap[name]
		return v, ok, nil
	}
	return persistence.MCPServer{}, false, nil
}

func (s *memMCPStore) Upsert(ctx context.Context, userID int64, srv persistence.MCPServer) (persistence.MCPServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(srv.Name) == "" {
		return persistence.MCPServer{}, errors.New("name required")
	}
	if s.m[userID] == nil {
		s.m[userID] = map[string]persistence.MCPServer{}
	}
	srv.UserID = userID
	// Assign a new ID if this is a new entry (ID not set or doesn't exist)
	if existing, ok := s.m[userID][srv.Name]; ok {
		srv.ID = existing.ID
	} else {
		s.nextID++
		srv.ID = s.nextID
	}
	s.m[userID][srv.Name] = srv
	return srv, nil
}

func (s *memMCPStore) Delete(ctx context.Context, userID int64, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[userID] == nil {
		return nil
	}
	delete(s.m[userID], name)
	return nil
}

type pgMCPStore struct {
	pool  *pgxpool.Pool
	codec secrets.Codec
}

func (s *pgMCPStore) Init(ctx context.Context) error {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return err
	}
	s.codec = codec
	_, err = s.pool.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS mcp_servers (
	id SERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	command TEXT NOT NULL DEFAULT '',
	args JSONB NOT NULL DEFAULT '[]',
	env JSONB NOT NULL DEFAULT '{}',
	url TEXT NOT NULL DEFAULT '',
	headers JSONB NOT NULL DEFAULT '{}',
	bearer_token TEXT NOT NULL DEFAULT '',
	origin TEXT NOT NULL DEFAULT '',
	protocol_version TEXT NOT NULL DEFAULT '',
	keep_alive_seconds INT NOT NULL DEFAULT 0,
	disabled BOOLEAN NOT NULL DEFAULT false,
	oauth_provider TEXT NOT NULL DEFAULT '',
	oauth_client_id TEXT NOT NULL DEFAULT '',
	oauth_client_secret TEXT NOT NULL DEFAULT '',
	oauth_access_token TEXT NOT NULL DEFAULT '',
	oauth_refresh_token TEXT NOT NULL DEFAULT '',
	oauth_expires_at TIMESTAMP WITH TIME ZONE,
	oauth_scopes JSONB NOT NULL DEFAULT '[]',
	UNIQUE(user_id, name)
);
`)
	if err != nil {
		return err
	}
	// Ensure new columns exist for upgrades
	if _, err := s.pool.Exec(ctx, `ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS oauth_client_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS oauth_client_secret TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return backfillPostgresMCPSecrets(ctx, s.pool, codec)
}

func (s *pgMCPStore) List(ctx context.Context, userID int64) ([]persistence.MCPServer, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return nil, err
	}
	s.codec = codec
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled, oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes
		FROM mcp_servers WHERE user_id = $1 ORDER BY name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []persistence.MCPServer
	for rows.Next() {
		var srv persistence.MCPServer
		var args, env, headers, scopes []byte
		var expiresAt *time.Time

		if err := rows.Scan(
			&srv.ID, &srv.UserID, &srv.Name, &srv.Command, &args, &env, &srv.URL, &headers, &srv.BearerToken, &srv.Origin, &srv.ProtocolVersion, &srv.KeepAliveSeconds, &srv.Disabled, &srv.OAuthProvider, &srv.OAuthClientID, &srv.OAuthClientSecret, &srv.OAuthAccessToken, &srv.OAuthRefreshToken, &expiresAt, &scopes,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(args, &srv.Args)
		_ = json.Unmarshal(env, &srv.Env)
		_ = json.Unmarshal(headers, &srv.Headers)
		_ = json.Unmarshal(scopes, &srv.OAuthScopes)
		if expiresAt != nil {
			srv.OAuthExpiresAt = *expiresAt
		}
		srv, err = decryptMCPServerFromStore(codec, srv)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *pgMCPStore) GetByName(ctx context.Context, userID int64, name string) (persistence.MCPServer, bool, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return persistence.MCPServer{}, false, err
	}
	s.codec = codec
	var srv persistence.MCPServer
	var args, env, headers, scopes []byte
	var expiresAt *time.Time

	err = s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled, oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes
		FROM mcp_servers WHERE user_id = $1 AND name = $2
	`, userID, name).Scan(
		&srv.ID, &srv.UserID, &srv.Name, &srv.Command, &args, &env, &srv.URL, &headers, &srv.BearerToken, &srv.Origin, &srv.ProtocolVersion, &srv.KeepAliveSeconds, &srv.Disabled, &srv.OAuthProvider, &srv.OAuthClientID, &srv.OAuthClientSecret, &srv.OAuthAccessToken, &srv.OAuthRefreshToken, &expiresAt, &scopes,
	)
	if err != nil {
		return persistence.MCPServer{}, false, nil
	}
	_ = json.Unmarshal(args, &srv.Args)
	_ = json.Unmarshal(env, &srv.Env)
	_ = json.Unmarshal(headers, &srv.Headers)
	_ = json.Unmarshal(scopes, &srv.OAuthScopes)
	if expiresAt != nil {
		srv.OAuthExpiresAt = *expiresAt
	}
	srv, err = decryptMCPServerFromStore(codec, srv)
	if err != nil {
		return persistence.MCPServer{}, false, err
	}
	return srv, true, nil
}

func (s *pgMCPStore) Upsert(ctx context.Context, userID int64, srv persistence.MCPServer) (persistence.MCPServer, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return persistence.MCPServer{}, err
	}
	s.codec = codec
	toStore, err := encryptMCPServerForStore(s.codec, userID, srv)
	if err != nil {
		return persistence.MCPServer{}, err
	}
	args, _ := json.Marshal(srv.Args)
	env, _ := json.Marshal(toStore.Env)
	headers, _ := json.Marshal(toStore.Headers)
	scopes, _ := json.Marshal(srv.OAuthScopes)
	var expiresAt *time.Time
	if !srv.OAuthExpiresAt.IsZero() {
		expiresAt = &srv.OAuthExpiresAt
	}

	err = s.pool.QueryRow(ctx, `
			INSERT INTO mcp_servers (user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled, oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (user_id, name) DO UPDATE SET
			command = EXCLUDED.command,
			args = EXCLUDED.args,
			env = EXCLUDED.env,
			url = EXCLUDED.url,
			headers = EXCLUDED.headers,
			bearer_token = EXCLUDED.bearer_token,
			origin = EXCLUDED.origin,
			protocol_version = EXCLUDED.protocol_version,
			keep_alive_seconds = EXCLUDED.keep_alive_seconds,
			disabled = EXCLUDED.disabled,
			oauth_provider = EXCLUDED.oauth_provider,
			oauth_client_id = EXCLUDED.oauth_client_id,
			oauth_client_secret = EXCLUDED.oauth_client_secret,
			oauth_access_token = EXCLUDED.oauth_access_token,
			oauth_refresh_token = EXCLUDED.oauth_refresh_token,
			oauth_expires_at = EXCLUDED.oauth_expires_at,
			oauth_scopes = EXCLUDED.oauth_scopes
			RETURNING id
		`, userID, srv.Name, srv.Command, args, env, srv.URL, headers, toStore.BearerToken, srv.Origin, srv.ProtocolVersion, srv.KeepAliveSeconds, srv.Disabled, srv.OAuthProvider, srv.OAuthClientID, toStore.OAuthClientSecret, toStore.OAuthAccessToken, toStore.OAuthRefreshToken, expiresAt, scopes).Scan(&srv.ID)

	srv.UserID = userID
	return srv, err
}

func (s *pgMCPStore) Delete(ctx context.Context, userID int64, name string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM mcp_servers WHERE user_id = $1 AND name = $2", userID, name)
	return err
}

type mcpSecretRecord struct {
	ID                int64
	UserID            int64
	Name              string
	Env               map[string]string
	Headers           map[string]string
	BearerToken       string
	OAuthClientSecret string
	OAuthAccessToken  string
	OAuthRefreshToken string
}

func backfillPostgresMCPSecrets(ctx context.Context, pool *pgxpool.Pool, codec secrets.Codec) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, user_id, name, env, headers, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token FROM mcp_servers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	records, err := scanMCPSecretRecords(rows)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !mcpRecordNeedsBackfill(codec, record) {
			continue
		}
		encrypted, err := encryptMCPServerForStore(codec, record.UserID, persistence.MCPServer{
			UserID:            record.UserID,
			Name:              record.Name,
			Env:               record.Env,
			Headers:           record.Headers,
			BearerToken:       record.BearerToken,
			OAuthClientSecret: record.OAuthClientSecret,
			OAuthAccessToken:  record.OAuthAccessToken,
			OAuthRefreshToken: record.OAuthRefreshToken,
		})
		if err != nil {
			return fmt.Errorf("backfill mcp secrets id=%d: %w", record.ID, err)
		}
		env, _ := json.Marshal(encrypted.Env)
		headers, _ := json.Marshal(encrypted.Headers)
		if _, err := tx.Exec(ctx, `UPDATE mcp_servers SET env = $1, headers = $2, bearer_token = $3, oauth_client_secret = $4, oauth_access_token = $5, oauth_refresh_token = $6 WHERE id = $7`,
			env, headers, encrypted.BearerToken, encrypted.OAuthClientSecret, encrypted.OAuthAccessToken, encrypted.OAuthRefreshToken, record.ID); err != nil {
			return fmt.Errorf("update mcp secret backfill id=%d: %w", record.ID, err)
		}
	}
	return tx.Commit(ctx)
}

func backfillSQLiteMCPSecrets(ctx context.Context, db *sql.DB, codec secrets.Codec) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT id, user_id, name, env, headers, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token FROM mcp_servers`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	records, err := scanMCPSecretRecords(rows)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !mcpRecordNeedsBackfill(codec, record) {
			continue
		}
		encrypted, err := encryptMCPServerForStore(codec, record.UserID, persistence.MCPServer{
			UserID:            record.UserID,
			Name:              record.Name,
			Env:               record.Env,
			Headers:           record.Headers,
			BearerToken:       record.BearerToken,
			OAuthClientSecret: record.OAuthClientSecret,
			OAuthAccessToken:  record.OAuthAccessToken,
			OAuthRefreshToken: record.OAuthRefreshToken,
		})
		if err != nil {
			return fmt.Errorf("backfill mcp secrets id=%d: %w", record.ID, err)
		}
		env, _ := json.Marshal(encrypted.Env)
		headers, _ := json.Marshal(encrypted.Headers)
		if _, err := tx.ExecContext(ctx, `UPDATE mcp_servers SET env = ?, headers = ?, bearer_token = ?, oauth_client_secret = ?, oauth_access_token = ?, oauth_refresh_token = ? WHERE id = ?`,
			string(env), string(headers), encrypted.BearerToken, encrypted.OAuthClientSecret, encrypted.OAuthAccessToken, encrypted.OAuthRefreshToken, record.ID); err != nil {
			return fmt.Errorf("update mcp secret backfill id=%d: %w", record.ID, err)
		}
	}
	return tx.Commit()
}

func scanMCPSecretRecords(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]mcpSecretRecord, error) {
	var records []mcpSecretRecord
	for rows.Next() {
		var record mcpSecretRecord
		var env, headers []byte
		if err := rows.Scan(&record.ID, &record.UserID, &record.Name, &env, &headers, &record.BearerToken, &record.OAuthClientSecret, &record.OAuthAccessToken, &record.OAuthRefreshToken); err != nil {
			return nil, err
		}
		if len(env) > 0 {
			if err := json.Unmarshal(env, &record.Env); err != nil {
				return nil, fmt.Errorf("mcp env id=%d: %w", record.ID, err)
			}
		}
		if len(headers) > 0 {
			if err := json.Unmarshal(headers, &record.Headers); err != nil {
				return nil, fmt.Errorf("mcp headers id=%d: %w", record.ID, err)
			}
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func mcpRecordNeedsBackfill(codec secrets.Codec, record mcpSecretRecord) bool {
	for _, value := range []string{record.BearerToken, record.OAuthClientSecret, record.OAuthAccessToken, record.OAuthRefreshToken} {
		if value != "" && !codec.IsSealed(value) {
			return true
		}
	}
	for _, value := range record.Headers {
		if value != "" && !codec.IsSealed(value) {
			return true
		}
	}
	for key, value := range record.Env {
		if isSecretMapKey(key) && value != "" && !codec.IsSealed(value) {
			return true
		}
	}
	return false
}
