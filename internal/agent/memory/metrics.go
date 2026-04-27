package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

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
