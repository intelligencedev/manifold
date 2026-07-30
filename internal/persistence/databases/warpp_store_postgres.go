package databases

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	persist "manifold/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresWarppStore returns a Postgres-backed WARPP workflow store.
func NewPostgresWarppStore(pool *pgxpool.Pool) persist.WarppWorkflowStore {
	if pool == nil {
		return &memWarppStore{records: map[int64]map[string]persist.WarppWorkflowRecord{}}
	}
	return &pgWarppStore{pool: pool}
}

type memWarppStore struct {
	mu      sync.RWMutex
	records map[int64]map[string]persist.WarppWorkflowRecord
}

func (s *memWarppStore) Init(context.Context) error { return nil }

func (s *memWarppStore) ListWorkflows(_ context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userRecords := s.records[userID]
	out := make([]persist.WarppWorkflowRecord, 0, len(userRecords))
	for _, record := range userRecords {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Document.ID) < strings.ToLower(out[j].Document.ID)
	})
	return out, nil
}

func (s *memWarppStore) GetWorkflow(_ context.Context, userID int64, workflowID string) (persist.WarppWorkflowRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if userRecords := s.records[userID]; userRecords != nil {
		record, ok := userRecords[workflowID]
		return record, ok, nil
	}
	return persist.WarppWorkflowRecord{}, false, nil
}

func (s *memWarppStore) UpsertWorkflow(_ context.Context, userID int64, record persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	workflowID := strings.TrimSpace(record.Document.ID)
	if workflowID == "" {
		return persist.WarppWorkflowRecord{}, false, errors.New("workflow id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[userID] == nil {
		s.records[userID] = map[string]persist.WarppWorkflowRecord{}
	}
	now := time.Now().UTC()
	existing, existed := s.records[userID][workflowID]
	if !existed {
		record.CreatedAt = now
	} else {
		record.CreatedAt = existing.CreatedAt
	}
	record.UserID = userID
	record.UpdatedAt = now
	s.records[userID][workflowID] = record
	return record, !existed, nil
}

func (s *memWarppStore) DeleteWorkflow(_ context.Context, userID int64, workflowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[userID] == nil {
		return nil
	}
	delete(s.records[userID], workflowID)
	return nil
}

type pgWarppStore struct{ pool *pgxpool.Pool }

func (s *pgWarppStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS warpp_workflows (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL DEFAULT 0,
  workflow_id TEXT NOT NULL,
  document JSONB NOT NULL,
  canvas JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS warpp_workflows_user_workflow_idx ON warpp_workflows(user_id, workflow_id);

DROP TABLE IF EXISTS flow_v2_workflows;
`)
	return err
}

func (s *pgWarppStore) ListWorkflows(ctx context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	rows, err := s.pool.Query(ctx, `
SELECT document, canvas, created_at, updated_at
FROM warpp_workflows
WHERE user_id=$1
ORDER BY workflow_id
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []persist.WarppWorkflowRecord{}
	for rows.Next() {
		record, err := scanWarppWorkflowRecord(rows, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *pgWarppStore) GetWorkflow(ctx context.Context, userID int64, workflowID string) (persist.WarppWorkflowRecord, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT document, canvas, created_at, updated_at
FROM warpp_workflows
WHERE user_id=$1 AND workflow_id=$2
`, userID, workflowID)
	record, err := scanWarppWorkflowRecord(row, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persist.WarppWorkflowRecord{}, false, nil
		}
		return persist.WarppWorkflowRecord{}, false, err
	}
	return record, true, nil
}

func (s *pgWarppStore) UpsertWorkflow(ctx context.Context, userID int64, record persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	workflowID := strings.TrimSpace(record.Document.ID)
	if workflowID == "" {
		return persist.WarppWorkflowRecord{}, false, errors.New("workflow id required")
	}
	_, existed, err := s.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	documentDoc, err := json.Marshal(record.Document)
	if err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	canvasDoc, err := json.Marshal(record.Canvas)
	if err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
INSERT INTO warpp_workflows(user_id, workflow_id, document, canvas, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (user_id, workflow_id) DO UPDATE
SET document = EXCLUDED.document,
	canvas = EXCLUDED.canvas,
	updated_at = EXCLUDED.updated_at
RETURNING created_at, updated_at
`, userID, workflowID, documentDoc, canvasDoc, now)

	var createdAt, updatedAt time.Time
	if err := row.Scan(&createdAt, &updatedAt); err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	record.UserID = userID
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return record, !existed, nil
}

func (s *pgWarppStore) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM warpp_workflows WHERE user_id=$1 AND workflow_id=$2`, userID, workflowID)
	return err
}

type warppWorkflowScanner interface {
	Scan(dest ...any) error
}

func scanWarppWorkflowRecord(scanner warppWorkflowScanner, userID int64) (persist.WarppWorkflowRecord, error) {
	var documentDoc, canvasDoc []byte
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&documentDoc, &canvasDoc, &createdAt, &updatedAt); err != nil {
		return persist.WarppWorkflowRecord{}, err
	}
	var record persist.WarppWorkflowRecord
	if err := json.Unmarshal(documentDoc, &record.Document); err != nil {
		return persist.WarppWorkflowRecord{}, err
	}
	if len(canvasDoc) > 0 {
		if err := json.Unmarshal(canvasDoc, &record.Canvas); err != nil {
			return persist.WarppWorkflowRecord{}, err
		}
	}
	record.UserID = userID
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return record, nil
}
