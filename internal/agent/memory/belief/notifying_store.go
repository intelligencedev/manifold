package belief

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// NotifyingStore decorates a belief store with non-blocking change events.
type NotifyingStore struct {
	Store

	queue       chan ChangeEvent
	listenersMu sync.RWMutex
	listeners   []ChangeListener
	dropped     atomic.Uint64
	closed      chan struct{}
}

// NewNotifyingStore wraps a belief store. queueSize <= 0 uses a conservative default.
func NewNotifyingStore(inner Store, queueSize int) *NotifyingStore {
	if queueSize <= 0 {
		queueSize = 256
	}
	s := &NotifyingStore{
		Store:  inner,
		queue:  make(chan ChangeEvent, queueSize),
		closed: make(chan struct{}),
	}
	go s.dispatch()
	return s
}

// RegisterListener adds a listener for future belief changes.
func (s *NotifyingStore) RegisterListener(listener ChangeListener) {
	if s == nil || listener == nil {
		return
	}
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	s.listeners = append(s.listeners, listener)
}

// DroppedEvents returns how many events were dropped because the queue was full.
func (s *NotifyingStore) DroppedEvents() uint64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

// Close stops the dispatch worker and closes the wrapped store when it supports Close.
func (s *NotifyingStore) Close() {
	if s == nil {
		return
	}
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	if closer, ok := s.Store.(interface{ Close() }); ok {
		closer.Close()
	}
}

// UpsertBelief delegates the write and emits status/confidence change events.
func (s *NotifyingStore) UpsertBelief(ctx context.Context, item Belief) (Belief, error) {
	if s == nil || s.Store == nil {
		return item, nil
	}
	var before Belief
	var existed bool
	if item.ID != "" {
		if got, ok, err := s.Store.GetBelief(ctx, item.TenantID, item.ID); err == nil && ok {
			before = got
			existed = true
		}
	}
	after, err := s.Store.UpsertBelief(ctx, item)
	if err != nil {
		return Belief{}, err
	}
	if !existed || before.ID == "" {
		return after, nil
	}
	now := time.Now().UTC()
	if before.Status != after.Status {
		s.emit(ChangeEvent{TenantID: after.TenantID, BeliefID: after.ID, Kind: ChangeStatus, Before: before, After: after, OccurredAt: now})
	}
	if before.Confidence != after.Confidence {
		s.emit(ChangeEvent{TenantID: after.TenantID, BeliefID: after.ID, Kind: ChangeConfidence, Before: before, After: after, OccurredAt: now})
	}
	if before.ExpiresAt == nil && after.ExpiresAt != nil && after.ExpiresAt.Before(now) {
		s.emit(ChangeEvent{TenantID: after.TenantID, BeliefID: after.ID, Kind: ChangeExpiry, Before: before, After: after, OccurredAt: now})
	}
	return after, nil
}

func (s *NotifyingStore) emit(ev ChangeEvent) {
	select {
	case s.queue <- ev:
	default:
		s.dropped.Add(1)
	}
}

func (s *NotifyingStore) dispatch() {
	for {
		select {
		case <-s.closed:
			return
		case ev := <-s.queue:
			s.listenersMu.RLock()
			listeners := append([]ChangeListener(nil), s.listeners...)
			s.listenersMu.RUnlock()
			for _, listener := range listeners {
				listener.OnBeliefChanged(context.Background(), ev)
			}
		}
	}
}
