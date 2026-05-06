package databases

import (
	"encoding/json"
	"strings"
	"time"

	"manifold/internal/agent/belief"
)

type scanner interface {
	Scan(dest ...any) error
}

func scanBeliefScope(row scanner) (belief.Scope, error) {
	var scope belief.Scope
	var kind string
	var parentID *string
	var metadata []byte
	if err := row.Scan(&scope.ID, &scope.TenantID, &kind, &parentID, &scope.Path, &scope.Label, &metadata, &scope.CreatedAt, &scope.UpdatedAt); err != nil {
		return belief.Scope{}, err
	}
	scope.Kind = belief.ScopeKind(kind)
	if parentID != nil {
		scope.ParentID = *parentID
	}
	scope.Metadata = unmarshalJSONMap(metadata)
	return scope, nil
}

func scanBeliefEpisode(row scanner) (belief.Episode, error) {
	var episode belief.Episode
	var endedAt *time.Time
	var evolvingEntryID *string
	var metadata []byte
	if err := row.Scan(&episode.ID, &episode.TenantID, &episode.ScopeID, &episode.ProjectID, &episode.ObjectiveID, &episode.SessionID, &episode.AgentRole, &episode.UserID, &episode.StartedAt, &endedAt, &episode.Outcome, &episode.OutcomeSignal, &evolvingEntryID, &metadata); err != nil {
		return belief.Episode{}, err
	}
	episode.EndedAt = endedAt
	if evolvingEntryID != nil {
		episode.EvolvingEntryID = *evolvingEntryID
	}
	episode.Metadata = unmarshalJSONMap(metadata)
	return episode, nil
}

func scanBelief(row scanner) (belief.Belief, error) {
	var item belief.Belief
	var status string
	var lastObserved *time.Time
	var expiresAt *time.Time
	var metadata []byte
	if err := row.Scan(&item.ID, &item.TenantID, &item.ScopeID, &item.Statement, &item.StatementHash, &item.Confidence, &item.EvidenceFor, &item.EvidenceAgainst, &status, &lastObserved, &expiresAt, &metadata, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return belief.Belief{}, err
	}
	item.Status = belief.BeliefStatus(status)
	item.LastObserved = lastObserved
	item.ExpiresAt = expiresAt
	item.Metadata = unmarshalJSONMap(metadata)
	return item, nil
}

func scanBeliefWithScore(row scanner) (belief.Belief, float64, error) {
	var item belief.Belief
	var status string
	var lastObserved *time.Time
	var expiresAt *time.Time
	var metadata []byte
	var score float64
	if err := row.Scan(&item.ID, &item.TenantID, &item.ScopeID, &item.Statement, &item.StatementHash, &item.Confidence, &item.EvidenceFor, &item.EvidenceAgainst, &status, &lastObserved, &expiresAt, &metadata, &item.CreatedAt, &item.UpdatedAt, &score); err != nil {
		return belief.Belief{}, 0, err
	}
	item.Status = belief.BeliefStatus(status)
	item.LastObserved = lastObserved
	item.ExpiresAt = expiresAt
	item.Metadata = unmarshalJSONMap(metadata)
	return item, score, nil
}

func scanBeliefEvidence(row scanner) (belief.Evidence, error) {
	var evidence belief.Evidence
	var episodeID *string
	var sourceKind string
	var polarity string
	var metadata []byte
	if err := row.Scan(&evidence.ID, &evidence.TenantID, &evidence.BeliefID, &episodeID, &sourceKind, &evidence.SourceID, &polarity, &evidence.Weight, &evidence.Note, &metadata, &evidence.CreatedAt); err != nil {
		return belief.Evidence{}, err
	}
	if episodeID != nil {
		evidence.EpisodeID = *episodeID
	}
	evidence.SourceKind = belief.SourceKind(sourceKind)
	evidence.Polarity = belief.EvidencePolarity(polarity)
	evidence.Metadata = unmarshalJSONMap(metadata)
	return evidence, nil
}

func scanBeliefPromotion(row scanner) (belief.Promotion, error) {
	var promotion belief.Promotion
	var actorUserID *int64
	var metadata []byte
	if err := row.Scan(&promotion.ID, &promotion.TenantID, &promotion.BeliefID, &promotion.FromScope, &promotion.ToScope, &promotion.Reason, &promotion.ConfidenceBefore, &promotion.ConfidenceAfter, &actorUserID, &metadata, &promotion.CreatedAt); err != nil {
		return belief.Promotion{}, err
	}
	promotion.ActorUserID = actorUserID
	promotion.Metadata = unmarshalJSONMap(metadata)
	return promotion, nil
}

func marshalJSONMap(metadata map[string]any) ([]byte, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return json.Marshal(metadata)
}

func unmarshalJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func nilIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func vectorLiteralOrEmpty(vector []float32) string {
	if len(vector) == 0 {
		return ""
	}
	return toVectorLiteral(vector)
}
