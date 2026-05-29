package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent/belief"
	"manifold/internal/agent/memory"
	"manifold/internal/memory/magma"
)

type evolvingMagmaSink struct {
	service     *magma.Service
	workerCount int
}

func (s evolvingMagmaSink) IngestEvolvingMemory(ctx context.Context, userID int64, sessionID string, entry *memory.MemoryEntry) (string, error) {
	if s.service == nil || entry == nil || strings.TrimSpace(entry.ID) == "" {
		return "", nil
	}
	s.service.StartConsolidationWorkers(context.Background(), s.workerCount)
	resp, err := s.service.Ingest(ctx, magma.IngestRequest{
		ID:        "evolving:" + entry.ID,
		Tenant:    fmt.Sprintf("user:%d", userID),
		SessionID: sessionID,
		Text:      evolvingMagmaText(entry),
		CreatedAt: entry.CreatedAt,
		Metadata: map[string]any{
			"source":      "evolving_memory",
			"entry_id":    entry.ID,
			"memory_type": string(entry.MemoryType),
			"scope":       string(entry.Scope),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.EventID, nil
}

func evolvingMagmaText(entry *memory.MemoryEntry) string {
	var text strings.Builder
	writeEvolvingMagmaLine(&text, "Input", entry.Input)
	writeEvolvingMagmaLine(&text, "Summary", entry.Summary)
	writeEvolvingMagmaLine(&text, "Feedback", entry.Feedback)
	writeEvolvingMagmaLine(&text, "Output", entry.Output)
	writeEvolvingMagmaLine(&text, "Strategy", entry.StrategyCard)
	writeEvolvingMagmaLine(&text, "Trace", entry.RawTrace)
	return strings.TrimSpace(text.String())
}

func writeEvolvingMagmaLine(text *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if text.Len() > 0 {
		text.WriteByte('\n')
	}
	text.WriteString(label)
	text.WriteString(": ")
	text.WriteString(value)
}

type beliefMagmaSink struct {
	service     *magma.Service
	workerCount int
}

func (s beliefMagmaSink) IngestBelief(ctx context.Context, episode belief.Episode, item belief.Belief) (string, error) {
	if s.service == nil || strings.TrimSpace(item.ID) == "" {
		return "", nil
	}
	s.service.StartConsolidationWorkers(context.Background(), s.workerCount)
	resp, err := s.service.Ingest(ctx, magma.IngestRequest{
		ID:        "belief:" + item.ID,
		Tenant:    fmt.Sprintf("user:%d", item.TenantID),
		SessionID: firstNonEmptyString(episode.SessionID, item.ScopeID),
		Text:      beliefMagmaText(episode, item),
		CreatedAt: firstNonZeroTime(item.UpdatedAt, item.CreatedAt, timeFromPointer(episode.EndedAt)),
		Metadata: map[string]any{
			"source":           "belief_memory",
			"belief_id":        item.ID,
			"episode_id":       episode.ID,
			"scope_id":         item.ScopeID,
			"kind":             string(item.Kind),
			"enforcement":      string(item.Enforcement),
			"confidence":       item.Confidence,
			"evidence_for":     item.EvidenceFor,
			"evidence_against": item.EvidenceAgainst,
			"status":           string(item.Status),
			"review_state":     string(item.ReviewState),
			"source_quality":   item.SourceQuality,
		},
	})
	if err != nil {
		return "", err
	}
	return resp.EventID, nil
}

func beliefMagmaText(episode belief.Episode, item belief.Belief) string {
	var text strings.Builder
	writeEvolvingMagmaLine(&text, "Belief", item.Statement)
	writeEvolvingMagmaLine(&text, "Kind", string(item.Kind))
	writeEvolvingMagmaLine(&text, "Enforcement", string(item.Enforcement))
	writeEvolvingMagmaLine(&text, "Confidence", fmt.Sprintf("%.2f", item.Confidence))
	writeEvolvingMagmaLine(&text, "Evidence", fmt.Sprintf("+%d/-%d", item.EvidenceFor, item.EvidenceAgainst))
	writeEvolvingMagmaLine(&text, "Status", string(item.Status))
	writeEvolvingMagmaLine(&text, "Scope", item.ScopeID)
	writeEvolvingMagmaLine(&text, "Episode", episode.ID)
	writeEvolvingMagmaLine(&text, "Project", episode.ProjectID)
	writeEvolvingMagmaLine(&text, "Objective", episode.ObjectiveID)
	return strings.TrimSpace(text.String())
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func timeFromPointer(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
