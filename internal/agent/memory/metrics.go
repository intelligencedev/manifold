package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

type LocalTelemetryConfig struct {
	Enabled   bool
	MaxEvents int
	Retention time.Duration
}

type LocalTelemetryTotals struct {
	Searches         int64
	Hits             int64
	AvgHitsPerSearch float64
	Evolves          int64
	EvolveErrors     int64
	SmartMerges      int64
	Pruned           int64
}

type LocalTelemetryLatency struct {
	AvgMs float64
}

type LocalTelemetrySize struct {
	User    string
	Session string
	Size    int64
}

type LocalTelemetryReason struct {
	Reason string
	Count  int64
}

type LocalTelemetryResult struct {
	Result string
	Count  int64
}

type LocalTelemetrySnapshot struct {
	Totals          LocalTelemetryTotals
	Latency         LocalTelemetryLatency
	Sizes           []LocalTelemetrySize
	PrunedByReason  []LocalTelemetryReason
	EvolvesByResult []LocalTelemetryResult
}

type localMemoryEvent struct {
	timestamp time.Time
	kind      string
	userID    int64
	sessionID string
	count     int64
	hits      int64
	duration  time.Duration
	size      int64
	result    string
	reason    string
}

var localMemoryTelemetry = struct {
	sync.RWMutex
	enabled   bool
	maxEvents int
	retention time.Duration
	events    []localMemoryEvent
}{
	enabled:   true,
	maxEvents: 10000,
	retention: time.Hour,
}

func ConfigureLocalTelemetry(cfg LocalTelemetryConfig) {
	localMemoryTelemetry.Lock()
	defer localMemoryTelemetry.Unlock()
	localMemoryTelemetry.enabled = cfg.Enabled
	if cfg.MaxEvents > 0 {
		localMemoryTelemetry.maxEvents = cfg.MaxEvents
	}
	if cfg.Retention > 0 {
		localMemoryTelemetry.retention = cfg.Retention
	}
	trimLocalMemoryEventsLocked(time.Now())
}

func LocalTelemetryForWindow(userID int64, window time.Duration) (LocalTelemetrySnapshot, time.Duration) {
	localMemoryTelemetry.RLock()
	defer localMemoryTelemetry.RUnlock()
	if !localMemoryTelemetry.enabled {
		return LocalTelemetrySnapshot{}, 0
	}
	if window <= 0 || window > localMemoryTelemetry.retention {
		window = localMemoryTelemetry.retention
	}
	now := time.Now()
	cutoff := now.Add(-window)
	snapshot := LocalTelemetrySnapshot{}
	latencyCount := int64(0)
	latencyTotal := time.Duration(0)
	sizes := map[string]LocalTelemetrySize{}
	prunedByReason := map[string]int64{}
	evolvesByResult := map[string]int64{}
	var earliest time.Time

	for _, event := range localMemoryTelemetry.events {
		if event.timestamp.Before(cutoff) {
			continue
		}
		if userID != 0 && event.userID != 0 && event.userID != userID {
			continue
		}
		if earliest.IsZero() || event.timestamp.Before(earliest) {
			earliest = event.timestamp
		}
		switch event.kind {
		case "search":
			snapshot.Totals.Searches += event.count
			snapshot.Totals.Hits += event.hits
			latencyCount += event.count
			latencyTotal += event.duration
			sizes[fmt.Sprintf("%d:%s", event.userID, event.sessionID)] = LocalTelemetrySize{User: fmt.Sprint(event.userID), Session: event.sessionID, Size: event.size}
		case "evolve":
			snapshot.Totals.Evolves += event.count
			if event.result == "error" {
				snapshot.Totals.EvolveErrors += event.count
			}
			result := event.result
			if result == "" {
				result = "unknown"
			}
			evolvesByResult[result] += event.count
			sizes[fmt.Sprintf("%d:%s", event.userID, event.sessionID)] = LocalTelemetrySize{User: fmt.Sprint(event.userID), Session: event.sessionID, Size: event.size}
		case "smart_merge":
			snapshot.Totals.SmartMerges += event.count
		case "pruned":
			snapshot.Totals.Pruned += event.count
			reason := event.reason
			if reason == "" {
				reason = "unknown"
			}
			prunedByReason[reason] += event.count
		}
	}
	if snapshot.Totals.Searches > 0 {
		snapshot.Totals.AvgHitsPerSearch = float64(snapshot.Totals.Hits) / float64(snapshot.Totals.Searches)
	}
	if latencyCount > 0 {
		snapshot.Latency.AvgMs = float64(latencyTotal.Milliseconds()) / float64(latencyCount)
	}
	for _, size := range sizes {
		snapshot.Sizes = append(snapshot.Sizes, size)
	}
	sort.Slice(snapshot.Sizes, func(left, right int) bool {
		return snapshot.Sizes[left].Size > snapshot.Sizes[right].Size
	})
	for reason, count := range prunedByReason {
		snapshot.PrunedByReason = append(snapshot.PrunedByReason, LocalTelemetryReason{Reason: reason, Count: count})
	}
	sort.Slice(snapshot.PrunedByReason, func(left, right int) bool {
		return snapshot.PrunedByReason[left].Count > snapshot.PrunedByReason[right].Count
	})
	for result, count := range evolvesByResult {
		snapshot.EvolvesByResult = append(snapshot.EvolvesByResult, LocalTelemetryResult{Result: result, Count: count})
	}
	sort.Slice(snapshot.EvolvesByResult, func(left, right int) bool {
		return snapshot.EvolvesByResult[left].Count > snapshot.EvolvesByResult[right].Count
	})
	if earliest.IsZero() {
		return snapshot, window
	}
	available := now.Sub(earliest)
	if available < window {
		return snapshot, available
	}
	return snapshot, window
}

type MemoryMetrics struct {
	once sync.Once

	searchLatency      otelmetric.Float64Histogram
	searchTotal        otelmetric.Int64Counter
	searchHits         otelmetric.Int64Counter
	evolveTotal        otelmetric.Int64Counter
	sizeGauge          otelmetric.Int64Gauge
	smartMergeTotal    otelmetric.Int64Counter
	prunedTotal        otelmetric.Int64Counter
	instrumentsEnabled bool
}

func NewMemoryMetrics() *MemoryMetrics {
	return &MemoryMetrics{}
}

func (m *MemoryMetrics) ensure() {
	if m == nil {
		return
	}
	m.once.Do(func() {
		meter := otel.Meter("internal/agent/memory")
		var err error
		m.searchLatency, err = meter.Float64Histogram(
			"evolving_memory_search_latency_seconds",
			otelmetric.WithDescription("Evolving memory search latency in seconds"),
		)
		if err != nil {
			return
		}
		m.searchTotal, err = meter.Int64Counter(
			"evolving_memory_search_total",
			otelmetric.WithDescription("Evolving memory search attempts"),
		)
		if err != nil {
			return
		}
		m.searchHits, err = meter.Int64Counter(
			"evolving_memory_search_hits",
			otelmetric.WithDescription("Retrieved evolving memory entries by score bucket"),
		)
		if err != nil {
			return
		}
		m.evolveTotal, err = meter.Int64Counter(
			"evolving_memory_evolve_total",
			otelmetric.WithDescription("Evolving memory write attempts by result"),
		)
		if err != nil {
			return
		}
		m.sizeGauge, err = meter.Int64Gauge(
			"evolving_memory_size",
			otelmetric.WithDescription("Latest observed evolving memory size for a user/session"),
		)
		if err != nil {
			return
		}
		m.smartMergeTotal, err = meter.Int64Counter(
			"evolving_memory_smart_merge_total",
			otelmetric.WithDescription("Smart merge operations in evolving memory"),
		)
		if err != nil {
			return
		}
		m.prunedTotal, err = meter.Int64Counter(
			"evolving_memory_pruned_total",
			otelmetric.WithDescription("Pruned evolving memory entries by reason"),
		)
		if err != nil {
			return
		}
		m.instrumentsEnabled = true
	})
}

func (m *MemoryMetrics) RecordSearch(ctx context.Context, duration time.Duration, hits int, size int, userID int64, sessionID string) {
	if m == nil {
		return
	}
	m.ensure()
	if !m.instrumentsEnabled {
		return
	}
	attrs := memoryMetricAttrs(userID, sessionID)
	m.searchLatency.Record(ctx, duration.Seconds(), otelmetric.WithAttributes(attrs...))
	m.searchTotal.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	if hits > 0 {
		for _, bucket := range scoreBuckets(hits) {
			m.searchHits.Add(ctx, bucket.count, otelmetric.WithAttributes(append(attrs, attribute.String("score_bucket", bucket.name))...))
		}
	}
	m.sizeGauge.Record(ctx, int64(size), otelmetric.WithAttributes(attrs...))
	recordLocalMemoryEvent(localMemoryEvent{timestamp: time.Now(), kind: "search", userID: userID, sessionID: sessionID, count: 1, hits: int64(hits), duration: duration, size: int64(size)})
}

func (m *MemoryMetrics) RecordEvolve(ctx context.Context, result string, size int, userID int64, sessionID string) {
	if m == nil {
		return
	}
	m.ensure()
	if !m.instrumentsEnabled {
		return
	}
	attrs := append(memoryMetricAttrs(userID, sessionID), attribute.String("result", result))
	m.evolveTotal.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	m.sizeGauge.Record(ctx, int64(size), otelmetric.WithAttributes(memoryMetricAttrs(userID, sessionID)...))
	recordLocalMemoryEvent(localMemoryEvent{timestamp: time.Now(), kind: "evolve", userID: userID, sessionID: sessionID, count: 1, size: int64(size), result: result})
}

func (m *MemoryMetrics) RecordSmartMerge(ctx context.Context, merged int) {
	if m == nil || merged <= 0 {
		return
	}
	m.ensure()
	if !m.instrumentsEnabled {
		return
	}
	m.smartMergeTotal.Add(ctx, int64(merged))
	recordLocalMemoryEvent(localMemoryEvent{timestamp: time.Now(), kind: "smart_merge", count: int64(merged)})
}

func (m *MemoryMetrics) RecordPruned(ctx context.Context, reason string, count int) {
	if m == nil || count <= 0 {
		return
	}
	m.ensure()
	if !m.instrumentsEnabled {
		return
	}
	m.prunedTotal.Add(ctx, int64(count), otelmetric.WithAttributes(attribute.String("reason", reason)))
	recordLocalMemoryEvent(localMemoryEvent{timestamp: time.Now(), kind: "pruned", count: int64(count), reason: reason})
}

func recordLocalMemoryEvent(event localMemoryEvent) {
	localMemoryTelemetry.Lock()
	defer localMemoryTelemetry.Unlock()
	if !localMemoryTelemetry.enabled {
		return
	}
	localMemoryTelemetry.events = append(localMemoryTelemetry.events, event)
	trimLocalMemoryEventsLocked(event.timestamp)
}

func trimLocalMemoryEventsLocked(now time.Time) {
	if localMemoryTelemetry.retention > 0 {
		cutoff := now.Add(-localMemoryTelemetry.retention)
		writeIndex := 0
		for _, event := range localMemoryTelemetry.events {
			if event.timestamp.After(cutoff) || event.timestamp.Equal(cutoff) {
				localMemoryTelemetry.events[writeIndex] = event
				writeIndex++
			}
		}
		localMemoryTelemetry.events = localMemoryTelemetry.events[:writeIndex]
	}
	if localMemoryTelemetry.maxEvents > 0 && len(localMemoryTelemetry.events) > localMemoryTelemetry.maxEvents {
		copy(localMemoryTelemetry.events, localMemoryTelemetry.events[len(localMemoryTelemetry.events)-localMemoryTelemetry.maxEvents:])
		localMemoryTelemetry.events = localMemoryTelemetry.events[:localMemoryTelemetry.maxEvents]
	}
}

func memoryMetricAttrs(userID int64, sessionID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("user", fmt.Sprint(userID)),
		attribute.String("session", sessionID),
	}
}

type scoreBucket struct {
	name  string
	count int64
}

func scoreBuckets(hits int) []scoreBucket {
	if hits <= 0 {
		return nil
	}
	return []scoreBucket{{name: "returned", count: int64(hits)}}
}
