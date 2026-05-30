package agentd

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (a *app) trustBudgetsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil || userID < 0 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			budgets, err := a.trustService.List(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, budgets)
		case http.MethodPost:
			http.Error(w, "use refill or spend endpoints", http.StatusBadRequest)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) trustBudgetActionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/trust/budgets/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		name, action := parts[0], parts[1]
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		amount := 1
		if v, ok := body["amount"].(float64); ok {
			amount = int(v)
		}
		if v, ok := body["quota"].(float64); ok {
			amount = int(v)
		}
		if raw := r.URL.Query().Get("amount"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				amount = parsed
			}
		}
		var resp any
		var err error
		switch action {
		case "spend":
			resp, err = a.trustService.Spend(r.Context(), name, amount)
		case "refill":
			resp, err = a.trustService.Refill(r.Context(), name, amount)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
