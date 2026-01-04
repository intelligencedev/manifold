package databases

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
)

// NewPostgresProjectsStore returns a Postgres-backed projects store.
// If pool is nil, a memory-backed fallback is used.
func NewPostgresProjectsStore(pool *pgxpool.Pool) persistence.ProjectsStore {
	if pool == nil {
		return newMemoryProjectsStore()
	}
	return &pgProjectsStore{pool: pool}
}

type pgProjectsStore struct {
	pool *pgxpool.Pool
}

func (s *pgProjectsStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres projects store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
-- Projects table stores project metadata
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revision BIGINT NOT NULL DEFAULT 1,
    bytes BIGINT NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0,
    storage_backend TEXT NOT NULL DEFAULT 'filesystem'
);

CREATE INDEX IF NOT EXISTS projects_user_updated_idx ON projects(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS projects_user_name_idx ON projects(user_id, name);

-- Project files index for fast directory listing
CREATE TABLE IF NOT EXISTS project_files (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    name TEXT NOT NULL,
    is_dir BOOLEAN NOT NULL DEFAULT FALSE,
    size BIGINT NOT NULL DEFAULT 0,
    mod_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    etag TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, path)
);

-- Index for listing files in a directory (parent path lookup)
CREATE INDEX IF NOT EXISTS project_files_parent_idx ON project_files(project_id, (CASE 
    WHEN path NOT LIKE '%/%' THEN '.'
    ELSE LEFT(path, LENGTH(path) - LENGTH(SUBSTRING(path FROM '[^/]*$')) - 1)
END));
`)
	return err
}

func (s *pgProjectsStore) Create(ctx context.Context, userID int64, name string) (persistence.Project, error) {
	if strings.TrimSpace(name) == "" {
		name = "Untitled"
	}
	id := uuid.New()
	now := time.Now().UTC()

	row := s.pool.QueryRow(ctx, `
INSERT INTO projects (id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend)
VALUES ($1, $2, $3, $4, $4, 1, 0, 0, 'filesystem')
RETURNING id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend`,
		id, userID, name, now)

	return s.scanProject(row)
}

// InsertWithID inserts a project with a specific ID (used for migration).
// This preserves existing project IDs when migrating from filesystem to database.
func (s *pgProjectsStore) InsertWithID(ctx context.Context, userID int64, projectID, name string, createdAt, updatedAt time.Time, bytes int64, fileCount int) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO projects (id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend)
VALUES ($1, $2, $3, $4, $5, 1, $6, $7, 'filesystem')
ON CONFLICT (id) DO NOTHING`,
		projectID, userID, name, createdAt, updatedAt, bytes, fileCount)
	return err
}

func (s *pgProjectsStore) Get(ctx context.Context, userID int64, projectID string) (persistence.Project, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
FROM projects WHERE id = $1`, projectID)

	p, err := s.scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.Project{}, persistence.ErrNotFound
		}
		return persistence.Project{}, err
	}
	if p.UserID != userID {
		return persistence.Project{}, persistence.ErrForbidden
	}
	return p, nil
}

func (s *pgProjectsStore) List(ctx context.Context, userID int64) ([]persistence.Project, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend
FROM projects
WHERE user_id = $1
ORDER BY updated_at DESC, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []persistence.Project
	for rows.Next() {
		p, err := s.scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if projects == nil {
		projects = []persistence.Project{}
	}
	return projects, nil
}

func (s *pgProjectsStore) Update(ctx context.Context, p persistence.Project) (persistence.Project, error) {
	now := time.Now().UTC()

	row := s.pool.QueryRow(ctx, `
UPDATE projects
SET name = $1, updated_at = $2, revision = revision + 1, storage_backend = $3
WHERE id = $4 AND revision = $5
RETURNING id, user_id, name, created_at, updated_at, revision, bytes, file_count, storage_backend`,
		p.Name, now, p.StorageBackend, p.ID, p.Revision)

	updated, err := s.scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Could be not found or revision conflict - check which
			var exists bool
			_ = s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)", p.ID).Scan(&exists)
			if !exists {
				return persistence.Project{}, persistence.ErrNotFound
			}
			return persistence.Project{}, persistence.ErrRevisionConflict
		}
		return persistence.Project{}, err
	}
	return updated, nil
}

func (s *pgProjectsStore) UpdateStats(ctx context.Context, projectID string, bytes int64, fileCount int) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
UPDATE projects SET bytes = $1, file_count = $2, updated_at = $3 WHERE id = $4`,
		bytes, fileCount, now, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *pgProjectsStore) Delete(ctx context.Context, userID int64, projectID string) error {
	// Check ownership first
	var owner int64
	err := s.pool.QueryRow(ctx, "SELECT user_id FROM projects WHERE id = $1", projectID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // Already deleted
		}
		return err
	}
	if owner != userID {
		return persistence.ErrForbidden
	}

	// Delete project (cascade will remove file index)
	_, err = s.pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	return err
}

// --- File Index Operations ---

func (s *pgProjectsStore) IndexFile(ctx context.Context, f persistence.ProjectFile) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
INSERT INTO project_files (project_id, path, name, is_dir, size, mod_time, etag, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (project_id, path) DO UPDATE SET
    name = EXCLUDED.name,
    is_dir = EXCLUDED.is_dir,
    size = EXCLUDED.size,
    mod_time = EXCLUDED.mod_time,
    etag = EXCLUDED.etag,
    updated_at = EXCLUDED.updated_at`,
		f.ProjectID, f.Path, f.Name, f.IsDir, f.Size, f.ModTime, f.ETag, now)
	return err
}

func (s *pgProjectsStore) RemoveFileIndex(ctx context.Context, projectID, filePath string) error {
	_, err := s.pool.Exec(ctx, `
DELETE FROM project_files WHERE project_id = $1 AND path = $2`,
		projectID, filePath)
	return err
}

func (s *pgProjectsStore) RemoveFileIndexPrefix(ctx context.Context, projectID, pathPrefix string) error {
	// Remove exact path and all children
	pattern := pathPrefix
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	_, err := s.pool.Exec(ctx, `
DELETE FROM project_files 
WHERE project_id = $1 AND (path = $2 OR path LIKE $3)`,
		projectID, pathPrefix, pattern+"%")
	return err
}

func (s *pgProjectsStore) ListFiles(ctx context.Context, projectID, dirPath string) ([]persistence.ProjectFile, error) {
	// Normalize path
	dirPath = normalizePath(dirPath)

	// For root directory, match entries with no "/" in path
	// For subdirectories, match entries whose parent is dirPath
	var rows pgx.Rows
	var err error

	if dirPath == "." || dirPath == "" {
		// Root directory: files with no "/" in path
		rows, err = s.pool.Query(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = $1 AND path NOT LIKE '%/%'
ORDER BY is_dir DESC, name ASC`, projectID)
	} else {
		// Subdirectory: files whose parent directory matches
		// Parent is everything before the last "/"
		rows, err = s.pool.Query(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = $1 
  AND path LIKE $2 || '/%'
  AND path NOT LIKE $2 || '/%/%'
ORDER BY is_dir DESC, name ASC`, projectID, dirPath)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []persistence.ProjectFile
	for rows.Next() {
		f, err := s.scanProjectFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if files == nil {
		files = []persistence.ProjectFile{}
	}
	return files, nil
}

func (s *pgProjectsStore) GetFile(ctx context.Context, projectID, filePath string) (persistence.ProjectFile, error) {
	filePath = normalizePath(filePath)
	row := s.pool.QueryRow(ctx, `
SELECT project_id, path, name, is_dir, size, mod_time, etag, updated_at
FROM project_files
WHERE project_id = $1 AND path = $2`, projectID, filePath)

	var f persistence.ProjectFile
	var modTime, updatedAt sql.NullTime
	err := row.Scan(&f.ProjectID, &f.Path, &f.Name, &f.IsDir, &f.Size, &modTime, &f.ETag, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ProjectFile{}, persistence.ErrNotFound
		}
		return persistence.ProjectFile{}, err
	}
	if modTime.Valid {
		f.ModTime = modTime.Time
	}
	if updatedAt.Valid {
		f.UpdatedAt = updatedAt.Time
	}
	return f, nil
}

// --- Helpers ---

func (s *pgProjectsStore) scanProject(row pgx.Row) (persistence.Project, error) {
	var p persistence.Project
	var storageBackend sql.NullString
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt,
		&p.Revision, &p.Bytes, &p.FileCount, &storageBackend)
	if err != nil {
		return persistence.Project{}, err
	}
	if storageBackend.Valid {
		p.StorageBackend = storageBackend.String
	}
	return p, nil
}

func (s *pgProjectsStore) scanProjectRow(rows pgx.Rows) (persistence.Project, error) {
	var p persistence.Project
	var storageBackend sql.NullString
	err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt,
		&p.Revision, &p.Bytes, &p.FileCount, &storageBackend)
	if err != nil {
		return persistence.Project{}, err
	}
	if storageBackend.Valid {
		p.StorageBackend = storageBackend.String
	}
	return p, nil
}

func (s *pgProjectsStore) scanProjectFile(rows pgx.Rows) (persistence.ProjectFile, error) {
	var f persistence.ProjectFile
	var modTime, updatedAt sql.NullTime
	err := rows.Scan(&f.ProjectID, &f.Path, &f.Name, &f.IsDir, &f.Size, &modTime, &f.ETag, &updatedAt)
	if err != nil {
		return persistence.ProjectFile{}, err
	}
	if modTime.Valid {
		f.ModTime = modTime.Time
	}
	if updatedAt.Valid {
		f.UpdatedAt = updatedAt.Time
	}
	return f, nil
}

// normalizePath normalizes directory paths for consistent lookups.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = path.Clean(p)
	if p == "/" || p == "" {
		return "."
	}
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return p
}
