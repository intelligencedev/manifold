package magma

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

func (s *Service) Prune(ctx context.Context, policy LifecyclePolicy) (stats LifecycleStats, err error) {
	ctx, span := startSpan(ctx, "magma.prune",
		attribute.Bool("magma.lifecycle.ttl_enabled", policy.EventTTL > 0),
		attribute.Int("magma.lifecycle.max_edges_per_source_rel", policy.MaxEdgesPerSourceRel),
		attribute.Float64("magma.lifecycle.min_semantic_weight", policy.MinSemanticWeight),
		attribute.Float64("magma.lifecycle.low_confidence_threshold", policy.LowConfidenceThreshold),
	)
	defer endSpan(span, &err)

	if s == nil || s.store == nil {
		return LifecycleStats{}, errors.New("magma service is not configured")
	}
	if !s.store.MaintenanceEnabled() {
		return LifecycleStats{MaintenanceDisabled: true}, nil
	}
	if policy == (LifecyclePolicy{}) {
		policy = s.cfg.Lifecycle
	}
	if err := s.pruneExpiredEvents(ctx, policy, &stats); err != nil {
		return stats, err
	}
	if err := s.pruneEdges(ctx, policy, &stats); err != nil {
		return stats, err
	}
	s.observeHistogram("magma_lifecycle_events_deleted", float64(stats.EventsDeleted), map[string]string{})
	s.observeHistogram("magma_lifecycle_edges_deleted", float64(stats.EdgesDeleted), map[string]string{})
	s.observeHistogram("magma_lifecycle_edges_flagged_review", float64(stats.EdgesFlaggedReview), map[string]string{})
	return stats, nil
}

func (s *Service) ReviewEdges(ctx context.Context) ([]ReviewEdge, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("magma service is not configured")
	}
	edges, err := s.store.ListEdges(ctx)
	if err != nil {
		return nil, err
	}
	review := make([]ReviewEdge, 0)
	for _, edge := range edges {
		state := stringProp(edge.Props, "review_state")
		if state == "" || state == "approved" {
			continue
		}
		review = append(review, ReviewEdge{
			Edge:        edge,
			ReviewState: state,
			Reason:      stringProp(edge.Props, "review_reason"),
		})
	}
	sort.Slice(review, func(i, j int) bool {
		if review[i].Source != review[j].Source {
			return review[i].Source < review[j].Source
		}
		return review[i].Target < review[j].Target
	})
	return review, nil
}

func (s *Service) ApproveEdge(ctx context.Context, selector EdgeSelector, reviewer string) error {
	var err error
	ctx, span := startSpan(ctx, "magma.approve_edge",
		attribute.String("magma.graph_type", string(selector.GraphType)),
		attribute.String("magma.edge.rel", selector.Rel),
	)
	defer endSpan(span, &err)
	err = s.updateSelectedEdge(ctx, selector, func(edge Edge) Edge {
		if edge.Props == nil {
			edge.Props = map[string]any{}
		}
		edge.Props["review_state"] = "approved"
		edge.Props["reviewed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		if strings.TrimSpace(reviewer) != "" {
			edge.Props["reviewed_by"] = strings.TrimSpace(reviewer)
		}
		return edge
	})
	return err
}

func (s *Service) RetractEdge(ctx context.Context, selector EdgeSelector, reason string) error {
	var err error
	ctx, span := startSpan(ctx, "magma.retract_edge",
		attribute.String("magma.graph_type", string(selector.GraphType)),
		attribute.String("magma.edge.rel", selector.Rel),
	)
	defer endSpan(span, &err)
	if s == nil || s.store == nil {
		return errors.New("magma service is not configured")
	}
	if err := s.store.DeleteEdge(ctx, selector); err != nil {
		return err
	}
	s.observeHistogram("magma_lifecycle_edges_deleted", 1, map[string]string{"reason": firstNonEmpty(strings.TrimSpace(reason), "manual_retraction")})
	return nil
}

func (s *Service) pruneExpiredEvents(ctx context.Context, policy LifecyclePolicy, stats *LifecycleStats) error {
	if policy.EventTTL <= 0 {
		return nil
	}
	events, err := s.store.ListEvents(ctx)
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-policy.EventTTL)
	for _, event := range events {
		if event.CreatedAt.IsZero() || !event.CreatedAt.Before(cutoff) {
			continue
		}
		if err := s.store.DeleteEvent(ctx, event.ID); err != nil {
			return err
		}
		if s.vector != nil {
			_ = s.vector.Delete(ctx, event.ID)
		}
		stats.EventsDeleted++
	}
	return nil
}

func (s *Service) pruneEdges(ctx context.Context, policy LifecyclePolicy, stats *LifecycleStats) error {
	edges, err := s.store.ListEdges(ctx)
	if err != nil {
		return err
	}
	grouped := make(map[string][]Edge)
	for _, edge := range edges {
		if shouldDeleteByWeight(edge, policy) {
			if err := s.store.DeleteEdge(ctx, selectorForEdge(edge)); err != nil {
				return err
			}
			stats.EdgesDeleted++
			continue
		}
		if shouldFlagForReview(edge, policy) {
			edge.Props = cloneAnyMap(edge.Props)
			if edge.Props == nil {
				edge.Props = map[string]any{}
			}
			if edge.Props["review_state"] == nil {
				edge.Props["review_state"] = "needs_review"
				edge.Props["review_reason"] = "low_confidence"
				edge.Props["flagged_at"] = time.Now().UTC().Format(time.RFC3339Nano)
				if err := s.store.UpdateEdgeProps(ctx, edge); err != nil {
					return err
				}
				stats.EdgesFlaggedReview++
			}
		}
		grouped[edgeGroupKey(edge)] = append(grouped[edgeGroupKey(edge)], edge)
	}
	if policy.MaxEdgesPerSourceRel <= 0 {
		return nil
	}
	for _, group := range grouped {
		if len(group) <= policy.MaxEdgesPerSourceRel {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool { return edgeRank(group[i]) > edgeRank(group[j]) })
		for _, edge := range group[policy.MaxEdgesPerSourceRel:] {
			if err := s.store.DeleteEdge(ctx, selectorForEdge(edge)); err != nil {
				return err
			}
			stats.EdgesDeleted++
		}
	}
	return nil
}

func (s *Service) updateSelectedEdge(ctx context.Context, selector EdgeSelector, update func(Edge) Edge) error {
	if s == nil || s.store == nil {
		return errors.New("magma service is not configured")
	}
	edges, err := s.store.ListEdges(ctx)
	if err != nil {
		return err
	}
	for _, edge := range edges {
		if selectorForEdge(edge) == selector {
			return s.store.UpdateEdgeProps(ctx, update(edge))
		}
	}
	return errors.New("magma edge not found")
}

func edgeGroupKey(edge Edge) string {
	return edge.Source + "\x00" + string(edge.GraphType) + "\x00" + edge.Rel
}

func selectorForEdge(edge Edge) EdgeSelector {
	return EdgeSelector{Source: edge.Source, GraphType: edge.GraphType, Rel: edge.Rel, Target: edge.Target}
}

func shouldDeleteByWeight(edge Edge, policy LifecyclePolicy) bool {
	return policy.MinSemanticWeight > 0 && edge.GraphType == GraphSemantic && edgeRank(edge) > 0 && edgeRank(edge) < policy.MinSemanticWeight
}

func shouldFlagForReview(edge Edge, policy LifecyclePolicy) bool {
	return policy.LowConfidenceThreshold > 0 && edgeConfidence(edge) > 0 && edgeConfidence(edge) < policy.LowConfidenceThreshold
}

func edgeRank(edge Edge) float64 {
	if edge.Weight != 0 {
		return edge.Weight
	}
	return floatProp(edge.Props, "weight")
}

func edgeConfidence(edge Edge) float64 {
	return floatProp(edge.Props, "confidence")
}

func edgeReviewState(edge Edge) string {
	return stringProp(edge.Props, "review_state")
}

func stringProp(props map[string]any, key string) string {
	if value, ok := props[key].(string); ok {
		return value
	}
	return ""
}

func floatProp(props map[string]any, key string) float64 {
	switch value := props[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}
