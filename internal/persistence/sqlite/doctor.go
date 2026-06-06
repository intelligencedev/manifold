package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

type DoctorResult struct {
	OK              bool      `json:"ok"`
	Path            string    `json:"path"`
	BusyTimeoutMs   int       `json:"busyTimeoutMs"`
	MaxOpenConns    int       `json:"maxOpenConns"`
	WALRequested    bool      `json:"walRequested"`
	JournalMode     string    `json:"journalMode"`
	FTS5            bool      `json:"fts5"`
	Vec1Info        bool      `json:"vec1Info"`
	Vec1InfoText    string    `json:"vec1InfoText,omitempty"`
	TempVectorQuery bool      `json:"tempVectorQuery"`
	StartedAt       time.Time `json:"startedAt"`
	CompletedAt     time.Time `json:"completedAt"`
	Error           string    `json:"error,omitempty"`
}

func Doctor(ctx context.Context, cfg Config) (result DoctorResult) {
	result.StartedAt = time.Now().UTC()
	defer func() {
		result.CompletedAt = time.Now().UTC()
		result.OK = result.Error == "" && result.FTS5 && result.Vec1Info && result.TempVectorQuery
	}()

	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Path = normalized.Path
	result.BusyTimeoutMs = normalized.BusyTimeoutMs
	result.MaxOpenConns = normalized.MaxOpenConns
	result.WALRequested = normalized.WAL

	db, err := Open(ctx, normalized)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() { _ = db.Close() }()

	if journalMode, err := queryJournalMode(ctx, db); err == nil {
		result.JournalMode = journalMode
	} else {
		result.Error = fmt.Sprintf("check sqlite journal mode: %v", err)
		return result
	}
	if err := probeFTS5(ctx, db); err != nil {
		result.Error = err.Error()
		return result
	}
	result.FTS5 = true
	vecInfo, err := queryVec1Info(ctx, db)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Vec1Info = true
	result.Vec1InfoText = vecInfo
	if err := probeTempVectorQuery(ctx, db); err != nil {
		result.Error = err.Error()
		return result
	}
	result.TempVectorQuery = true
	return result
}

func queryJournalMode(ctx context.Context, db *sql.DB) (string, error) {
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		return "", err
	}
	return journalMode, nil
}

func probeFTS5(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS temp.manifold_fts5_doctor USING fts5(value)`); err != nil {
		return fmt.Errorf("verify sqlite FTS5: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS temp.manifold_fts5_doctor`); err != nil {
		return fmt.Errorf("cleanup sqlite FTS5 check: %w", err)
	}
	return nil
}

func queryVec1Info(ctx context.Context, db *sql.DB) (string, error) {
	var info string
	if err := db.QueryRowContext(ctx, `SELECT vec1_info()`).Scan(&info); err != nil {
		return "", fmt.Errorf("verify sqlite Vec1 info: %w", err)
	}
	return info, nil
}

func probeTempVectorQuery(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS temp.manifold_vec1_doctor USING vec1(vector)`); err != nil {
		return fmt.Errorf("create temp Vec1 table: %w", err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS temp.manifold_vec1_doctor`) }()

	query := vectorDoctorBlob([]float32{1, 0})
	if _, err := db.ExecContext(ctx, `
INSERT INTO temp.manifold_vec1_doctor(rowid, vector)
VALUES(1, ?), (2, ?)
`, query, vectorDoctorBlob([]float32{0, 1})); err != nil {
		return fmt.Errorf("insert temp Vec1 vectors: %w", err)
	}
	var rowid int64
	err := db.QueryRowContext(ctx, `
WITH candidates(rowid, vector) AS (
	SELECT rowid, vector
	FROM temp.manifold_vec1_doctor(?, '{K:1, streaming:1}')
	LIMIT 1
)
SELECT rowid
FROM candidates
ORDER BY vec1_cos_distance(?, vector)
LIMIT 1
`, query, query).Scan(&rowid)
	if err != nil {
		return fmt.Errorf("query temp Vec1 vector: %w", err)
	}
	if rowid != 1 {
		return fmt.Errorf("query temp Vec1 vector: expected rowid 1, got %d", rowid)
	}
	return nil
}

func vectorDoctorBlob(values []float32) []byte {
	out := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(value))
	}
	return out
}
