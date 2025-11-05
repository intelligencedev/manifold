package llm

import (
	"testing"
	"time"
)

func TestTokenTotalsForWindow(t *testing.T) {
	resetTokenMetricsState()
	defer resetTokenMetricsState()

	base := time.Date(2024, 1, 12, 12, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	recordTokenMetrics("gpt-5", 100, 50, base.Add(-30*time.Minute))
	recordTokenMetrics("gpt-5", 200, 150, base.Add(-90*time.Minute))
	recordTokenMetrics("gpt-4", 10, 10, base.Add(-10*time.Minute))

	totals, applied := TokenTotalsForWindow(time.Hour)
	if len(totals) != 2 {
		t.Fatalf("expected 2 models in window, got %d", len(totals))
	}
	if totals[0].Model != "gpt-5" || totals[0].Prompt != 100 || totals[0].Completion != 50 {
		t.Fatalf("unexpected totals for gpt-5: %+v", totals[0])
	}
	if totals[1].Model != "gpt-4" || totals[1].Prompt != 10 || totals[1].Completion != 10 {
		t.Fatalf("unexpected totals for gpt-4: %+v", totals[1])
	}
	if applied <= 0 || applied > time.Hour {
		t.Fatalf("expected applied window to be within (0, 1h], got %v", applied)
	}

	totalsAll, appliedAll := TokenTotalsForWindow(0)
	if appliedAll != 0 {
		t.Fatalf("expected zero applied window for all-time totals, got %v", appliedAll)
	}
	if len(totalsAll) != 2 || totalsAll[0].Total != 500 {
		t.Fatalf("unexpected all-time totals: %+v", totalsAll)
	}
}

func TestTokenTotalsRetention(t *testing.T) {
	resetTokenMetricsState()
	defer resetTokenMetricsState()

	base := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	old := base.Add(-60 * 24 * time.Hour)
	recent := base.Add(-2 * time.Hour)

	recordTokenMetrics("gpt-5", 500, 500, old)
	recordTokenMetrics("gpt-5", 100, 100, recent)

	totals, applied := TokenTotalsForWindow(30 * 24 * time.Hour)
	if len(totals) != 1 {
		t.Fatalf("expected single model after retention, got %d", len(totals))
	}
	if totals[0].Total != 200 {
		t.Fatalf("expected only recent totals to remain, got %+v", totals[0])
	}
	if applied <= 0 || applied > 30*24*time.Hour {
		t.Fatalf("unexpected applied window %v", applied)
	}
}
