package databases

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"manifold/internal/playground"
	"manifold/internal/playground/experiment"
)

func (s *PlaygroundStore) CreateExperiment(ctx context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return experiment.ExperimentSpec{}, err
	}
	uid := userIDFromContext(ctx)
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_experiments (id, user_id, payload) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload`, spec.ID, uid, payload)
	if err != nil {
		return experiment.ExperimentSpec{}, err
	}
	return spec, nil
}

// GetExperiment retrieves the experiment spec by ID.
func (s *PlaygroundStore) GetExperiment(ctx context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	uid := userIDFromContext(ctx)
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_experiments WHERE id=$1 AND user_id=$2`, id, uid)
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
	uid := userIDFromContext(ctx)
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_experiments WHERE user_id=$1`, uid)
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
	uid := userIDFromContext(ctx)
	_, err = s.pool.Exec(ctx, `INSERT INTO playground_runs (id, experiment_id, user_id, payload) VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET experiment_id=EXCLUDED.experiment_id, payload=EXCLUDED.payload`, run.ID, run.ExperimentID, uid, payload)
	if err != nil {
		return playground.Run{}, err
	}
	return run, nil
}

// UpdateRunStatus updates the stored run with new status values.
func (s *PlaygroundStore) UpdateRunStatus(ctx context.Context, id string, status playground.RunStatus, endedAt time.Time, errMsg string, metrics map[string]float64) error {
	run, ok, err := s.getRun(ctx, id, userIDFromContext(ctx))
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
	_, err = s.pool.Exec(ctx, `UPDATE playground_runs SET payload=$1 WHERE id=$2 AND user_id=$3`, payload, id, userIDFromContext(ctx))
	return err
}

// AppendResults persists run results.
func (s *PlaygroundStore) AppendResults(ctx context.Context, runID string, results []playground.RunResult) error {
	batch := &pgx.Batch{}
	uid := userIDFromContext(ctx)
	for _, res := range results {
		payload, err := json.Marshal(res)
		if err != nil {
			return err
		}
		batch.Queue(`INSERT INTO playground_run_results (id, run_id, user_id, payload) VALUES ($1,$2,$3,$4)
			ON CONFLICT (id) DO UPDATE SET run_id=EXCLUDED.run_id, payload=EXCLUDED.payload`, res.ID, runID, uid, payload)
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
	uid := userIDFromContext(ctx)
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_runs WHERE experiment_id=$1 AND user_id=$2`, experimentID, uid)
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
	uid := userIDFromContext(ctx)
	rows, err := s.pool.Query(ctx, `SELECT payload FROM playground_run_results WHERE run_id=$1 AND user_id=$2`, runID, uid)
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
