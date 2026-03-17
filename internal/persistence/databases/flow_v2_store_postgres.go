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

// NewPostgresFlowV2Store returns a Postgres-backed Flow v2 workflow store.
func NewPostgresFlowV2Store(pool *pgxpool.Pool) persist.FlowV2WorkflowStore {
	if pool == nil {
		return &memFlowV2Store{records: map[int64]map[string]persist.FlowV2WorkflowRecord{}}
	}
	return &pgFlowV2Store{pool: pool}
}

type memFlowV2Store struct {
	mu      sync.RWMutex
	records map[int64]map[string]persist.FlowV2WorkflowRecord
}

func (s *memFlowV2Store) Init(context.Context) error { return nil }

func (s *memFlowV2Store) ListWorkflows(_ context.Context, userID int64) ([]persist.FlowV2WorkflowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userRecords := s.records[userID]
	if len(userRecords) == 0 {
		return []persist.FlowV2WorkflowRecord{}, nil
	}
	out := make([]persist.FlowV2WorkflowRecord, 0, len(userRecords))
	for _, record := range userRecords {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Workflow.ID) < strings.ToLower(out[j].Workflow.ID)
	})
	return out, nil
}

func (s *memFlowV2Store) GetWorkflow(_ context.Context, userID int64, workflowID string) (persist.FlowV2WorkflowRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if userRecords := s.records[userID]; userRecords != nil {
		record, ok := userRecords[workflowID]
		return record, ok, nil
	}
	return persist.FlowV2WorkflowRecord{}, false, nil
}

func (s *memFlowV2Store) UpsertWorkflow(_ context.Context, userID int64, record persist.FlowV2WorkflowRecord) (persist.FlowV2WorkflowRecord, bool, error) {
	workflowID := strings.TrimSpace(record.Workflow.ID)
	if workflowID == "" {
		return persist.FlowV2WorkflowRecord{}, false, errors.New("workflow id required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[userID] == nil {
		s.records[userID] = map[string]persist.FlowV2WorkflowRecord{}
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

func (s *memFlowV2Store) DeleteWorkflow(_ context.Context, userID int64, workflowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[userID] == nil {
		return nil
	}
	delete(s.records[userID], workflowID)
	return nil
}

type pgFlowV2Store struct{ pool *pgxpool.Pool }

func (s *pgFlowV2Store) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS flow_v2_workflows (
  id SERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL DEFAULT 0,
  workflow_id TEXT NOT NULL,
  workflow JSONB NOT NULL,
  canvas JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS flow_v2_workflows_user_workflow_idx ON flow_v2_workflows(user_id, workflow_id);
`)
	return err
}

func (s *pgFlowV2Store) ListWorkflows(ctx context.Context, userID int64) ([]persist.FlowV2WorkflowRecord, error) {
	rows, err := s.pool.Query(ctx, `
SELECT workflow, canvas, created_at, updated_at
FROM flow_v2_workflows
WHERE user_id=$1
ORDER BY workflow_id
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []persist.FlowV2WorkflowRecord{}
	for rows.Next() {
		record, err := scanFlowV2WorkflowRecord(rows, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *pgFlowV2Store) GetWorkflow(ctx context.Context, userID int64, workflowID string) (persist.FlowV2WorkflowRecord, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT workflow, canvas, created_at, updated_at
FROM flow_v2_workflows
WHERE user_id=$1 AND workflow_id=$2
`, userID, workflowID)
	record, err := scanFlowV2WorkflowRecord(row, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persist.FlowV2WorkflowRecord{}, false, nil
		}
		return persist.FlowV2WorkflowRecord{}, false, err
	}
	return record, true, nil
}

func (s *pgFlowV2Store) UpsertWorkflow(ctx context.Context, userID int64, record persist.FlowV2WorkflowRecord) (persist.FlowV2WorkflowRecord, bool, error) {
	workflowID := strings.TrimSpace(record.Workflow.ID)
	if workflowID == "" {
		return persist.FlowV2WorkflowRecord{}, false, errors.New("workflow id required")
	}

	_, existed, err := s.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return persist.FlowV2WorkflowRecord{}, false, err
	}

	workflowDoc, err := json.Marshal(record.Workflow)
	if err != nil {
		return persist.FlowV2WorkflowRecord{}, false, err
	}
	canvasDoc, err := json.Marshal(record.Canvas)
	if err != nil {
		return persist.FlowV2WorkflowRecord{}, false, err
	}

	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
INSERT INTO flow_v2_workflows(user_id, workflow_id, workflow, canvas, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (user_id, workflow_id) DO UPDATE
SET workflow = EXCLUDED.workflow,
	canvas = EXCLUDED.canvas,
	updated_at = EXCLUDED.updated_at
RETURNING created_at, updated_at
`, userID, workflowID, workflowDoc, canvasDoc, now)

	var createdAt time.Time
	var updatedAt time.Time
	if err := row.Scan(&createdAt, &updatedAt); err != nil {
		return persist.FlowV2WorkflowRecord{}, false, err
	}
	record.UserID = userID
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return record, !existed, nil
}

func (s *pgFlowV2Store) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM flow_v2_workflows WHERE user_id=$1 AND workflow_id=$2`, userID, workflowID)
	return err
}

type flowV2WorkflowScanner interface {
	Scan(dest ...any) error
}

func scanFlowV2WorkflowRecord(scanner flowV2WorkflowScanner, userID int64) (persist.FlowV2WorkflowRecord, error) {
	var workflowDoc []byte
	var canvasDoc []byte
	var createdAt time.Time
	var updatedAt time.Time
	if err := scanner.Scan(&workflowDoc, &canvasDoc, &createdAt, &updatedAt); err != nil {
		return persist.FlowV2WorkflowRecord{}, err
	}
	var record persist.FlowV2WorkflowRecord
	if err := json.Unmarshal(workflowDoc, &record.Workflow); err != nil {
		return persist.FlowV2WorkflowRecord{}, err
	}
	if len(canvasDoc) > 0 {
		if err := json.Unmarshal(canvasDoc, &record.Canvas); err != nil {
			return persist.FlowV2WorkflowRecord{}, err
		}
	}
	record.UserID = userID
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return record, nil
}
