package agentd

import (
	"context"
	"fmt"
	"strings"

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
