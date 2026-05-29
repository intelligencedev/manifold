package magma

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"manifold/internal/persistence/databases"
)

type Store struct {
	graph databases.GraphDB
}

func NewStore(graph databases.GraphDB) *Store {
	return &Store{graph: graph}
}

func (s *Store) StoreEvent(ctx context.Context, event EventNode) error {
	if s == nil || s.graph == nil {
		return nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	props := map[string]any{
		"tenant":          event.Tenant,
		"session":         event.Session,
		"text":            event.Text,
		"graphs":          mustJSON(event.Graphs),
		"created_at":      event.CreatedAt.Format(time.RFC3339Nano),
		"temporal_attrs":  mustJSON(event.TemporalAttrs),
		"entity_mentions": mustJSON(event.EntityMentions),
	}
	return s.graph.UpsertNode(ctx, event.ID, []string{"MagmaEvent"}, props)
}

func (s *Store) GetEvent(ctx context.Context, id string) (EventNode, bool) {
	if s == nil || s.graph == nil {
		return EventNode{}, false
	}
	node, ok := s.graph.GetNode(ctx, id)
	if !ok {
		return EventNode{}, false
	}
	return eventFromNode(node), true
}

func (s *Store) BatchUpsert(ctx context.Context, req BatchUpsertRequest) error {
	event := req.Event
	if req.TemporalAttrs != (TemporalAttrs{}) {
		event.TemporalAttrs = req.TemporalAttrs
	}
	if len(req.Entities) > 0 {
		event.EntityMentions = req.Entities
	}
	if err := s.StoreEvent(ctx, event); err != nil {
		return err
	}
	for _, entity := range req.Entities {
		if err := s.StoreEntity(ctx, entity, event.Tenant); err != nil {
			return err
		}
		if err := s.UpsertEdge(ctx, Edge{Source: event.ID, GraphType: GraphEntity, Rel: "MENTIONS", Target: entity.ID}); err != nil {
			return err
		}
		if err := s.UpsertEdge(ctx, Edge{Source: entity.ID, GraphType: GraphEntity, Rel: "MENTIONS", Target: event.ID}); err != nil {
			return err
		}
	}
	for i := range req.Entities {
		for j := i + 1; j < len(req.Entities); j++ {
			left := req.Entities[i].ID
			right := req.Entities[j].ID
			if left == "" || right == "" || left == right {
				continue
			}
			if err := s.UpsertEdge(ctx, Edge{Source: left, GraphType: GraphEntity, Rel: "RELATED_TO", Target: right}); err != nil {
				return err
			}
			if err := s.UpsertEdge(ctx, Edge{Source: right, GraphType: GraphEntity, Rel: "RELATED_TO", Target: left}); err != nil {
				return err
			}
		}
	}
	for _, edge := range req.Edges {
		if err := s.UpsertEdge(ctx, edge); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) StoreEntity(ctx context.Context, entity EntityMention, tenant string) error {
	if s == nil || s.graph == nil || entity.ID == "" {
		return nil
	}
	props := map[string]any{
		"tenant":         tenant,
		"type":           entity.Type,
		"canonical_name": entity.Name,
		"role":           entity.Role,
	}
	return s.graph.UpsertNode(ctx, entity.ID, []string{"MagmaEntity"}, props)
}

func (s *Store) UpsertEdge(ctx context.Context, edge Edge) error {
	if s == nil || s.graph == nil {
		return nil
	}
	props := map[string]any{}
	for k, v := range edge.Props {
		props[k] = v
	}
	if edge.Weight != 0 {
		props["weight"] = edge.Weight
	}
	return databases.TypedUpsertEdge(ctx, s.graph, edge.Source, string(edge.GraphType), edge.Rel, edge.Target, props)
}

func (s *Store) Neighbors(ctx context.Context, id string, graphType GraphType, rel string) ([]string, error) {
	if s == nil || s.graph == nil {
		return nil, nil
	}
	return databases.TypedNeighbors(ctx, s.graph, id, string(graphType), rel)
}

func eventFromNode(node databases.Node) EventNode {
	event := EventNode{ID: node.ID}
	if s, ok := node.Props["tenant"].(string); ok {
		event.Tenant = s
	}
	if s, ok := node.Props["session"].(string); ok {
		event.Session = s
	}
	if s, ok := node.Props["text"].(string); ok {
		event.Text = s
	}
	if s, ok := node.Props["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			event.CreatedAt = t
		}
	}
	decodeJSON(node.Props["graphs"], &event.Graphs)
	decodeJSON(node.Props["temporal_attrs"], &event.TemporalAttrs)
	decodeJSON(node.Props["entity_mentions"], &event.EntityMentions)
	return event
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeJSON(raw any, dst any) {
	switch v := raw.(type) {
	case string:
		_ = json.Unmarshal([]byte(v), dst)
	case []byte:
		_ = json.Unmarshal(v, dst)
	}
}

func eventID(tenant, sourceID string) string {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		sourceID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if strings.HasPrefix(sourceID, "event:") {
		return sourceID
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}
	return "event:" + tenant + ":" + sourceID
}
