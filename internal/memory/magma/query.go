package magma

import (
	"context"
	"sort"
	"strings"
)

type QueryEngine struct {
	Service *Service
}

func ClassifyIntent(query string) (IntentCategory, float64) {
	lower := strings.ToLower(query)
	var intent IntentCategory
	if strings.Contains(lower, "when") || strings.Contains(lower, "before") || strings.Contains(lower, "after") {
		intent |= IntentTemporal
	}
	if strings.Contains(lower, "who") || strings.Contains(lower, "friend") || strings.Contains(lower, "instrument") || strings.Contains(lower, "person") {
		intent |= IntentEntity
	}
	if strings.Contains(lower, "why") || strings.Contains(lower, "caused") || strings.Contains(lower, "because") {
		intent |= IntentCausal
	}
	if intent == 0 || strings.Contains(lower, "about") || strings.Contains(lower, "talking") {
		intent |= IntentSemantic
	}
	return intent, 0.8
}

func SelectPolicy(intent IntentCategory) TraversalPolicy {
	if intent == 0 || intent == IntentSemantic {
		return TraversalPolicy{Intent: intent, GraphViews: []GraphType{GraphSemantic}, MaxHops: 2, MaxNodes: 10, AnchorStrategy: AnchorVector}
	}

	views := make([]GraphType, 0, 4)
	maxHops := 2
	maxNodes := 10
	anchorStrategy := AnchorVector
	if intent&IntentCausal != 0 {
		views = appendGraphViews(views, GraphCausal, GraphTemporal, GraphSemantic)
		maxHops = max(maxHops, 3)
	}
	if intent&IntentTemporal != 0 {
		views = appendGraphViews(views, GraphTemporal, GraphSemantic)
	}
	if intent&IntentEntity != 0 {
		views = appendGraphViews(views, GraphEntity, GraphSemantic)
		maxHops = max(maxHops, 2)
		maxNodes = max(maxNodes, 10)
		anchorStrategy = AnchorEntity
	}
	if intent&IntentSemantic != 0 {
		views = appendGraphViews(views, GraphSemantic)
	}
	return TraversalPolicy{Intent: intent, GraphViews: views, MaxHops: maxHops, MaxNodes: maxNodes, AnchorStrategy: anchorStrategy}
}

func (q QueryEngine) Query(ctx context.Context, query string, opt QueryOptions) (StructuredContext, error) {
	if q.Service == nil || q.Service.store == nil {
		return StructuredContext{}, nil
	}
	intent := opt.IntentHint
	if intent == 0 {
		intent, _ = ClassifyIntent(query)
	}
	policy := SelectPolicy(intent)
	if opt.MaxHops > 0 {
		policy.MaxHops = opt.MaxHops
	}
	if opt.MaxNodes > 0 {
		policy.MaxNodes = opt.MaxNodes
	}
	anchors, err := q.anchors(ctx, query, opt.Tenant, policy)
	if err != nil {
		return StructuredContext{}, err
	}
	subgraphs := map[GraphType]Subgraph{}
	for _, view := range policy.GraphViews {
		subgraphs[view] = q.traverse(ctx, anchors, view, policy)
	}
	return BuildContext(subgraphs), nil
}

func (q QueryEngine) anchors(ctx context.Context, query, tenant string, policy TraversalPolicy) ([]string, error) {
	limit := policy.MaxNodes
	if limit <= 0 {
		limit = 10
	}
	anchors := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, entity := range ResolveEntities(query) {
		neighbors, err := q.Service.store.Neighbors(ctx, entity.ID, GraphEntity, "MENTIONS")
		if err != nil {
			return nil, err
		}
		for _, neighbor := range neighbors {
			if seen[neighbor] {
				continue
			}
			seen[neighbor] = true
			anchors = append(anchors, neighbor)
			if len(anchors) >= limit {
				return anchors, nil
			}
		}
	}

	if q.Service.vector == nil {
		return anchors, nil
	}
	vec, err := q.Service.embed(ctx, query)
	if err != nil {
		return nil, err
	}
	results, err := q.Service.vector.SimilaritySearch(ctx, vec, limit, map[string]string{"tenant": tenant})
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		if result.Metadata["kind"] != "magma_event" || seen[result.ID] {
			continue
		}
		seen[result.ID] = true
		anchors = append(anchors, result.ID)
		if len(anchors) >= limit {
			break
		}
	}
	return anchors, nil
}

func (q QueryEngine) traverse(ctx context.Context, anchors []string, graphType GraphType, policy TraversalPolicy) Subgraph {
	subgraph := Subgraph{GraphType: graphType, Nodes: map[string]EventNode{}}
	frontier := append([]string(nil), anchors...)
	seen := map[string]bool{}
	rels := relsForGraph(graphType)
	for hop := 0; hop <= policy.MaxHops && len(frontier) > 0 && len(subgraph.Nodes) < policy.MaxNodes; hop++ {
		next := []string{}
		for _, id := range frontier {
			if seen[id] || len(subgraph.Nodes) >= policy.MaxNodes {
				continue
			}
			seen[id] = true
			if event, ok := q.Service.store.GetEvent(ctx, id); ok {
				subgraph.Nodes[id] = event
			}
			for _, rel := range rels {
				neighbors, err := q.Service.store.Neighbors(ctx, id, graphType, rel)
				if err != nil {
					continue
				}
				for _, neighbor := range neighbors {
					subgraph.Edges = append(subgraph.Edges, Edge{Source: id, GraphType: graphType, Rel: rel, Target: neighbor})
					if !seen[neighbor] {
						next = append(next, neighbor)
					}
				}
			}
		}
		frontier = next
	}
	return subgraph
}

func BuildContext(subgraphs map[GraphType]Subgraph) StructuredContext {
	eventsByID := map[string]EventNode{}
	for _, subgraph := range subgraphs {
		for id, event := range subgraph.Nodes {
			eventsByID[id] = event
		}
	}
	events := make([]EventNode, 0, len(eventsByID))
	for _, event := range eventsByID {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })

	ctx := StructuredContext{RawEvents: events, EntityProfile: map[string]EntityProfile{}}
	var text strings.Builder
	for _, event := range events {
		date := event.TemporalAttrs.Date
		if date == "" && !event.CreatedAt.IsZero() {
			date = event.CreatedAt.Format("2006-01-02")
		}
		ctx.TemporalTimeline = append(ctx.TemporalTimeline, TimelineEntry{EventID: event.ID, Date: date, Text: event.Text})
		if date != "" {
			text.WriteString("- ")
			text.WriteString(date)
			text.WriteString(": ")
		} else {
			text.WriteString("- ")
		}
		text.WriteString(event.Text)
		text.WriteByte('\n')
		for _, entity := range event.EntityMentions {
			profile := ctx.EntityProfile[entity.ID]
			profile.Entity = entity
			profile.Events = append(profile.Events, event)
			ctx.EntityProfile[entity.ID] = profile
		}
	}
	if len(events) > 0 {
		ctx.SemanticCluster = append(ctx.SemanticCluster, SemanticGroup{Topic: "related events", Events: events})
	}
	for _, subgraph := range subgraphs {
		for _, edge := range subgraph.Edges {
			if edge.GraphType == GraphCausal && edge.Rel == "CAUSES" {
				ctx.CausalChain = append(ctx.CausalChain, CausalLink{Cause: edge.Source, Effect: edge.Target})
			}
		}
	}
	ctx.Text = strings.TrimSpace(text.String())
	return ctx
}

func relsForGraph(graphType GraphType) []string {
	switch graphType {
	case GraphSemantic:
		return []string{"SIMILAR_TO"}
	case GraphTemporal:
		return []string{"BEFORE", "AFTER", "CONCURRENT"}
	case GraphCausal:
		return []string{"CAUSES", "EFFECT_OF"}
	case GraphEntity:
		return []string{"MENTIONS", "RELATED_TO"}
	default:
		return nil
	}
}

func appendGraphViews(views []GraphType, add ...GraphType) []GraphType {
	seen := make(map[GraphType]bool, len(views)+len(add))
	for _, view := range views {
		seen[view] = true
	}
	for _, view := range add {
		if seen[view] {
			continue
		}
		seen[view] = true
		views = append(views, view)
	}
	return views
}
