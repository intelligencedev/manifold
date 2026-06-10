package magma

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
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
		"metadata":        mustJSON(event.Metadata),
		"graphs":          mustJSON(event.Graphs),
		"semantic_top_k":  event.SemanticTopK,
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
	if !ok || !slices.Contains(node.Labels, "MagmaEvent") {
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
	if !req.SkipEntityLinks {
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
	return databases.TypedUpsertEdge(ctx, s.graph, databases.TypedEdgeInput{
		Source:    edge.Source,
		GraphType: string(edge.GraphType),
		Rel:       edge.Rel,
		Target:    edge.Target,
		Props:     props,
	})
}

func (s *Store) Neighbors(ctx context.Context, id string, graphType GraphType, rel string) ([]string, error) {
	if s == nil || s.graph == nil {
		return nil, nil
	}
	return databases.TypedNeighbors(ctx, s.graph, id, string(graphType), rel)
}

func (s *Store) NeighborEdges(ctx context.Context, id string, graphType GraphType, rel string) ([]Edge, error) {
	edges, err := s.ListEdges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Edge, 0)
	for _, edge := range edges {
		if edge.Source == id && edge.GraphType == graphType && edge.Rel == rel {
			out = append(out, edge)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out, nil
}

func (s *Store) ListEdges(ctx context.Context) ([]Edge, error) {
	if s == nil || s.graph == nil {
		return nil, nil
	}
	maintenance, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	if !ok {
		return nil, nil
	}
	stored, err := maintenance.ListMagmaEdges(ctx)
	if err != nil {
		return nil, err
	}
	edges := make([]Edge, 0, len(stored))
	for _, edge := range stored {
		edges = append(edges, Edge{
			Source:    edge.Source,
			GraphType: GraphType(edge.GraphType),
			Rel:       edge.Rel,
			Target:    edge.Target,
			Weight:    edge.Weight,
			Props:     cloneAnyMap(edge.Props),
		})
	}
	return edges, nil
}

func (s *Store) DeleteEdge(ctx context.Context, selector EdgeSelector) error {
	if s == nil || s.graph == nil {
		return nil
	}
	maintenance, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	if !ok {
		return nil
	}
	return maintenance.DeleteMagmaEdge(ctx, selector.Source, string(selector.GraphType), selector.Rel, selector.Target)
}

func (s *Store) UpdateEdgeProps(ctx context.Context, edge Edge) error {
	if s == nil || s.graph == nil {
		return nil
	}
	maintenance, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	if !ok {
		return nil
	}
	return maintenance.UpsertMagmaEdgeProps(ctx, databases.TypedEdge{
		Source:    edge.Source,
		GraphType: string(edge.GraphType),
		Rel:       edge.Rel,
		Target:    edge.Target,
		Weight:    edge.Weight,
		Props:     cloneAnyMap(edge.Props),
	})
}

func (s *Store) ListEvents(ctx context.Context) ([]EventNode, error) {
	if s == nil || s.graph == nil {
		return nil, nil
	}
	maintenance, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	if !ok {
		return nil, nil
	}
	summaries, err := maintenance.ListMagmaEvents(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]EventNode, 0, len(summaries))
	for _, summary := range summaries {
		event, ok := s.GetEvent(ctx, summary.ID)
		if !ok {
			event = EventNode{ID: summary.ID, Tenant: summary.Tenant, Session: summary.Session, CreatedAt: summary.CreatedAt}
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) DeleteEvent(ctx context.Context, id string) error {
	if s == nil || s.graph == nil {
		return nil
	}
	maintenance, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	if !ok {
		return nil
	}
	return maintenance.DeleteMagmaEvent(ctx, id)
}

func (s *Store) ArchiveEvent(ctx context.Context, event EventNode, reason string) error {
	if s == nil || s.graph == nil || strings.TrimSpace(event.ID) == "" {
		return nil
	}
	archivedAt := time.Now().UTC()
	props := map[string]any{
		"archive_kind": "event",
		"original_id":  event.ID,
		"tenant":       event.Tenant,
		"session":      event.Session,
		"reason":       strings.TrimSpace(reason),
		"payload":      mustJSON(event),
		"archived_at":  archivedAt.Format(time.RFC3339Nano),
	}
	return s.graph.UpsertNode(ctx, magmaArchiveID("event", event.ID, archivedAt), []string{"MagmaArchive"}, props)
}

func (s *Store) ArchiveEdge(ctx context.Context, edge Edge, reason string) error {
	if s == nil || s.graph == nil || strings.TrimSpace(edge.Source) == "" || strings.TrimSpace(edge.Target) == "" {
		return nil
	}
	archivedAt := time.Now().UTC()
	selector := selectorForEdge(edge)
	originalID := strings.Join([]string{selector.Source, string(selector.GraphType), selector.Rel, selector.Target}, "\x00")
	props := map[string]any{
		"archive_kind": "edge",
		"original_id":  originalID,
		"source":       edge.Source,
		"graph_type":   string(edge.GraphType),
		"rel":          edge.Rel,
		"target":       edge.Target,
		"reason":       strings.TrimSpace(reason),
		"payload":      mustJSON(edge),
		"archived_at":  archivedAt.Format(time.RFC3339Nano),
	}
	return s.graph.UpsertNode(ctx, magmaArchiveID("edge", originalID, archivedAt), []string{"MagmaArchive"}, props)
}

func (s *Store) MaintenanceEnabled() bool {
	if s == nil || s.graph == nil {
		return false
	}
	_, ok := s.graph.(databases.MagmaGraphMaintenanceDB)
	return ok
}

func magmaArchiveID(kind, original string, archivedAt time.Time) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + original + "\x00" + archivedAt.Format(time.RFC3339Nano)))
	return fmt.Sprintf("magma_archive:%s:%x", kind, sum[:12])
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
	event.SemanticTopK = intProp(node.Props["semantic_top_k"])
	if s, ok := node.Props["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			event.CreatedAt = t
		}
	}
	decodeJSON(node.Props["graphs"], &event.Graphs)
	decodeJSON(node.Props["metadata"], &event.Metadata)
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

func intProp(raw any) int {
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
