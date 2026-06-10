package artifact

import "time"

// ArtifactKind identifies the external system an artifact came from.
type ArtifactKind string

const (
	// ArtifactGitCommit records a git commit.
	ArtifactGitCommit ArtifactKind = "git_commit"
	// ArtifactGitPR records a pull request.
	ArtifactGitPR ArtifactKind = "git_pr"
	// ArtifactChatMessage records a chat message.
	ArtifactChatMessage ArtifactKind = "chat_message"
	// ArtifactTicket records an external ticket.
	ArtifactTicket ArtifactKind = "ticket"
	// ArtifactDocument records a document.
	ArtifactDocument ArtifactKind = "document"
	// ArtifactTransitKey records a transit key version.
	ArtifactTransitKey ArtifactKind = "transit_key"
)

// Artifact is an immutable, content-addressed reference to an external object.
type Artifact struct {
	ID          string         `json:"id"`
	TenantID    int64          `json:"tenantId"`
	Kind        ArtifactKind   `json:"kind"`
	ExternalID  string         `json:"externalId"`
	URI         string         `json:"uri"`
	Title       string         `json:"title"`
	Excerpt     string         `json:"excerpt"`
	ContentHash string         `json:"contentHash"`
	AuthoredBy  string         `json:"authoredBy"`
	AuthoredAt  time.Time      `json:"authoredAt"`
	CapturedAt  time.Time      `json:"capturedAt"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// SearchQuery constrains artifact full-text retrieval.
type SearchQuery struct {
	TenantID int64          `json:"tenantId"`
	Kinds    []ArtifactKind `json:"kinds,omitempty"`
	Query    string         `json:"query,omitempty"`
	Limit    int            `json:"limit,omitempty"`
}

// SearchResult is a scored artifact retrieval result.
type SearchResult struct {
	Artifact Artifact `json:"artifact"`
	Score    float64  `json:"score"`
}
