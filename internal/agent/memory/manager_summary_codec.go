package memory

import (
	"encoding/json"
	"strings"
)

// encodeDualSummary encodes a dual summary to JSON for storage.
func encodeDualSummary(ds dualSummary) string {
	if ds.Compaction == "" && ds.Plain == "" {
		return ""
	}
	raw, err := json.Marshal(ds)
	if err != nil {
		return ds.Plain
	}
	return string(raw)
}

// decodeDualSummary decodes a stored summary. It handles both the new dual format
// and legacy single-summary formats (plain text or compaction JSON).
func decodeDualSummary(summary string) dualSummary {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return dualSummary{}
	}
	if ds, ok := decodeStoredDualSummary(trimmed); ok {
		return ds
	}
	if _, ok := decodeCompactionSummary(trimmed); ok {
		return dualSummary{Compaction: trimmed}
	}
	return dualSummary{Plain: trimmed}
}

func decodeStoredDualSummary(summary string) (dualSummary, bool) {
	if !strings.HasPrefix(summary, "{") {
		return dualSummary{}, false
	}
	var ds dualSummary
	if err := json.Unmarshal([]byte(summary), &ds); err != nil {
		return dualSummary{}, false
	}
	return ds, ds.Compaction != "" || ds.Plain != ""
}

// PlainTextSummary returns the plain-text portion of a persisted summary.
// Dual summaries expose their plain fallback, legacy plain summaries are
// returned as-is, and compaction-only summaries intentionally resolve to empty.
func PlainTextSummary(summary string) string {
	return strings.TrimSpace(decodeDualSummary(summary).Plain)
}
