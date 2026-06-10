package decision

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/agent/memory/belief"
)

const (
	maxCandidateConfidence = 0.90
)

// EmbedFunc embeds candidate statements for semantic search.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

// ArtifactRef is the bounded artifact context offered to decision distillers.
type ArtifactRef struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ExternalID string `json:"externalId,omitempty"`
	URI        string `json:"uri,omitempty"`
	Title      string `json:"title,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
}

// DistillationInput contains the episode envelope for candidate decision extraction.
type DistillationInput struct {
	Episode        belief.Episode `json:"episode"`
	UserRequest    string         `json:"userRequest,omitempty"`
	FinalAnswer    string         `json:"finalAnswer,omitempty"`
	Summary        string         `json:"summary,omitempty"`
	ToolSummary    string         `json:"toolSummary,omitempty"`
	ReasoningTrace []string       `json:"reasoningTrace,omitempty"`
	Artifacts      []ArtifactRef  `json:"artifacts,omitempty"`
}

// Distiller extracts candidate decisions from episode evidence.
type Distiller interface {
	Distill(ctx context.Context, input DistillationInput) ([]Candidate, error)
}

// DistillationResult contains accepted candidates and audit rows.
type DistillationResult struct {
	Candidates []Candidate
	Audit      []Candidate
	RawPayload string
}

// AuditDistiller extracts candidates and returns validation audit rows.
type AuditDistiller interface {
	DistillWithAudit(ctx context.Context, input DistillationInput) (DistillationResult, error)
}

// NoopDistiller disables decision extraction.
type NoopDistiller struct{}

func (NoopDistiller) Distill(context.Context, DistillationInput) ([]Candidate, error) {
	return nil, nil
}

// SimpleDistiller records an explicit decision only when the episode text looks decision-like.
type SimpleDistiller struct {
	Embed EmbedFunc
}

// Distill returns at most one conservative candidate from explicit decision language.
func (d SimpleDistiller) Distill(ctx context.Context, input DistillationInput) ([]Candidate, error) {
	statement := explicitDecisionStatement(input)
	if statement == "" {
		return nil, nil
	}
	statement = NormalizeStatement(statement)
	candidate := Candidate{
		TenantID:         input.Episode.TenantID,
		EpisodeID:        input.Episode.ID,
		ScopeID:          input.Episode.ScopeID,
		Title:            titleFromStatement(statement),
		Statement:        statement,
		StatementHash:    StatementHash(statement),
		Rationale:        "Extracted from explicit decision language in the episode.",
		Confidence:       0.55,
		ReviewState:      ReviewStateNeedsReview,
		ValidationStatus: CandidateValidationQueued,
		Model:            "simple",
		Metadata: map[string]any{
			"distiller":       "simple",
			"projectId":       input.Episode.ProjectID,
			"objectiveId":     input.Episode.ObjectiveID,
			"agentRole":       input.Episode.AgentRole,
			"evolvingEntryId": input.Episode.EvolvingEntryID,
		},
	}
	for _, artifact := range input.Artifacts {
		if strings.TrimSpace(artifact.ID) == "" {
			continue
		}
		candidate.EvidenceHints = append(candidate.EvidenceHints, EvidenceHint{
			SourceKind: "artifact",
			SourceID:   artifact.ID,
			Polarity:   EvidencePolarityFor,
			Note:       strings.TrimSpace(artifact.Title),
		})
	}
	if d.Embed != nil {
		if embeddings, err := d.Embed(ctx, []string{statement}); err == nil && len(embeddings) == 1 {
			candidate.Metadata["embedding"] = "attached"
		}
	}
	return []Candidate{candidate}, nil
}

func explicitDecisionStatement(input DistillationInput) string {
	for _, text := range []string{input.FinalAnswer, input.Summary, input.ToolSummary} {
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. )\t"))
			lower := strings.ToLower(line)
			for _, prefix := range []string{"decision:", "we decided to ", "decided to ", "we chose to ", "chose to ", "adopt "} {
				if strings.HasPrefix(lower, prefix) {
					cleaned := strings.TrimSpace(line)
					if strings.HasPrefix(lower, "decision:") {
						cleaned = strings.TrimSpace(line[len("decision:"):])
					}
					if len([]rune(cleaned)) >= 10 && !strings.HasSuffix(cleaned, "?") {
						return fmt.Sprintf("We decided to %s", strings.TrimPrefix(cleaned, "we decided to "))
					}
				}
			}
		}
	}
	return ""
}
