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

type memoryObservabilityOverviewResponse struct {
	Timestamp     int64                         `json:"timestamp"`
	WindowSeconds int64                         `json:"windowSeconds,omitempty"`
	Source        string                        `json:"source"`
	Config        memoryObservabilityConfig     `json:"config"`
	Totals        memoryMetricTotals            `json:"totals"`
	Latency       memoryLatencyMetrics          `json:"latency"`
	Graph         memoryObservabilityGraphStats `json:"graph"`
	Magma         memoryObservabilityMagmaStats `json:"magma"`
	Lanes         []memoryObservabilityLane     `json:"lanes"`
	Warnings      []string                      `json:"warnings,omitempty"`
}

type memoryObservabilityConfig struct {
	MemoryEnabled   bool   `json:"memoryEnabled"`
	EvolvingEnabled bool   `json:"evolvingEnabled"`
	BeliefEnabled   bool   `json:"beliefEnabled"`
	MagmaEnabled    bool   `json:"magmaEnabled"`
	GraphBackend    string `json:"graphBackend,omitempty"`
	VectorBackend   string `json:"vectorBackend,omitempty"`
}

type memoryObservabilityMagmaStats struct {
	Enabled            bool   `json:"enabled"`
	MaintenanceEnabled bool   `json:"maintenanceEnabled"`
	QueueDepth         int    `json:"queueDepth"`
	ProcessedTotal     uint64 `json:"processedTotal"`
	FailedTotal        uint64 `json:"failedTotal"`
	DroppedTotal       uint64 `json:"droppedTotal"`
	LastError          string `json:"lastError,omitempty"`
}

type memoryObservabilityGraphStats struct {
	Nodes       int            `json:"nodes"`
	Edges       int            `json:"edges"`
	Events      int            `json:"events"`
	Entities    int            `json:"entities"`
	ReviewEdges int            `json:"reviewEdges"`
	ByType      map[string]int `json:"byType,omitempty"`
}

type memoryObservabilityLane struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

type memoryObservabilityGraphResponse struct {
	Timestamp int64                         `json:"timestamp"`
	Graph     memoryObservabilityGraphStats `json:"graph"`
	Nodes     []memoryObservabilityNode     `json:"nodes"`
	Edges     []memoryObservabilityEdge     `json:"edges"`
	Warnings  []string                      `json:"warnings,omitempty"`
}

type memoryObservabilityNode struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Label     string         `json:"label"`
	Tenant    string         `json:"tenant,omitempty"`
	Session   string         `json:"session,omitempty"`
	Text      string         `json:"text,omitempty"`
	CreatedAt string         `json:"createdAt,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type memoryObservabilityEdge struct {
	ID          string         `json:"id"`
	Source      string         `json:"source"`
	Target      string         `json:"target"`
	GraphType   string         `json:"graphType"`
	Rel         string         `json:"rel"`
	Weight      float64        `json:"weight,omitempty"`
	Confidence  float64        `json:"confidence,omitempty"`
	ReviewState string         `json:"reviewState,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Props       map[string]any `json:"props,omitempty"`
}

type memoryObservabilityTimelineResponse struct {
	Timestamp int64                         `json:"timestamp"`
	Items     []memoryObservabilityTimeline `json:"items"`
}

type memoryObservabilityTimeline struct {
	ID        string `json:"id"`
	Time      string `json:"time,omitempty"`
	Lane      string `json:"lane"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Detail    string `json:"detail,omitempty"`
	Severity  string `json:"severity,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

type memoryObservabilityReviewResponse struct {
	Timestamp int64                     `json:"timestamp"`
	Edges     []memoryObservabilityEdge `json:"edges"`
}

type memoryObservabilityExplainResponse struct {
	Query       string                    `json:"query"`
	Intent      string                    `json:"intent"`
	GraphViews  []string                  `json:"graphViews"`
	AnchorCount int                       `json:"anchorCount"`
	MaxHops     int                       `json:"maxHops"`
	MaxNodes    int                       `json:"maxNodes"`
	Context     string                    `json:"context"`
	Events      []memoryObservabilityNode `json:"events"`
	Diagnostics map[string]any            `json:"diagnostics,omitempty"`
}

type memoryObservabilityActionResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Result  any    `json:"result,omitempty"`
}

func (a *app) memoryObservabilityHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.memoryObservabilityAuthorize(w, r) {
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/observability/memory"), "/")
		switch path {
		case "overview":
			a.handleMemoryObservabilityOverview(w, r)
		case "graph":
			a.handleMemoryObservabilityGraph(w, r)
		case "timeline":
			a.handleMemoryObservabilityTimeline(w, r)
		case "review-edges":
			a.handleMemoryObservabilityReviewEdges(w, r)
		case "retrieval/explain":
			a.handleMemoryObservabilityExplain(w, r)
		case "actions/prune":
			a.handleMemoryObservabilityPrune(w, r)
		case "actions/approve-edge":
			a.handleMemoryObservabilityApproveEdge(w, r)
		case "actions/retract-edge":
			a.handleMemoryObservabilityRetractEdge(w, r)
		case "actions/drain-consolidation":
			a.handleMemoryObservabilityDrainConsolidation(w, r)
		case "actions/rebuild-embeddings":
			a.handleMemoryObservabilityRebuildEmbeddings(w, r)
		default:
			writeJSON(w, http.StatusOK, map[string]string{
				"overview":         "/api/observability/memory/overview",
				"graph":            "/api/observability/memory/graph",
				"timeline":         "/api/observability/memory/timeline",
				"reviewEdges":      "/api/observability/memory/review-edges",
				"retrievalExplain": "/api/observability/memory/retrieval/explain",
				"actions":          "/api/observability/memory/actions/{action}",
			})
		}
	}
}

func (a *app) memoryObservabilityAuthorize(w http.ResponseWriter, r *http.Request) bool {
	if a == nil || a.cfg == nil || !a.cfg.Auth.Enabled {
		return true
	}
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (a *app) handleMemoryObservabilityOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid, err := a.memoryObservabilityUserID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	window, err := parseWindowParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	snapshot, appliedWindow, source := a.memoryObservabilityMetrics(r, uid, window)
	graphStats := a.memoryObservabilityGraphStats(r)
	magmaStats := a.memoryObservabilityMagmaStats()
	resp := memoryObservabilityOverviewResponse{
		Timestamp: time.Now().Unix(),
		Source:    source,
		Config:    a.memoryObservabilityConfig(),
		Totals:    snapshot.Totals,
		Latency:   snapshot.Latency,
		Graph:     graphStats,
		Magma:     magmaStats,
		Lanes:     a.memoryObservabilityLanes(magmaStats, graphStats),
		Warnings:  snapshot.Warnings,
	}
	if appliedWindow > 0 {
		resp.WindowSeconds = int64(appliedWindow.Seconds())
	} else if window > 0 {
		resp.WindowSeconds = int64(window.Seconds())
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) handleMemoryObservabilityGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nodes, edges, stats, warnings, err := a.memoryObservabilityGraphSnapshot(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityGraphResponse{
		Timestamp: time.Now().Unix(),
		Graph:     stats,
		Nodes:     nodes,
		Edges:     edges,
		Warnings:  warnings,
	})
}

func (a *app) handleMemoryObservabilityTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		writeJSON(w, http.StatusOK, memoryObservabilityTimelineResponse{Timestamp: time.Now().Unix()})
		return
	}
	events, err := ms.Events(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	filter := a.memoryObservabilityFilter(r)
	items := make([]memoryObservabilityTimeline, 0, len(events)+1)
	for _, event := range events {
		if !filter.event(event) {
			continue
		}
		lane := memoryEventLane(event)
		items = append(items, memoryObservabilityTimeline{
			ID:        event.ID,
			Time:      formatTime(event.CreatedAt),
			Lane:      lane,
			Kind:      "magma_event",
			Title:     nodeLabel(event.ID, event.Text),
			Detail:    previewText(event.Text, 180),
			Severity:  "info",
			SessionID: event.Session,
		})
	}
	if stats := ms.Stats(); stats.LastError != "" {
		items = append(items, memoryObservabilityTimeline{
			ID:       "magma:last-error",
			Time:     time.Now().UTC().Format(time.RFC3339Nano),
			Lane:     "magma",
			Kind:     "error",
			Title:    "MAGMA consolidation error",
			Detail:   stats.LastError,
			Severity: "error",
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Time > items[j].Time })
	limit := parseLimitQuery(r, "limit", 100, 500)
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, memoryObservabilityTimelineResponse{Timestamp: time.Now().Unix(), Items: items})
}

func (a *app) handleMemoryObservabilityReviewEdges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		writeJSON(w, http.StatusOK, memoryObservabilityReviewResponse{Timestamp: time.Now().Unix()})
		return
	}
	review, err := ms.ReviewEdges(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	edges := make([]memoryObservabilityEdge, 0, len(review))
	for _, edge := range review {
		edges = append(edges, memoryObservabilityEdgeFromMagma(edge.Edge))
		edges[len(edges)-1].ReviewState = edge.ReviewState
		edges[len(edges)-1].Reason = edge.Reason
	}
	writeJSON(w, http.StatusOK, memoryObservabilityReviewResponse{Timestamp: time.Now().Unix(), Edges: edges})
}

func (a *app) handleMemoryObservabilityExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		http.Error(w, "magma service is not configured", http.StatusNotFound)
		return
	}
	ctxOut, err := ms.Query(r.Context(), query, magma.QueryOptions{
		Tenant:               firstNonEmptyString(a.memoryObservabilityFilter(r).Tenant, strings.TrimSpace(r.URL.Query().Get("tenant"))),
		MaxHops:              parseLimitQuery(r, "maxHops", a.cfg.Magma.Retrieval.DefaultHops, 6),
		MaxNodes:             parseLimitQuery(r, "maxNodes", a.cfg.Magma.Retrieval.DefaultMaxNodes, 100),
		ContextFormat:        firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("contextFormat")), a.cfg.Magma.Retrieval.ContextFormat),
		IntentClassification: firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("intentClassification")), a.cfg.Magma.Retrieval.IntentClassification),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	events := make([]memoryObservabilityNode, 0, len(ctxOut.RawEvents))
	for _, event := range ctxOut.RawEvents {
		events = append(events, memoryObservabilityNodeFromEvent(event))
	}
	writeJSON(w, http.StatusOK, memoryObservabilityExplainResponse{
		Query:       query,
		Intent:      ctxOut.Intent.String(),
		GraphViews:  graphTypesToStrings(ctxOut.GraphViews),
		AnchorCount: ctxOut.AnchorCount,
		MaxHops:     ctxOut.MaxHops,
		MaxNodes:    ctxOut.MaxNodes,
		Context:     ctxOut.Text,
		Events:      events,
		Diagnostics: map[string]any{"anchor_strategy": string(ctxOut.AnchorStrategy), "events": len(ctxOut.RawEvents)},
	})
}

func (a *app) handleMemoryObservabilityPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		http.Error(w, "magma service is not configured", http.StatusNotFound)
		return
	}
	var req struct {
		DryRun                 bool    `json:"dryRun"`
		EventTTLHours          int     `json:"eventTTLHours"`
		MaxEdgesPerSourceRel   int     `json:"maxEdgesPerSourceRel"`
		MinSemanticWeight      float64 `json:"minSemanticWeight"`
		LowConfidenceThreshold float64 `json:"lowConfidenceThreshold"`
		RequireReviewApproval  bool    `json:"requireReviewApproval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	policy := magma.LifecyclePolicy{
		EventTTL:               time.Duration(req.EventTTLHours) * time.Hour,
		MaxEdgesPerSourceRel:   req.MaxEdgesPerSourceRel,
		MinSemanticWeight:      req.MinSemanticWeight,
		LowConfidenceThreshold: req.LowConfidenceThreshold,
		RequireReviewApproval:  req.RequireReviewApproval,
	}
	if req.DryRun {
		review, _ := ms.ReviewEdges(r.Context())
		writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "Dry run completed.", Result: map[string]any{"reviewEdges": len(review), "maintenanceEnabled": ms.MaintenanceEnabled()}})
		return
	}
	stats, err := ms.Prune(r.Context(), policy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "MAGMA lifecycle pruning completed.", Result: stats})
}

func (a *app) handleMemoryObservabilityApproveEdge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Selector magma.EdgeSelector `json:"selector"`
		Reviewer string             `json:"reviewer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateMagmaSelector(req.Selector); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		http.Error(w, "magma service is not configured", http.StatusNotFound)
		return
	}
	if err := ms.ApproveEdge(r.Context(), req.Selector, req.Reviewer); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "Edge approved."})
}

func (a *app) handleMemoryObservabilityRetractEdge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Selector magma.EdgeSelector `json:"selector"`
		Reason   string             `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateMagmaSelector(req.Selector); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ms := a.ragServiceMagma()
	if ms == nil {
		http.Error(w, "magma service is not configured", http.StatusNotFound)
		return
	}
	if err := ms.RetractEdge(r.Context(), req.Selector, req.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "Edge retracted."})
}

func (a *app) handleMemoryObservabilityDrainConsolidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Limit int `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	ms := a.ragServiceMagma()
	if ms == nil {
		http.Error(w, "magma service is not configured", http.StatusNotFound)
		return
	}
	processed, err := ms.DrainConsolidation(r.Context(), req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "Consolidation queue drained.", Result: map[string]any{"processed": processed}})
}

func (a *app) handleMemoryObservabilityRebuildEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"sessionId"`
		UserID    int64  `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	uid, err := a.memoryObservabilityUserID(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !a.cfg.Auth.Enabled && req.UserID > 0 {
		uid = req.UserID
	}
	em := a.getOrCreateEvolvingMemoryForSession(uid, req.SessionID)
	if em == nil {
		http.Error(w, "evolving memory is not configured", http.StatusNotFound)
		return
	}
	if err := em.RebuildEmbeddings(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, memoryObservabilityActionResponse{OK: true, Message: "Evolving memory embeddings rebuilt.", Result: map[string]any{"entries": len(em.ExportMemories())}})
}

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
