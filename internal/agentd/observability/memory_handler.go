// Package observability contains HTTP routing for observability domains.
package observability

import (
	"encoding/json"
	"net/http"
	"strings"
)

// MemoryHandlerDeps is the narrow boundary for memory observability routing.
type MemoryHandlerDeps struct {
	Authorize          func(http.ResponseWriter, *http.Request) bool
	Overview           http.HandlerFunc
	Graph              http.HandlerFunc
	Timeline           http.HandlerFunc
	ReviewEdges        http.HandlerFunc
	Explain            http.HandlerFunc
	Prune              http.HandlerFunc
	ApproveEdge        http.HandlerFunc
	RetractEdge        http.HandlerFunc
	DeleteNode         http.HandlerFunc
	DrainConsolidation http.HandlerFunc
	RebuildEmbeddings  http.HandlerFunc
}

// MemoryHandler routes the stable memory observability HTTP surface.
func MemoryHandler(deps MemoryHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Authorize == nil || !deps.Authorize(w, r) {
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
			deps.Overview(w, r)
		case "graph":
			deps.Graph(w, r)
		case "timeline":
			deps.Timeline(w, r)
		case "review-edges":
			deps.ReviewEdges(w, r)
		case "retrieval/explain":
			deps.Explain(w, r)
		case "actions/prune":
			deps.Prune(w, r)
		case "actions/approve-edge":
			deps.ApproveEdge(w, r)
		case "actions/retract-edge":
			deps.RetractEdge(w, r)
		case "actions/delete-node":
			deps.DeleteNode(w, r)
		case "actions/drain-consolidation":
			deps.DrainConsolidation(w, r)
		case "actions/rebuild-embeddings":
			deps.RebuildEmbeddings(w, r)
		default:
			if strings.HasPrefix(path, "actions/") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
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
