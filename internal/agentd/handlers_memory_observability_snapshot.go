package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"manifold/internal/agent/memory/magma"
	"manifold/internal/auth"
)

type memoryObservabilityFilter struct {
	Tenant    string
	SessionID string
	GraphType string
	Query     string
	Limit     int
}

func (a *app) memoryObservabilityFilter(r *http.Request) memoryObservabilityFilter {
	tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
	if a == nil || a.cfg == nil || !a.cfg.Auth.Enabled {
		tenant = fmt.Sprintf("user:%d", systemUserID)
	} else if tenant == "" {
		if uid, err := a.memoryObservabilityUserID(r); err == nil && uid > 0 {
			tenant = fmt.Sprintf("user:%d", uid)
		}
	}
	return memoryObservabilityFilter{
		Tenant:    tenant,
		SessionID: strings.TrimSpace(r.URL.Query().Get("sessionId")),
		GraphType: strings.TrimSpace(r.URL.Query().Get("graphType")),
		Query:     strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))),
		Limit:     parseLimitQuery(r, "limit", 200, 1000),
	}
}

func (f memoryObservabilityFilter) event(event magma.EventNode) bool {
	if f.Tenant != "" && event.Tenant != "" && event.Tenant != f.Tenant {
		return false
	}
	if f.SessionID != "" && event.Session != f.SessionID {
		return false
	}
	if f.Query != "" && !strings.Contains(strings.ToLower(event.ID+" "+event.Text), f.Query) {
		return false
	}
	return true
}

func (f memoryObservabilityFilter) edge(edge magma.Edge) bool {
	if f.GraphType != "" && string(edge.GraphType) != f.GraphType {
		return false
	}
	if f.Query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(edge.Source+" "+edge.Target+" "+edge.Rel), f.Query)
}

func (a *app) memoryObservabilityGraphSnapshot(r *http.Request) ([]memoryObservabilityNode, []memoryObservabilityEdge, memoryObservabilityGraphStats, []string, error) {
	ms := a.ragServiceMagma()
	if ms == nil {
		return nil, nil, memoryObservabilityGraphStats{ByType: map[string]int{}}, []string{"MAGMA service is not configured."}, nil
	}
	filter := a.memoryObservabilityFilter(r)
	events, err := ms.Events(r.Context())
	if err != nil {
		return nil, nil, memoryObservabilityGraphStats{}, nil, err
	}
	edgesRaw, err := ms.Edges(r.Context())
	if err != nil {
		return nil, nil, memoryObservabilityGraphStats{}, nil, err
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.After(events[j].CreatedAt) })
	nodesByID := map[string]memoryObservabilityNode{}
	for _, event := range events {
		if !filter.event(event) {
			continue
		}
		nodesByID[event.ID] = memoryObservabilityNodeFromEvent(event)
		if len(nodesByID) >= filter.Limit {
			break
		}
	}
	edgeLimit := parseLimitQuery(r, "edgeLimit", 500, 2000)
	edges := make([]memoryObservabilityEdge, 0, edgeLimit)
	for _, edge := range edgesRaw {
		if !filter.edge(edge) {
			continue
		}
		_, sourceIncluded := nodesByID[edge.Source]
		_, targetIncluded := nodesByID[edge.Target]
		if len(nodesByID) > 0 && !sourceIncluded && !targetIncluded && filter.Query == "" {
			continue
		}
		edges = append(edges, memoryObservabilityEdgeFromMagma(edge))
		if !sourceIncluded {
			nodesByID[edge.Source] = inferredMemoryNode(edge.Source)
		}
		if !targetIncluded {
			nodesByID[edge.Target] = inferredMemoryNode(edge.Target)
		}
		if len(edges) >= edgeLimit {
			break
		}
	}
	nodes := make([]memoryObservabilityNode, 0, len(nodesByID))
	for _, node := range nodesByID {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type < nodes[j].Type
		}
		return nodes[i].ID < nodes[j].ID
	})
	stats := graphStats(nodes, edges)
	warnings := []string{}
	if len(edges) >= edgeLimit {
		warnings = append(warnings, "Graph edge limit reached; narrow filters to inspect more.")
	}
	return nodes, edges, stats, warnings, nil
}

func (a *app) memoryObservabilityGraphStats(r *http.Request) memoryObservabilityGraphStats {
	nodes, edges, stats, _, err := a.memoryObservabilityGraphSnapshot(r)
	if err != nil {
		return memoryObservabilityGraphStats{ByType: map[string]int{}}
	}
	stats.Nodes = len(nodes)
	stats.Edges = len(edges)
	return stats
}

func (a *app) memoryObservabilityMetrics(r *http.Request, uid int64, window time.Duration) (memoryMetricsSnapshot, time.Duration, string) {
	for _, provider := range a.memoryMetrics {
		if provider == nil {
			continue
		}
		snapshot, appliedWindow, err := provider.MemoryMetrics(r.Context(), uid, window)
		if err != nil {
			continue
		}
		return snapshot, appliedWindow, provider.Source()
	}
	return memoryMetricsSnapshot{}, 0, "none"
}

func (a *app) memoryObservabilityUserID(r *http.Request) (int64, error) {
	if a.cfg != nil && a.cfg.Auth.Enabled {
		u, ok := auth.CurrentUser(r.Context())
		if !ok {
			return 0, fmt.Errorf("unauthorized")
		}
		return u.ID, nil
	}
	if value := parseIntQuery(r, "user_id"); value > 0 {
		return int64(value), nil
	}
	return systemUserID, nil
}

func (a *app) memoryObservabilityConfig() memoryObservabilityConfig {
	if a == nil || a.cfg == nil {
		return memoryObservabilityConfig{}
	}
	return memoryObservabilityConfig{
		MemoryEnabled:   a.cfg.Memory.Enabled,
		EvolvingEnabled: a.cfg.EvolvingMemory.Enabled,
		BeliefEnabled:   a.cfg.BeliefMemory.Enabled,
		MagmaEnabled:    a.cfg.Magma.Enabled,
		GraphBackend:    a.cfg.Databases.Graph.Backend,
		VectorBackend:   a.cfg.Databases.Vector.Backend,
	}
}

func (a *app) memoryObservabilityMagmaStats() memoryObservabilityMagmaStats {
	ms := a.ragServiceMagma()
	if ms == nil {
		return memoryObservabilityMagmaStats{}
	}
	stats := ms.Stats()
	return memoryObservabilityMagmaStats{
		Enabled:            a.cfg != nil && a.cfg.Magma.Enabled,
		MaintenanceEnabled: ms.MaintenanceEnabled(),
		QueueDepth:         stats.QueueDepth,
		ProcessedTotal:     stats.ProcessedTotal,
		FailedTotal:        stats.FailedTotal,
		DroppedTotal:       stats.DroppedTotal,
		LastError:          stats.LastError,
	}
}

func (a *app) memoryObservabilityLanes(magmaStats memoryObservabilityMagmaStats, graphStats memoryObservabilityGraphStats) []memoryObservabilityLane {
	cfg := a.memoryObservabilityConfig()
	return []memoryObservabilityLane{
		{ID: "evolving", Label: "Evolving", Enabled: cfg.EvolvingEnabled, Status: boolStatus(cfg.EvolvingEnabled), Detail: "Experience summaries and retrieval scoring"},
		{ID: "belief", Label: "Belief", Enabled: cfg.BeliefEnabled, Status: boolStatus(cfg.BeliefEnabled), Detail: "Shared beliefs, evidence, and promotion"},
		{ID: "magma", Label: "MAGMA", Enabled: cfg.MagmaEnabled, Status: magmaLaneStatus(magmaStats), Detail: fmt.Sprintf("%d nodes · %d edges · %d queued", graphStats.Nodes, graphStats.Edges, magmaStats.QueueDepth)},
		{ID: "rag", Label: "RAG + Embeddings", Enabled: cfg.VectorBackend != "", Status: boolStatus(cfg.VectorBackend != ""), Detail: "Vector retrieval and graph augmentation"},
	}
}

func boolStatus(enabled bool) string {
	if enabled {
		return "online"
	}
	return "disabled"
}

func magmaLaneStatus(stats memoryObservabilityMagmaStats) string {
	switch {
	case !stats.Enabled:
		return "disabled"
	case stats.LastError != "":
		return "attention"
	case stats.QueueDepth > 0:
		return "working"
	default:
		return "online"
	}
}

func graphStats(nodes []memoryObservabilityNode, edges []memoryObservabilityEdge) memoryObservabilityGraphStats {
	stats := memoryObservabilityGraphStats{ByType: map[string]int{}}
	seenReview := map[string]struct{}{}
	for _, node := range nodes {
		switch node.Type {
		case "event":
			stats.Events++
		case "entity":
			stats.Entities++
		}
	}
	for _, edge := range edges {
		stats.ByType[edge.GraphType]++
		if edge.ReviewState != "" && edge.ReviewState != "approved" {
			seenReview[edge.ID] = struct{}{}
		}
	}
	stats.Nodes = len(nodes)
	stats.Edges = len(edges)
	stats.ReviewEdges = len(seenReview)
	return stats
}

func memoryObservabilityNodeFromEvent(event magma.EventNode) memoryObservabilityNode {
	return memoryObservabilityNode{
		ID:        event.ID,
		Type:      "event",
		Label:     nodeLabel(event.ID, event.Text),
		Tenant:    event.Tenant,
		Session:   event.Session,
		Text:      event.Text,
		CreatedAt: formatTime(event.CreatedAt),
		Metadata:  event.Metadata,
	}
}

func inferredMemoryNode(id string) memoryObservabilityNode {
	nodeType := "unknown"
	if strings.HasPrefix(id, "entity:") {
		nodeType = "entity"
	}
	return memoryObservabilityNode{ID: id, Type: nodeType, Label: nodeLabel(id, "")}
}

func memoryObservabilityEdgeFromMagma(edge magma.Edge) memoryObservabilityEdge {
	reviewState := stringMapValue(edge.Props, "review_state")
	return memoryObservabilityEdge{
		ID:          edgeID(edge),
		Source:      edge.Source,
		Target:      edge.Target,
		GraphType:   string(edge.GraphType),
		Rel:         edge.Rel,
		Weight:      edge.Weight,
		Confidence:  floatMapValue(edge.Props, "confidence"),
		ReviewState: reviewState,
		Reason:      stringMapValue(edge.Props, "review_reason"),
		Props:       edge.Props,
	}
}

func edgeID(edge magma.Edge) string {
	return strings.Join([]string{edge.Source, string(edge.GraphType), edge.Rel, edge.Target}, "|")
}

func validateMagmaSelector(selector magma.EdgeSelector) error {
	if strings.TrimSpace(selector.Source) == "" || strings.TrimSpace(selector.Target) == "" || strings.TrimSpace(selector.Rel) == "" || strings.TrimSpace(string(selector.GraphType)) == "" {
		return fmt.Errorf("selector source, graphType, rel, and target are required")
	}
	return nil
}

func parseLimitQuery(r *http.Request, key string, fallback, maxValue int) int {
	value := parseIntQuery(r, key)
	if value <= 0 {
		value = fallback
	}
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	if value <= 0 {
		return 1
	}
	return value
}

func stringMapValue(props map[string]any, key string) string {
	if len(props) == 0 {
		return ""
	}
	if value, ok := props[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func floatMapValue(props map[string]any, key string) float64 {
	switch value := props[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(value, 64)
		return f
	default:
		return 0
	}
}

func nodeLabel(id, text string) string {
	text = strings.TrimSpace(text)
	if text != "" {
		return previewText(text, 48)
	}
	parts := strings.Split(id, ":")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return id
}

func previewText(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func memoryEventLane(event magma.EventNode) string {
	source := strings.ToLower(stringMapValue(event.Metadata, "source"))
	switch source {
	case "belief_memory":
		return "belief"
	case "evolving_memory":
		return "evolving"
	case "transit":
		return "transit"
	default:
		return "magma"
	}
}

func graphTypesToStrings(types []magma.GraphType) []string {
	out := make([]string, 0, len(types))
	for _, graphType := range types {
		out = append(out, string(graphType))
	}
	return out
}
