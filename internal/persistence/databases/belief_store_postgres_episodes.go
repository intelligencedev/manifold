package databases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/agent/memory/belief"
)

func (s *pgBeliefStore) UpsertEpisode(ctx context.Context, episode belief.Episode) (belief.Episode, error) {
	episode.TenantID = normalizeTenantID(episode.TenantID)
	if strings.TrimSpace(episode.ID) == "" {
		episode.ID = uuid.NewString()
	}
	if episode.StartedAt.IsZero() {
		episode.StartedAt = time.Now().UTC()
	}
	if episode.Outcome == "" {
		episode.Outcome = "unknown"
	}
	if episode.OutcomeSignal == "" {
		episode.OutcomeSignal = "implicit"
	}
	metadata, err := marshalJSONMap(episode.Metadata)
	if err != nil {
		return belief.Episode{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_episodes (
    id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    scope_id = EXCLUDED.scope_id,
    project_id = EXCLUDED.project_id,
    objective_id = EXCLUDED.objective_id,
    session_id = EXCLUDED.session_id,
    agent_role = EXCLUDED.agent_role,
    user_id = EXCLUDED.user_id,
    started_at = EXCLUDED.started_at,
    ended_at = EXCLUDED.ended_at,
    outcome = EXCLUDED.outcome,
    outcome_signal = EXCLUDED.outcome_signal,
    evolving_entry_id = EXCLUDED.evolving_entry_id,
    metadata = EXCLUDED.metadata
RETURNING id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
`, episode.ID, episode.TenantID, episode.ScopeID, episode.ProjectID, episode.ObjectiveID, episode.SessionID, episode.AgentRole, episode.UserID, episode.StartedAt, episode.EndedAt, episode.Outcome, episode.OutcomeSignal, nilIfEmpty(episode.EvolvingEntryID), metadata)
	return scanBeliefEpisode(row)
}

func (s *pgBeliefStore) GetEpisode(ctx context.Context, tenantID int64, id string) (belief.Episode, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
FROM belief_episodes
WHERE tenant_id = $1 AND id = $2
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	episode, err := scanBeliefEpisode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.Episode{}, false, nil
	}
	if err != nil {
		return belief.Episode{}, false, err
	}
	return episode, true, nil
}
