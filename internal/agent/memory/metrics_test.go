package memory

import (
	"testing"
	"time"
)

func TestLocalTelemetryForWindowAggregatesRecentEvents(t *testing.T) {
	ConfigureLocalTelemetry(LocalTelemetryConfig{Enabled: true, MaxEvents: 10, Retention: time.Hour})
	localMemoryTelemetry.Lock()
	localMemoryTelemetry.events = nil
	localMemoryTelemetry.Unlock()
	t.Cleanup(func() {
		ConfigureLocalTelemetry(LocalTelemetryConfig{Enabled: true, MaxEvents: 10000, Retention: time.Hour})
		localMemoryTelemetry.Lock()
		localMemoryTelemetry.events = nil
		localMemoryTelemetry.Unlock()
	})

	now := time.Now()
	recordLocalMemoryEvent(localMemoryEvent{timestamp: now.Add(-2 * time.Minute), kind: "search", userID: 7, sessionID: "alpha", count: 1, hits: 3, duration: 20 * time.Millisecond, size: 11})
	recordLocalMemoryEvent(localMemoryEvent{timestamp: now.Add(-time.Minute), kind: "evolve", userID: 7, sessionID: "alpha", count: 1, size: 12, result: "success"})
	recordLocalMemoryEvent(localMemoryEvent{timestamp: now.Add(-30 * time.Second), kind: "pruned", count: 2, reason: "relevance"})
	recordLocalMemoryEvent(localMemoryEvent{timestamp: now.Add(-30 * time.Second), kind: "smart_merge", count: 4})
	recordLocalMemoryEvent(localMemoryEvent{timestamp: now.Add(-2 * time.Hour), kind: "search", userID: 7, sessionID: "old", count: 1, hits: 99, duration: time.Second, size: 99})

	snapshot, applied := LocalTelemetryForWindow(7, 10*time.Minute)
	if applied <= 0 || applied > 10*time.Minute {
		t.Fatalf("applied window = %s, want >0 and <=10m", applied)
	}
	if snapshot.Totals.Searches != 1 || snapshot.Totals.Hits != 3 || snapshot.Totals.AvgHitsPerSearch != 3 {
		t.Fatalf("unexpected search totals: %+v", snapshot.Totals)
	}
	if snapshot.Totals.Evolves != 1 || snapshot.Totals.Pruned != 2 || snapshot.Totals.SmartMerges != 4 {
		t.Fatalf("unexpected write/maintenance totals: %+v", snapshot.Totals)
	}
	if snapshot.Latency.AvgMs != 20 {
		t.Fatalf("avg latency = %f, want 20", snapshot.Latency.AvgMs)
	}
	if len(snapshot.Sizes) != 1 || snapshot.Sizes[0].User != "7" || snapshot.Sizes[0].Session != "alpha" || snapshot.Sizes[0].Size != 12 {
		t.Fatalf("unexpected sizes: %+v", snapshot.Sizes)
	}
	if len(snapshot.PrunedByReason) != 1 || snapshot.PrunedByReason[0].Reason != "relevance" || snapshot.PrunedByReason[0].Count != 2 {
		t.Fatalf("unexpected prune rows: %+v", snapshot.PrunedByReason)
	}
	if len(snapshot.EvolvesByResult) != 1 || snapshot.EvolvesByResult[0].Result != "success" || snapshot.EvolvesByResult[0].Count != 1 {
		t.Fatalf("unexpected evolve rows: %+v", snapshot.EvolvesByResult)
	}
}

func TestLocalTelemetryForWindowDisabled(t *testing.T) {
	ConfigureLocalTelemetry(LocalTelemetryConfig{Enabled: false, MaxEvents: 10, Retention: time.Hour})
	t.Cleanup(func() {
		ConfigureLocalTelemetry(LocalTelemetryConfig{Enabled: true, MaxEvents: 10000, Retention: time.Hour})
	})

	snapshot, applied := LocalTelemetryForWindow(0, time.Hour)
	if applied != 0 {
		t.Fatalf("applied window = %s, want 0", applied)
	}
	if snapshot.Totals.Searches != 0 || len(snapshot.Sizes) != 0 {
		t.Fatalf("disabled telemetry returned data: %+v", snapshot)
	}
}
