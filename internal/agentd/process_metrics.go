package agentd

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
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
	LogDetail(ctx context.Context, window time.Duration, id string) (*LogEntry, time.Duration, error)
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

func (p *processLogMetrics) LogDetail(_ context.Context, window time.Duration, id string) (*LogEntry, time.Duration, error) {
	if p == nil {
		return nil, 0, nil
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	needle := strings.TrimSpace(id)
	if needle == "" {
		return nil, window, nil
	}
	cutoff := time.Now().Add(-window).Unix()
	p.mu.RLock()
	defer p.mu.RUnlock()
	for index := len(p.logs) - 1; index >= 0; index-- {
		entry := p.logs[index]
		if entry.Timestamp < cutoff {
			continue
		}
		if entry.ID == needle {
			copyEntry := entry
			return &copyEntry, window, nil
		}
	}
	return nil, window, nil
}

func parseProcessLogEntry(data []byte) LogEntry {
	var raw map[string]any
	entry := LogEntry{Timestamp: time.Now().Unix(), Level: "info", Service: "manifold", Message: strings.TrimSpace(string(data))}
	if err := json.Unmarshal(data, &raw); err != nil {
		populateLogEntryMeta(&entry)
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
	entry.Attributes = normalizeLogAttributes(raw, map[string]struct{}{
		"time":     {},
		"level":    {},
		"message":  {},
		"msg":      {},
		"service":  {},
		"trace_id": {},
		"span_id":  {},
	})
	populateLogEntryMeta(&entry)
	return entry
}

func populateLogEntryMeta(entry *LogEntry) {
	if entry == nil {
		return
	}
	entry.Level = strings.ToLower(strings.TrimSpace(entry.Level))
	if entry.Level == "" {
		entry.Level = "info"
	}
	entry.Message = strings.TrimSpace(entry.Message)
	entry.Service = strings.TrimSpace(entry.Service)
	entry.TraceID = strings.TrimSpace(entry.TraceID)
	entry.SpanID = strings.TrimSpace(entry.SpanID)
	entry.ID = buildLogEntryID(entry.Timestamp, entry.Level, entry.Message, entry.Service, entry.TraceID, entry.SpanID)
	entry.Tags = buildLogTags(*entry)
}

func buildLogEntryID(timestamp int64, level, message, service, traceID, spanID string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(fmt.Sprintf("%d|%s|%s|%s|%s|%s", timestamp, level, service, traceID, spanID, message)))
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeLogAttributes(raw map[string]any, exclude map[string]struct{}) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	attrs := make(map[string]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if _, skip := exclude[trimmedKey]; skip {
			continue
		}
		if formatted := stringifyLogValue(value); formatted != "" {
			attrs[trimmedKey] = formatted
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func stringifyLogValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.RawMessage:
		return strings.TrimSpace(string(typed))
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	case float32:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, bool:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(encoded))
	}
}

func buildLogTags(entry LogEntry) []string {
	tags := make([]string, 0, 12)
	seen := make(map[string]struct{})
	appendTag := func(key, value string) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		tag := key + ":" + value
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	appendTag("level", entry.Level)
	appendTag("service", entry.Service)
	appendTag("trace_id", entry.TraceID)
	appendTag("span_id", entry.SpanID)
	for key, value := range entry.ResourceAttributes {
		if shouldTagAttribute(key, value) {
			appendTag(key, value)
		}
	}
	for key, value := range entry.Attributes {
		if shouldTagAttribute(key, value) {
			appendTag(key, value)
		}
	}
	sort.Strings(tags)
	return tags
}

func shouldTagAttribute(key, value string) bool {
	trimmedKey := strings.ToLower(strings.TrimSpace(key))
	trimmedValue := strings.TrimSpace(value)
	if trimmedKey == "" || trimmedValue == "" {
		return false
	}
	switch trimmedKey {
	case "prompt", "prompt_raw", "response", "prompt_preview":
		return false
	}
	return len(trimmedValue) <= 120
}
