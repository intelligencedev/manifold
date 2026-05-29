package service

import (
	"context"
	"time"

	"manifold/internal/memory/magma"
	"manifold/internal/rag/retrieve"
)

func (s *Service) retrieveMagma(ctx context.Context, q string, opt retrieve.RetrieveOptions, start time.Time) (retrieve.RetrieveResponse, error) {
	magmaCtx, err := s.magma.Query(ctx, q, magma.QueryOptions{
		Tenant:               opt.Tenant,
		IntentHint:           parseIntentHint(opt.Magma.IntentHint),
		MaxHops:              opt.Magma.MaxHops,
		MaxNodes:             opt.Magma.MaxNodes,
		ContextFormat:        opt.Magma.ContextFormat,
		IntentClassification: opt.Magma.IntentClassification,
	})
	if err != nil {
		return retrieve.RetrieveResponse{}, err
	}
	items := make([]retrieve.RetrievedItem, 0, len(magmaCtx.RawEvents))
	for i, event := range magmaCtx.RawEvents {
		score := float64(len(magmaCtx.RawEvents) - i)
		item := retrieve.RetrievedItem{
			ID:    event.ID,
			DocID: event.ID,
			Score: score,
			Text:  event.Text,
			Metadata: map[string]string{
				"tenant":  event.Tenant,
				"session": event.Session,
				"source":  "magma",
			},
			Explanation: map[string]any{"source": "magma"},
		}
		if !opt.IncludeText {
			item.Text = ""
			item.Snippet = event.Text
		}
		items = append(items, item)
	}
	totalMS := ms(s.clock.Now().Sub(start))
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(totalMS), map[string]string{"stage": "magma_total", "tenant": opt.Tenant})
	return retrieve.RetrieveResponse{
		Query: q,
		Items: items,
		Debug: map[string]any{
			"magma": map[string]any{
				"enabled":        true,
				"context":        magmaCtx.Text,
				"context_format": opt.Magma.ContextFormat,
				"events":         len(magmaCtx.RawEvents),
			},
			"diagnostics": map[string]any{"total_ms": totalMS},
		},
	}, nil
}

func parseIntentHint(hint string) magma.IntentCategory {
	switch hint {
	case "temporal":
		return magma.IntentTemporal
	case "entity":
		return magma.IntentEntity
	case "causal":
		return magma.IntentCausal
	case "semantic":
		return magma.IntentSemantic
	default:
		return 0
	}
}
