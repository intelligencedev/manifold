package obs

import "testing"

func TestMockMetrics_RecordsCountsAndHists(t *testing.T) {
	m := NewMockMetrics()
	m.IncCounter("ingestion_docs_total", map[string]string{"tenant": "t1"})
	m.IncCounter("ingestion_docs_total", map[string]string{"tenant": "t1"})
	m.ObserveHistogram("ingestion_stage_ms", 12, map[string]string{"stage": "preprocess"})
	m.ObserveHistogram("ingestion_stage_ms", 34, map[string]string{"stage": "chunk"})
	if m.Counters["ingestion_docs_total"] != 2 {
		t.Fatalf("expected 2 docs, got %d", m.Counters["ingestion_docs_total"])
	}
	if len(m.Hists["ingestion_stage_ms"]) != 2 {
		t.Fatalf("expected 2 histogram records, got %d", len(m.Hists["ingestion_stage_ms"]))
	}
}
