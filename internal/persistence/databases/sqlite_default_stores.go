package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/config"
	persist "manifold/internal/persistence"
	"manifold/internal/secrets"
	"manifold/internal/transit"
)

type sqliteUserPreferencesStore struct {
	db *sql.DB
}

func NewSQLiteUserPreferencesStore(db *sql.DB) persist.UserPreferencesStore {
	return &sqliteUserPreferencesStore{db: db}
}

func (s *sqliteUserPreferencesStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite user preferences store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS user_preferences (
	user_id INTEGER PRIMARY KEY,
	active_project_id TEXT,
	updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_preferences_active_project
	ON user_preferences(active_project_id)
	WHERE active_project_id IS NOT NULL;
`)
	return err
}

func (s *sqliteUserPreferencesStore) Get(ctx context.Context, userID int64) (persist.UserPreferences, error) {
	if err := s.Init(ctx); err != nil {
		return persist.UserPreferences{}, err
	}
	var prefs persist.UserPreferences
	var activeProjectID sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT user_id, active_project_id, updated_at
FROM user_preferences
WHERE user_id = ?`, userID).Scan(&prefs.UserID, &activeProjectID, &prefs.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.UserPreferences{UserID: userID}, nil
	}
	if err != nil {
		return persist.UserPreferences{}, err
	}
	if activeProjectID.Valid {
		prefs.ActiveProjectID = activeProjectID.String
	}
	return prefs, nil
}

func (s *sqliteUserPreferencesStore) SetActiveProject(ctx context.Context, userID int64, projectID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	var activeProjectID any
	if strings.TrimSpace(projectID) != "" {
		activeProjectID = strings.TrimSpace(projectID)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_preferences(user_id, active_project_id, updated_at)
VALUES(?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
	active_project_id = excluded.active_project_id,
	updated_at = excluded.updated_at
`, userID, activeProjectID, time.Now().UTC())
	return err
}

type sqliteCommandPolicyStore struct {
	db *sql.DB
}

func NewSQLiteCommandPolicyStore(db *sql.DB) persist.CommandPolicyStore {
	return &sqliteCommandPolicyStore{db: db}
}

func (s *sqliteCommandPolicyStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite command policy store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS command_policy_rules (
	user_id INTEGER NOT NULL DEFAULT 0,
	id TEXT NOT NULL,
	decision TEXT NOT NULL,
	pattern TEXT NOT NULL DEFAULT '[]',
	contexts TEXT NOT NULL DEFAULT '[]',
	justification TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY(user_id, id),
	CHECK (json_valid(pattern)),
	CHECK (json_valid(contexts))
);
CREATE TABLE IF NOT EXISTS command_policy_session_overrides (
	user_id INTEGER NOT NULL DEFAULT 0,
	session_id TEXT NOT NULL,
	allow_all_commands INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY(user_id, session_id)
);
`)
	return err
}

func (s *sqliteCommandPolicyStore) ListRules(ctx context.Context, userID int64) ([]config.ExecCommandRule, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, decision, pattern, contexts, justification
FROM command_policy_rules
WHERE user_id = ?
ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []config.ExecCommandRule{}
	for rows.Next() {
		var rule config.ExecCommandRule
		var pattern, contexts string
		if err := rows.Scan(&rule.ID, &rule.Decision, &pattern, &contexts, &rule.Justification); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(pattern), &rule.Pattern)
		_ = json.Unmarshal([]byte(contexts), &rule.Contexts)
		out = append(out, cloneStoredCommandRule(rule))
	}
	return out, rows.Err()
}

func (s *sqliteCommandPolicyStore) UpsertRule(ctx context.Context, userID int64, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	if err := s.Init(ctx); err != nil {
		return config.ExecCommandRule{}, err
	}
	rule = prepareStoredCommandRule(rule)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO command_policy_rules(user_id, id, decision, pattern, contexts, justification, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, id) DO UPDATE SET
	decision = excluded.decision,
	pattern = excluded.pattern,
	contexts = excluded.contexts,
	justification = excluded.justification,
	updated_at = excluded.updated_at
`, userID, rule.ID, rule.Decision, encodeJSON(rule.Pattern, "[]"), encodeJSON(rule.Contexts, "[]"), rule.Justification, now, now)
	return cloneStoredCommandRule(rule), err
}

func (s *sqliteCommandPolicyStore) GetSessionOverride(ctx context.Context, userID int64, sessionID string) (persist.CommandPolicySessionOverride, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return persist.CommandPolicySessionOverride{}, false, nil
	}
	if err := s.Init(ctx); err != nil {
		return persist.CommandPolicySessionOverride{}, false, err
	}
	var override persist.CommandPolicySessionOverride
	err := s.db.QueryRowContext(ctx, `
SELECT user_id, session_id, allow_all_commands, created_at, updated_at
FROM command_policy_session_overrides
WHERE user_id = ? AND session_id = ?`, userID, sessionID).Scan(&override.UserID, &override.SessionID, &override.AllowAllCommands, &override.CreatedAt, &override.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.CommandPolicySessionOverride{}, false, nil
	}
	if err != nil {
		return persist.CommandPolicySessionOverride{}, false, err
	}
	return override, true, nil
}

func (s *sqliteCommandPolicyStore) SetSessionAllowAll(ctx context.Context, userID int64, sessionID string, allow bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id required")
	}
	if !allow {
		return s.DeleteSessionOverride(ctx, userID, sessionID)
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO command_policy_session_overrides(user_id, session_id, allow_all_commands, created_at, updated_at)
VALUES(?, ?, 1, ?, ?)
ON CONFLICT(user_id, session_id) DO UPDATE SET
	allow_all_commands = 1,
	updated_at = excluded.updated_at
`, userID, sessionID, now, now)
	return err
}

func (s *sqliteCommandPolicyStore) DeleteSessionOverride(ctx context.Context, userID int64, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM command_policy_session_overrides
WHERE user_id = ? AND session_id = ?`, userID, sessionID)
	return err
}

type sqliteMCPStore struct {
	db    *sql.DB
	codec secrets.Codec
}

func NewSQLiteMCPStore(db *sql.DB) persist.MCPStore {
	return &sqliteMCPStore{db: db}
}

func NewSQLiteMCPStoreWithCodec(db *sql.DB, codec secrets.Codec) persist.MCPStore {
	return &sqliteMCPStore{db: db, codec: codec}
}

func (s *sqliteMCPStore) ensureCodec() (secrets.Codec, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return nil, err
	}
	s.codec = codec
	return codec, nil
}

func (s *sqliteMCPStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite mcp store requires db")
	}
	codec, err := s.ensureCodec()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS mcp_servers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	command TEXT NOT NULL DEFAULT '',
	args TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(args)),
	env TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(env)),
	url TEXT NOT NULL DEFAULT '',
	headers TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(headers)),
	bearer_token TEXT NOT NULL DEFAULT '',
	origin TEXT NOT NULL DEFAULT '',
	protocol_version TEXT NOT NULL DEFAULT '',
	keep_alive_seconds INTEGER NOT NULL DEFAULT 0,
	disabled INTEGER NOT NULL DEFAULT 0,
	oauth_provider TEXT NOT NULL DEFAULT '',
	oauth_client_id TEXT NOT NULL DEFAULT '',
	oauth_client_secret TEXT NOT NULL DEFAULT '',
	oauth_access_token TEXT NOT NULL DEFAULT '',
	oauth_refresh_token TEXT NOT NULL DEFAULT '',
	oauth_expires_at DATETIME,
	oauth_scopes TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(oauth_scopes)),
	UNIQUE(user_id, name)
	);
	`)
	if err != nil {
		return err
	}
	return backfillSQLiteMCPSecrets(ctx, s.db, codec)
}

func (s *sqliteMCPStore) List(ctx context.Context, userID int64) ([]persist.MCPServer, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled,
	oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes
FROM mcp_servers
WHERE user_id = ?
ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persist.MCPServer{}
	for rows.Next() {
		srv, err := scanSQLiteMCPServer(rows)
		if err != nil {
			return nil, err
		}
		srv, err = decryptMCPServerFromStore(s.codec, srv)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *sqliteMCPStore) GetByName(ctx context.Context, userID int64, name string) (persist.MCPServer, bool, error) {
	if err := s.Init(ctx); err != nil {
		return persist.MCPServer{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled,
	oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes
FROM mcp_servers
WHERE user_id = ? AND name = ?`, userID, name)
	srv, err := scanSQLiteMCPServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.MCPServer{}, false, nil
	}
	if err != nil {
		return persist.MCPServer{}, false, err
	}
	srv, err = decryptMCPServerFromStore(s.codec, srv)
	if err != nil {
		return persist.MCPServer{}, false, err
	}
	return srv, true, nil
}

func (s *sqliteMCPStore) Upsert(ctx context.Context, userID int64, srv persist.MCPServer) (persist.MCPServer, error) {
	if strings.TrimSpace(srv.Name) == "" {
		return persist.MCPServer{}, errors.New("name required")
	}
	if err := s.Init(ctx); err != nil {
		return persist.MCPServer{}, err
	}
	toStore, err := encryptMCPServerForStore(s.codec, userID, srv)
	if err != nil {
		return persist.MCPServer{}, err
	}
	var expiresAt any
	if !srv.OAuthExpiresAt.IsZero() {
		expiresAt = srv.OAuthExpiresAt.UTC()
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO mcp_servers(user_id, name, command, args, env, url, headers, bearer_token, origin, protocol_version, keep_alive_seconds, disabled,
	oauth_provider, oauth_client_id, oauth_client_secret, oauth_access_token, oauth_refresh_token, oauth_expires_at, oauth_scopes)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, name) DO UPDATE SET
	command = excluded.command,
	args = excluded.args,
	env = excluded.env,
	url = excluded.url,
	headers = excluded.headers,
	bearer_token = excluded.bearer_token,
	origin = excluded.origin,
	protocol_version = excluded.protocol_version,
	keep_alive_seconds = excluded.keep_alive_seconds,
	disabled = excluded.disabled,
	oauth_provider = excluded.oauth_provider,
	oauth_client_id = excluded.oauth_client_id,
	oauth_client_secret = excluded.oauth_client_secret,
	oauth_access_token = excluded.oauth_access_token,
	oauth_refresh_token = excluded.oauth_refresh_token,
	oauth_expires_at = excluded.oauth_expires_at,
		oauth_scopes = excluded.oauth_scopes
	RETURNING id
	`, userID, srv.Name, srv.Command, encodeJSON(srv.Args, "[]"), encodeJSON(toStore.Env, "{}"), srv.URL, encodeJSON(toStore.Headers, "{}"),
		toStore.BearerToken, srv.Origin, srv.ProtocolVersion, srv.KeepAliveSeconds, srv.Disabled, srv.OAuthProvider, srv.OAuthClientID, toStore.OAuthClientSecret,
		toStore.OAuthAccessToken, toStore.OAuthRefreshToken, expiresAt, encodeJSON(srv.OAuthScopes, "[]"))
	if err := row.Scan(&srv.ID); err != nil {
		return persist.MCPServer{}, err
	}
	srv.UserID = userID
	return srv, nil
}

func (s *sqliteMCPStore) Delete(ctx context.Context, userID int64, name string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE user_id = ? AND name = ?`, userID, name)
	return err
}

func scanSQLiteMCPServer(row interface{ Scan(dest ...any) error }) (persist.MCPServer, error) {
	var srv persist.MCPServer
	var args, env, headers, scopes string
	var expiresAt sql.NullTime
	if err := row.Scan(
		&srv.ID, &srv.UserID, &srv.Name, &srv.Command, &args, &env, &srv.URL, &headers, &srv.BearerToken, &srv.Origin, &srv.ProtocolVersion,
		&srv.KeepAliveSeconds, &srv.Disabled, &srv.OAuthProvider, &srv.OAuthClientID, &srv.OAuthClientSecret, &srv.OAuthAccessToken, &srv.OAuthRefreshToken, &expiresAt, &scopes,
	); err != nil {
		return persist.MCPServer{}, err
	}
	_ = json.Unmarshal([]byte(args), &srv.Args)
	_ = json.Unmarshal([]byte(env), &srv.Env)
	_ = json.Unmarshal([]byte(headers), &srv.Headers)
	_ = json.Unmarshal([]byte(scopes), &srv.OAuthScopes)
	if expiresAt.Valid {
		srv.OAuthExpiresAt = expiresAt.Time.UTC()
	}
	return srv, nil
}

type sqliteProjectsStore struct {
	db *sql.DB
}

func NewSQLiteProjectsStore(db *sql.DB) persist.ProjectsStore {
	return &sqliteProjectsStore{db: db}
}

func (s *sqliteProjectsStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite projects store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	revision INTEGER NOT NULL DEFAULT 1,
	bytes INTEGER NOT NULL DEFAULT 0,
	file_count INTEGER NOT NULL DEFAULT 0,
	storage_backend TEXT NOT NULL DEFAULT 'filesystem'
);
CREATE INDEX IF NOT EXISTS projects_user_updated_idx ON projects(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS projects_user_name_idx ON projects(user_id, name);
CREATE TABLE IF NOT EXISTS project_files (
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	name TEXT NOT NULL,
	is_dir INTEGER NOT NULL DEFAULT 0,
	size INTEGER NOT NULL DEFAULT 0,
	mod_time DATETIME NOT NULL,
	etag TEXT NOT NULL DEFAULT '',
	updated_at DATETIME NOT NULL,
	PRIMARY KEY(project_id, path)
);
CREATE INDEX IF NOT EXISTS project_files_project_path_idx ON project_files(project_id, path);
`)
	return err
}

func (s *sqliteProjectsStore) Create(ctx context.Context, userID int64, name string) (persist.Project, error) {
	if err := s.Init(ctx); err != nil {
		return persist.Project{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, `
INSERT INTO projects(id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend)
VALUES(?, ?, ?, ?, ?, 1, 0, 0, 'filesystem')
RETURNING id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
`, uuid.NewString(), userID, name, now, now)
	return scanSQLiteProject(row)
}

func (s *sqliteProjectsStore) Get(ctx context.Context, userID int64, projectID string) (persist.Project, error) {
	if err := s.Init(ctx); err != nil {
		return persist.Project{}, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
FROM projects
WHERE id = ?`, projectID)
	project, err := scanSQLiteProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.Project{}, persist.ErrNotFound
	}
	if err != nil {
		return persist.Project{}, err
	}
	if project.UserID != userID {
		return persist.Project{}, persist.ErrForbidden
	}
	return project, nil
}

func (s *sqliteProjectsStore) List(ctx context.Context, userID int64) ([]persist.Project, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
FROM projects
WHERE user_id = ?
ORDER BY updated_at DESC, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persist.Project{}
	for rows.Next() {
		project, err := scanSQLiteProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, project)
	}
	return out, rows.Err()
}

func (s *sqliteProjectsStore) Update(ctx context.Context, project persist.Project) (persist.Project, error) {
	if err := s.Init(ctx); err != nil {
		return persist.Project{}, err
	}
	now := time.Now().UTC()
	storageBackend := strings.TrimSpace(project.StorageBackend)
	if storageBackend == "" {
		storageBackend = "filesystem"
	}
	row := s.db.QueryRowContext(ctx, `
UPDATE projects
SET name = ?, updated_at = ?, revision = revision + 1, storage_backend = ?
WHERE id = ? AND revision = ?
RETURNING id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
`, project.Name, now, storageBackend, project.ID, project.Revision)
	updated, err := scanSQLiteProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if scanErr := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM projects WHERE id = ?`, project.ID).Scan(&exists); scanErr != nil {
			return persist.Project{}, scanErr
		}
		if exists == 0 {
			return persist.Project{}, persist.ErrNotFound
		}
		return persist.Project{}, persist.ErrRevisionConflict
	}
	if err != nil {
		return persist.Project{}, err
	}
	return updated, nil
}

func (s *sqliteProjectsStore) UpdateStats(ctx context.Context, projectID string, bytes int64, fileCount int) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE projects
SET bytes = ?, file_count = ?, updated_at = ?
WHERE id = ?`, bytes, fileCount, time.Now().UTC(), projectID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return persist.ErrNotFound
	}
	return nil
}

func (s *sqliteProjectsStore) Delete(ctx context.Context, userID int64, projectID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	var owner int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM projects WHERE id = ?`, projectID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != userID {
		return persist.ErrForbidden
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID)
	return err
}

func (s *sqliteProjectsStore) IndexFile(ctx context.Context, f persist.ProjectFile) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	f.Path = normalizePath(f.Path)
	if f.ModTime.IsZero() {
		f.ModTime = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_files(project_id, path, name, is_dir, size, mod_time, etag, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, path) DO UPDATE SET
	name = excluded.name,
	is_dir = excluded.is_dir,
	size = excluded.size,
	mod_time = excluded.mod_time,
	etag = excluded.etag,
	updated_at = excluded.updated_at
`, f.ProjectID, f.Path, f.Name, f.IsDir, f.Size, f.ModTime.UTC(), f.ETag, time.Now().UTC())
	return err
}

func (s *sqliteProjectsStore) RemoveFileIndex(ctx context.Context, projectID, filePath string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM project_files WHERE project_id = ? AND path = ?`, projectID, normalizePath(filePath))
	return err
}

func (s *sqliteProjectsStore) RemoveFileIndexPrefix(ctx context.Context, projectID, pathPrefix string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	pathPrefix = normalizePath(pathPrefix)
	pattern := pathPrefix
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM project_files
WHERE project_id = ? AND (path = ? OR path LIKE ?)`, projectID, pathPrefix, pattern+"%")
	return err
}

func (s *sqliteProjectsStore) ListFiles(ctx context.Context, projectID, dirPath string) ([]persist.ProjectFile, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	dirPath = normalizePath(dirPath)
	var rows *sql.Rows
	var err error
	if dirPath == "." || dirPath == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = ? AND instr(path, '/') = 0
ORDER BY is_dir DESC, name ASC`, projectID)
	} else {
		prefix := dirPath + "/"
		rows, err = s.db.QueryContext(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = ? AND path LIKE ? AND instr(substr(path, ?), '/') = 0
ORDER BY is_dir DESC, name ASC`, projectID, prefix+"%", len(prefix)+1)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []persist.ProjectFile{}
	for rows.Next() {
		f, err := scanSQLiteProjectFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *sqliteProjectsStore) GetFile(ctx context.Context, projectID, filePath string) (persist.ProjectFile, error) {
	if err := s.Init(ctx); err != nil {
		return persist.ProjectFile{}, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = ? AND path = ?`, projectID, normalizePath(filePath))
	f, err := scanSQLiteProjectFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.ProjectFile{}, persist.ErrNotFound
	}
	if err != nil {
		return persist.ProjectFile{}, err
	}
	return f, nil
}

func scanSQLiteProject(row interface{ Scan(dest ...any) error }) (persist.Project, error) {
	var project persist.Project
	var storageBackend sql.NullString
	if err := row.Scan(&project.ID, &project.UserID, &project.Name, &project.CreatedAt, &project.UpdatedAt, &project.Revision, &project.Bytes, &project.FileCount, &storageBackend); err != nil {
		return persist.Project{}, err
	}
	if storageBackend.Valid {
		project.StorageBackend = storageBackend.String
	}
	return project, nil
}

func scanSQLiteProjectFile(row interface{ Scan(dest ...any) error }) (persist.ProjectFile, error) {
	var f persist.ProjectFile
	if err := row.Scan(&f.ProjectID, &f.Path, &f.Name, &f.IsDir, &f.Size, &f.ModTime, &f.ETag, &f.UpdatedAt); err != nil {
		return persist.ProjectFile{}, err
	}
	return f, nil
}

type sqliteMatrixMessageStore struct {
	db *sql.DB
}

func NewSQLiteMatrixMessageStore(db *sql.DB) persist.MatrixMessageStore {
	return &sqliteMatrixMessageStore{db: db}
}

func (s *sqliteMatrixMessageStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite matrix message store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS matrix_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL DEFAULT '',
	direction TEXT NOT NULL,
	sender TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL DEFAULT '',
	formatted_body TEXT NOT NULL DEFAULT '',
	msg_type TEXT NOT NULL DEFAULT 'm.text',
	media_url TEXT NOT NULL DEFAULT '',
	media_mime TEXT NOT NULL DEFAULT '',
	media_size INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS matrix_messages_event_idx
	ON matrix_messages(event_id)
	WHERE event_id <> '';
CREATE INDEX IF NOT EXISTS matrix_messages_room_created_idx
	ON matrix_messages(room_id, created_at DESC, id DESC);
`)
	return err
}

func (s *sqliteMatrixMessageStore) Append(ctx context.Context, message persist.MatrixMessage, maxMessages int) (persist.MatrixMessage, error) {
	if err := s.Init(ctx); err != nil {
		return persist.MatrixMessage{}, err
	}
	message.RoomID = strings.TrimSpace(message.RoomID)
	message.EventID = strings.TrimSpace(message.EventID)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return persist.MatrixMessage{}, err
	}
	defer rollbackQuietly(tx)
	if message.EventID != "" {
		existing, ok, err := sqliteGetMatrixMessageByEvent(ctx, tx, message.EventID)
		if err != nil {
			return persist.MatrixMessage{}, err
		}
		if ok {
			if err := tx.Commit(); err != nil {
				return persist.MatrixMessage{}, err
			}
			return existing, nil
		}
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO matrix_messages(room_id, event_id, direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, room_id, event_id, direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at
`, message.RoomID, message.EventID, message.Direction, message.Sender, message.Target, message.Body, message.FormattedBody, message.MsgType, message.MediaURL, message.MediaMIME, message.MediaSize, message.CreatedAt.UTC())
	if err := scanMatrixMessage(row, &message); err != nil {
		return persist.MatrixMessage{}, err
	}
	if maxMessages > 0 {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM matrix_messages
WHERE room_id = ?
  AND id NOT IN (
	SELECT id FROM matrix_messages
	WHERE room_id = ?
	ORDER BY created_at DESC, id DESC
	LIMIT ?
  )`, message.RoomID, message.RoomID, maxMessages); err != nil {
			return persist.MatrixMessage{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return persist.MatrixMessage{}, err
	}
	return message, nil
}

func (s *sqliteMatrixMessageStore) ListByRoom(ctx context.Context, roomID string, limit int, beforeID int64) ([]persist.MatrixMessage, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, room_id, event_id, direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at
FROM matrix_messages
WHERE room_id = ? AND (? = 0 OR id < ?)
ORDER BY created_at DESC, id DESC
LIMIT ?`, strings.TrimSpace(roomID), beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persist.MatrixMessage{}
	for rows.Next() {
		var msg persist.MatrixMessage
		if err := scanMatrixMessage(rows, &msg); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *sqliteMatrixMessageStore) Prune(ctx context.Context, roomID string, maxMessages int) error {
	if maxMessages <= 0 {
		return nil
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	roomID = strings.TrimSpace(roomID)
	_, err := s.db.ExecContext(ctx, `
DELETE FROM matrix_messages
WHERE room_id = ?
  AND id NOT IN (
	SELECT id FROM matrix_messages
	WHERE room_id = ?
	ORDER BY created_at DESC, id DESC
	LIMIT ?
  )`, roomID, roomID, maxMessages)
	return err
}

func (s *sqliteMatrixMessageStore) RoomStats(ctx context.Context, roomID string) (persist.MatrixRoomStats, error) {
	if err := s.Init(ctx); err != nil {
		return persist.MatrixRoomStats{}, err
	}
	stats := persist.MatrixRoomStats{RoomID: strings.TrimSpace(roomID)}
	var lastActivity sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*), MAX(created_at)
FROM matrix_messages
WHERE room_id = ?`, stats.RoomID).Scan(&stats.MessageCount, &lastActivity); err != nil {
		return persist.MatrixRoomStats{}, err
	}
	if lastActivity.Valid {
		stats.LastActivityAt = lastActivity.Time.UTC()
	}
	_ = s.db.QueryRowContext(ctx, `
SELECT sender
FROM matrix_messages
WHERE room_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1`, stats.RoomID).Scan(&stats.LastSender)
	return stats, nil
}

func (s *sqliteMatrixMessageStore) Close() {}

func sqliteGetMatrixMessageByEvent(ctx context.Context, tx *sql.Tx, eventID string) (persist.MatrixMessage, bool, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, room_id, event_id, direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at
FROM matrix_messages
WHERE event_id = ?`, eventID)
	var msg persist.MatrixMessage
	if err := scanMatrixMessage(row, &msg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persist.MatrixMessage{}, false, nil
		}
		return persist.MatrixMessage{}, false, err
	}
	return msg, true, nil
}

type sqliteTransitStore struct {
	db *sql.DB
}

func NewSQLiteTransitStore(db *sql.DB) transit.Store {
	return &sqliteTransitStore{db: db}
}

func (s *sqliteTransitStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite transit store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS transit_memories (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	key_name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	value TEXT NOT NULL DEFAULT '',
	base64 INTEGER NOT NULL DEFAULT 0,
	embed INTEGER NOT NULL DEFAULT 1,
	embed_source TEXT NOT NULL DEFAULT 'value',
	version INTEGER NOT NULL DEFAULT 1,
	created_by INTEGER NOT NULL,
	updated_by INTEGER NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(tenant_id, key_name)
);
CREATE INDEX IF NOT EXISTS transit_memories_tenant_key_prefix_idx ON transit_memories(tenant_id, key_name);
CREATE INDEX IF NOT EXISTS transit_memories_tenant_updated_idx ON transit_memories(tenant_id, updated_at DESC);
`)
	return err
}

func (s *sqliteTransitStore) Create(ctx context.Context, tenantID, actorID int64, items []transit.CreateMemoryItem) ([]transit.Record, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollbackQuietly(tx)

	now := time.Now().UTC()
	out := make([]transit.Record, 0, len(items))
	for _, item := range items {
		record := transit.Record{
			ID:          uuid.NewString(),
			TenantID:    tenantID,
			KeyName:     item.KeyName,
			Description: item.Description,
			Value:       item.Value,
			Base64:      item.Base64 != nil && *item.Base64,
			Embed:       item.Embed == nil || *item.Embed,
			EmbedSource: transit.NormalizeEmbedSource(item.EmbedSource),
			Version:     1,
			CreatedBy:   actorID,
			UpdatedBy:   actorID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO transit_memories(id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, record.ID, record.TenantID, record.KeyName, record.Description, record.Value, record.Base64, record.Embed, record.EmbedSource, record.Version, record.CreatedBy, record.UpdatedBy, record.CreatedAt, record.UpdatedAt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "constraint") {
				return nil, persist.ErrRevisionConflict
			}
			return nil, err
		}
		out = append(out, record)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *sqliteTransitStore) Get(ctx context.Context, tenantID int64, keys []string) ([]transit.Record, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	out := make([]transit.Record, 0, len(keys))
	for _, key := range keys {
		row := s.db.QueryRowContext(ctx, sqliteTransitSelectSQL+` WHERE tenant_id = ? AND key_name = ?`, tenantID, strings.TrimSpace(key))
		record, err := scanSQLiteTransitRecord(row)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *sqliteTransitStore) Update(ctx context.Context, tenantID, actorID int64, req transit.UpdateMemoryRequest) (transit.Record, error) {
	if err := s.Init(ctx); err != nil {
		return transit.Record{}, err
	}
	existingRows, err := s.Get(ctx, tenantID, []string{req.KeyName})
	if err != nil {
		return transit.Record{}, err
	}
	if len(existingRows) == 0 {
		return transit.Record{}, transit.NotFoundError(req.KeyName)
	}
	existing := existingRows[0]
	if req.IfVersion > 0 && existing.Version != req.IfVersion {
		return transit.Record{}, persist.ErrRevisionConflict
	}
	if req.Base64 != nil {
		existing.Base64 = *req.Base64
	}
	if req.Embed != nil {
		existing.Embed = *req.Embed
	}
	if strings.TrimSpace(req.EmbedSource) != "" {
		existing.EmbedSource = transit.NormalizeEmbedSource(req.EmbedSource)
	}
	existing.Value = req.Value
	existing.Version++
	existing.UpdatedBy = actorID
	existing.UpdatedAt = time.Now().UTC()

	row := s.db.QueryRowContext(ctx, `
UPDATE transit_memories
SET value = ?, base64 = ?, embed = ?, embed_source = ?, updated_by = ?, updated_at = ?, version = ?
WHERE tenant_id = ? AND key_name = ?
RETURNING id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at
`, existing.Value, existing.Base64, existing.Embed, existing.EmbedSource, existing.UpdatedBy, existing.UpdatedAt, existing.Version, tenantID, req.KeyName)
	return scanSQLiteTransitRecord(row)
}

func (s *sqliteTransitStore) Delete(ctx context.Context, tenantID int64, keys []string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `DELETE FROM transit_memories WHERE tenant_id = ? AND key_name = ?`, tenantID, strings.TrimSpace(key)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteTransitStore) ListKeys(ctx context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT key_name, description, base64, embed, embed_source, version, created_at, updated_at
FROM transit_memories
WHERE tenant_id = ? AND (? = '' OR key_name LIKE ?)
ORDER BY key_name ASC
LIMIT ?`, tenantID, req.Prefix, req.Prefix+"%", normalizeTransitLimit(req.Limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSQLiteTransitMetadata(rows)
}

func (s *sqliteTransitStore) ListRecent(ctx context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT key_name, description, base64, embed, embed_source, version, created_at, updated_at
FROM transit_memories
WHERE tenant_id = ? AND (? = '' OR key_name LIKE ?)
ORDER BY updated_at DESC, key_name ASC
LIMIT ?`, tenantID, req.Prefix, req.Prefix+"%", normalizeTransitLimit(req.Limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSQLiteTransitMetadata(rows)
}

func (s *sqliteTransitStore) SearchText(ctx context.Context, tenantID int64, req transit.SearchRequest) ([]transit.SearchCandidate, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	rows, err := s.db.QueryContext(ctx, sqliteTransitSelectSQL+`
WHERE tenant_id = ?
  AND (? = '' OR key_name LIKE ?)
ORDER BY updated_at DESC
LIMIT ?`, tenantID, req.Prefix, req.Prefix+"%", normalizeTransitLimit(req.Limit)*10)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]transit.SearchCandidate, 0)
	for rows.Next() {
		record, err := scanSQLiteTransitRecord(rows)
		if err != nil {
			return nil, err
		}
		if !transitMatchesTimeFilter(record, req) {
			continue
		}
		text := strings.ToLower(record.Description + "\n" + record.Value)
		score := 1.0
		if query != "" {
			score = 0
			for _, term := range strings.Fields(query) {
				score += float64(strings.Count(text, term))
			}
		}
		if score <= 0 {
			continue
		}
		out = append(out, transit.SearchCandidate{Record: record, Score: score, Snippet: truncateSnippet(record.Value)})
		if len(out) >= normalizeTransitLimit(req.Limit) {
			break
		}
	}
	return out, rows.Err()
}

const sqliteTransitSelectSQL = `
SELECT id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at
FROM transit_memories`

func scanSQLiteTransitRecord(row interface{ Scan(dest ...any) error }) (transit.Record, error) {
	var record transit.Record
	if err := row.Scan(
		&record.ID,
		&record.TenantID,
		&record.KeyName,
		&record.Description,
		&record.Value,
		&record.Base64,
		&record.Embed,
		&record.EmbedSource,
		&record.Version,
		&record.CreatedBy,
		&record.UpdatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return transit.Record{}, err
	}
	return record, nil
}

func scanSQLiteTransitMetadata(rows *sql.Rows) ([]transit.Metadata, error) {
	out := []transit.Metadata{}
	for rows.Next() {
		var metadata transit.Metadata
		if err := rows.Scan(
			&metadata.KeyName,
			&metadata.Description,
			&metadata.Base64,
			&metadata.Embed,
			&metadata.EmbedSource,
			&metadata.Version,
			&metadata.CreatedAt,
			&metadata.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, metadata)
	}
	return out, rows.Err()
}

func normalizeTransitLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return limit
}
