package memory

import (
	"context"
	"time"
)

const defaultSummaryCallTimeout = 120 * time.Second

// AuxiliarySummaryContext returns a detached context for best-effort summary
// and compaction work. It preserves request values for tracing and scoping, but
// it does not inherit transport cancellation from the parent.
func AuxiliarySummaryContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	base := context.WithoutCancel(parent)
	if timeout <= 0 {
		timeout = defaultSummaryCallTimeout
	}
	return context.WithTimeout(base, timeout)
}
