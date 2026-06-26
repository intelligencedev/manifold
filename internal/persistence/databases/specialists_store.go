package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"manifold/internal/persistence"
	"manifold/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewSpecialistsStore returns a Postgres-backed store if a pool is provided, otherwise an in-memory store.
func NewSpecialistsStore(pool *pgxpool.Pool) persistence.SpecialistsStore {
	if pool == nil {
		return &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	}
	return &pgSpecStore{pool: pool}
}

func NewSpecialistsStoreWithCodec(pool *pgxpool.Pool, codec secrets.Codec) persistence.SpecialistsStore {
	if pool == nil {
		return &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	}
	return &pgSpecStore{pool: pool, codec: codec}
}

type sqliteSpecStore struct {
	db       *sql.DB
	codec    secrets.Codec
	initOnce sync.Once
	initErr  error
}

func NewSQLiteSpecialistsStore(db *sql.DB) persistence.SpecialistsStore {
	return &sqliteSpecStore{db: db}
}

func NewSQLiteSpecialistsStoreWithCodec(db *sql.DB, codec secrets.Codec) persistence.SpecialistsStore {
	return &sqliteSpecStore{db: db, codec: codec}
}

func (s *sqliteSpecStore) ensureCodec() (secrets.Codec, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return nil, err
	}
	s.codec = codec
	return codec, nil
}

func (s *sqliteSpecStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite specialists store requires db")
	}
	codec, err := s.ensureCodec()
	if err != nil {
		return err
	}
	s.initOnce.Do(func() {
		if _, err := s.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS specialists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	base_url TEXT NOT NULL DEFAULT '',
	api_key TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	summary_context_window_tokens INTEGER NOT NULL DEFAULT 0,
	enable_tools INTEGER NOT NULL DEFAULT 0,
	request_info_enabled INTEGER DEFAULT NULL,
	image_generation INTEGER NOT NULL DEFAULT 0,
	video_generation INTEGER NOT NULL DEFAULT 0,
	auto_discover INTEGER DEFAULT NULL,
	paused INTEGER NOT NULL DEFAULT 0,
	allow_tools TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(allow_tools)),
	reasoning_effort TEXT NOT NULL DEFAULT '',
	system TEXT NOT NULL DEFAULT '',
	extra_headers TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(extra_headers)),
	extra_params TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(extra_params)),
	harness TEXT DEFAULT NULL,
	provider TEXT NOT NULL DEFAULT '',
	UNIQUE(user_id, name)
	);
	CREATE INDEX IF NOT EXISTS specialists_user_name_idx ON specialists(user_id, name);
	`); err != nil {
			s.initErr = err
			return
		}
		if err := ensureSQLiteColumn(ctx, s.db, "specialists", "video_generation", "INTEGER NOT NULL DEFAULT 0"); err != nil {
			s.initErr = err
			return
		}
		s.initErr = backfillSQLiteSpecialistSecrets(ctx, s.db, codec)
	})
	return s.initErr
}

func (s *sqliteSpecStore) List(ctx context.Context, userID int64) ([]persistence.Specialist, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider FROM specialists WHERE user_id=? ORDER BY LOWER(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []persistence.Specialist{}
	for rows.Next() {
		sp, err := scanSQLiteSpecialist(rows)
		if err != nil {
			return nil, err
		}
		sp, err = decryptSpecialistFromStore(s.codec, sp)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *sqliteSpecStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.Specialist{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider FROM specialists WHERE user_id=? AND name=?`, userID, name)
	sp, err := scanSQLiteSpecialist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persistence.Specialist{}, false, nil
	}
	if err != nil {
		return persistence.Specialist{}, false, err
	}
	sp, err = decryptSpecialistFromStore(s.codec, sp)
	if err != nil {
		return persistence.Specialist{}, false, err
	}
	return sp, true, nil
}

func (s *sqliteSpecStore) Upsert(ctx context.Context, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	if err := s.Init(ctx); err != nil {
		return persistence.Specialist{}, err
	}
	toStore, err := encryptSpecialistForStore(s.codec, userID, sp)
	if err != nil {
		return persistence.Specialist{}, err
	}
	allow, _ := json.Marshal(sp.AllowTools)
	headers, _ := json.Marshal(toStore.ExtraHeaders)
	params, _ := json.Marshal(sp.ExtraParams)
	harness := encodeSpecialistHarness(sp.Harness)
	row := s.db.QueryRowContext(ctx, `
INSERT INTO specialists(user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(user_id, name) DO UPDATE SET
	description=excluded.description,
	base_url=excluded.base_url,
	api_key=CASE
		WHEN NULLIF(TRIM(excluded.api_key), '') IS NULL THEN specialists.api_key
		ELSE excluded.api_key
	END,
	model=excluded.model,
	summary_context_window_tokens=excluded.summary_context_window_tokens,
	enable_tools=excluded.enable_tools,
	request_info_enabled=excluded.request_info_enabled,
	image_generation=excluded.image_generation,
	video_generation=excluded.video_generation,
	auto_discover=excluded.auto_discover,
	paused=excluded.paused,
	allow_tools=excluded.allow_tools,
	reasoning_effort=excluded.reasoning_effort,
	system=excluded.system,
	extra_headers=excluded.extra_headers,
		extra_params=excluded.extra_params,
		harness=excluded.harness,
		provider=excluded.provider
	RETURNING id, api_key`, userID, sp.Name, sp.Description, sp.BaseURL, toStore.APIKey, sp.Model, sp.SummaryContextWindowTokens, sp.EnableTools, nullableBool(sp.RequestInfoEnabled), sp.ImageGeneration, sp.VideoGeneration, nullableBool(sp.AutoDiscover), sp.Paused, string(allow), sp.ReasoningEffort, sp.System, string(headers), string(params), nullableJSON(harness), sp.Provider)
	if err := row.Scan(&sp.ID, &sp.APIKey); err != nil {
		return persistence.Specialist{}, err
	}
	sp.UserID = userID
	return decryptSpecialistFromStore(s.codec, sp)
}

func boolPtr(value bool) *bool {
	return &value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func (s *sqliteSpecStore) Delete(ctx context.Context, userID int64, name string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM specialists WHERE user_id=? AND name=?`, userID, name)
	return err
}

func scanSQLiteSpecialist(row interface{ Scan(dest ...any) error }) (persistence.Specialist, error) {
	var sp persistence.Specialist
	var allow, headers, params string
	var harness sql.NullString
	var requestInfo, autoDiscover sql.NullBool
	if err := row.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.Description, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.SummaryContextWindowTokens, &sp.EnableTools, &requestInfo, &sp.ImageGeneration, &sp.VideoGeneration, &autoDiscover, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params, &harness, &sp.Provider); err != nil {
		return persistence.Specialist{}, err
	}
	_ = json.Unmarshal([]byte(allow), &sp.AllowTools)
	_ = json.Unmarshal([]byte(headers), &sp.ExtraHeaders)
	_ = json.Unmarshal([]byte(params), &sp.ExtraParams)
	if requestInfo.Valid {
		sp.RequestInfoEnabled = boolPtr(requestInfo.Bool)
	}
	if autoDiscover.Valid {
		sp.AutoDiscover = boolPtr(autoDiscover.Bool)
	}
	if harness.Valid {
		sp.Harness = decodeSpecialistHarness([]byte(harness.String))
	}
	return sp, nil
}

type memSpecStore struct {
	m map[int64]map[string]persistence.Specialist
}

func (s *memSpecStore) Init(ctx context.Context) error { return nil }

func (s *memSpecStore) List(ctx context.Context, userID int64) ([]persistence.Specialist, error) {
	userMap := s.m[userID]
	if userMap == nil {
		return []persistence.Specialist{}, nil
	}
	out := make([]persistence.Specialist, 0, len(userMap))
	for _, v := range userMap {
		out = append(out, v)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *memSpecStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	if userMap := s.m[userID]; userMap != nil {
		v, ok := userMap[name]
		return v, ok, nil
	}
	return persistence.Specialist{}, false, nil
}

func (s *memSpecStore) Upsert(ctx context.Context, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	if s.m[userID] == nil {
		s.m[userID] = map[string]persistence.Specialist{}
	}
	if existing, ok := s.m[userID][sp.Name]; ok {
		if strings.TrimSpace(sp.APIKey) == "" {
			sp.APIKey = existing.APIKey
		}
	}
	sp.UserID = userID
	s.m[userID][sp.Name] = sp
	return sp, nil
}

func (s *memSpecStore) Delete(ctx context.Context, userID int64, name string) error {
	if s.m[userID] == nil {
		return nil
	}
	delete(s.m[userID], name)
	return nil
}

type pgSpecStore struct {
	pool  *pgxpool.Pool
	codec secrets.Codec
}

func (s *pgSpecStore) Init(ctx context.Context) error {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return err
	}
	s.codec = codec
	_, err = s.pool.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS specialists (
	id SERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	base_url TEXT NOT NULL DEFAULT '',
	api_key TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	summary_context_window_tokens INT NOT NULL DEFAULT 0,
	enable_tools BOOLEAN NOT NULL DEFAULT false,
	request_info_enabled BOOLEAN DEFAULT NULL,
	image_generation BOOLEAN NOT NULL DEFAULT false,
	video_generation BOOLEAN NOT NULL DEFAULT false,
	auto_discover BOOLEAN DEFAULT NULL,
	paused BOOLEAN NOT NULL DEFAULT false,
	allow_tools JSONB NOT NULL DEFAULT '[]',
	reasoning_effort TEXT NOT NULL DEFAULT '',
	system TEXT NOT NULL DEFAULT '',
	extra_headers JSONB NOT NULL DEFAULT '{}',
	extra_params JSONB NOT NULL DEFAULT '{}',
	harness JSONB DEFAULT NULL,
	provider TEXT NOT NULL DEFAULT ''
);

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '';

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS summary_context_window_tokens INT NOT NULL DEFAULT 0;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS auto_discover BOOLEAN DEFAULT NULL;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS request_info_enabled BOOLEAN DEFAULT NULL;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS image_generation BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS video_generation BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS harness JSONB DEFAULT NULL;

ALTER TABLE specialists
	DROP CONSTRAINT IF EXISTS specialists_name_key;

	CREATE UNIQUE INDEX IF NOT EXISTS specialists_user_name_idx ON specialists(user_id, name);
	`)
	if err != nil {
		return err
	}
	return backfillPostgresSpecialistSecrets(ctx, s.pool, codec)
}

func (s *pgSpecStore) List(ctx context.Context, userID int64) ([]persistence.Specialist, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return nil, err
	}
	s.codec = codec
	rows, err := s.pool.Query(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider FROM specialists WHERE user_id=$1 ORDER BY LOWER(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.Specialist
	for rows.Next() {
		var sp persistence.Specialist
		var allow, headers, params, harness []byte
		if err := rows.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.Description, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.SummaryContextWindowTokens, &sp.EnableTools, &sp.RequestInfoEnabled, &sp.ImageGeneration, &sp.VideoGeneration, &sp.AutoDiscover, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params, &harness, &sp.Provider); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(allow, &sp.AllowTools)
		_ = json.Unmarshal(headers, &sp.ExtraHeaders)
		_ = json.Unmarshal(params, &sp.ExtraParams)
		sp.Harness = decodeSpecialistHarness(harness)
		sp, err = decryptSpecialistFromStore(codec, sp)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *pgSpecStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return persistence.Specialist{}, false, err
	}
	s.codec = codec
	row := s.pool.QueryRow(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider FROM specialists WHERE user_id=$1 AND name=$2`, userID, name)
	var sp persistence.Specialist
	var allow, headers, params, harness []byte
	if err := row.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.Description, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.SummaryContextWindowTokens, &sp.EnableTools, &sp.RequestInfoEnabled, &sp.ImageGeneration, &sp.VideoGeneration, &sp.AutoDiscover, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params, &harness, &sp.Provider); err != nil {
		return persistence.Specialist{}, false, nil
	}
	_ = json.Unmarshal(allow, &sp.AllowTools)
	_ = json.Unmarshal(headers, &sp.ExtraHeaders)
	_ = json.Unmarshal(params, &sp.ExtraParams)
	sp.Harness = decodeSpecialistHarness(harness)
	sp, err = decryptSpecialistFromStore(codec, sp)
	if err != nil {
		return persistence.Specialist{}, false, err
	}
	return sp, true, nil
}

func (s *pgSpecStore) Upsert(ctx context.Context, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	codec, err := databaseSecretCodec(s.codec)
	if err != nil {
		return persistence.Specialist{}, err
	}
	s.codec = codec
	toStore, err := encryptSpecialistForStore(s.codec, userID, sp)
	if err != nil {
		return persistence.Specialist{}, err
	}
	allow, _ := json.Marshal(sp.AllowTools)
	headers, _ := json.Marshal(toStore.ExtraHeaders)
	params, _ := json.Marshal(sp.ExtraParams)
	harness := encodeSpecialistHarness(sp.Harness)
	row := s.pool.QueryRow(ctx, `
INSERT INTO specialists(user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,request_info_enabled,image_generation,video_generation,auto_discover,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,harness,provider)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (user_id, name) DO UPDATE SET description=EXCLUDED.description, base_url=EXCLUDED.base_url,
		api_key=CASE
			WHEN NULLIF(BTRIM(EXCLUDED.api_key), '') IS NULL THEN specialists.api_key
			ELSE EXCLUDED.api_key
		END,
		model=EXCLUDED.model,
		summary_context_window_tokens=EXCLUDED.summary_context_window_tokens, enable_tools=EXCLUDED.enable_tools, request_info_enabled=EXCLUDED.request_info_enabled, image_generation=EXCLUDED.image_generation, video_generation=EXCLUDED.video_generation, auto_discover=EXCLUDED.auto_discover, paused=EXCLUDED.paused, allow_tools=EXCLUDED.allow_tools,
		reasoning_effort=EXCLUDED.reasoning_effort, system=EXCLUDED.system, extra_headers=EXCLUDED.extra_headers, extra_params=EXCLUDED.extra_params, harness=EXCLUDED.harness, provider=EXCLUDED.provider
	RETURNING id, api_key;`, userID, sp.Name, sp.Description, sp.BaseURL, toStore.APIKey, sp.Model, sp.SummaryContextWindowTokens, sp.EnableTools, sp.RequestInfoEnabled, sp.ImageGeneration, sp.VideoGeneration, sp.AutoDiscover, sp.Paused, allow, sp.ReasoningEffort, sp.System, headers, params, harness, sp.Provider)
	if err := row.Scan(&sp.ID, &sp.APIKey); err != nil {
		return persistence.Specialist{}, err
	}
	sp.UserID = userID
	return decryptSpecialistFromStore(s.codec, sp)
}

func decodeSpecialistHarness(data []byte) *persistence.SpecialistHarness {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var cfg persistence.SpecialistHarness
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func encodeSpecialistHarness(cfg *persistence.SpecialistHarness) []byte {
	if cfg == nil {
		return nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	return data
}

func (s *pgSpecStore) Delete(ctx context.Context, userID int64, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM specialists WHERE user_id=$1 AND name=$2`, userID, name)
	return err
}

type specialistSecretRecord struct {
	ID           int64
	UserID       int64
	Name         string
	APIKey       string
	ExtraHeaders map[string]string
}

func backfillSQLiteSpecialistSecrets(ctx context.Context, db *sql.DB, codec secrets.Codec) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT id, user_id, name, api_key, extra_headers FROM specialists`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	records, err := scanSpecialistSecretRecords(rows)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !specialistRecordNeedsBackfill(codec, record) {
			continue
		}
		encrypted, err := encryptSpecialistForStore(codec, record.UserID, persistence.Specialist{
			UserID:       record.UserID,
			Name:         record.Name,
			APIKey:       record.APIKey,
			ExtraHeaders: record.ExtraHeaders,
		})
		if err != nil {
			return fmt.Errorf("backfill specialist secrets id=%d: %w", record.ID, err)
		}
		headers, _ := json.Marshal(encrypted.ExtraHeaders)
		if _, err := tx.ExecContext(ctx, `UPDATE specialists SET api_key = ?, extra_headers = ? WHERE id = ?`, encrypted.APIKey, string(headers), record.ID); err != nil {
			return fmt.Errorf("update specialist secret backfill id=%d: %w", record.ID, err)
		}
	}
	return tx.Commit()
}

func backfillPostgresSpecialistSecrets(ctx context.Context, pool *pgxpool.Pool, codec secrets.Codec) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT id, user_id, name, api_key, extra_headers FROM specialists`)
	if err != nil {
		return err
	}
	defer rows.Close()
	records, err := scanSpecialistSecretRecords(rows)
	if err != nil {
		return err
	}

	for _, record := range records {
		if !specialistRecordNeedsBackfill(codec, record) {
			continue
		}
		encrypted, err := encryptSpecialistForStore(codec, record.UserID, persistence.Specialist{
			UserID:       record.UserID,
			Name:         record.Name,
			APIKey:       record.APIKey,
			ExtraHeaders: record.ExtraHeaders,
		})
		if err != nil {
			return fmt.Errorf("backfill specialist secrets id=%d: %w", record.ID, err)
		}
		headers, _ := json.Marshal(encrypted.ExtraHeaders)
		if _, err := tx.Exec(ctx, `UPDATE specialists SET api_key = $1, extra_headers = $2 WHERE id = $3`, encrypted.APIKey, headers, record.ID); err != nil {
			return fmt.Errorf("update specialist secret backfill id=%d: %w", record.ID, err)
		}
	}
	return tx.Commit(ctx)
}

func scanSpecialistSecretRecords(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]specialistSecretRecord, error) {
	var records []specialistSecretRecord
	for rows.Next() {
		var record specialistSecretRecord
		var headers []byte
		if err := rows.Scan(&record.ID, &record.UserID, &record.Name, &record.APIKey, &headers); err != nil {
			return nil, err
		}
		if len(headers) > 0 {
			if err := json.Unmarshal(headers, &record.ExtraHeaders); err != nil {
				return nil, fmt.Errorf("specialist extra_headers id=%d: %w", record.ID, err)
			}
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func specialistRecordNeedsBackfill(codec secrets.Codec, record specialistSecretRecord) bool {
	if record.APIKey != "" && !codec.IsSealed(record.APIKey) {
		return true
	}
	for _, value := range record.ExtraHeaders {
		if value != "" && !codec.IsSealed(value) {
			return true
		}
	}
	return false
}
