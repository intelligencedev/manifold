package agentd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/persistence"
	transitdomain "manifold/internal/transit"
)

func (a *app) transitMemoriesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.transitService == nil {
			http.NotFound(w, r)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Items []transitdomain.CreateMemoryItem `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			records, err := a.transitService.CreateMemory(r.Context(), userID, userID, req.Items)
			if err != nil {
				writeTransitError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, records)
		case http.MethodGet:
			keys := splitKeys(r.URL.Query().Get("keys"), r.URL.Query()["key"])
			records, err := a.transitService.GetMemory(r.Context(), userID, keys)
			if err != nil {
				writeTransitError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, records)
		case http.MethodDelete:
			keys := splitKeys(r.URL.Query().Get("keys"), r.URL.Query()["key"])
			if err := a.transitService.DeleteMemory(r.Context(), userID, keys); err != nil {
				writeTransitError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) transitMemoryDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.transitService == nil {
			http.NotFound(w, r)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/api/transit/memories/")
		key = strings.TrimSpace(key)
		if key == "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req transitdomain.UpdateMemoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		req.KeyName = key
		record, err := a.transitService.UpdateMemory(r.Context(), userID, userID, req)
		if err != nil {
			writeTransitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	}
}

func (a *app) transitKeysHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.transitService == nil {
			http.NotFound(w, r)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, err := a.transitService.ListKeys(r.Context(), userID, transitdomain.ListRequest{
			Prefix: strings.TrimSpace(r.URL.Query().Get("prefix")),
			Limit:  parseIntQuery(r, "limit"),
		})
		if err != nil {
			writeTransitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func (a *app) transitRecentHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.transitService == nil {
			http.NotFound(w, r)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, err := a.transitService.ListRecent(r.Context(), userID, transitdomain.ListRequest{
			Prefix: strings.TrimSpace(r.URL.Query().Get("prefix")),
			Limit:  parseIntQuery(r, "limit"),
		})
		if err != nil {
			writeTransitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func (a *app) transitSearchHandler() http.HandlerFunc {
	return a.transitSearchLikeHandler(false)
}

func (a *app) transitDiscoverHandler() http.HandlerFunc {
	return a.transitSearchLikeHandler(true)
}

func (a *app) transitSearchLikeHandler(discover bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.transitService == nil {
			http.NotFound(w, r)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req transitdomain.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if discover {
			items, err := a.transitService.DiscoverMemories(r.Context(), userID, req)
			if err != nil {
				writeTransitError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
			return
		}
		items, err := a.transitService.SearchMemories(r.Context(), userID, req)
		if err != nil {
			writeTransitError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func splitKeys(joined string, repeated []string) []string {
	if len(repeated) > 0 {
		return repeated
	}
	joined = strings.TrimSpace(joined)
	if joined == "" {
		return nil
	}
	parts := strings.Split(joined, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseIntQuery(r *http.Request, key string) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0
	}
	var parsed int
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func writeTransitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, persistence.ErrRevisionConflict):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
