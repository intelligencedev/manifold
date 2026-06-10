package decisiontools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"manifold/internal/agent/memory/archaeology"
	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/belief"
	decisionmem "manifold/internal/agent/memory/decision"
	"manifold/internal/auth"
)

const systemUserID int64 = 0

type searchTool struct {
	store decisionmem.Store
}

type reconstructTool struct {
	service *archaeology.Service
}

type recordTool struct {
	service *decisionmem.Service
}

type reviewTool struct {
	service *decisionmem.Service
}

// NewSearchTool returns a tool for retrieving recorded decisions.
func NewSearchTool(store decisionmem.Store) *searchTool {
	return &searchTool{store: store}
}

// NewReconstructTool returns a tool for reconstructing why decisions exist.
func NewReconstructTool(decisions *decisionmem.Service, beliefs belief.Store, artifacts artifact.Store) *reconstructTool {
	return &reconstructTool{service: &archaeology.Service{
		Decisions: decisions,
		Beliefs:   beliefs,
		Artifacts: artifacts,
	}}
}

// NewRecordTool returns a tool for recording explicit operator decisions.
func NewRecordTool(service *decisionmem.Service) *recordTool {
	return &recordTool{service: service}
}

// NewReviewTool returns a tool for reviewing candidates and lifecycle state.
func NewReviewTool(service *decisionmem.Service) *reviewTool {
	return &reviewTool{service: service}
}

func (t *searchTool) Name() string      { return "decision_search" }
func (t *reconstructTool) Name() string { return "decision_reconstruct" }
func (t *recordTool) Name() string      { return "decision_record" }
func (t *reviewTool) Name() string      { return "decision_review" }

func (t *searchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Search recorded architecture, implementation, and product decisions.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string"},
				"scopeIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"statuses": map[string]any{"type": "array", "items": map[string]any{
					"type": "string",
					"enum": []any{"proposed", "active", "stale", "superseded", "revoked"},
				}},
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
			},
		},
	}
}

func (t *reconstructTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Reconstruct the evidence, assumptions, alternatives, and lifecycle history behind matching decisions.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query":        map[string]any{"type": "string"},
				"scopeIds":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"includeStale": map[string]any{"type": "boolean"},
				"maxDecisions": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
				"asOf":         map[string]any{"type": "string", "format": "date-time"},
			},
		},
	}
}

func (t *recordTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Record an explicit decision with optional evidence, assumptions, and rejected alternatives.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"scopeId", "statement"},
			"properties": map[string]any{
				"scopeId":     map[string]any{"type": "string"},
				"episodeId":   map[string]any{"type": "string"},
				"title":       map[string]any{"type": "string"},
				"statement":   map[string]any{"type": "string"},
				"rationale":   map[string]any{"type": "string"},
				"status":      decisionStatusSchema(),
				"reviewState": reviewStateSchema(),
				"confidence":  map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				"evidence":    evidenceArraySchema(),
				"assumptions": assumptionArraySchema(),
				"alternatives": map[string]any{"type": "array", "items": map[string]any{
					"type":     "object",
					"required": []string{"statement"},
					"properties": map[string]any{
						"statement":       map[string]any{"type": "string"},
						"rejectionReason": map[string]any{"type": "string"},
					},
				}},
				"metadata": map[string]any{"type": "object"},
			},
		},
	}
}

func (t *reviewTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Review decision candidates or update recorded decision lifecycle state.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"action"},
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []any{"accept_candidate", "reject_candidate", "reaffirm", "revoke", "mark_stale", "needs_review"},
				},
				"candidateId":  map[string]any{"type": "string"},
				"decisionId":   map[string]any{"type": "string"},
				"reason":       map[string]any{"type": "string"},
				"triggerId":    map[string]any{"type": "string"},
				"supersededBy": map[string]any{"type": "string"},
			},
		},
	}
}

func decisionStatusSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []any{"proposed", "active", "stale", "superseded", "revoked"}}
}

func reviewStateSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []any{"auto_active", "needs_review", "operator_approved", "operator_rejected"}}
}

func evidenceArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{
		"type":     "object",
		"required": []string{"sourceKind", "sourceId"},
		"properties": map[string]any{
			"sourceKind": map[string]any{"type": "string"},
			"sourceId":   map[string]any{"type": "string"},
			"polarity":   map[string]any{"type": "string", "enum": []any{"for", "against"}},
			"weight":     map[string]any{"type": "number"},
			"note":       map[string]any{"type": "string"},
		},
	}}
}

func assumptionArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{
		"type":     "object",
		"required": []string{"beliefId"},
		"properties": map[string]any{
			"beliefId":                  map[string]any{"type": "string"},
			"criticality":               map[string]any{"type": "string", "enum": []any{"load_bearing", "supporting", "contextual"}},
			"beliefStatementAtLink":     map[string]any{"type": "string"},
			"beliefConfidenceAtLink":    map[string]any{"type": "number"},
			"note":                      map[string]any{"type": "string"},
			"belief_statement_at_link":  map[string]any{"type": "string"},
			"belief_confidence_at_link": map[string]any{"type": "number"},
		},
	}}
}

func (t *searchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t == nil || t.store == nil {
		return nil, decisionmem.ErrStoreRequired
	}
	var args struct {
		Query    string                       `json:"query"`
		ScopeIDs []string                     `json:"scopeIds"`
		Statuses []decisionmem.DecisionStatus `json:"statuses"`
		Limit    int                          `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	return t.store.SearchDecisions(ctx, decisionmem.SearchQuery{
		TenantID: currentTenantID(ctx),
		Query:    args.Query,
		ScopeIDs: args.ScopeIDs,
		Statuses: args.Statuses,
		Limit:    args.Limit,
	})
}

func (t *reconstructTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t == nil || t.service == nil {
		return nil, decisionmem.ErrStoreRequired
	}
	var args struct {
		Query        string   `json:"query"`
		ScopeIDs     []string `json:"scopeIds"`
		IncludeStale bool     `json:"includeStale"`
		MaxDecisions int      `json:"maxDecisions"`
		AsOf         string   `json:"asOf"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	var asOf *time.Time
	if strings.TrimSpace(args.AsOf) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(args.AsOf))
		if err != nil {
			return nil, err
		}
		asOf = &parsed
	}
	return t.service.Reconstruct(ctx, currentTenantID(ctx), args.Query, archaeology.ReconstructOptions{
		ScopeIDs:     args.ScopeIDs,
		AsOf:         asOf,
		IncludeStale: args.IncludeStale,
		MaxDecisions: args.MaxDecisions,
	})
}

func (t *recordTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t == nil || t.service == nil || t.service.Store == nil {
		return nil, decisionmem.ErrStoreRequired
	}
	var args recordArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	tenantID := currentTenantID(ctx)
	actor := currentActorUserID(ctx)
	recorded, err := t.service.CreateDecision(ctx, decisionmem.Decision{
		TenantID:    tenantID,
		ScopeID:     strings.TrimSpace(args.ScopeID),
		EpisodeID:   strings.TrimSpace(args.EpisodeID),
		Title:       strings.TrimSpace(args.Title),
		Statement:   strings.TrimSpace(args.Statement),
		Rationale:   strings.TrimSpace(args.Rationale),
		DecidedBy:   decidedBy(actor),
		Status:      decisionmem.NormalizeDecisionStatus(args.Status),
		ReviewState: decisionmem.NormalizeReviewState(args.ReviewState),
		Confidence:  args.Confidence,
		Metadata:    args.Metadata,
	})
	if err != nil {
		return nil, err
	}
	evidence, assumptions, alternatives, err := t.attachContext(ctx, tenantID, recorded.ID, args)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"decision":     recorded,
		"evidence":     evidence,
		"assumptions":  assumptions,
		"alternatives": alternatives,
	}, nil
}

func (t *recordTool) attachContext(ctx context.Context, tenantID int64, decisionID string, args recordArgs) ([]decisionmem.DecisionEvidence, []decisionmem.AssumptionLink, []decisionmem.Alternative, error) {
	evidence := make([]decisionmem.DecisionEvidence, 0, len(args.Evidence))
	for _, ev := range args.Evidence {
		if strings.TrimSpace(ev.SourceKind) == "" || strings.TrimSpace(ev.SourceID) == "" {
			continue
		}
		if ev.Weight == 0 {
			ev.Weight = 1
		}
		created, err := t.service.Store.AddEvidence(ctx, decisionmem.DecisionEvidence{
			TenantID:   tenantID,
			DecisionID: decisionID,
			SourceKind: strings.TrimSpace(ev.SourceKind),
			SourceID:   strings.TrimSpace(ev.SourceID),
			Polarity:   decisionmem.NormalizeEvidencePolarity(ev.Polarity),
			Weight:     ev.Weight,
			Note:       strings.TrimSpace(ev.Note),
		})
		if err != nil {
			return nil, nil, nil, err
		}
		evidence = append(evidence, created)
	}
	assumptions := make([]decisionmem.AssumptionLink, 0, len(args.Assumptions))
	for _, assumption := range args.Assumptions {
		if strings.TrimSpace(assumption.BeliefID) == "" {
			continue
		}
		statement := firstNonEmpty(assumption.BeliefStatementAtLink, assumption.BeliefStatementAtLinkSnake)
		confidence := assumption.BeliefConfidenceAtLink
		if confidence == 0 {
			confidence = assumption.BeliefConfidenceAtLinkSnake
		}
		created, err := t.service.Store.AddAssumption(ctx, decisionmem.AssumptionLink{
			TenantID:               tenantID,
			DecisionID:             decisionID,
			BeliefID:               strings.TrimSpace(assumption.BeliefID),
			Criticality:            decisionmem.NormalizeAssumptionCriticality(assumption.Criticality),
			BeliefStatementAtLink:  strings.TrimSpace(statement),
			BeliefConfidenceAtLink: confidence,
			Note:                   strings.TrimSpace(assumption.Note),
		})
		if err != nil {
			return nil, nil, nil, err
		}
		assumptions = append(assumptions, created)
	}
	alternatives := make([]decisionmem.Alternative, 0, len(args.Alternatives))
	for _, alt := range args.Alternatives {
		if strings.TrimSpace(alt.Statement) == "" {
			continue
		}
		created, err := t.service.Store.AddAlternative(ctx, decisionmem.Alternative{
			TenantID:        tenantID,
			DecisionID:      decisionID,
			Statement:       strings.TrimSpace(alt.Statement),
			RejectionReason: strings.TrimSpace(alt.RejectionReason),
		})
		if err != nil {
			return nil, nil, nil, err
		}
		alternatives = append(alternatives, created)
	}
	return evidence, assumptions, alternatives, nil
}

func (t *reviewTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t == nil || t.service == nil || t.service.Store == nil {
		return nil, decisionmem.ErrStoreRequired
	}
	var args struct {
		Action       string `json:"action"`
		CandidateID  string `json:"candidateId"`
		DecisionID   string `json:"decisionId"`
		Reason       string `json:"reason"`
		TriggerID    string `json:"triggerId"`
		SupersededBy string `json:"supersededBy"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	tenantID := currentTenantID(ctx)
	actor := currentActorUserID(ctx)
	switch strings.TrimSpace(args.Action) {
	case "accept_candidate":
		return t.service.AcceptCandidate(ctx, tenantID, args.CandidateID, actor)
	case "reject_candidate":
		return t.rejectCandidate(ctx, tenantID, args.CandidateID, args.Reason)
	case "reaffirm":
		return t.service.ReaffirmDecision(ctx, tenantID, args.DecisionID, args.Reason, actor)
	case "revoke":
		return t.service.RevokeDecision(ctx, tenantID, args.DecisionID, args.Reason, actor)
	case "mark_stale":
		return t.service.TransitionDecision(ctx, tenantID, args.DecisionID, decisionmem.DecisionStatusStale, decisionmem.TransitionRequest{
			Reason:       args.Reason,
			TriggerKind:  decisionmem.TriggerOperator,
			TriggerID:    args.TriggerID,
			ActorUserID:  actor,
			SupersededBy: args.SupersededBy,
		})
	case "needs_review":
		return t.service.MarkNeedsReview(ctx, tenantID, args.DecisionID, args.Reason, args.TriggerID)
	default:
		return nil, errors.New("unsupported decision review action")
	}
}

func (t *reviewTool) rejectCandidate(ctx context.Context, tenantID int64, candidateID, reason string) (decisionmem.Candidate, error) {
	candidate, ok, err := t.service.Store.GetCandidate(ctx, tenantID, strings.TrimSpace(candidateID))
	if err != nil || !ok {
		if err != nil {
			return decisionmem.Candidate{}, err
		}
		return decisionmem.Candidate{}, errors.New("candidate not found")
	}
	candidate.ValidationStatus = decisionmem.CandidateValidationRejected
	candidate.ReviewState = decisionmem.ReviewStateOperatorRejected
	candidate.RejectionReason = strings.TrimSpace(reason)
	return t.service.Store.RecordCandidate(ctx, candidate)
}

type recordArgs struct {
	ScopeID      string                     `json:"scopeId"`
	EpisodeID    string                     `json:"episodeId"`
	Title        string                     `json:"title"`
	Statement    string                     `json:"statement"`
	Rationale    string                     `json:"rationale"`
	Status       decisionmem.DecisionStatus `json:"status"`
	ReviewState  decisionmem.ReviewState    `json:"reviewState"`
	Confidence   float64                    `json:"confidence"`
	Evidence     []recordEvidenceArg        `json:"evidence"`
	Assumptions  []recordAssumptionArg      `json:"assumptions"`
	Alternatives []recordAlternativeArg     `json:"alternatives"`
	Metadata     map[string]any             `json:"metadata"`
}

type recordEvidenceArg struct {
	SourceKind string                       `json:"sourceKind"`
	SourceID   string                       `json:"sourceId"`
	Polarity   decisionmem.EvidencePolarity `json:"polarity"`
	Weight     float64                      `json:"weight"`
	Note       string                       `json:"note"`
}

type recordAssumptionArg struct {
	BeliefID                    string                            `json:"beliefId"`
	Criticality                 decisionmem.AssumptionCriticality `json:"criticality"`
	BeliefStatementAtLink       string                            `json:"beliefStatementAtLink"`
	BeliefConfidenceAtLink      float64                           `json:"beliefConfidenceAtLink"`
	BeliefStatementAtLinkSnake  string                            `json:"belief_statement_at_link"`
	BeliefConfidenceAtLinkSnake float64                           `json:"belief_confidence_at_link"`
	Note                        string                            `json:"note"`
}

type recordAlternativeArg struct {
	Statement       string `json:"statement"`
	RejectionReason string `json:"rejectionReason"`
}

func currentTenantID(ctx context.Context) int64 {
	if user, ok := auth.CurrentUser(ctx); ok && user != nil {
		return user.ID
	}
	return systemUserID
}

func currentActorUserID(ctx context.Context) *int64 {
	if user, ok := auth.CurrentUser(ctx); ok && user != nil {
		id := user.ID
		return &id
	}
	return nil
}

func decidedBy(actor *int64) string {
	if actor == nil {
		return "tool:decision_record"
	}
	return "human:" + jsonNumber(*actor)
}

func jsonNumber(value int64) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
