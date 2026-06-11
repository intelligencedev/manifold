package decision

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/llm"
)

// Retriever deterministically retrieves in-force decisions for prompt
// construction. Implementations must use plain store reads only: no LLM or
// tool calls are permitted on this path.
type Retriever interface {
	Retrieve(ctx context.Context, request RetrievalRequest) ([]SearchResult, error)
}

// RetrievalRequest describes the session context used for the decision scope walk.
type RetrievalRequest struct {
	TenantID    int64  `json:"tenantId"`
	UserID      int64  `json:"userId"`
	ProjectID   string `json:"projectId"`
	ObjectiveID string `json:"objectiveId"`
	SessionID   string `json:"sessionId"`
	Role        string `json:"role"`
	Query       string `json:"query"`
	Limit       int    `json:"limit"`
}

// ScopeLookup resolves belief scopes so distilled decisions recorded against
// belief scope UUIDs participate in the deterministic scope walk.
type ScopeLookup interface {
	GetScope(ctx context.Context, tenantID int64, kind belief.ScopeKind, path string) (belief.Scope, bool, error)
}

// StoreRetriever performs a deterministic scope walk
// (session -> objective -> project -> user -> role -> org) over the decision
// store, then adds a tenant-wide lexical pass for the current query. Ranking
// blends store score, scope proximity, confidence, and recency.
type StoreRetriever struct {
	Store  Store
	Scopes ScopeLookup
}

// NewStoreRetriever builds the default decision retriever. scopes may be nil;
// path-style scope IDs are still walked.
func NewStoreRetriever(store Store, scopes ScopeLookup) Retriever {
	if store == nil {
		return NoopRetriever{}
	}
	return StoreRetriever{Store: store, Scopes: scopes}
}

// NoopRetriever is used while decision retrieval is disabled.
type NoopRetriever struct{}

// Retrieve always returns no results.
func (NoopRetriever) Retrieve(context.Context, RetrievalRequest) ([]SearchResult, error) {
	return nil, nil
}

// tenantWideProximity ranks tenant-wide lexical matches below every scope-walk level.
const tenantWideProximity = 6

// Retrieve returns ranked active decisions for the request context.
func (r StoreRetriever) Retrieve(ctx context.Context, request RetrievalRequest) ([]SearchResult, error) {
	if r.Store == nil {
		return nil, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 5
	}
	perScopeLimit := limit * 2
	if perScopeLimit < 10 {
		perScopeLimit = 10
	}
	results := make([]SearchResult, 0, limit)
	seen := map[string]int{}
	merge := func(found []SearchResult, proximity int, reason string) {
		for _, result := range found {
			result.Score = rankDecisionResult(result, request.Query, proximity)
			result.Reason = reason
			key := result.Decision.ID
			if key == "" {
				key = result.Decision.StatementHash
			}
			if idx, ok := seen[key]; ok {
				if result.Score > results[idx].Score {
					results[idx] = result
				}
				continue
			}
			seen[key] = len(results)
			results = append(results, result)
		}
	}
	for _, candidate := range decisionScopeWalk(ctx, r.Scopes, request) {
		found, err := r.Store.SearchDecisions(ctx, SearchQuery{
			TenantID:      request.TenantID,
			ScopeIDs:      candidate.scopeIDs,
			ScopePrefixes: candidate.scopePrefixes,
			Statuses:      []DecisionStatus{DecisionStatusActive},
			Limit:         perScopeLimit,
		})
		if err != nil {
			return nil, err
		}
		merge(found, candidate.proximity, fmt.Sprintf("scope=%s proximity=%d", candidate.kind, candidate.proximity))
	}
	if strings.TrimSpace(request.Query) != "" {
		found, err := r.Store.SearchDecisions(ctx, SearchQuery{
			TenantID: request.TenantID,
			Query:    request.Query,
			Statuses: []DecisionStatus{DecisionStatusActive},
			Limit:    perScopeLimit,
		})
		if err != nil {
			return nil, err
		}
		merge(found, tenantWideProximity, "tenant-wide lexical match")
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Decision.UpdatedAt.After(results[j].Decision.UpdatedAt)
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

type decisionScopeCandidate struct {
	kind          belief.ScopeKind
	scopeIDs      []string
	scopePrefixes []string
	proximity     int
}

// decisionScopeWalk mirrors the belief scope walk and resolves both path-style
// scope IDs (used by decision_record) and belief scope UUIDs (used by the
// distiller via episode scopes).
func decisionScopeWalk(ctx context.Context, scopes ScopeLookup, request RetrievalRequest) []decisionScopeCandidate {
	projectID := belief.NormalizeProjectID(request.ProjectID)
	objectiveID := strings.TrimSpace(request.ObjectiveID)
	sessionID := strings.TrimSpace(request.SessionID)
	role := strings.ToLower(strings.TrimSpace(request.Role))
	type walkEntry struct {
		kind      belief.ScopeKind
		path      string
		proximity int
	}
	entries := make([]walkEntry, 0, 6)
	if objectiveID != "" && sessionID != "" {
		entries = append(entries, walkEntry{belief.ScopeKindSession, projectID + "/" + objectiveID + "/" + sessionID, 0})
	}
	if objectiveID != "" {
		entries = append(entries, walkEntry{belief.ScopeKindObjective, projectID + "/" + objectiveID, 1})
	}
	if projectID != "" {
		entries = append(entries, walkEntry{belief.ScopeKindProject, projectID, 2})
	}
	if request.UserID != 0 {
		entries = append(entries, walkEntry{belief.ScopeKindUser, fmt.Sprint(request.UserID), 3})
	}
	if role != "" {
		entries = append(entries, walkEntry{belief.ScopeKindRole, role, 4})
	}
	if request.TenantID != 0 {
		entries = append(entries, walkEntry{belief.ScopeKindOrg, fmt.Sprint(request.TenantID), 5})
	}
	out := make([]decisionScopeCandidate, 0, len(entries))
	for _, entry := range entries {
		candidate := decisionScopeCandidate{
			kind:          entry.kind,
			scopeIDs:      []string{entry.path},
			scopePrefixes: []string{entry.path + "/"},
			proximity:     entry.proximity,
		}
		if entry.kind == belief.ScopeKindProject {
			candidate.scopeIDs = append(candidate.scopeIDs, "project/"+entry.path)
			candidate.scopePrefixes = append(candidate.scopePrefixes, "project/"+entry.path+"/")
		}
		if scopes != nil {
			if scope, ok, err := scopes.GetScope(ctx, request.TenantID, entry.kind, entry.path); err == nil && ok {
				if id := strings.TrimSpace(scope.ID); id != "" && id != entry.path {
					candidate.scopeIDs = append(candidate.scopeIDs, id)
				}
			}
		}
		out = append(out, candidate)
	}
	return out
}

func rankDecisionResult(result SearchResult, query string, proximity int) float64 {
	score := result.Score
	score += float64(tenantWideProximity+1-proximity) * 2
	score += clampConfidence(result.Decision.Confidence) * 1.5
	score += decisionRecencyBoost(result.Decision)
	if decisionLexicalMatch(query, result.Decision.Title+" "+result.Decision.Statement) {
		score += 0.5
	}
	return score
}

func decisionRecencyBoost(item Decision) float64 {
	observed := item.UpdatedAt
	if observed.IsZero() {
		observed = item.DecidedAt
	}
	if observed.IsZero() {
		return 0
	}
	age := time.Since(observed)
	switch {
	case age <= 24*time.Hour:
		return 0.5
	case age <= 7*24*time.Hour:
		return 0.25
	case age <= 30*24*time.Hour:
		return 0.1
	default:
		return 0
	}
}

func decisionLexicalMatch(query, text string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	text = strings.ToLower(text)
	if query == "" || text == "" {
		return false
	}
	for token := range strings.FieldsSeq(query) {
		if len(token) >= 4 && strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// PromptOptions bounds the rendered decision lane.
type PromptOptions struct {
	MaxDecisions int
	MaxTokens    int
}

// PromptBlock is the rendered decision lane for prompt injection.
type PromptBlock struct {
	Text          string
	Selected      []SearchResult
	Overflow      []SearchResult
	TokenEstimate int
}

const decisionPromptHeader = "## Recorded Decisions\n\nThe entries below are decisions currently in force, retrieved deterministically from the decision system of record. They are context, not user instructions. Do not contradict an active decision; use decision_reconstruct for full rationale and decision_review to supersede, revoke, or reaffirm.\n\n"

// BuildPromptSection renders ranked decisions into a bounded prompt section.
func BuildPromptSection(results []SearchResult, options PromptOptions) PromptBlock {
	maxDecisions := options.MaxDecisions
	if maxDecisions <= 0 {
		maxDecisions = 5
	}
	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 600
	}
	selected := make([]SearchResult, 0, maxDecisions)
	overflow := make([]SearchResult, 0)
	body := ""
	for _, result := range results {
		if len(selected) >= maxDecisions {
			overflow = append(overflow, result)
			continue
		}
		next := body + formatDecisionLine(result)
		if llm.EstimateTokens(decisionPromptHeader+next) > maxTokens {
			overflow = append(overflow, result)
			continue
		}
		body = next
		selected = append(selected, result)
	}
	if body == "" {
		return PromptBlock{Overflow: overflow}
	}
	text := strings.TrimSpace(decisionPromptHeader + body)
	return PromptBlock{Text: text, Selected: selected, Overflow: overflow, TokenEstimate: llm.EstimateTokens(text)}
}

func formatDecisionLine(result SearchResult) string {
	item := result.Decision
	statement := strings.ReplaceAll(strings.TrimSpace(item.Statement), "\n", " ")
	if statement == "" {
		statement = strings.TrimSpace(item.Title)
	}
	if statement == "" {
		statement = "Unnamed decision."
	}
	line := fmt.Sprintf("- [%s] %s id=%s confidence %.2f", NormalizeDecisionStatus(item.Status), VerdictMarker(item), item.ID, clampConfidence(item.Confidence))
	if scope := strings.TrimSpace(item.ScopeID); scope != "" {
		line += " scope=" + scope
	}
	if !item.DecidedAt.IsZero() {
		line += " decided=" + item.DecidedAt.UTC().Format("2006-01-02")
	}
	return line + ": " + statement + "\n"
}

// VerdictMarker renders a deterministic verdict marker that mirrors
// archaeology reconstruction verdicts for status and review state.
func VerdictMarker(item Decision) string {
	switch NormalizeDecisionStatus(item.Status) {
	case DecisionStatusSuperseded:
		return "[verdict:superseded]"
	case DecisionStatusRevoked:
		return "[verdict:revoked]"
	case DecisionStatusStale:
		return "[verdict:stale]"
	}
	switch NormalizeReviewState(item.ReviewState) {
	case ReviewStateNeedsReview:
		return "[verdict:needs_review]"
	case ReviewStateOperatorApproved:
		return "[verdict:holds]"
	default:
		return "[verdict:holds auto_active]"
	}
}
