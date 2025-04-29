package agent

import (
	"context"
	"math"
)

// RingMemory is a simple in-process memory implementation that stores a fixed number of items.
type RingMemory struct {
	cap   int
	items []MemoryItem
}

func NewRingMemory(capacity int) *RingMemory {
	return &RingMemory{cap: capacity}
}

func (m *RingMemory) Recall(_ context.Context, _ string, k int) ([]MemoryItem, error) {
	// naive recency-weighted recall
	n := int(math.Min(float64(k), float64(len(m.items))))
	out := make([]MemoryItem, n)
	copy(out, m.items[len(m.items)-n:])
	return out, nil
}

func (m *RingMemory) Store(_ context.Context, item MemoryItem) error {
	if len(m.items) >= m.cap {
		m.items = m.items[1:]
	}
	m.items = append(m.items, item)
	return nil
}
