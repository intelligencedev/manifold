package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"manifold/internal/agent/memory/artifact"
)

type sqliteArtifactStore struct {
	db *sql.DB
}

// NewSQLiteArtifactStore returns a SQLite-backed artifact store.
func NewSQLiteArtifactStore(db *sql.DB) artifact.Store {
	if db == nil {
		return NewMemoryArtifactStore()
	}
	return &sqliteArtifactStore{db: db}
}

func (s *sqliteArtifactStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite artifact store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS artifact_records (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	external_id TEXT NOT NULL,
	uri TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	excerpt TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL,
	authored_by TEXT NOT NULL DEFAULT '',
	authored_at DATETIME,
	captured_at DATETIME NOT NULL,
	metadata TEXT,
	payload TEXT NOT NULL CHECK(json_valid(payload))
);
CREATE UNIQUE INDEX IF NOT EXISTS artifact_records_immutable_source
	ON artifact_records(tenant_id, kind, external_id, content_hash);
CREATE INDEX IF NOT EXISTS artifact_records_external_id
	ON artifact_records(tenant_id, kind, external_id);
`)
	return err
}

func (s *sqliteArtifactStore) UpsertArtifact(ctx context.Context, item artifact.Artifact) (artifact.Artifact, error) {
	if err := s.Init(ctx); err != nil {
		return artifact.Artifact{}, err
	}
	item = prepareArtifact(item)
	if existingID, ok, err := s.findBySourceHash(ctx, item); err != nil {
		return artifact.Artifact{}, err
	} else if ok {
		item.ID = existingID
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return artifact.Artifact{}, err
	}
	metadata, _ := json.Marshal(item.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO artifact_records(id, tenant_id, kind, external_id, uri, title, excerpt, content_hash, authored_by, authored_at, captured_at, metadata, payload)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	uri=excluded.uri,
	title=excluded.title,
	excerpt=excluded.excerpt,
	authored_by=excluded.authored_by,
	authored_at=excluded.authored_at,
	captured_at=excluded.captured_at,
	metadata=excluded.metadata,
	payload=excluded.payload
`, item.ID, item.TenantID, item.Kind, item.ExternalID, item.URI, item.Title, item.Excerpt, item.ContentHash, item.AuthoredBy, sqliteTime{Time: item.AuthoredAt}, sqliteTime{Time: item.CapturedAt}, string(metadata), string(payload))
	if err != nil {
		return artifact.Artifact{}, err
	}
	return cloneArtifact(item), nil
}

func (s *sqliteArtifactStore) GetArtifact(ctx context.Context, tenantID int64, id string) (artifact.Artifact, bool, error) {
	if err := s.Init(ctx); err != nil {
		return artifact.Artifact{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT payload FROM artifact_records WHERE tenant_id = ? AND id = ?`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, ok, err := scanJSONRow[artifact.Artifact](row)
	if err != nil || !ok {
		return artifact.Artifact{}, ok, err
	}
	return cloneArtifact(item), true, nil
}

func (s *sqliteArtifactStore) FindByExternalID(ctx context.Context, tenantID int64, kind artifact.ArtifactKind, externalID string) ([]artifact.Artifact, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM artifact_records WHERE tenant_id = ? AND kind = ? AND external_id = ? ORDER BY captured_at ASC`, normalizeTenantID(tenantID), kind, strings.TrimSpace(externalID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []artifact.Artifact{}
	for rows.Next() {
		item, err := scanJSONRows[artifact.Artifact](rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cloneArtifact(item))
	}
	return out, rows.Err()
}

func (s *sqliteArtifactStore) SearchArtifacts(ctx context.Context, query artifact.SearchQuery) ([]artifact.SearchResult, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM artifact_records WHERE tenant_id = ?`, normalizeTenantID(query.TenantID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	kindSet := map[artifact.ArtifactKind]bool{}
	for _, kind := range query.Kinds {
		kindSet[kind] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	out := []artifact.SearchResult{}
	for rows.Next() {
		item, err := scanJSONRows[artifact.Artifact](rows)
		if err != nil {
			return nil, err
		}
		if len(kindSet) > 0 && !kindSet[item.Kind] {
			continue
		}
		text := strings.ToLower(item.Title + " " + item.Excerpt + " " + item.ExternalID)
		if needle != "" && !strings.Contains(text, needle) {
			continue
		}
		score := 1.0
		if needle != "" {
			score = 1.1
		}
		out = append(out, artifact.SearchResult{Artifact: cloneArtifact(item), Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Artifact.CapturedAt.After(out[j].Artifact.CapturedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *sqliteArtifactStore) findBySourceHash(ctx context.Context, item artifact.Artifact) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
SELECT id FROM artifact_records
WHERE tenant_id = ? AND kind = ? AND external_id = ? AND content_hash = ?`,
		item.TenantID, item.Kind, item.ExternalID, item.ContentHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}
