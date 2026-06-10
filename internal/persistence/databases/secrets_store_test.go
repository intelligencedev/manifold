package databases

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testDatabaseSecretsCodec(t *testing.T) secrets.Codec {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 11)
	}
	codec, err := secrets.NewCodec(key)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return codec
}

func testSecretsKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 31)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func testPostgresDSN() string {
	dsn := strings.TrimSpace(os.Getenv("MANIFOLD_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	}
	return dsn
}

func TestSQLiteSpecialistsStoreEncryptsAndBackfillsSecrets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestSQLite(t)
	codec := testDatabaseSecretsCodec(t)
	store := NewSQLiteSpecialistsStoreWithCodec(db, codec)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := store.Upsert(ctx, 7, persistence.Specialist{
		Name:         "writer",
		Provider:     "openai",
		APIKey:       "sk-specialist",
		ExtraHeaders: map[string]string{"Authorization": "Bearer specialist-header"},
		Model:        "gpt-5-mini",
	}); err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}

	rawAPIKey, rawHeaders := rawSQLiteSpecialistSecrets(t, db, 7, "writer")
	if !codec.IsSealed(rawAPIKey) || strings.Contains(rawAPIKey, "sk-specialist") {
		t.Fatalf("expected encrypted api key, got %q", rawAPIKey)
	}
	if !codec.IsSealed(rawHeaders["Authorization"]) || strings.Contains(rawHeaders["Authorization"], "specialist-header") {
		t.Fatalf("expected encrypted header, got %q", rawHeaders["Authorization"])
	}

	got, ok, err := store.GetByName(ctx, 7, "writer")
	if err != nil {
		t.Fatalf("get specialist: %v", err)
	}
	if !ok || got.APIKey != "sk-specialist" || got.ExtraHeaders["Authorization"] != "Bearer specialist-header" {
		t.Fatalf("unexpected decrypted specialist: ok=%v got=%+v", ok, got)
	}

	if err := store.Init(ctx); err != nil {
		t.Fatalf("re-init store: %v", err)
	}
	afterAPIKey, afterHeaders := rawSQLiteSpecialistSecrets(t, db, 7, "writer")
	if afterAPIKey != rawAPIKey || afterHeaders["Authorization"] != rawHeaders["Authorization"] {
		t.Fatalf("already encrypted specialist row was modified by backfill")
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO specialists(user_id, name, api_key, extra_headers)
VALUES(?, ?, ?, ?)`, 7, "legacy", "legacy-specialist-key", `{"Authorization":"Bearer legacy-header"}`); err != nil {
		t.Fatalf("insert legacy specialist row: %v", err)
	}
	legacyStore := NewSQLiteSpecialistsStoreWithCodec(db, codec)
	if err := legacyStore.Init(ctx); err != nil {
		t.Fatalf("backfill legacy specialist: %v", err)
	}
	legacyAPIKey, legacyHeaders := rawSQLiteSpecialistSecrets(t, db, 7, "legacy")
	if !codec.IsSealed(legacyAPIKey) || !codec.IsSealed(legacyHeaders["Authorization"]) {
		t.Fatalf("expected legacy specialist row to be encrypted, api=%q headers=%+v", legacyAPIKey, legacyHeaders)
	}
	legacy, ok, err := legacyStore.GetByName(ctx, 7, "legacy")
	if err != nil {
		t.Fatalf("get legacy specialist: %v", err)
	}
	if !ok || legacy.APIKey != "legacy-specialist-key" || legacy.ExtraHeaders["Authorization"] != "Bearer legacy-header" {
		t.Fatalf("unexpected legacy specialist after backfill: ok=%v got=%+v", ok, legacy)
	}
}

func TestSQLiteMCPStoreEncryptsAndBackfillsSecrets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestSQLite(t)
	codec := testDatabaseSecretsCodec(t)
	store := NewSQLiteMCPStoreWithCodec(db, codec)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := store.Upsert(ctx, 7, persistence.MCPServer{
		Name:              "remote",
		URL:               "https://mcp.example.test",
		Headers:           map[string]string{"Authorization": "Bearer mcp-header"},
		BearerToken:       "bearer-secret",
		OAuthProvider:     "custom",
		OAuthClientID:     "public-client-id",
		OAuthClientSecret: "client-secret",
		OAuthAccessToken:  "access-token",
		OAuthRefreshToken: "refresh-token",
		OAuthExpiresAt:    time.Now().UTC().Add(time.Hour),
		OAuthScopes:       []string{"tools.read"},
		Env:               map[string]string{"OPENAI_API_KEY": "env-secret", "PATH": "/usr/bin"},
	}); err != nil {
		t.Fatalf("upsert mcp: %v", err)
	}

	raw := rawSQLiteMCPSecrets(t, db, 7, "remote")
	assertSealedValue(t, codec, raw.bearerToken, "bearer-secret")
	assertSealedValue(t, codec, raw.oauthClientSecret, "client-secret")
	assertSealedValue(t, codec, raw.oauthAccessToken, "access-token")
	assertSealedValue(t, codec, raw.oauthRefreshToken, "refresh-token")
	assertSealedValue(t, codec, raw.headers["Authorization"], "mcp-header")
	assertSealedValue(t, codec, raw.env["OPENAI_API_KEY"], "env-secret")
	if raw.env["PATH"] != "/usr/bin" {
		t.Fatalf("expected non-secret env key to remain plaintext, got %q", raw.env["PATH"])
	}

	got, ok, err := store.GetByName(ctx, 7, "remote")
	if err != nil {
		t.Fatalf("get mcp: %v", err)
	}
	if !ok || got.BearerToken != "bearer-secret" || got.OAuthClientSecret != "client-secret" || got.OAuthAccessToken != "access-token" || got.OAuthRefreshToken != "refresh-token" {
		t.Fatalf("unexpected decrypted mcp server: ok=%v got=%+v", ok, got)
	}
	if got.Headers["Authorization"] != "Bearer mcp-header" || got.Env["OPENAI_API_KEY"] != "env-secret" || got.Env["PATH"] != "/usr/bin" {
		t.Fatalf("unexpected decrypted mcp maps: headers=%+v env=%+v", got.Headers, got.Env)
	}

	if err := store.Init(ctx); err != nil {
		t.Fatalf("re-init store: %v", err)
	}
	after := rawSQLiteMCPSecrets(t, db, 7, "remote")
	if after.bearerToken != raw.bearerToken || after.headers["Authorization"] != raw.headers["Authorization"] || after.env["OPENAI_API_KEY"] != raw.env["OPENAI_API_KEY"] {
		t.Fatalf("already encrypted mcp row was modified by backfill")
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO mcp_servers(user_id, name, url, headers, env, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, 7, "legacy", "https://legacy.example.test", `{"Authorization":"Bearer legacy-header"}`, `{"OPENAI_API_KEY":"legacy-env","PATH":"/bin"}`, "legacy-bearer", "legacy-client", "legacy-access", "legacy-refresh"); err != nil {
		t.Fatalf("insert legacy mcp row: %v", err)
	}
	legacyStore := NewSQLiteMCPStoreWithCodec(db, codec)
	if err := legacyStore.Init(ctx); err != nil {
		t.Fatalf("backfill legacy mcp: %v", err)
	}
	legacyRaw := rawSQLiteMCPSecrets(t, db, 7, "legacy")
	assertSealedValue(t, codec, legacyRaw.bearerToken, "legacy-bearer")
	assertSealedValue(t, codec, legacyRaw.oauthClientSecret, "legacy-client")
	assertSealedValue(t, codec, legacyRaw.oauthAccessToken, "legacy-access")
	assertSealedValue(t, codec, legacyRaw.oauthRefreshToken, "legacy-refresh")
	assertSealedValue(t, codec, legacyRaw.headers["Authorization"], "legacy-header")
	assertSealedValue(t, codec, legacyRaw.env["OPENAI_API_KEY"], "legacy-env")
	if legacyRaw.env["PATH"] != "/bin" {
		t.Fatalf("expected legacy non-secret env key to remain plaintext, got %q", legacyRaw.env["PATH"])
	}
	legacy, ok, err := legacyStore.GetByName(ctx, 7, "legacy")
	if err != nil {
		t.Fatalf("get legacy mcp: %v", err)
	}
	if !ok || legacy.BearerToken != "legacy-bearer" || legacy.OAuthAccessToken != "legacy-access" || legacy.OAuthRefreshToken != "legacy-refresh" {
		t.Fatalf("unexpected legacy mcp after backfill: ok=%v got=%+v", ok, legacy)
	}
}

func TestPostgresSpecialistsStoreEncryptsAndBackfillsSecrets(t *testing.T) {
	t.Parallel()

	dsn := testPostgresDSN()
	if dsn == "" {
		t.Skip("set MANIFOLD_TEST_POSTGRES_DSN or POSTGRES_TEST_DSN to run Postgres specialist encryption tests")
	}
	ctx := context.Background()
	pool := openTestPostgresSchema(t, dsn)
	codec := testDatabaseSecretsCodec(t)
	store := NewSpecialistsStoreWithCodec(pool, codec)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := store.Upsert(ctx, 7, persistence.Specialist{
		Name:         "writer",
		Provider:     "openai",
		APIKey:       "sk-specialist",
		ExtraHeaders: map[string]string{"Authorization": "Bearer specialist-header"},
		Model:        "gpt-5-mini",
	}); err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	rawAPIKey, rawHeaders := rawPostgresSpecialistSecrets(t, pool, 7, "writer")
	assertSealedValue(t, codec, rawAPIKey, "sk-specialist")
	assertSealedValue(t, codec, rawHeaders["Authorization"], "specialist-header")

	got, ok, err := store.GetByName(ctx, 7, "writer")
	if err != nil {
		t.Fatalf("get specialist: %v", err)
	}
	if !ok || got.APIKey != "sk-specialist" || got.ExtraHeaders["Authorization"] != "Bearer specialist-header" {
		t.Fatalf("unexpected decrypted specialist: ok=%v got=%+v", ok, got)
	}

	if err := store.Init(ctx); err != nil {
		t.Fatalf("re-init store: %v", err)
	}
	afterAPIKey, afterHeaders := rawPostgresSpecialistSecrets(t, pool, 7, "writer")
	if afterAPIKey != rawAPIKey || afterHeaders["Authorization"] != rawHeaders["Authorization"] {
		t.Fatalf("already encrypted specialist row was modified by backfill")
	}

	if _, err := pool.Exec(ctx, `
INSERT INTO specialists(user_id, name, api_key, extra_headers)
VALUES($1, $2, $3, $4)`, 7, "legacy", "legacy-specialist-key", []byte(`{"Authorization":"Bearer legacy-header"}`)); err != nil {
		t.Fatalf("insert legacy specialist row: %v", err)
	}
	legacyStore := NewSpecialistsStoreWithCodec(pool, codec)
	if err := legacyStore.Init(ctx); err != nil {
		t.Fatalf("backfill legacy specialist: %v", err)
	}
	legacyAPIKey, legacyHeaders := rawPostgresSpecialistSecrets(t, pool, 7, "legacy")
	assertSealedValue(t, codec, legacyAPIKey, "legacy-specialist-key")
	assertSealedValue(t, codec, legacyHeaders["Authorization"], "legacy-header")
}

func TestPostgresMCPStoreEncryptsAndBackfillsSecrets(t *testing.T) {
	t.Parallel()

	dsn := testPostgresDSN()
	if dsn == "" {
		t.Skip("set MANIFOLD_TEST_POSTGRES_DSN or POSTGRES_TEST_DSN to run Postgres MCP encryption tests")
	}
	ctx := context.Background()
	pool := openTestPostgresSchema(t, dsn)
	codec := testDatabaseSecretsCodec(t)
	store := NewMCPStoreWithCodec(pool, codec)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := store.Upsert(ctx, 7, persistence.MCPServer{
		Name:              "remote",
		URL:               "https://mcp.example.test",
		Headers:           map[string]string{"Authorization": "Bearer mcp-header"},
		BearerToken:       "bearer-secret",
		OAuthProvider:     "custom",
		OAuthClientID:     "public-client-id",
		OAuthClientSecret: "client-secret",
		OAuthAccessToken:  "access-token",
		OAuthRefreshToken: "refresh-token",
		OAuthExpiresAt:    time.Now().UTC().Add(time.Hour),
		OAuthScopes:       []string{"tools.read"},
		Env:               map[string]string{"OPENAI_API_KEY": "env-secret", "PATH": "/usr/bin"},
	}); err != nil {
		t.Fatalf("upsert mcp: %v", err)
	}
	raw := rawPostgresMCPSecrets(t, pool, 7, "remote")
	assertSealedValue(t, codec, raw.bearerToken, "bearer-secret")
	assertSealedValue(t, codec, raw.oauthClientSecret, "client-secret")
	assertSealedValue(t, codec, raw.oauthAccessToken, "access-token")
	assertSealedValue(t, codec, raw.oauthRefreshToken, "refresh-token")
	assertSealedValue(t, codec, raw.headers["Authorization"], "mcp-header")
	assertSealedValue(t, codec, raw.env["OPENAI_API_KEY"], "env-secret")
	if raw.env["PATH"] != "/usr/bin" {
		t.Fatalf("expected non-secret env key to remain plaintext, got %q", raw.env["PATH"])
	}

	got, ok, err := store.GetByName(ctx, 7, "remote")
	if err != nil {
		t.Fatalf("get mcp: %v", err)
	}
	if !ok || got.BearerToken != "bearer-secret" || got.OAuthClientSecret != "client-secret" || got.OAuthAccessToken != "access-token" || got.OAuthRefreshToken != "refresh-token" {
		t.Fatalf("unexpected decrypted mcp server: ok=%v got=%+v", ok, got)
	}

	if err := store.Init(ctx); err != nil {
		t.Fatalf("re-init store: %v", err)
	}
	after := rawPostgresMCPSecrets(t, pool, 7, "remote")
	if after.bearerToken != raw.bearerToken || after.headers["Authorization"] != raw.headers["Authorization"] || after.env["OPENAI_API_KEY"] != raw.env["OPENAI_API_KEY"] {
		t.Fatalf("already encrypted mcp row was modified by backfill")
	}

	if _, err := pool.Exec(ctx, `
INSERT INTO mcp_servers(user_id, name, url, headers, env, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)`, 7, "legacy", "https://legacy.example.test", []byte(`{"Authorization":"Bearer legacy-header"}`), []byte(`{"OPENAI_API_KEY":"legacy-env","PATH":"/bin"}`), "legacy-bearer", "legacy-client", "legacy-access", "legacy-refresh"); err != nil {
		t.Fatalf("insert legacy mcp row: %v", err)
	}
	legacyStore := NewMCPStoreWithCodec(pool, codec)
	if err := legacyStore.Init(ctx); err != nil {
		t.Fatalf("backfill legacy mcp: %v", err)
	}
	legacyRaw := rawPostgresMCPSecrets(t, pool, 7, "legacy")
	assertSealedValue(t, codec, legacyRaw.bearerToken, "legacy-bearer")
	assertSealedValue(t, codec, legacyRaw.oauthClientSecret, "legacy-client")
	assertSealedValue(t, codec, legacyRaw.oauthAccessToken, "legacy-access")
	assertSealedValue(t, codec, legacyRaw.oauthRefreshToken, "legacy-refresh")
	assertSealedValue(t, codec, legacyRaw.headers["Authorization"], "legacy-header")
	assertSealedValue(t, codec, legacyRaw.env["OPENAI_API_KEY"], "legacy-env")
}

func TestManagerSQLiteSecretsKeyRequired(t *testing.T) {
	ctx := context.Background()
	t.Setenv(secrets.EnvKeyName, "")

	_, err := NewManager(ctx, config.DBConfig{
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "manifold.db")},
	})
	if err == nil || !strings.Contains(err.Error(), secrets.EnvKeyName) {
		t.Fatalf("expected missing secrets key error, got %v", err)
	}
}

func TestManagerSQLiteSecretsKeyRejectsInvalidFormat(t *testing.T) {
	ctx := context.Background()
	t.Setenv(secrets.EnvKeyName, "not-base64")

	_, err := NewManager(ctx, config.DBConfig{
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "manifold.db")},
	})
	if err == nil || !strings.Contains(err.Error(), "base64-encoded 32 raw bytes") {
		t.Fatalf("expected invalid secrets key error, got %v", err)
	}
}

func TestManagerMemoryBackendsDoNotRequireSecretsKey(t *testing.T) {
	ctx := context.Background()
	t.Setenv(secrets.EnvKeyName, "")

	mgr, err := NewManager(ctx, config.DBConfig{
		Backend: "memory",
		Search:  config.SearchConfig{Backend: "none"},
		Vector:  config.VectorConfig{Backend: "none"},
		Graph:   config.GraphConfig{Backend: "none"},
		Chat:    config.ChatConfig{Backend: "memory"},
	})
	if err != nil {
		t.Fatalf("NewManager memory backends: %v", err)
	}
	defer mgr.Close()
}

func rawSQLiteSpecialistSecrets(t *testing.T, db *sql.DB, userID int64, name string) (string, map[string]string) {
	t.Helper()
	var rawAPIKey string
	var rawHeaders []byte
	if err := db.QueryRow(`SELECT api_key, extra_headers FROM specialists WHERE user_id = ? AND name = ?`, userID, name).Scan(&rawAPIKey, &rawHeaders); err != nil {
		t.Fatalf("query raw specialist row: %v", err)
	}
	return rawAPIKey, decodeSecretStringMap(t, rawHeaders)
}

func rawPostgresSpecialistSecrets(t *testing.T, pool *pgxpool.Pool, userID int64, name string) (string, map[string]string) {
	t.Helper()
	var rawAPIKey string
	var rawHeaders []byte
	if err := pool.QueryRow(context.Background(), `SELECT api_key, extra_headers FROM specialists WHERE user_id = $1 AND name = $2`, userID, name).Scan(&rawAPIKey, &rawHeaders); err != nil {
		t.Fatalf("query raw specialist row: %v", err)
	}
	return rawAPIKey, decodeSecretStringMap(t, rawHeaders)
}

type rawMCPSecrets struct {
	env               map[string]string
	headers           map[string]string
	bearerToken       string
	oauthClientSecret string
	oauthAccessToken  string
	oauthRefreshToken string
}

func rawSQLiteMCPSecrets(t *testing.T, db *sql.DB, userID int64, name string) rawMCPSecrets {
	t.Helper()
	var raw rawMCPSecrets
	var env, headers []byte
	if err := db.QueryRow(`
SELECT env, headers, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token
FROM mcp_servers WHERE user_id = ? AND name = ?`, userID, name).Scan(&env, &headers, &raw.bearerToken, &raw.oauthClientSecret, &raw.oauthAccessToken, &raw.oauthRefreshToken); err != nil {
		t.Fatalf("query raw mcp row: %v", err)
	}
	raw.env = decodeSecretStringMap(t, env)
	raw.headers = decodeSecretStringMap(t, headers)
	return raw
}

func rawPostgresMCPSecrets(t *testing.T, pool *pgxpool.Pool, userID int64, name string) rawMCPSecrets {
	t.Helper()
	var raw rawMCPSecrets
	var env, headers []byte
	if err := pool.QueryRow(context.Background(), `
SELECT env, headers, bearer_token, oauth_client_secret, oauth_access_token, oauth_refresh_token
FROM mcp_servers WHERE user_id = $1 AND name = $2`, userID, name).Scan(&env, &headers, &raw.bearerToken, &raw.oauthClientSecret, &raw.oauthAccessToken, &raw.oauthRefreshToken); err != nil {
		t.Fatalf("query raw mcp row: %v", err)
	}
	raw.env = decodeSecretStringMap(t, env)
	raw.headers = decodeSecretStringMap(t, headers)
	return raw
}

func decodeSecretStringMap(t *testing.T, data []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	if len(data) == 0 {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode string map %s: %v", string(data), err)
	}
	return out
}

func assertSealedValue(t *testing.T, codec secrets.Codec, value, plaintextFragment string) {
	t.Helper()
	if !codec.IsSealed(value) {
		t.Fatalf("expected sealed value, got %q", value)
	}
	if strings.Contains(value, plaintextFragment) {
		t.Fatalf("sealed value contains plaintext fragment %q: %q", plaintextFragment, value)
	}
}
