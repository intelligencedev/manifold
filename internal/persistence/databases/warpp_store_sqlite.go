package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	persist "manifold/internal/persistence"
)

type sqliteWarppStore struct {
	db *sql.DB
}

// NewSQLiteWarppStore returns a SQLite-backed WARPP workflow store.
func NewSQLiteWarppStore(db *sql.DB) persist.WarppWorkflowStore {
	return &sqliteWarppStore{db: db}
}

func (s *sqliteWarppStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite warpp store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS warpp_workflows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL DEFAULT 0,
	workflow_id TEXT NOT NULL,
	document TEXT NOT NULL CHECK (json_valid(document)),
	canvas TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(canvas)),
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(user_id, workflow_id)
);
DROP TABLE IF EXISTS flow_v2_workflows;
`)
	return err
}

func (s *sqliteWarppStore) ListWorkflows(ctx context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT document, canvas, created_at, updated_at
FROM warpp_workflows
WHERE user_id = ?
ORDER BY workflow_id`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []persist.WarppWorkflowRecord{}
	for rows.Next() {
		record, err := scanSQLiteWarppWorkflowRecord(rows, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *sqliteWarppStore) GetWorkflow(ctx context.Context, userID int64, workflowID string) (persist.WarppWorkflowRecord, bool, error) {
	if err := s.Init(ctx); err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT document, canvas, created_at, updated_at
FROM warpp_workflows
WHERE user_id = ? AND workflow_id = ?`, userID, workflowID)
	record, err := scanSQLiteWarppWorkflowRecord(row, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return persist.WarppWorkflowRecord{}, false, nil
	}
	if err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	return record, true, nil
}

func (s *sqliteWarppStore) UpsertWorkflow(ctx context.Context, userID int64, record persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	workflowID := strings.TrimSpace(record.Document.ID)
	if workflowID == "" {
		return persist.WarppWorkflowRecord{}, false, errors.New("workflow id required")
	}
	if err := s.Init(ctx); err != nil {
		return persist.WarppWorkflowRecord{}, false, err
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
	row := s.db.QueryRowContext(ctx, `
INSERT INTO warpp_workflows(user_id, workflow_id, document, canvas, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, workflow_id) DO UPDATE SET
	document = excluded.document,
	canvas = excluded.canvas,
	updated_at = excluded.updated_at
RETURNING created_at, updated_at
`, userID, workflowID, string(documentDoc), string(canvasDoc), now, now)
	if err := row.Scan(&record.CreatedAt, &record.UpdatedAt); err != nil {
		return persist.WarppWorkflowRecord{}, false, err
	}
	record.UserID = userID
	return record, !existed, nil
}

func (s *sqliteWarppStore) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM warpp_workflows WHERE user_id = ? AND workflow_id = ?`, userID, workflowID)
	return err
}

func scanSQLiteWarppWorkflowRecord(row interface{ Scan(dest ...any) error }, userID int64) (persist.WarppWorkflowRecord, error) {
	var documentDoc, canvasDoc string
	var record persist.WarppWorkflowRecord
	if err := row.Scan(&documentDoc, &canvasDoc, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return persist.WarppWorkflowRecord{}, err
	}
	if err := json.Unmarshal([]byte(documentDoc), &record.Document); err != nil {
		return persist.WarppWorkflowRecord{}, err
	}
	if strings.TrimSpace(canvasDoc) != "" {
		if err := json.Unmarshal([]byte(canvasDoc), &record.Canvas); err != nil {
			return persist.WarppWorkflowRecord{}, err
		}
	}
	record.UserID = userID
	return record, nil
}
