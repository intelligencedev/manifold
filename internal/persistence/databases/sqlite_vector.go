package databases

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/config"
)

type sqliteVector struct {
	db                *sql.DB
	dimensions        int
	metric            string
	nprobe            float64
	annEnabled        bool
	annMinRows        int
	annRebuildChanges int
	rebuildMu         sync.Mutex
}

func NewSQLiteVector(db *sql.DB, cfg config.VectorConfig, sqliteCfg config.SQLiteVectorConfig) (VectorStore, error) {
	metric := strings.ToLower(strings.TrimSpace(cfg.Metric))
	if metric == "" {
		metric = "cosine"
	}
	switch metric {
	case "cos", "cosine":
		metric = "cos"
	case "l2", "euclidean":
		metric = "l2"
	case "ip", "dot":
		return nil, errors.New("sqlite vector backend does not support ip/dot metric")
	default:
		return nil, fmt.Errorf("unsupported sqlite vector metric: %s", cfg.Metric)
	}
	nprobe := sqliteCfg.NProbe
	if nprobe <= 0 {
		nprobe = 0.08
	}
	annMinRows := sqliteCfg.ANNMinRows
	if annMinRows <= 0 {
		annMinRows = 5000
	}
	annRebuildChanges := sqliteCfg.ANNRebuildChanges
	if annRebuildChanges <= 0 {
		annRebuildChanges = 1000
	}
	return &sqliteVector{
		db:                db,
		dimensions:        cfg.Dimensions,
		metric:            metric,
		nprobe:            nprobe,
		annEnabled:        sqliteCfg.ANNEnabled,
		annMinRows:        annMinRows,
		annRebuildChanges: annRebuildChanges,
	}, nil
}

func (s *sqliteVector) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite vector store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS manifold_embeddings_meta (
	rowid INTEGER PRIMARY KEY,
	id TEXT NOT NULL UNIQUE,
	metadata TEXT NOT NULL DEFAULT '{}',
	tenant TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT '',
	doc_id TEXT NOT NULL DEFAULT '',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE VIRTUAL TABLE IF NOT EXISTS manifold_embeddings_vec USING vec1(vector, tenant, type, doc_id);
CREATE TABLE IF NOT EXISTS manifold_embeddings_vec_state (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	if err := s.ensureFlatIndex(ctx); err != nil {
		return err
	}
	return nil
}

func (s *sqliteVector) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	if s.dimensions > 0 && len(vector) != s.dimensions {
		return fmt.Errorf("sqlite vector dimension mismatch: got %d, want %d", len(vector), s.dimensions)
	}
	blob := float32Blob(vector)
	tenant, typ, docID := promotedVectorMetadata(metadata)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO manifold_embeddings_meta(id, metadata, tenant, type, doc_id, updated_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
	metadata=excluded.metadata,
	tenant=excluded.tenant,
	type=excluded.type,
	doc_id=excluded.doc_id,
	updated_at=excluded.updated_at
`, id, encodeStringMap(metadata), tenant, typ, docID); err != nil {
		return err
	}
	var rowid int64
	if err := tx.QueryRowContext(ctx, `SELECT rowid FROM manifold_embeddings_meta WHERE id = ?`, id).Scan(&rowid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_embeddings_vec WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO manifold_embeddings_vec(rowid, vector, tenant, type, doc_id) VALUES(?, ?, ?, ?, ?)`, rowid, blob, tenant, typ, docID); err != nil {
		return err
	}
	if err := incrementSQLiteVectorChangedRows(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.scheduleANNRebuildIfNeeded()
	return nil
}

func (s *sqliteVector) Delete(ctx context.Context, id string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	var rowid int64
	if err := tx.QueryRowContext(ctx, `SELECT rowid FROM manifold_embeddings_meta WHERE id = ?`, id).Scan(&rowid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_embeddings_vec WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_embeddings_meta WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if err := incrementSQLiteVectorChangedRows(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.scheduleANNRebuildIfNeeded()
	return nil
}

func (s *sqliteVector) SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]VectorResult, error) {
	if k <= 0 {
		k = 10
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	if s.dimensions > 0 && len(vector) != s.dimensions {
		return nil, fmt.Errorf("sqlite vector dimension mismatch: got %d, want %d", len(vector), s.dimensions)
	}
	blob := float32Blob(vector)
	overfetch := max(k*10, 100)
	if overfetch > 1000 {
		overfetch = 1000
	}
	query, args, remainingFilter := s.similarityQuery(blob, overfetch, filter)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	results := make([]VectorResult, 0, k)
	for rows.Next() {
		var result VectorResult
		var mdRaw string
		var distance float64
		if err := rows.Scan(&result.ID, &mdRaw, &distance); err != nil {
			return nil, err
		}
		result.Metadata = decodeStringMap(mdRaw)
		if !metaMatches(result.Metadata, remainingFilter) {
			continue
		}
		if s.metric == "l2" {
			result.Score = -distance
		} else {
			result.Score = 1 - distance
		}
		results = append(results, result)
		if len(results) >= k {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > k {
		results = results[:k]
	}
	return results, nil
}

func (s *sqliteVector) similarityQuery(blob []byte, overfetch int, filter map[string]string) (string, []any, map[string]string) {
	remaining := copyMap(filter)
	where := []string{}
	args := []any{blob, fmt.Sprintf(`{K:%d, streaming:1, nprobe:%g}`, overfetch, s.nprobe)}
	for _, key := range []string{"tenant", "type", "doc_id"} {
		if value, ok := remaining[key]; ok {
			where = append(where, fmt.Sprintf("%s = ?", key))
			args = append(args, value)
			delete(remaining, key)
		}
	}
	args = append(args, overfetch, blob, overfetch)
	distanceFn := "vec1_cos_distance"
	if s.metric == "l2" {
		distanceFn = "vec1_l2_distance"
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	query := fmt.Sprintf(`
WITH candidates(rowid, vector) AS (
	SELECT rowid, vector
	FROM manifold_embeddings_vec(?, ?)
	%s
	LIMIT ?
)
SELECT m.id, m.metadata, %s(?, c.vector) AS distance
FROM candidates c
JOIN manifold_embeddings_meta m ON m.rowid = c.rowid
ORDER BY distance ASC
LIMIT ?`, whereSQL, distanceFn)
	return query, args, remaining
}

func (s *sqliteVector) Dimension() int {
	return s.dimensions
}

func (s *sqliteVector) ensureFlatIndex(ctx context.Context) error {
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM manifold_embeddings_vec_state WHERE key = 'index_mode'`).Scan(&existing)
	if err == nil && strings.TrimSpace(existing) != "" {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO manifold_embeddings_vec(cmd, arg) VALUES('rebuild', ?)`, fmt.Sprintf(`{index:"flat", distance:"%s"}`, s.metric)); err != nil {
		return err
	}
	return s.setVectorState(ctx, "index_mode", "flat")
}

func (s *sqliteVector) scheduleANNRebuildIfNeeded() {
	if !s.annEnabled {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = s.rebuildANNIfNeeded(ctx)
	}()
}

func (s *sqliteVector) rebuildANNIfNeeded(ctx context.Context) error {
	if !s.annEnabled {
		return nil
	}
	s.rebuildMu.Lock()
	defer s.rebuildMu.Unlock()
	if err := s.Init(ctx); err != nil {
		return err
	}
	rowCount, err := s.vectorRowCount(ctx)
	if err != nil {
		return err
	}
	if rowCount < s.annMinRows {
		return nil
	}
	changedRows, err := s.vectorChangedRows(ctx)
	if err != nil {
		return err
	}
	mode, err := s.vectorIndexMode(ctx)
	if err != nil {
		return err
	}
	if mode == "ann" && changedRows < s.annRebuildChanges {
		return nil
	}
	return s.rebuildANN(ctx, rowCount)
}

func (s *sqliteVector) rebuildANN(ctx context.Context, rowCount int) error {
	nbucket := int(math.Sqrt(float64(rowCount)))
	if nbucket < 2 {
		nbucket = 2
	}
	modelParams := fmt.Sprintf(`{distance:"%s", codesize:0, nbucket:%d}`, s.metric, nbucket)
	var model []byte
	if err := s.db.QueryRowContext(ctx, `SELECT vec1_train(vector, ?) FROM manifold_embeddings_vec`, modelParams).Scan(&model); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO manifold_embeddings_vec(cmd, arg) VALUES('rebuild', ?)`, model); err != nil {
		return err
	}
	if err := s.setVectorState(ctx, "index_mode", "ann"); err != nil {
		return err
	}
	return s.setVectorState(ctx, "changed_rows", "0")
}

func (s *sqliteVector) vectorRowCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM manifold_embeddings_meta`).Scan(&count)
	return count, err
}

func (s *sqliteVector) vectorChangedRows(ctx context.Context) (int, error) {
	var changedRows int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(CAST(value AS INTEGER), 0) FROM manifold_embeddings_vec_state WHERE key = 'changed_rows'`).Scan(&changedRows)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return changedRows, err
}

func (s *sqliteVector) vectorIndexMode(ctx context.Context) (string, error) {
	var mode string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM manifold_embeddings_vec_state WHERE key = 'index_mode'`).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return "flat", nil
	}
	return mode, err
}

func (s *sqliteVector) setVectorState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO manifold_embeddings_vec_state(key, value)
VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, key, value)
	return err
}

func incrementSQLiteVectorChangedRows(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO manifold_embeddings_vec_state(key, value)
VALUES('changed_rows', '1')
ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1
`)
	return err
}

func promotedVectorMetadata(metadata map[string]string) (tenant, typ, docID string) {
	if metadata == nil {
		return "", "", ""
	}
	return metadata["tenant"], metadata["type"], metadata["doc_id"]
}

func float32Blob(values []float32) []byte {
	out := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(value))
	}
	return out
}
