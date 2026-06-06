package databases

import (
	"context"
	"encoding/json"
	"manifold/internal/playground/dataset"
	"sort"

	"github.com/jackc/pgx/v5"
)

func (s *PlaygroundStore) CreateDataset(ctx context.Context, ds dataset.Dataset) (dataset.Dataset, error) {
	data, err := json.Marshal(ds)
	if err != nil {
		return dataset.Dataset{}, err
	}
	uid := userIDFromContext(ctx)
	_, err = s.exec(ctx, `INSERT INTO playground_datasets (id, user_id, payload) VALUES ($1,$2,$3)`, ds.ID, uid, data)
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
	uid := userIDFromContext(ctx)
	rowsAffected, err := s.exec(ctx, `UPDATE playground_datasets SET payload=$3 WHERE id=$1 AND user_id=$2`, ds.ID, uid, data)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if rowsAffected == 0 {
		return dataset.Dataset{}, dataset.ErrDatasetNotFound
	}
	return ds, nil
}

// GetDataset fetches dataset metadata.
func (s *PlaygroundStore) GetDataset(ctx context.Context, id string) (dataset.Dataset, bool, error) {
	uid := userIDFromContext(ctx)
	payload, ok, err := s.queryOnePayload(ctx, `SELECT payload FROM playground_datasets WHERE id=$1 AND user_id=$2`, id, uid)
	if err != nil || !ok {
		return dataset.Dataset{}, ok, err
	}
	var ds dataset.Dataset
	if err := json.Unmarshal(payload, &ds); err != nil {
		return dataset.Dataset{}, false, err
	}
	return ds, true, nil
}

// ListDatasets returns all dataset metadata sorted by creation time descending.
func (s *PlaygroundStore) ListDatasets(ctx context.Context) ([]dataset.Dataset, error) {
	uid := userIDFromContext(ctx)
	payloads, err := s.queryPayloads(ctx, `SELECT payload FROM playground_datasets WHERE user_id=$1`, uid)
	if err != nil {
		return nil, err
	}
	var datasets []dataset.Dataset
	for _, payload := range payloads {
		var ds dataset.Dataset
		if err := json.Unmarshal(payload, &ds); err != nil {
			return nil, err
		}
		datasets = append(datasets, ds)
	}
	sort.Slice(datasets, func(i, j int) bool { return datasets[i].CreatedAt.After(datasets[j].CreatedAt) })
	return datasets, nil
}

// CreateSnapshot stores snapshot metadata and rows.
func (s *PlaygroundStore) CreateSnapshot(ctx context.Context, snapshot dataset.Snapshot, rows []dataset.Row) (dataset.Snapshot, error) {
	if s.db != nil {
		return s.createSQLiteSnapshot(ctx, snapshot, rows)
	}
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
	uid := userIDFromContext(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO playground_snapshots (dataset_id,id,created_at,user_id,payload) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (dataset_id, id) DO UPDATE SET created_at=EXCLUDED.created_at, payload=EXCLUDED.payload`, snapshot.DatasetID, snapshot.ID, snapshot.CreatedAt.UTC(), uid, meta); err != nil {
		return dataset.Snapshot{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playground_rows WHERE dataset_id=$1 AND snapshot_id=$2 AND user_id=$3`, snapshot.DatasetID, snapshot.ID, uid); err != nil {
		return dataset.Snapshot{}, err
	}
	for _, row := range rows {
		payload, mErr := json.Marshal(row)
		if mErr != nil {
			err = mErr
			return dataset.Snapshot{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO playground_rows (dataset_id,snapshot_id,row_id,user_id,payload) VALUES ($1,$2,$3,$4,$5)`, snapshot.DatasetID, snapshot.ID, row.ID, uid, payload); err != nil {
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
	uid := userIDFromContext(ctx)
	payloads, err := s.queryPayloads(ctx, `SELECT payload FROM playground_rows WHERE dataset_id=$1 AND snapshot_id=$2 AND user_id=$3 ORDER BY row_id ASC`, datasetID, snapshotID, uid)
	if err != nil {
		return nil, err
	}

	var out []dataset.Row
	for _, payload := range payloads {
		var row dataset.Row
		if err := json.Unmarshal(payload, &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *PlaygroundStore) createSQLiteSnapshot(ctx context.Context, snapshot dataset.Snapshot, rows []dataset.Row) (dataset.Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return dataset.Snapshot{}, err
	}
	defer rollbackQuietly(tx)

	meta, err := json.Marshal(snapshot)
	if err != nil {
		return dataset.Snapshot{}, err
	}
	uid := userIDFromContext(ctx)
	if _, err = tx.ExecContext(ctx, `INSERT INTO playground_snapshots (dataset_id,id,created_at,user_id,payload) VALUES (?,?,?,?,?)
		ON CONFLICT (dataset_id, id) DO UPDATE SET created_at=excluded.created_at, payload=excluded.payload`, snapshot.DatasetID, snapshot.ID, snapshot.CreatedAt.UTC(), uid, meta); err != nil {
		return dataset.Snapshot{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM playground_rows WHERE dataset_id=? AND snapshot_id=? AND user_id=?`, snapshot.DatasetID, snapshot.ID, uid); err != nil {
		return dataset.Snapshot{}, err
	}
	for _, row := range rows {
		payload, mErr := json.Marshal(row)
		if mErr != nil {
			return dataset.Snapshot{}, mErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO playground_rows (dataset_id,snapshot_id,row_id,user_id,payload) VALUES (?,?,?,?,?)`, snapshot.DatasetID, snapshot.ID, row.ID, uid, payload); err != nil {
			return dataset.Snapshot{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return dataset.Snapshot{}, err
	}
	return snapshot, nil
}
