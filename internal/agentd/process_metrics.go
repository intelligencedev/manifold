package agentd

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/memory"
	llmpkg "manifold/internal/llm"
)

type traceMetricsProvider interface {
	Traces(ctx context.Context, window time.Duration, limit int) ([]llmpkg.TraceSnapshot, time.Duration, error)
	TracesForUser(ctx context.Context, userID int64, window time.Duration, limit int) ([]llmpkg.TraceSnapshot, time.Duration, error)
	Source() string
}

type logMetricsProvider interface {
	Logs(ctx context.Context, window time.Duration, limit int) ([]LogEntry, time.Duration, error)
	Source() string
}

type processTokenMetrics struct{}

func (processTokenMetrics) TokenTotals(_ context.Context, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error) {
	totals, applied := llmpkg.TokenTotalsForWindow(window)
	return totals, applied, nil
}

func (processTokenMetrics) TokenTotalsForUser(context.Context, int64, time.Duration) ([]llmpkg.TokenTotal, time.Duration, error) {
	return nil, 0, nil
}

func (processTokenMetrics) Source() string { return "process" }

type processTraceMetrics struct{}

func (processTraceMetrics) Traces(_ context.Context, window time.Duration, limit int) ([]llmpkg.TraceSnapshot, time.Duration, error) {
	traces, applied := llmpkg.TracesForWindow(window, limit)
	return traces, applied, nil
}

func (processTraceMetrics) TracesForUser(context.Context, int64, time.Duration, int) ([]llmpkg.TraceSnapshot, time.Duration, error) {
	return nil, 0, nil
}

func (processTraceMetrics) Source() string { return "process" }

type processMemoryMetrics struct{}

func (processMemoryMetrics) Source() string { return "process" }

func (processMemoryMetrics) MemoryMetrics(_ context.Context, userID int64, window time.Duration) (memoryMetricsSnapshot, time.Duration, error) {
	local, applied := memory.LocalTelemetryForWindow(userID, window)
	snapshot := memoryMetricsSnapshot{
		Totals: memoryMetricTotals{
			Searches:         local.Totals.Searches,
			Hits:             local.Totals.Hits,
			AvgHitsPerSearch: local.Totals.AvgHitsPerSearch,
			Evolves:          local.Totals.Evolves,
			EvolveErrors:     local.Totals.EvolveErrors,
			SmartMerges:      local.Totals.SmartMerges,
			Pruned:           local.Totals.Pruned,
		},
		Latency: memoryLatencyMetrics{AvgMs: local.Latency.AvgMs},
	}
	for _, size := range local.Sizes {
		snapshot.Sizes = append(snapshot.Sizes, memorySizeMetric{User: size.User, Session: size.Session, Size: size.Size})
	}
	for _, reason := range local.PrunedByReason {
		snapshot.PrunedByReason = append(snapshot.PrunedByReason, memoryReasonMetric{Reason: reason.Reason, Count: reason.Count})
	}
	for _, result := range local.EvolvesByResult {
		snapshot.EvolvesByResult = append(snapshot.EvolvesByResult, memoryResultMetric{Result: result.Result, Count: result.Count})
	}
	return snapshot, applied, nil
}

type processLogMetrics struct {
	mu      sync.RWMutex
	maxLogs int
	logs    []LogEntry
}

func newProcessLogMetrics(maxLogs int) *processLogMetrics {
	if maxLogs <= 0 {
		maxLogs = 5000
	}
	return &processLogMetrics{maxLogs: maxLogs}
}

func (p *processLogMetrics) Source() string { return "process" }

func (p *processLogMetrics) Write(data []byte) (int, error) {
	entry := parseProcessLogEntry(data)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, entry)
	if len(p.logs) > p.maxLogs {
		copy(p.logs, p.logs[len(p.logs)-p.maxLogs:])
		p.logs = p.logs[:p.maxLogs]
	}
	return len(data), nil
}

func (p *processLogMetrics) Logs(_ context.Context, window time.Duration, limit int) ([]LogEntry, time.Duration, error) {
	if p == nil {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	cutoff := time.Now().Add(-window).Unix()
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]LogEntry, 0, min(limit, len(p.logs)))
	for index := len(p.logs) - 1; index >= 0; index-- {
		entry := p.logs[index]
		if entry.Timestamp < cutoff {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out, window, nil
}

func parseProcessLogEntry(data []byte) LogEntry {
	var raw map[string]any
	entry := LogEntry{Timestamp: time.Now().Unix(), Level: "info", Service: "manifold", Message: strings.TrimSpace(string(data))}
	if err := json.Unmarshal(data, &raw); err != nil {
		return entry
	}
	if rawTime, ok := raw["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, rawTime); err == nil {
			entry.Timestamp = parsed.Unix()
		}
	}
	if level, ok := raw["level"].(string); ok && strings.TrimSpace(level) != "" {
		entry.Level = strings.ToLower(strings.TrimSpace(level))
	}
	if message, ok := raw["message"].(string); ok {
		entry.Message = message
	} else if message, ok := raw["msg"].(string); ok {
		entry.Message = message
	}
	if service, ok := raw["service"].(string); ok {
		entry.Service = service
	}
	if traceID, ok := raw["trace_id"].(string); ok {
		entry.TraceID = traceID
	}
	if spanID, ok := raw["span_id"].(string); ok {
		entry.SpanID = spanID
	}
	return entry
}
