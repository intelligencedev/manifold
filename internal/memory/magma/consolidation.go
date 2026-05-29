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
	return ResolveEntitiesForTenant(text, "")
}

func ResolveEntitiesForTenant(text, tenant string) []EntityMention {
	matches := entityTokenRE.FindAllString(text, -1)
	seen := map[string]bool{}
	entities := make([]EntityMention, 0, len(matches))
	for _, name := range matches {
		if slices.Contains([]string{"I", "The", "This", "That", "When", "What", "Who", "Why", "Yesterday", "Tomorrow"}, name) {
			continue
		}
		id := entityID(tenant, name)
		if seen[id] {
			continue
		}
		seen[id] = true
		entities = append(entities, EntityMention{ID: id, Type: "Entity", Name: name})
	}
	return entities
}

func entityID(tenant, name string) string {
	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if tenant == "" {
		tenant = "default"
	}
	name = strings.ToLower(strings.TrimSpace(name))
	return "entity:" + tenant + ":" + name
}

func ExtractCausalEdges(event EventNode) []Edge {
	cause, effect, ok := ExtractCausalStatement(event.Text)
	if !ok {
		return nil
	}
	return []Edge{{
		Source:    event.ID,
		GraphType: GraphCausal,
		Rel:       "CAUSES",
		Target:    event.ID,
		Props:     map[string]any{"confidence": 0.5, "extractor": "rule", "cause_text": cause, "effect_text": effect},
	}}
}

func ExtractCausalStatement(text string) (cause string, effect string, ok bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	if i := strings.Index(lower, " because "); i >= 0 {
		effect = strings.TrimSpace(trimmed[:i])
		cause = strings.TrimSpace(trimmed[i+9:])
		return cause, effect, cause != "" && effect != ""
	}
	if i := strings.Index(lower, " caused "); i >= 0 {
		cause = strings.TrimSpace(trimmed[:i])
		effect = strings.TrimSpace(trimmed[i+8:])
		return cause, effect, cause != "" && effect != ""
	}
	if i := strings.Index(lower, " so "); i >= 0 {
		cause = strings.TrimSpace(trimmed[:i])
		effect = strings.TrimSpace(trimmed[i+4:])
		return cause, effect, cause != "" && effect != ""
	}
	return "", "", false
}

func (s *Service) semanticEdges(ctx context.Context, event EventNode) []Edge {
	if s == nil || s.vector == nil || len(event.Embedding) == 0 {
		return nil
	}
	results, err := s.vector.SimilaritySearch(ctx, event.Embedding, s.cfg.SemanticTopK, map[string]string{"tenant": event.Tenant})
	if err != nil {
		return nil
	}
	edges := make([]Edge, 0, len(results)*2)
	for _, result := range results {
		if result.ID == event.ID || result.Score < s.cfg.SimilarityThreshold {
			continue
		}
		edges = append(edges,
			Edge{Source: event.ID, GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: result.ID, Weight: result.Score},
			Edge{Source: result.ID, GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: event.ID, Weight: result.Score},
		)
	}
	return edges
}

func (s *Service) temporalEdges(ctx context.Context, event EventNode, attrs TemporalAttrs) []Edge {
	if event.ID == "" {
		return nil
	}
	edges := []Edge{{
		Source:    event.ID,
		GraphType: GraphTemporal,
		Rel:       "CONCURRENT",
		Target:    event.ID,
		Props:     map[string]any{"date": firstNonEmpty(attrs.Date, event.CreatedAt.Format(time.DateOnly))},
	}}
	if s == nil || s.vector == nil || len(event.Embedding) == 0 {
		return edges
	}
	results, err := s.vector.SimilaritySearch(ctx, event.Embedding, 50, map[string]string{"tenant": event.Tenant})
	if err != nil {
		return edges
	}
	sourceTime := eventTemporalTime(event, attrs)
	for _, result := range results {
		if result.ID == event.ID {
			continue
		}
		other, ok := s.store.GetEvent(ctx, result.ID)
		if !ok {
			continue
		}
		targetTime := eventTemporalTime(other, other.TemporalAttrs)
		switch {
		case sourceTime.Before(targetTime):
			edges = append(edges,
				Edge{Source: event.ID, GraphType: GraphTemporal, Rel: "BEFORE", Target: other.ID},
				Edge{Source: other.ID, GraphType: GraphTemporal, Rel: "AFTER", Target: event.ID},
			)
		case sourceTime.After(targetTime):
			edges = append(edges,
				Edge{Source: event.ID, GraphType: GraphTemporal, Rel: "AFTER", Target: other.ID},
				Edge{Source: other.ID, GraphType: GraphTemporal, Rel: "BEFORE", Target: event.ID},
			)
		default:
			edges = append(edges,
				Edge{Source: event.ID, GraphType: GraphTemporal, Rel: "CONCURRENT", Target: other.ID},
				Edge{Source: other.ID, GraphType: GraphTemporal, Rel: "CONCURRENT", Target: event.ID},
			)
		}
	}
	return edges
}

func eventTemporalTime(event EventNode, attrs TemporalAttrs) time.Time {
	if attrs.Date != "" {
		if t, err := time.Parse(time.DateOnly, attrs.Date); err == nil {
			return t
		}
	}
	if !event.CreatedAt.IsZero() {
		return event.CreatedAt
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
