package obs

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OtelMetrics is a thin adapter over OpenTelemetry metrics that satisfies
// the service.Metrics interface from internal/rag/service.
type OtelMetrics struct {
	meter metric.Meter
	mu    sync.RWMutex
	// cache instruments by name
	counters   map[string]metric.Int64Counter
	histograms map[string]metric.Float64Histogram
}

// NewOtelMetrics constructs an OtelMetrics using the global Meter provider.
func NewOtelMetrics() *OtelMetrics {
	return &OtelMetrics{
		meter:      otel.Meter("rag"),
		counters:   make(map[string]metric.Int64Counter),
		histograms: make(map[string]metric.Float64Histogram),
	}
}

func (o *OtelMetrics) IncCounter(name string, labels map[string]string) {
	if o == nil {
		return
	}
	c, ok := o.getCounter(name)
	if !ok {
		return
	}
	attrs := toAttrs(labels)
	c.Add(context.Background(), 1, metric.WithAttributes(attrs...))
}

func (o *OtelMetrics) ObserveHistogram(name string, value float64, labels map[string]string) {
	if o == nil {
		return
	}
	h, ok := o.getHistogram(name)
	if !ok {
		return
	}
	attrs := toAttrs(labels)
	h.Record(context.Background(), value, metric.WithAttributes(attrs...))
}

func (o *OtelMetrics) getCounter(name string) (metric.Int64Counter, bool) {
	o.mu.RLock()
	c, ok := o.counters[name]
	o.mu.RUnlock()
	if ok {
		return c, true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok = o.counters[name]; ok {
		return c, true
	}
	ctr, err := o.meter.Int64Counter(name)
	if err != nil {
		return ctr, false
	}
	o.counters[name] = ctr
	return ctr, true
}

func (o *OtelMetrics) getHistogram(name string) (metric.Float64Histogram, bool) {
	o.mu.RLock()
	h, ok := o.histograms[name]
	o.mu.RUnlock()
	if ok {
		return h, true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if h, ok = o.histograms[name]; ok {
		return h, true
	}
	hist, err := o.meter.Float64Histogram(name)
	if err != nil {
		return hist, false
	}
	o.histograms[name] = hist
	return hist, true
}

func toAttrs(labels map[string]string) []attribute.KeyValue {
	if len(labels) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(labels))
	for k, v := range labels {
		out = append(out, attribute.String(k, v))
	}
	return out
}

// MockMetrics is an in-memory metrics sink for tests.
type MockMetrics struct {
	mu       sync.Mutex
	Counters map[string]int
	Hists    map[string][]float64
	Labels   map[string][]map[string]string
}

func NewMockMetrics() *MockMetrics {
	return &MockMetrics{
		Counters: map[string]int{},
		Hists:    map[string][]float64{},
		Labels:   map[string][]map[string]string{},
	}
}

func (m *MockMetrics) IncCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name]++
	m.Labels[name] = append(m.Labels[name], clone(labels))
}

func (m *MockMetrics) ObserveHistogram(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Hists[name] = append(m.Hists[name], value)
	m.Labels[name] = append(m.Labels[name], clone(labels))
}

func clone(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
