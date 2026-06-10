package belief

import (
	"context"
	"time"
)

// ChangeKind identifies the belief mutation that downstream systems may react to.
type ChangeKind string

const (
	// ChangeStatus marks a status transition such as active to retracted.
	ChangeStatus ChangeKind = "status"
	// ChangeConfidence marks a confidence change.
	ChangeConfidence ChangeKind = "confidence"
	// ChangeExpiry marks a belief passing its ExpiresAt time.
	ChangeExpiry ChangeKind = "expiry"
)

// ChangeEvent describes one belief change observed by a notifying store.
type ChangeEvent struct {
	TenantID   int64
	BeliefID   string
	Kind       ChangeKind
	Before     Belief
	After      Belief
	OccurredAt time.Time
}

// ChangeListener receives belief change events asynchronously.
type ChangeListener interface {
	OnBeliefChanged(ctx context.Context, ev ChangeEvent)
}
