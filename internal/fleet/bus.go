package fleet

import (
	"context"
	"sync"
	"time"
)

type EventKind string

const (
	EventRunStarted    EventKind = "run_started"
	EventRunFinished   EventKind = "run_finished"
	EventRunFailed     EventKind = "run_failed"
	EventToolStart     EventKind = "tool_start"
	EventToolResult    EventKind = "tool_result"
	EventDelegation    EventKind = "delegation"
	EventInputRequest  EventKind = "input_request"
	EventInputAnswered EventKind = "input_answered"
	EventError         EventKind = "error"
	EventFlow          EventKind = "flow_event"
)

type Event struct {
	Kind         EventKind      `json:"kind"`
	RunID        string         `json:"run_id,omitempty"`
	SessionID    string         `json:"session_id,omitempty"`
	ProjectID    string         `json:"project_id,omitempty"`
	ObjectiveID  string         `json:"objective_id,omitempty"`
	WorkflowID   string         `json:"workflow_id,omitempty"`
	Specialist   string         `json:"specialist,omitempty"`
	Agent        string         `json:"agent,omitempty"`
	CallID       string         `json:"call_id,omitempty"`
	ParentCallID string         `json:"parent_call_id,omitempty"`
	ToolID       string         `json:"tool_id,omitempty"`
	Depth        int            `json:"depth,omitempty"`
	Status       string         `json:"status,omitempty"`
	Title        string         `json:"title,omitempty"`
	Message      string         `json:"message,omitempty"`
	At           time.Time      `json:"at"`
	Data         map[string]any `json:"data,omitempty"`
	UserID       int64          `json:"-"`
}

type Bus struct {
	mu          sync.RWMutex
	subs        map[int64]map[chan Event]struct{}
	recent      map[int64][]Event
	recentLimit int
}

func NewBus(recentLimit int) *Bus {
	if recentLimit <= 0 {
		recentLimit = 256
	}
	return &Bus{
		subs:        map[int64]map[chan Event]struct{}{},
		recent:      map[int64][]Event{},
		recentLimit: recentLimit,
	}
}

func (b *Bus) Publish(ev Event) {
	if b == nil {
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	b.mu.Lock()
	recent := append(b.recent[ev.UserID], ev)
	if len(recent) > b.recentLimit {
		recent = append([]Event(nil), recent[len(recent)-b.recentLimit:]...)
	}
	b.recent[ev.UserID] = recent
	subs := make([]chan Event, 0, len(b.subs[ev.UserID]))
	for ch := range b.subs[ev.UserID] {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *Bus) Subscribe(ctx context.Context, userID int64) ([]Event, <-chan Event) {
	if b == nil {
		ch := make(chan Event)
		close(ch)
		return nil, ch
	}
	ch := make(chan Event, 64)
	b.mu.Lock()
	if b.subs[userID] == nil {
		b.subs[userID] = map[chan Event]struct{}{}
	}
	b.subs[userID][ch] = struct{}{}
	snapshot := append([]Event(nil), b.recent[userID]...)
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		if subs := b.subs[userID]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subs, userID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}()
	return snapshot, ch
}

func (b *Bus) Recent(userID int64) []Event {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]Event(nil), b.recent[userID]...)
}
