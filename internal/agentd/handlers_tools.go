package agentd

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	llmpkg "manifold/internal/llm"
)

type tokenMetricsResponse struct {
	Timestamp     int64               `json:"timestamp"`
	WindowSeconds int64               `json:"windowSeconds,omitempty"`
	Source        string              `json:"source"`
	Models        []llmpkg.TokenTotal `json:"models"`
}

func (a *app) metricsTokensHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := tokenMetricsResponse{
			Timestamp: time.Now().Unix(),
			Source:    "process",
			Models:    llmpkg.TokenTotalsSnapshot(),
		}
		if a.tokenMetrics != nil {
			if totals, window, err := a.tokenMetrics.TokenTotals(r.Context()); err != nil {
				log.Warn().Err(err).Msg("token metrics query failed")
			} else if len(totals) > 0 {
				resp.Models = totals
				resp.Source = a.tokenMetrics.Source()
				if window > 0 {
					resp.WindowSeconds = int64(window.Seconds())
				}
			}
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode token metrics response")
		}
	}
}

func (a *app) warppToolsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		schemas := a.baseToolRegistry.Schemas()
		out := make([]map[string]any, 0, len(schemas))
		for _, s := range schemas {
			out = append(out, map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  s.Parameters,
			})
		}
		json.NewEncoder(w).Encode(out)
	}
}
