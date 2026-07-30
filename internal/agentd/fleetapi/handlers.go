// Package fleetapi owns fleet HTTP transport while the server composes data
// sources through narrow callbacks.
package fleetapi

import (
	"context"
	"encoding/json"
	"net/http"

	"manifold/internal/fleet"
)

type Deps struct {
	RequireUserID func(*http.Request) (int64, error)
	BuildState    func(context.Context, int64) any
	Subscribe     func(context.Context, int64) ([]fleet.Event, <-chan fleet.Event)
}

// StateHandler serves a snapshot assembled by the composition root.
func StateHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.RequireUserID == nil || deps.BuildState == nil {
			http.Error(w, "fleet unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deps.BuildState(r.Context(), userID))
	}
}

// EventsHandler serves a fleet event snapshot then live SSE stream.
func EventsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.RequireUserID == nil || deps.Subscribe == nil {
			http.Error(w, "fleet unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		snapshot, events := deps.Subscribe(r.Context(), userID)
		writeEvents(w, flusher, snapshot)
		for event := range events {
			writeEvents(w, flusher, []fleet.Event{event})
		}
	}
}

func writeEvents(w http.ResponseWriter, flusher http.Flusher, events []fleet.Event) {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}
