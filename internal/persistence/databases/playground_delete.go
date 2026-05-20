package databases

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *PlaygroundStore) DeletePrompt(ctx context.Context, id string) error {
	batch := &pgx.Batch{}
	uid := userIDFromContext(ctx)
	batch.Queue(`DELETE FROM playground_prompt_versions WHERE prompt_id=$1 AND user_id=$2`, id, uid)
	batch.Queue(`DELETE FROM playground_prompts WHERE id=$1 AND user_id=$2`, id, uid)
	br := s.pool.SendBatch(ctx, batch)
	for range 2 {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return err
		}
	}
	return br.Close()
}

// DeleteDataset removes dataset metadata, snapshots and rows.
func (s *PlaygroundStore) DeleteDataset(ctx context.Context, id string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	// delete rows for all snapshots of this dataset
	uid := userIDFromContext(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM playground_rows WHERE dataset_id=$1 AND user_id=$2`, id, uid); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_snapshots WHERE dataset_id=$1 AND user_id=$2`, id, uid); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_datasets WHERE id=$1 AND user_id=$2`, id, uid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteExperiment removes an experiment and its runs/results.
func (s *PlaygroundStore) DeleteExperiment(ctx context.Context, id string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	// find runs
	uid := userIDFromContext(ctx)
	rows, err := tx.Query(ctx, `SELECT id FROM playground_runs WHERE experiment_id=$1 AND user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	var runIDs []string
	for rows.Next() {
		var rid string
		if scanErr := rows.Scan(&rid); scanErr != nil {
			err = scanErr
			rows.Close()
			return err
		}
		runIDs = append(runIDs, rid)
	}
	rows.Close()
	// delete results for runs
	for _, rid := range runIDs {
		if _, err = tx.Exec(ctx, `DELETE FROM playground_run_results WHERE run_id=$1 AND user_id=$2`, rid, uid); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_runs WHERE experiment_id=$1 AND user_id=$2`, id, uid); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_experiments WHERE id=$1 AND user_id=$2`, id, uid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
