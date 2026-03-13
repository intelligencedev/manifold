package databases

import (
	"context"
	"strings"
	"sync"
	"time"

	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewReactiveClaimStore returns a Postgres-backed reactive claim store when a
// pool is provided, otherwise an in-memory implementation.
func NewReactiveClaimStore(pool *pgxpool.Pool) persistence.ReactiveClaimStore {
	if pool == nil {
		return &memReactiveClaimStore{claims: map[string]persistence.ReactiveClaim{}}
	}
	return &pgReactiveClaimStore{pool: pool}
}

type memReactiveClaimStore struct {
	mu     sync.RWMutex
	claims map[string]persistence.ReactiveClaim
}

func (s *memReactiveClaimStore) Init(ctx context.Context) error { return nil }

func (s *memReactiveClaimStore) TryClaim(ctx context.Context, roomID, botID, token, triggerEventID string, leaseUntil time.Time) (bool, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return false, persistence.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	claim, ok := s.claims[roomID]
	if ok && claim.ClaimToken != strings.TrimSpace(token) && claim.ExpiresAt.After(now) {
		return false, nil
	}

	s.claims[roomID] = persistence.ReactiveClaim{
		RoomID:         roomID,
		BotID:          strings.TrimSpace(botID),
		ClaimToken:     strings.TrimSpace(token),
		TriggerEventID: strings.TrimSpace(triggerEventID),
		ClaimedAt:      now,
		ExpiresAt:      leaseUntil.UTC(),
	}
	return true, nil
}

func (s *memReactiveClaimStore) GetActiveClaim(ctx context.Context, roomID string) (persistence.ReactiveClaim, error) {
	roomID = strings.TrimSpace(roomID)
	s.mu.RLock()
	claim, ok := s.claims[roomID]
	s.mu.RUnlock()
	if !ok {
		return persistence.ReactiveClaim{}, persistence.ErrNotFound
	}
	if !claim.ExpiresAt.After(time.Now().UTC()) {
		s.mu.Lock()
		delete(s.claims, roomID)
		s.mu.Unlock()
		return persistence.ReactiveClaim{}, persistence.ErrNotFound
	}
	return claim, nil
}

func (s *memReactiveClaimStore) Release(ctx context.Context, roomID, token string) error {
	roomID = strings.TrimSpace(roomID)
	token = strings.TrimSpace(token)

	s.mu.Lock()
	defer s.mu.Unlock()

	claim, ok := s.claims[roomID]
	if !ok {
		return persistence.ErrNotFound
	}
	if claim.ClaimToken != token {
		return persistence.ErrRevisionConflict
	}
	delete(s.claims, roomID)
	return nil
}

type pgReactiveClaimStore struct {
	pool *pgxpool.Pool
}

func (s *pgReactiveClaimStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS reactive_claims (
    room_id TEXT PRIMARY KEY,
    bot_id TEXT NOT NULL,
    claim_token TEXT NOT NULL,
    trigger_event_id TEXT,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reactive_claims_expires_at ON reactive_claims(expires_at);
`)
	return err
}

func (s *pgReactiveClaimStore) TryClaim(ctx context.Context, roomID, botID, token, triggerEventID string, leaseUntil time.Time) (bool, error) {
	cmd, err := s.pool.Exec(ctx, `
INSERT INTO reactive_claims (room_id, bot_id, claim_token, trigger_event_id, claimed_at, expires_at)
VALUES ($1, $2, $3, NULLIF($4, ''), NOW(), $5)
ON CONFLICT (room_id) DO UPDATE SET
    bot_id = EXCLUDED.bot_id,
    claim_token = EXCLUDED.claim_token,
    trigger_event_id = EXCLUDED.trigger_event_id,
    claimed_at = NOW(),
    expires_at = EXCLUDED.expires_at
WHERE reactive_claims.expires_at <= NOW() OR reactive_claims.claim_token = EXCLUDED.claim_token
`, strings.TrimSpace(roomID), strings.TrimSpace(botID), strings.TrimSpace(token), strings.TrimSpace(triggerEventID), leaseUntil.UTC())
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *pgReactiveClaimStore) GetActiveClaim(ctx context.Context, roomID string) (persistence.ReactiveClaim, error) {
	var claim persistence.ReactiveClaim
	var triggerEventID *string
	err := s.pool.QueryRow(ctx, `
SELECT room_id, bot_id, claim_token, trigger_event_id, claimed_at, expires_at
FROM reactive_claims
WHERE room_id = $1 AND expires_at > NOW()
`, strings.TrimSpace(roomID)).Scan(
		&claim.RoomID,
		&claim.BotID,
		&claim.ClaimToken,
		&triggerEventID,
		&claim.ClaimedAt,
		&claim.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return persistence.ReactiveClaim{}, persistence.ErrNotFound
		}
		return persistence.ReactiveClaim{}, err
	}
	if triggerEventID != nil {
		claim.TriggerEventID = *triggerEventID
	}
	claim.ClaimedAt = claim.ClaimedAt.UTC()
	claim.ExpiresAt = claim.ExpiresAt.UTC()
	return claim, nil
}

func (s *pgReactiveClaimStore) Release(ctx context.Context, roomID, token string) error {
	cmd, err := s.pool.Exec(ctx, `
DELETE FROM reactive_claims
WHERE room_id = $1 AND claim_token = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(token))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		active, getErr := s.GetActiveClaim(ctx, roomID)
		if getErr == persistence.ErrNotFound {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if active.ClaimToken != strings.TrimSpace(token) {
			return persistence.ErrRevisionConflict
		}
	}
	return nil
}
