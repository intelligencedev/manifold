package magma

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"
)

var entityTokenRE = regexp.MustCompile(`\b[A-Z][a-zA-Z0-9_-]{2,}\b`)

func NormalizeTemporal(text string, anchor time.Time) TemporalAttrs {
	if anchor.IsZero() {
		anchor = time.Now().UTC()
	}
	lower := strings.ToLower(text)
	date := anchor
	offset := ""
	switch {
	case strings.Contains(lower, "yesterday"):
		date = anchor.AddDate(0, 0, -1)
		offset = "-24h"
	case strings.Contains(lower, "tomorrow"):
		date = anchor.AddDate(0, 0, 1)
		offset = "24h"
	case strings.Contains(lower, "last week"):
		date = anchor.AddDate(0, 0, -7)
		offset = "-168h"
	case strings.Contains(lower, "next week"):
		date = anchor.AddDate(0, 0, 7)
		offset = "168h"
	}
	return TemporalAttrs{Date: date.Format(time.DateOnly), Offset: offset}
}

func ResolveEntities(text string) []EntityMention {
	matches := entityTokenRE.FindAllString(text, -1)
	seen := map[string]bool{}
	entities := make([]EntityMention, 0, len(matches))
	for _, name := range matches {
		if slices.Contains([]string{"I", "The", "This", "That", "When", "What", "Who", "Why", "Yesterday", "Tomorrow"}, name) {
			continue
		}
		id := "entity:" + strings.ToLower(name)
		if seen[id] {
			continue
		}
		seen[id] = true
		entities = append(entities, EntityMention{ID: id, Type: "Entity", Name: name})
	}
	return entities
}

func ExtractCausalEdges(event EventNode) []Edge {
	lower := strings.ToLower(event.Text)
	if !strings.Contains(lower, "because") && !strings.Contains(lower, "caused") && !strings.Contains(lower, "so ") {
		return nil
	}
	return []Edge{{
		Source:    event.ID,
		GraphType: GraphCausal,
		Rel:       "CAUSES",
		Target:    event.ID,
		Props:     map[string]any{"confidence": 0.5, "extractor": "rule"},
	}}
}

func (s *Service) semanticEdges(ctx context.Context, event EventNode) []Edge {
	if s == nil || s.vector == nil || len(event.Embedding) == 0 {
		return nil
	}
	results, err := s.vector.SimilaritySearch(ctx, event.Embedding, 20, map[string]string{"tenant": event.Tenant})
	if err != nil {
		return nil
	}
	edges := make([]Edge, 0, len(results)*2)
	for _, result := range results {
		if result.ID == event.ID || result.Score < 0.7 {
			continue
		}
		edges = append(edges,
			Edge{Source: event.ID, GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: result.ID, Weight: result.Score},
			Edge{Source: result.ID, GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: event.ID, Weight: result.Score},
		)
	}
	return edges
}

func temporalEdges(event EventNode) []Edge {
	if event.ID == "" {
		return nil
	}
	return []Edge{{
		Source:    event.ID,
		GraphType: GraphTemporal,
		Rel:       "CONCURRENT",
		Target:    event.ID,
		Props:     map[string]any{"date": event.CreatedAt.Format(time.DateOnly)},
	}}
}
