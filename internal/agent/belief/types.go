package belief

import "time"

// ScopeKind identifies where a belief-memory record applies in the hierarchy.
type ScopeKind string

const (
	ScopeKindOrg       ScopeKind = "org"
	ScopeKindProject   ScopeKind = "project"
	ScopeKindObjective ScopeKind = "objective"
	ScopeKindSession   ScopeKind = "session"
	ScopeKindUser      ScopeKind = "user"
	ScopeKindRole      ScopeKind = "role"
)

// BeliefStatus describes whether a belief should influence retrieval.
type BeliefStatus string

const (
	BeliefStatusActive     BeliefStatus = "active"
	BeliefStatusSuperseded BeliefStatus = "superseded"
	BeliefStatusRetracted  BeliefStatus = "retracted"
)

type BeliefKind string

const (
	BeliefKindFact       BeliefKind = "fact"
	BeliefKindPreference BeliefKind = "preference"
	BeliefKindProcedure  BeliefKind = "procedure"
	BeliefKindConstraint BeliefKind = "constraint"
	BeliefKindCapability BeliefKind = "capability"
)

type EnforcementMode string

const (
	EnforcementNone           EnforcementMode = "none"
	EnforcementPrompt         EnforcementMode = "prompt"
	EnforcementSoftPolicy     EnforcementMode = "soft_policy"
	EnforcementHardConstraint EnforcementMode = "hard_constraint"
)

type ReviewState string

const (
	ReviewStateAutoActive       ReviewState = "auto_active"
	ReviewStateNeedsReview      ReviewState = "needs_review"
	ReviewStateOperatorApproved ReviewState = "operator_approved"
	ReviewStateOperatorRejected ReviewState = "operator_rejected"
)

type CandidateValidationStatus string

const (
	CandidateValidationAccepted CandidateValidationStatus = "accepted"
	CandidateValidationQueued   CandidateValidationStatus = "queued"
	CandidateValidationRejected CandidateValidationStatus = "rejected"
)

// EvidencePolarity describes whether evidence supports or contradicts a belief.
type EvidencePolarity string

const (
	EvidencePolarityFor     EvidencePolarity = "for"
	EvidencePolarityAgainst EvidencePolarity = "against"
)

// SourceKind identifies the origin of a belief evidence item.
type SourceKind string

const (
	SourceKindEpisode       SourceKind = "episode"
	SourceKindHumanFeedback SourceKind = "human_feedback"
	SourceKindToolResult    SourceKind = "tool_result"
	SourceKindRAGDoc        SourceKind = "rag_doc"
	SourceKindRAGChunk      SourceKind = "rag_chunk"
	SourceKindTransit       SourceKind = "transit"
	SourceKindBelief        SourceKind = "belief"
)

// Scope normalizes the scope hierarchy used by belief memory.
type Scope struct {
	ID        string         `json:"id"`
	TenantID  int64          `json:"tenantId"`
	Kind      ScopeKind      `json:"kind"`
	ParentID  string         `json:"parentId,omitempty"`
	Path      string         `json:"path"`
	Label     string         `json:"label"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// Episode is the synchronous run envelope that links runtime execution to beliefs.
type Episode struct {
	ID              string         `json:"id"`
	TenantID        int64          `json:"tenantId"`
	ScopeID         string         `json:"scopeId"`
	ProjectID       string         `json:"projectId"`
	ObjectiveID     string         `json:"objectiveId"`
	SessionID       string         `json:"sessionId"`
	AgentRole       string         `json:"agentRole"`
	UserID          int64          `json:"userId"`
	StartedAt       time.Time      `json:"startedAt"`
	EndedAt         *time.Time     `json:"endedAt,omitempty"`
	Outcome         string         `json:"outcome"`
	OutcomeSignal   string         `json:"outcomeSignal"`
	EvolvingEntryID string         `json:"evolvingEntryId,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Belief stores one probabilistic claim at one scope.
type Belief struct {
	ID              string          `json:"id"`
	TenantID        int64           `json:"tenantId"`
	ScopeID         string          `json:"scopeId"`
	Statement       string          `json:"statement"`
	StatementHash   string          `json:"statementHash"`
	Kind            BeliefKind      `json:"kind"`
	Enforcement     EnforcementMode `json:"enforcement"`
	SourceQuality   float64         `json:"sourceQuality"`
	ReviewState     ReviewState     `json:"reviewState"`
	Confidence      float64         `json:"confidence"`
	EvidenceFor     int             `json:"evidenceFor"`
	EvidenceAgainst int             `json:"evidenceAgainst"`
	Status          BeliefStatus    `json:"status"`
	LastObserved    *time.Time      `json:"lastObserved,omitempty"`
	ExpiresAt       *time.Time      `json:"expiresAt,omitempty"`
	Embedding       []float32       `json:"-"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Evidence links a belief to a source that supports or contradicts it.
type Evidence struct {
	ID         string           `json:"id"`
	TenantID   int64            `json:"tenantId"`
	BeliefID   string           `json:"beliefId"`
	EpisodeID  string           `json:"episodeId,omitempty"`
	SourceKind SourceKind       `json:"sourceKind"`
	SourceID   string           `json:"sourceId"`
	Polarity   EvidencePolarity `json:"polarity"`
	Weight     float64          `json:"weight"`
	Note       string           `json:"note"`
	Metadata   map[string]any   `json:"metadata,omitempty"`
	CreatedAt  time.Time        `json:"createdAt"`
}

// Promotion audits a belief movement from one scope to another.
type Promotion struct {
	ID               string         `json:"id"`
	TenantID         int64          `json:"tenantId"`
	BeliefID         string         `json:"beliefId"`
	FromScope        string         `json:"fromScope"`
	ToScope          string         `json:"toScope"`
	Reason           string         `json:"reason"`
	ConfidenceBefore float64        `json:"confidenceBefore"`
	ConfidenceAfter  float64        `json:"confidenceAfter"`
	ActorUserID      *int64         `json:"actorUserId,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
}

// SearchQuery constrains belief retrieval by tenant and scope.
type SearchQuery struct {
	TenantID int64          `json:"tenantId"`
	ScopeIDs []string       `json:"scopeIds"`
	Query    string         `json:"query"`
	Statuses []BeliefStatus `json:"statuses"`
	Limit    int            `json:"limit"`
}

type EvidenceQuery struct {
	TenantID  int64            `json:"tenantId"`
	BeliefID  string           `json:"beliefId,omitempty"`
	EpisodeID string           `json:"episodeId,omitempty"`
	SourceID  string           `json:"sourceId,omitempty"`
	Source    SourceKind       `json:"sourceKind,omitempty"`
	Polarity  EvidencePolarity `json:"polarity,omitempty"`
	Limit     int              `json:"limit,omitempty"`
}

type PromotionQuery struct {
	TenantID  int64  `json:"tenantId"`
	BeliefID  string `json:"beliefId,omitempty"`
	FromScope string `json:"fromScope,omitempty"`
	ToScope   string `json:"toScope,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// SearchResult is a scored belief retrieval result.
type SearchResult struct {
	Belief Belief  `json:"belief"`
	Scope  Scope   `json:"scope"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

// Candidate is a possible belief produced by a distiller.
type Candidate struct {
	Statement     string           `json:"statement"`
	StatementHash string           `json:"statementHash"`
	Kind          BeliefKind       `json:"kind"`
	Enforcement   EnforcementMode  `json:"enforcement"`
	SourceQuality float64          `json:"sourceQuality"`
	ReviewState   ReviewState      `json:"reviewState"`
	Confidence    float64          `json:"confidence"`
	Polarity      EvidencePolarity `json:"polarity"`
	EvidenceNote  string           `json:"evidenceNote"`
	Embedding     []float32        `json:"-"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
}

type CandidateRecord struct {
	ID                string                    `json:"id"`
	TenantID          int64                     `json:"tenantId"`
	EpisodeID         string                    `json:"episodeId"`
	ScopeID           string                    `json:"scopeId"`
	Statement         string                    `json:"statement"`
	StatementHash     string                    `json:"statementHash"`
	Kind              BeliefKind                `json:"kind"`
	Enforcement       EnforcementMode           `json:"enforcement"`
	Polarity          EvidencePolarity          `json:"polarity"`
	Confidence        float64                   `json:"confidence"`
	SourceQuality     float64                   `json:"sourceQuality"`
	ReviewState       ReviewState               `json:"reviewState"`
	EvidenceNote      string                    `json:"evidenceNote"`
	RawPayload        string                    `json:"rawPayload,omitempty"`
	NormalizedPayload map[string]any            `json:"normalizedPayload,omitempty"`
	ValidationStatus  CandidateValidationStatus `json:"validationStatus"`
	RejectionReason   string                    `json:"rejectionReason,omitempty"`
	AcceptedBeliefID  string                    `json:"acceptedBeliefId,omitempty"`
	Model             string                    `json:"model,omitempty"`
	Metadata          map[string]any            `json:"metadata,omitempty"`
	CreatedAt         time.Time                 `json:"createdAt"`
	UpdatedAt         time.Time                 `json:"updatedAt"`
}

type CandidateQuery struct {
	TenantID         int64                     `json:"tenantId"`
	EpisodeID        string                    `json:"episodeId,omitempty"`
	ReviewState      ReviewState               `json:"reviewState,omitempty"`
	ValidationStatus CandidateValidationStatus `json:"validationStatus,omitempty"`
	Limit            int                       `json:"limit,omitempty"`
}
