package agentd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

type traceMetricsResponse struct {
	Timestamp     int64                  `json:"timestamp"`
	WindowSeconds int64                  `json:"windowSeconds,omitempty"`
	Source        string                 `json:"source"`
	Traces        []llmpkg.TraceSnapshot `json:"traces"`
}

type logMetricsResponse struct {
	Timestamp     int64      `json:"timestamp"`
	WindowSeconds int64      `json:"windowSeconds,omitempty"`
	Source        string     `json:"source"`
	Logs          []LogEntry `json:"logs"`
}

type logDetailResponse struct {
	Timestamp     int64     `json:"timestamp"`
	WindowSeconds int64     `json:"windowSeconds,omitempty"`
	Source        string    `json:"source"`
	Log           *LogEntry `json:"log,omitempty"`
}

type memoryMetricsResponse struct {
	Timestamp       int64                `json:"timestamp"`
	WindowSeconds   int64                `json:"windowSeconds,omitempty"`
	Source          string               `json:"source"`
	Totals          memoryMetricTotals   `json:"totals"`
	Latency         memoryLatencyMetrics `json:"latency"`
	Sizes           []memorySizeMetric   `json:"sizes"`
	PrunedByReason  []memoryReasonMetric `json:"prunedByReason"`
	EvolvesByResult []memoryResultMetric `json:"evolvesByResult"`
	Warnings        []string             `json:"warnings,omitempty"`
}

func (a *app) metricsTokensHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var uid int64 = systemUserID
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			uid = u.ID
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if window < 0 {
			http.Error(w, "window must be positive", http.StatusBadRequest)
			return
		}

		resp := tokenMetricsResponse{
			Timestamp: time.Now().Unix(),
			Source:    "none",
			Models:    nil,
		}
		appliedWindow := time.Duration(0)
		for _, provider := range a.tokenMetrics {
			if provider == nil {
				continue
			}
			var (
				totals   []llmpkg.TokenTotal
				chWindow time.Duration
				err      error
			)
			if a.cfg.Auth.Enabled {
				totals, chWindow, err = provider.TokenTotalsForUser(r.Context(), uid, window)
			} else {
				totals, chWindow, err = provider.TokenTotals(r.Context(), window)
			}
			if err != nil {
				log.Warn().Err(err).Msg("token metrics query failed")
				continue
			}
			resp.Models = totals
			resp.Source = provider.Source()
			appliedWindow = chWindow
			break
		}
		if appliedWindow > 0 {
			resp.WindowSeconds = int64(appliedWindow.Seconds())
		} else if window > 0 {
			resp.WindowSeconds = int64(window.Seconds())
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode token metrics response")
		}
	}
}

func (a *app) metricsMemoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var uid int64 = systemUserID
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			uid = u.ID
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if window < 0 {
			http.Error(w, "window must be positive", http.StatusBadRequest)
			return
		}

		resp := memoryMetricsResponse{
			Timestamp: time.Now().Unix(),
			Source:    "none",
		}
		if window > 0 {
			resp.WindowSeconds = int64(window.Seconds())
		}
		for _, provider := range a.memoryMetrics {
			if provider == nil {
				continue
			}
			snapshot, appliedWindow, err := provider.MemoryMetrics(r.Context(), uid, window)
			if err != nil {
				log.Warn().Err(err).Msg("memory metrics query failed")
				continue
			}
			resp.Source = provider.Source()
			resp.Totals = snapshot.Totals
			resp.Latency = snapshot.Latency
			resp.Sizes = snapshot.Sizes
			resp.PrunedByReason = snapshot.PrunedByReason
			resp.EvolvesByResult = snapshot.EvolvesByResult
			resp.Warnings = snapshot.Warnings
			if appliedWindow > 0 {
				resp.WindowSeconds = int64(appliedWindow.Seconds())
			}
			break
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode memory metrics response")
		}
	}
}

func (a *app) metricsTracesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var uid int64 = systemUserID
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			uid = u.ID
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		limit := parseLimitParam(r, 200)

		traces := []llmpkg.TraceSnapshot{}
		applied := time.Duration(0)
		source := "none"
		for _, provider := range a.traceMetrics {
			if provider == nil {
				continue
			}
			var (
				chTraces []llmpkg.TraceSnapshot
				chWindow time.Duration
				err      error
			)
			if a.cfg.Auth.Enabled {
				chTraces, chWindow, err = provider.TracesForUser(r.Context(), uid, window, limit)
			} else {
				chTraces, chWindow, err = provider.Traces(r.Context(), window, limit)
			}
			if err != nil {
				log.Warn().Err(err).Msg("trace metrics query failed")
				continue
			}
			traces = chTraces
			applied = chWindow
			source = provider.Source()
			break
		}
		resp := traceMetricsResponse{
			Timestamp: time.Now().Unix(),
			Source:    source,
			Traces:    traces,
		}
		if applied > 0 {
			resp.WindowSeconds = int64(applied.Seconds())
		} else if window > 0 {
			resp.WindowSeconds = int64(window.Seconds())
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode trace metrics response")
		}
	}
}

func (a *app) metricsLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			_, ok := auth.CurrentUser(r.Context())
			if !ok {
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

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		limit := parseLimitParam(r, 200)

		logs := []LogEntry{}
		source := "none"
		applied := time.Duration(0)
		for _, provider := range a.logMetrics {
			if provider == nil {
				continue
			}
			// For system observability, show all application logs regardless of user.
			// The LogsForUser filter only applies to user-scoped logs (chat logs, etc.)
			// which include an enduser.id attribute.
			chLogs, queriedWindow, err := provider.Logs(r.Context(), window, limit)
			if err != nil {
				log.Warn().Err(err).Msg("log metrics query failed")
				continue
			}
			logs = chLogs
			applied = queriedWindow
			source = provider.Source()
			break
		}

		resp := logMetricsResponse{
			Timestamp: time.Now().Unix(),
			Source:    source,
			Logs:      logs,
		}
		if applied > 0 {
			resp.WindowSeconds = int64(applied.Seconds())
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode log metrics response")
		}
	}
}

func (a *app) metricsLogDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			_, ok := auth.CurrentUser(r.Context())
			if !ok {
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

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		logID := strings.TrimSpace(r.URL.Query().Get("id"))
		if logID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		resp := logDetailResponse{
			Timestamp: time.Now().Unix(),
			Source:    "none",
		}
		for _, provider := range a.logMetrics {
			if provider == nil {
				continue
			}
			entry, applied, err := provider.LogDetail(r.Context(), window, logID)
			if err != nil {
				log.Warn().Err(err).Msg("log detail query failed")
				continue
			}
			resp.Log = entry
			resp.Source = provider.Source()
			if applied > 0 {
				resp.WindowSeconds = int64(applied.Seconds())
			}
			break
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Warn().Err(err).Msg("failed to encode log detail response")
		}
	}
}

func parseWindowParam(r *http.Request) (time.Duration, error) {
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("windowSeconds")); raw != "" {
		secs, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || secs <= 0 {
			return 0, errors.New("invalid windowSeconds parameter")
		}
		return time.Duration(secs) * time.Second, nil
	}
	if raw := strings.TrimSpace(q.Get("window")); raw != "" {
		dur, err := parseFlexibleDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid window parameter: %w", err)
		}
		return dur, nil
	}
	return 0, nil
}

func parseFlexibleDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	if dur, err := time.ParseDuration(raw); err == nil {
		if dur <= 0 {
			return 0, errors.New("duration must be positive")
		}
		return dur, nil
	}
	if len(raw) < 2 {
		return 0, errors.New("duration is too short")
	}
	base := strings.TrimSpace(raw[:len(raw)-1])
	unit := raw[len(raw)-1]
	multiplier, ok := map[byte]time.Duration{
		'd': 24 * time.Hour,
		'w': 7 * 24 * time.Hour,
	}[unit]
	if !ok {
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
	value, err := strconv.ParseFloat(base, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid magnitude %q", base)
	}
	if value <= 0 {
		return 0, errors.New("duration must be positive")
	}
	dur := time.Duration(value * float64(multiplier))
	if dur <= 0 {
		return 0, errors.New("duration underflows")
	}
	return dur, nil
}

func parseLimitParam(r *http.Request, defaultValue int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue
	}
	if v, err := strconv.Atoi(raw); err == nil && v > 0 {
		return v
	}
	return defaultValue
}

func (a *app) toolsCatalogHandler() http.HandlerFunc {
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
		// Use baseToolRegistry to show ALL available tools (not filtered by allowList)
		// This allows the UI to configure tool allow lists for specialists
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
