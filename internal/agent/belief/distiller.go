package belief

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	implicitSuccessConfidence  = 0.25
	weakErrorConfidence        = 0.35
	explicitPositiveConfidence = 0.75
	explicitNegativeConfidence = 0.80
	maxCandidateConfidence     = 0.90
)

var whitespacePattern = regexp.MustCompile(`\s+`)

type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)

// DistillationInput contains the minimum envelope for candidate belief extraction.
type DistillationInput struct {
	Episode Episode        `json:"episode"`
	Lesson  string         `json:"lesson,omitempty"`
	Summary string         `json:"summary,omitempty"`
	Signals map[string]any `json:"signals,omitempty"`
}

// Distiller extracts candidate beliefs from episodes and evidence.
type Distiller interface {
	Distill(ctx context.Context, input DistillationInput) ([]Candidate, error)
}

// NoopDistiller is used while belief distillation is disabled or unconfigured.
type NoopDistiller struct{}

func (NoopDistiller) Distill(context.Context, DistillationInput) ([]Candidate, error) {
	return nil, nil
}

// SimpleDistiller is the conservative MVP distiller. It creates at most one
// low-confidence candidate from the completed episode envelope and optional
// summary text, leaving richer extraction to later LLM-backed distillers.
type SimpleDistiller struct {
	Embed EmbedFunc
}

func (d SimpleDistiller) Distill(ctx context.Context, input DistillationInput) ([]Candidate, error) {
	episode := input.Episode
	if strings.TrimSpace(episode.ObjectiveID) == "" || strings.TrimSpace(episode.ScopeID) == "" {
		return nil, nil
	}
	statement := candidateStatement(input)
	if statement == "" {
		return nil, nil
	}
	polarity, confidence := outcomePolarityAndConfidence(episode)
	statement = NormalizeStatement(statement)
	if statement == "" {
		return nil, nil
	}
	candidate := Candidate{
		Statement:     statement,
		StatementHash: StatementHash(statement),
		Confidence:    confidence,
		Polarity:      polarity,
		EvidenceNote:  evidenceNote(episode),
		Metadata: map[string]any{
			"distiller":       "simple",
			"outcome":         episode.Outcome,
			"outcomeSignal":   episode.OutcomeSignal,
			"projectId":       episode.ProjectID,
			"objectiveId":     episode.ObjectiveID,
			"agentRole":       episode.AgentRole,
			"evolvingEntryId": episode.EvolvingEntryID,
		},
	}
	if d.Embed != nil {
		if embeddings, err := d.Embed(ctx, []string{statement}); err == nil && len(embeddings) == 1 {
			candidate.Embedding = embeddings[0]
		}
	}
	return []Candidate{candidate}, nil
}

func candidateStatement(input DistillationInput) string {
	if text := firstMeaningfulSentence(input.Lesson); text != "" {
		return text
	}
	if text := firstMeaningfulSentence(input.Summary); text != "" {
		return text
	}
	episode := input.Episode
	role := strings.TrimSpace(episode.AgentRole)
	if role == "" {
		role = "orchestrator"
	}
	projectID := NormalizeProjectID(episode.ProjectID)
	return fmt.Sprintf("%s can make progress on objective %s in project %s", role, strings.TrimSpace(episode.ObjectiveID), projectID)
}

func firstMeaningfulSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*•0123456789. )\t"))
		if len([]rune(line)) < 24 {
			continue
		}
		for _, sep := range []string{". ", "! ", "? "} {
			if idx := strings.Index(line, sep); idx > 0 {
				line = line[:idx+1]
				break
			}
		}
		return line
	}
	return ""
}

func outcomePolarityAndConfidence(episode Episode) (EvidencePolarity, float64) {
	signal := strings.ToLower(strings.TrimSpace(episode.OutcomeSignal))
	outcome := strings.ToLower(strings.TrimSpace(episode.Outcome))
	switch signal {
	case "human_rejected", "rejected", "reviewer_rejected":
		return EvidencePolarityAgainst, explicitNegativeConfidence
	case "tests_passed", "tool_succeeded", "user_accepted", "reviewer_accepted":
		return EvidencePolarityFor, explicitPositiveConfidence
	case "runtime_error", "tool_failed", "tests_failed":
		return EvidencePolarityAgainst, weakErrorConfidence
	}
	if outcome == "error" || outcome == "failed" || outcome == "failure" {
		return EvidencePolarityAgainst, weakErrorConfidence
	}
	return EvidencePolarityFor, implicitSuccessConfidence
}

func evidenceNote(episode Episode) string {
	role := strings.TrimSpace(episode.AgentRole)
	if role == "" {
		role = "agent"
	}
	return fmt.Sprintf("%s episode %s ended with %s/%s", role, strings.TrimSpace(episode.ID), strings.TrimSpace(episode.Outcome), strings.TrimSpace(episode.OutcomeSignal))
}

func NormalizeStatement(statement string) string {
	statement = whitespacePattern.ReplaceAllString(strings.TrimSpace(statement), " ")
	statement = strings.Trim(statement, " \t\n\r-*")
	if statement == "" {
		return ""
	}
	runes := []rune(statement)
	runes[0] = unicode.ToUpper(runes[0])
	statement = string(runes)
	if !strings.HasSuffix(statement, ".") && !strings.HasSuffix(statement, "!") && !strings.HasSuffix(statement, "?") {
		statement += "."
	}
	return statement
}

func StatementHash(statement string) string {
	normalized := strings.ToLower(strings.TrimSpace(NormalizeStatement(statement)))
	digest := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", digest)
}

func ApplyCandidates(ctx context.Context, store Store, episode Episode, candidates []Candidate) ([]Belief, error) {
	if store == nil || len(candidates) == 0 {
		return nil, nil
	}
	applied := make([]Belief, 0, len(candidates))
	for _, candidate := range candidates {
		belief, err := applyCandidate(ctx, store, episode, candidate)
		if err != nil {
			return applied, err
		}
		if strings.TrimSpace(belief.ID) != "" {
			applied = append(applied, belief)
		}
	}
	return applied, nil
}

func applyCandidate(ctx context.Context, store Store, episode Episode, candidate Candidate) (Belief, error) {
	candidate.Statement = NormalizeStatement(candidate.Statement)
	if candidate.Statement == "" || strings.TrimSpace(episode.ScopeID) == "" {
		return Belief{}, nil
	}
	if strings.TrimSpace(candidate.StatementHash) == "" {
		candidate.StatementHash = StatementHash(candidate.Statement)
	}
	candidate.Polarity = normalizeCandidatePolarity(candidate.Polarity)
	candidate.Confidence = clampConfidence(candidate.Confidence)

	existing, ok, err := findExistingBelief(ctx, store, episode, candidate)
	if err != nil {
		return Belief{}, err
	}
	now := time.Now().UTC()
	item := mergeCandidateBelief(existing, ok, episode, candidate, now)
	item, err = store.UpsertBelief(ctx, item)
	if err != nil {
		return Belief{}, err
	}
	_, err = store.AddEvidence(ctx, Evidence{
		TenantID:   episode.TenantID,
		BeliefID:   item.ID,
		EpisodeID:  episode.ID,
		SourceKind: SourceKindEpisode,
		SourceID:   episode.ID,
		Polarity:   candidate.Polarity,
		Weight:     candidate.Confidence,
		Note:       strings.TrimSpace(candidate.EvidenceNote),
		Metadata:   cloneCandidateMetadata(candidate.Metadata),
	})
	if err != nil {
		return Belief{}, err
	}
	return item, nil
}

func findExistingBelief(ctx context.Context, store Store, episode Episode, candidate Candidate) (Belief, bool, error) {
	results, err := store.SearchBeliefs(ctx, SearchQuery{
		TenantID: episode.TenantID,
		ScopeIDs: []string{episode.ScopeID},
		Query:    candidate.Statement,
		Statuses: []BeliefStatus{BeliefStatusActive, BeliefStatusSuperseded, BeliefStatusRetracted},
		Limit:    10,
	})
	if err != nil {
		return Belief{}, false, err
	}
	for _, result := range results {
		if result.Belief.StatementHash == candidate.StatementHash {
			return result.Belief, true, nil
		}
	}
	return Belief{}, false, nil
}

func mergeCandidateBelief(existing Belief, ok bool, episode Episode, candidate Candidate, now time.Time) Belief {
	item := existing
	if !ok {
		item = Belief{
			TenantID:      episode.TenantID,
			ScopeID:       episode.ScopeID,
			Statement:     candidate.Statement,
			StatementHash: candidate.StatementHash,
			Status:        BeliefStatusActive,
			Metadata:      map[string]any{},
		}
	}
	item.TenantID = episode.TenantID
	item.ScopeID = episode.ScopeID
	item.Statement = candidate.Statement
	item.StatementHash = candidate.StatementHash
	item.Embedding = append([]float32(nil), candidate.Embedding...)
	item.Status = BeliefStatusActive
	item.LastObserved = &now
	item.Metadata = mergeCandidateMetadata(item.Metadata, candidate.Metadata)
	if candidate.Polarity == EvidencePolarityAgainst {
		item.EvidenceAgainst++
		item.Confidence = decreaseConfidence(item.Confidence, candidate.Confidence, ok)
		return item
	}
	item.EvidenceFor++
	item.Confidence = increaseConfidence(item.Confidence, candidate.Confidence, ok)
	return item
}

func increaseConfidence(current, evidence float64, hasExisting bool) float64 {
	evidence = clampConfidence(evidence)
	if !hasExisting || current <= 0 {
		return math.Min(maxCandidateConfidence, evidence)
	}
	delta := evidence * 0.20 * (1 - current)
	return math.Min(maxCandidateConfidence, current+delta)
}

func decreaseConfidence(current, evidence float64, hasExisting bool) float64 {
	evidence = clampConfidence(evidence)
	if !hasExisting || current <= 0 {
		current = 0.50
	}
	delta := evidence * 0.35 * current
	return math.Max(0, current-delta)
}

func clampConfidence(confidence float64) float64 {
	if confidence <= 0 {
		return implicitSuccessConfidence
	}
	if confidence > 1 {
		return 1
	}
	return confidence
}

func normalizeCandidatePolarity(polarity EvidencePolarity) EvidencePolarity {
	if polarity == EvidencePolarityAgainst {
		return EvidencePolarityAgainst
	}
	return EvidencePolarityFor
}

func mergeCandidateMetadata(existing, candidate map[string]any) map[string]any {
	out := cloneCandidateMetadata(existing)
	if out == nil {
		out = map[string]any{}
	}
	maps.Copy(out, candidate)
	return out
}

func cloneCandidateMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}
