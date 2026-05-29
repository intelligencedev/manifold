package service

import (
	"context"
	"strings"
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
	labels := map[string]string{"tenant": opt.Tenant, "intent": magmaCtx.Intent.String()}
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(totalMS), map[string]string{"stage": "magma_total", "tenant": opt.Tenant})
	s.metrics.ObserveHistogram("magma_query_ms", float64(totalMS), labels)
	s.metrics.ObserveHistogram("magma_traversal_hops", float64(magmaCtx.MaxHops), labels)
	s.metrics.ObserveHistogram("magma_context_tokens", float64(approxTokens(magmaCtx.Text)), labels)
	s.metrics.IncCounter("magma_intent_distribution", labels)
	return retrieve.RetrieveResponse{
		Query: q,
		Items: items,
		Debug: map[string]any{
			"magma": map[string]any{
				"enabled":        true,
				"context":        magmaCtx.Text,
				"context_format": opt.Magma.ContextFormat,
				"events":         len(magmaCtx.RawEvents),
				"intent":         magmaCtx.Intent.String(),
				"graphs":         graphViewLabels(magmaCtx.GraphViews),
				"anchor":         string(magmaCtx.AnchorStrategy),
				"anchors":        magmaCtx.AnchorCount,
				"max_hops":       magmaCtx.MaxHops,
				"max_nodes":      magmaCtx.MaxNodes,
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

func graphViewLabels(views []magma.GraphType) string {
	if len(views) == 0 {
		return ""
	}
	labels := make([]string, 0, len(views))
	for _, view := range views {
		labels = append(labels, string(view))
	}
	return strings.Join(labels, ",")
}
