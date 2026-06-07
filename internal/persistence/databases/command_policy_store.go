package databases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewCommandPolicyStore returns a Postgres-backed store if a pool is provided,
// otherwise an in-memory store.
func NewCommandPolicyStore(pool *pgxpool.Pool) persistence.CommandPolicyStore {
	if pool == nil {
		return &memCommandPolicyStore{
			m:                map[int64]map[string]config.ExecCommandRule{},
			sessionOverrides: map[int64]map[string]persistence.CommandPolicySessionOverride{},
		}
	}
	return &pgCommandPolicyStore{pool: pool}
}

type memCommandPolicyStore struct {
	mu               sync.RWMutex
	m                map[int64]map[string]config.ExecCommandRule
	sessionOverrides map[int64]map[string]persistence.CommandPolicySessionOverride
}

func (s *memCommandPolicyStore) Init(ctx context.Context) error { return nil }

func (s *memCommandPolicyStore) ListRules(ctx context.Context, userID int64) ([]config.ExecCommandRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userRules := s.m[userID]
	if len(userRules) == 0 {
		return []config.ExecCommandRule{}, nil
	}
	out := make([]config.ExecCommandRule, 0, len(userRules))
	for _, rule := range userRules {
		out = append(out, cloneStoredCommandRule(rule))
	}
	sortCommandRules(out)
	return out, nil
}

func (s *memCommandPolicyStore) UpsertRule(ctx context.Context, userID int64, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	rule = prepareStoredCommandRule(rule)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[userID] == nil {
		s.m[userID] = map[string]config.ExecCommandRule{}
	}
	s.m[userID][rule.ID] = cloneStoredCommandRule(rule)
	return cloneStoredCommandRule(rule), nil
}

func (s *memCommandPolicyStore) GetSessionOverride(ctx context.Context, userID int64, sessionID string) (persistence.CommandPolicySessionOverride, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return persistence.CommandPolicySessionOverride{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	userOverrides := s.sessionOverrides[userID]
	if userOverrides == nil {
		return persistence.CommandPolicySessionOverride{}, false, nil
	}
	override, ok := userOverrides[sessionID]
	return override, ok, nil
}

func (s *memCommandPolicyStore) SetSessionAllowAll(ctx context.Context, userID int64, sessionID string, allow bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !allow {
		if s.sessionOverrides[userID] != nil {
			delete(s.sessionOverrides[userID], sessionID)
		}
		return nil
	}
	if s.sessionOverrides[userID] == nil {
		s.sessionOverrides[userID] = map[string]persistence.CommandPolicySessionOverride{}
	}
	now := time.Now().UTC()
	override := s.sessionOverrides[userID][sessionID]
	if override.CreatedAt.IsZero() {
		override.CreatedAt = now
	}
	override.UserID = userID
	override.SessionID = sessionID
	override.AllowAllCommands = true
	override.UpdatedAt = now
	s.sessionOverrides[userID][sessionID] = override
	return nil
}

func (s *memCommandPolicyStore) DeleteSessionOverride(ctx context.Context, userID int64, sessionID string) error {
	return s.SetSessionAllowAll(ctx, userID, sessionID, false)
}

type pgCommandPolicyStore struct {
	pool *pgxpool.Pool
}

func (s *pgCommandPolicyStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS command_policy_rules (
	user_id BIGINT NOT NULL DEFAULT 0,
	id TEXT NOT NULL,
	decision TEXT NOT NULL,
	pattern JSONB NOT NULL DEFAULT '[]',
	contexts JSONB NOT NULL DEFAULT '[]',
	justification TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	PRIMARY KEY(user_id, id)
);
CREATE TABLE IF NOT EXISTS command_policy_session_overrides (
	user_id BIGINT NOT NULL DEFAULT 0,
	session_id TEXT NOT NULL,
	allow_all_commands BOOLEAN NOT NULL DEFAULT false,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
	PRIMARY KEY(user_id, session_id)
);
`)
	return err
}

func (s *pgCommandPolicyStore) ListRules(ctx context.Context, userID int64) ([]config.ExecCommandRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, decision, pattern, contexts, justification
		FROM command_policy_rules
		WHERE user_id = $1
		ORDER BY id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []config.ExecCommandRule
	for rows.Next() {
		var rule config.ExecCommandRule
		var pattern, contexts []byte
		if err := rows.Scan(&rule.ID, &rule.Decision, &pattern, &contexts, &rule.Justification); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(pattern, &rule.Pattern)
		_ = json.Unmarshal(contexts, &rule.Contexts)
		out = append(out, cloneStoredCommandRule(rule))
	}
	return out, rows.Err()
}

func (s *pgCommandPolicyStore) UpsertRule(ctx context.Context, userID int64, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	rule = prepareStoredCommandRule(rule)
	pattern, _ := json.Marshal(rule.Pattern)
	contexts, _ := json.Marshal(rule.Contexts)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO command_policy_rules (user_id, id, decision, pattern, contexts, justification, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (user_id, id) DO UPDATE SET
			decision = EXCLUDED.decision,
			pattern = EXCLUDED.pattern,
			contexts = EXCLUDED.contexts,
			justification = EXCLUDED.justification,
			updated_at = EXCLUDED.updated_at
	`, userID, rule.ID, rule.Decision, pattern, contexts, rule.Justification)
	return cloneStoredCommandRule(rule), err
}

func (s *pgCommandPolicyStore) GetSessionOverride(ctx context.Context, userID int64, sessionID string) (persistence.CommandPolicySessionOverride, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return persistence.CommandPolicySessionOverride{}, false, nil
	}
	var override persistence.CommandPolicySessionOverride
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, session_id, allow_all_commands, created_at, updated_at
		FROM command_policy_session_overrides
		WHERE user_id = $1 AND session_id = $2
	`, userID, sessionID).Scan(&override.UserID, &override.SessionID, &override.AllowAllCommands, &override.CreatedAt, &override.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.CommandPolicySessionOverride{}, false, nil
	}
	if err != nil {
		return persistence.CommandPolicySessionOverride{}, false, err
	}
	return override, true, nil
}

func (s *pgCommandPolicyStore) SetSessionAllowAll(ctx context.Context, userID int64, sessionID string, allow bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id required")
	}
	if !allow {
		return s.DeleteSessionOverride(ctx, userID, sessionID)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO command_policy_session_overrides (user_id, session_id, allow_all_commands, created_at, updated_at)
		VALUES ($1, $2, true, NOW(), NOW())
		ON CONFLICT (user_id, session_id) DO UPDATE SET
			allow_all_commands = true,
			updated_at = EXCLUDED.updated_at
	`, userID, sessionID)
	return err
}

func (s *pgCommandPolicyStore) DeleteSessionOverride(ctx context.Context, userID int64, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM command_policy_session_overrides
		WHERE user_id = $1 AND session_id = $2
	`, userID, sessionID)
	return err
}

func prepareStoredCommandRule(rule config.ExecCommandRule) config.ExecCommandRule {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Decision = strings.ToLower(strings.TrimSpace(rule.Decision))
	rule.Justification = strings.TrimSpace(rule.Justification)
	rule.Pattern = trimStoredStrings(rule.Pattern, false)
	rule.Contexts = trimStoredStrings(rule.Contexts, true)
	if rule.ID == "" {
		rule.ID = generatedCommandRuleID(rule)
	}
	return rule
}

func generatedCommandRuleID(rule config.ExecCommandRule) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		rule.Decision,
		strings.Join(rule.Pattern, "\x00"),
		strings.Join(rule.Contexts, "\x00"),
	}, "\x1f")))
	return "rule:" + hex.EncodeToString(h[:])[:16]
}

func trimStoredStrings(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if lower {
			value = strings.ToLower(value)
		}
		out = append(out, value)
	}
	return out
}

func cloneStoredCommandRule(rule config.ExecCommandRule) config.ExecCommandRule {
	out := rule
	out.Pattern = append([]string(nil), rule.Pattern...)
	out.Contexts = append([]string(nil), rule.Contexts...)
	return out
}

func sortCommandRules(rules []config.ExecCommandRule) {
	for i := 1; i < len(rules); i++ {
		for j := i; j > 0 && rules[j].ID < rules[j-1].ID; j-- {
			rules[j], rules[j-1] = rules[j-1], rules[j]
		}
	}
}
