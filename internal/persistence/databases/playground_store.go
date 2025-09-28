package databases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"intelligence.dev/internal/playground"
	"intelligence.dev/internal/playground/dataset"
	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/registry"
)

// PlaygroundStore persists playground entities into Postgres using JSONB columns.
type PlaygroundStore struct {
	pool *pgxpool.Pool
}

// NewPlaygroundStore creates the store and ensures schema exists.
func NewPlaygroundStore(ctx context.Context, pool *pgxpool.Pool) (*PlaygroundStore, error) {
	store := &PlaygroundStore{pool: pool}
	if err := store.initSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// NewPlaygroundStoreFromDSN constructs a new store using its own connection pool.
func NewPlaygroundStoreFromDSN(ctx context.Context, dsn string) (*PlaygroundStore, error) {
	pool, err := newPgPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	store, err := NewPlaygroundStore(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PlaygroundStore) initSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS playground_prompts (
            id TEXT PRIMARY KEY,
            payload JSONB NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS playground_prompt_versions (
            id TEXT PRIMARY KEY,
            prompt_id TEXT NOT NULL,
            created_at TIMESTAMPTZ NOT NULL,
            payload JSONB NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS playground_datasets (
            id TEXT PRIMARY KEY,
            payload JSONB NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS playground_snapshots (
            id TEXT NOT NULL,
            dataset_id TEXT NOT NULL,
            created_at TIMESTAMPTZ NOT NULL,
            payload JSONB NOT NULL,
            PRIMARY KEY (dataset_id, id)
        );`,
		`CREATE TABLE IF NOT EXISTS playground_rows (
            dataset_id TEXT NOT NULL,
            snapshot_id TEXT NOT NULL,
            row_id TEXT NOT NULL,
            payload JSONB NOT NULL,
            PRIMARY KEY (dataset_id, snapshot_id, row_id)
        );`,
		`CREATE TABLE IF NOT EXISTS playground_experiments (
            id TEXT PRIMARY KEY,
            payload JSONB NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS playground_runs (
            id TEXT PRIMARY KEY,
            experiment_id TEXT NOT NULL,
            payload JSONB NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS playground_run_results (
            id TEXT PRIMARY KEY,
            run_id TEXT NOT NULL,
            payload JSONB NOT NULL
        );`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("init playground schema: %w", err)
		}
	}
	return nil
}

// CreatePrompt inserts a new prompt.
func (s *PlaygroundStore) CreatePrompt(ctx context.Context, prompt registry.Prompt) (registry.Prompt, error) {
	data, err := json.Marshal(prompt)
	if err != nil {
		return registry.Prompt{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_prompts (id, payload) VALUES ($1, $2)`, prompt.ID, data)
	if err != nil {
		if isPGConstraint(err) {
			return registry.Prompt{}, registry.ErrPromptExists
		}
		return registry.Prompt{}, err
	}
	return prompt, nil
}

// GetPrompt loads a prompt by ID.
func (s *PlaygroundStore) GetPrompt(ctx context.Context, id string) (registry.Prompt, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_prompts WHERE id=$1`, id)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.Prompt{}, false, nil
		}
		return registry.Prompt{}, false, err
	}
	var prompt registry.Prompt
	if err := json.Unmarshal(payload, &prompt); err != nil {
		return registry.Prompt{}, false, err
	}
	return prompt, true, nil
}

// ListPrompts fetches all prompts and filters in memory.
func (s *PlaygroundStore) ListPrompts(ctx context.Context, filter registry.ListFilter) ([]registry.Prompt, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_prompts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prompts []registry.Prompt
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var prompt registry.Prompt
		if err := json.Unmarshal(payload, &prompt); err != nil {
			return nil, err
		}
		prompts = append(prompts, prompt)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	filtered := prompts[:0]
	for _, prompt := range prompts {
		if filter.Query != "" && !strings.Contains(strings.ToLower(prompt.Name+prompt.Description), strings.ToLower(filter.Query)) {
			continue
		}
		if filter.Tag != "" {
			found := false
			for _, tag := range prompt.Tags {
				if strings.EqualFold(tag, filter.Tag) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, prompt)
	}
	prompts = filtered
	sort.Slice(prompts, func(i, j int) bool { return prompts[i].CreatedAt.After(prompts[j].CreatedAt) })

	page := filter.Page
	perPage := filter.PerPage
	if perPage <= 0 {
		perPage = len(prompts)
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > len(prompts) {
		return []registry.Prompt{}, nil
	}
	end := start + perPage
	if end > len(prompts) {
		end = len(prompts)
	}
	return prompts[start:end], nil
}

// CreatePromptVersion stores the version payload.
func (s *PlaygroundStore) CreatePromptVersion(ctx context.Context, version registry.PromptVersion) (registry.PromptVersion, error) {
	data, err := json.Marshal(version)
	if err != nil {
		return registry.PromptVersion{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_prompt_versions (id, prompt_id, created_at, payload) VALUES ($1,$2,$3,$4)`, version.ID, version.PromptID, version.CreatedAt.UTC(), data)
	if err != nil {
		return registry.PromptVersion{}, err
	}
	return version, nil
}

// ListPromptVersions returns all versions for a prompt newest first.
func (s *PlaygroundStore) ListPromptVersions(ctx context.Context, promptID string) ([]registry.PromptVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_prompt_versions WHERE prompt_id=$1 ORDER BY created_at DESC`, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []registry.PromptVersion
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var version registry.PromptVersion
		if err := json.Unmarshal(payload, &version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// GetPromptVersion fetches a prompt version by ID.
func (s *PlaygroundStore) GetPromptVersion(ctx context.Context, id string) (registry.PromptVersion, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_prompt_versions WHERE id=$1`, id)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.PromptVersion{}, false, nil
		}
		return registry.PromptVersion{}, false, err
	}
	var version registry.PromptVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return registry.PromptVersion{}, false, err
	}
	return version, true, nil
}

// CreateDataset stores dataset metadata.
func (s *PlaygroundStore) CreateDataset(ctx context.Context, ds dataset.Dataset) (dataset.Dataset, error) {
	data, err := json.Marshal(ds)
	if err != nil {
		return dataset.Dataset{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_datasets (id, payload) VALUES ($1,$2)`, ds.ID, data)
	if err != nil {
		return dataset.Dataset{}, err
	}
	return ds, nil
}

// UpdateDataset updates dataset metadata payload.
func (s *PlaygroundStore) UpdateDataset(ctx context.Context, ds dataset.Dataset) (dataset.Dataset, error) {
	data, err := json.Marshal(ds)
	if err != nil {
		return dataset.Dataset{}, err
	}
	cmd, err := s.pool.Exec(ctx, `UPDATE playground_datasets SET payload=$2 WHERE id=$1`, ds.ID, data)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if cmd.RowsAffected() == 0 {
		return dataset.Dataset{}, dataset.ErrDatasetNotFound
	}
	return ds, nil
}

// GetDataset fetches dataset metadata.
func (s *PlaygroundStore) GetDataset(ctx context.Context, id string) (dataset.Dataset, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_datasets WHERE id=$1`, id)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dataset.Dataset{}, false, nil
		}
		return dataset.Dataset{}, false, err
	}
	var ds dataset.Dataset
	if err := json.Unmarshal(payload, &ds); err != nil {
		return dataset.Dataset{}, false, err
	}
	return ds, true, nil
}

// ListDatasets returns all dataset metadata sorted by creation time descending.
func (s *PlaygroundStore) ListDatasets(ctx context.Context) ([]dataset.Dataset, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_datasets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var datasets []dataset.Dataset
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var ds dataset.Dataset
		if err := json.Unmarshal(payload, &ds); err != nil {
			return nil, err
		}
		datasets = append(datasets, ds)
	}
	sort.Slice(datasets, func(i, j int) bool { return datasets[i].CreatedAt.After(datasets[j].CreatedAt) })
	return datasets, rows.Err()
}

// CreateSnapshot stores snapshot metadata and rows.
func (s *PlaygroundStore) CreateSnapshot(ctx context.Context, snapshot dataset.Snapshot, rows []dataset.Row) (dataset.Snapshot, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return dataset.Snapshot{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	meta, err := json.Marshal(snapshot)
	if err != nil {
		return dataset.Snapshot{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO playground_snapshots (dataset_id,id,created_at,payload) VALUES ($1,$2,$3,$4)
        ON CONFLICT (dataset_id, id) DO UPDATE SET created_at=EXCLUDED.created_at, payload=EXCLUDED.payload`, snapshot.DatasetID, snapshot.ID, snapshot.CreatedAt.UTC(), meta); err != nil {
		return dataset.Snapshot{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_rows WHERE dataset_id=$1 AND snapshot_id=$2`, snapshot.DatasetID, snapshot.ID); err != nil {
		return dataset.Snapshot{}, err
	}
	for _, row := range rows {
		payload, mErr := json.Marshal(row)
		if mErr != nil {
			err = mErr
			return dataset.Snapshot{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO playground_rows (dataset_id,snapshot_id,row_id,payload) VALUES ($1,$2,$3,$4)`, snapshot.DatasetID, snapshot.ID, row.ID, payload); err != nil {
			return dataset.Snapshot{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return dataset.Snapshot{}, err
	}
	return snapshot, nil
}

// ListSnapshotRows returns rows for a snapshot ordered by row_id.
func (s *PlaygroundStore) ListSnapshotRows(ctx context.Context, datasetID, snapshotID string) ([]dataset.Row, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_rows WHERE dataset_id=$1 AND snapshot_id=$2 ORDER BY row_id ASC`, datasetID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dataset.Row
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var row dataset.Row
		if err := json.Unmarshal(payload, &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CreateExperiment persists an experiment spec as JSON.
func (s *PlaygroundStore) CreateExperiment(ctx context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return experiment.ExperimentSpec{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_experiments (id, payload) VALUES ($1,$2)
        ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload`, spec.ID, payload)
	if err != nil {
		return experiment.ExperimentSpec{}, err
	}
	return spec, nil
}

// GetExperiment retrieves the experiment spec by ID.
func (s *PlaygroundStore) GetExperiment(ctx context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_experiments WHERE id=$1`, id)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return experiment.ExperimentSpec{}, false, nil
		}
		return experiment.ExperimentSpec{}, false, err
	}
	var spec experiment.ExperimentSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return experiment.ExperimentSpec{}, false, err
	}
	return spec, true, nil
}

// ListExperiments returns all experiments sorted by creation time descending.
func (s *PlaygroundStore) ListExperiments(ctx context.Context) ([]experiment.ExperimentSpec, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_experiments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var specs []experiment.ExperimentSpec
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var spec experiment.ExperimentSpec
		if err := json.Unmarshal(payload, &spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].CreatedAt.After(specs[j].CreatedAt) })
	return specs, rows.Err()
}

// CreateRun stores the run payload.
func (s *PlaygroundStore) CreateRun(ctx context.Context, run playground.Run) (playground.Run, error) {
	payload, err := json.Marshal(run)
	if err != nil {
		return playground.Run{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_runs (id, experiment_id, payload) VALUES ($1,$2,$3)
        ON CONFLICT (id) DO UPDATE SET experiment_id=EXCLUDED.experiment_id, payload=EXCLUDED.payload`, run.ID, run.ExperimentID, payload)
	if err != nil {
		return playground.Run{}, err
	}
	return run, nil
}

// UpdateRunStatus updates the stored run with new status values.
func (s *PlaygroundStore) UpdateRunStatus(ctx context.Context, id string, status playground.RunStatus, endedAt time.Time, errMsg string, metrics map[string]float64) error {
	run, ok, err := s.getRun(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	run.Status = status
	run.EndedAt = endedAt
	run.Error = errMsg
	if metrics != nil {
		run.Metrics = cloneMetrics(metrics)
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE playground_runs SET payload=$1 WHERE id=$2`, payload, id)
	return err
}

// AppendResults persists run results.
func (s *PlaygroundStore) AppendResults(ctx context.Context, runID string, results []playground.RunResult) error {
	batch := &pgx.Batch{}
	for _, res := range results {
		payload, err := json.Marshal(res)
		if err != nil {
			return err
		}
		batch.Queue(`INSERT INTO playground_run_results (id, run_id, payload) VALUES ($1,$2,$3)
            ON CONFLICT (id) DO UPDATE SET run_id=EXCLUDED.run_id, payload=EXCLUDED.payload`, res.ID, runID, payload)
	}
	br := s.pool.SendBatch(ctx, batch)
	for range results {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return err
		}
	}
	return br.Close()
}

// ListRuns returns runs for an experiment ordered by creation time.
func (s *PlaygroundStore) ListRuns(ctx context.Context, experimentID string) ([]playground.Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_runs WHERE experiment_id=$1`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []playground.Run
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var run playground.Run
		if err := json.Unmarshal(payload, &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, rows.Err()
}

// ListRunResults returns persisted results for a run ordered by row and variant.
func (s *PlaygroundStore) ListRunResults(ctx context.Context, runID string) ([]playground.RunResult, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_run_results WHERE run_id=$1`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []playground.RunResult
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var res playground.RunResult
		if err := json.Unmarshal(payload, &res); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.RowID != b.RowID {
			return a.RowID < b.RowID
		}
		if a.VariantID != b.VariantID {
			return a.VariantID < b.VariantID
		}
		return a.ID < b.ID
	})
	return results, nil
}

func cloneMetrics(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *PlaygroundStore) getRun(ctx context.Context, id string) (playground.Run, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_runs WHERE id=$1`, id)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return playground.Run{}, false, nil
		}
		return playground.Run{}, false, err
	}
	var run playground.Run
	if err := json.Unmarshal(payload, &run); err != nil {
		return playground.Run{}, false, err
	}
	return run, true, nil
}

func isPGConstraint(err error) bool {
	// Any constraint violation surfaces via pgx's pgconn.PgError with Code starting with "23".
	type causer interface{ SQLState() string }
	var c causer
	if errors.As(err, &c) {
		if strings.HasPrefix(c.SQLState(), "23") {
			return true
		}
	}
	return false
}

// Close releases the underlying connection pool.
func (s *PlaygroundStore) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}
