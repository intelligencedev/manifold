package agentd

import (
	"encoding/json"
	"net/http"
	"sort"
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
