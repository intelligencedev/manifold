package agentd

import (
	"sync"

	"manifold/internal/codeqa"
)

type codeQARuntime struct {
	mu          sync.RWMutex
	subscribers map[int64]map[string]map[chan codeqa.RunEvent]struct{}
}

func newCodeQARuntime() *codeQARuntime {
	return &codeQARuntime{subscribers: make(map[int64]map[string]map[chan codeqa.RunEvent]struct{})}
}

func (r *codeQARuntime) subscribeRun(userID int64, runID string) chan codeqa.RunEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subscribers[userID]; !ok {
		r.subscribers[userID] = make(map[string]map[chan codeqa.RunEvent]struct{})
	}
	if _, ok := r.subscribers[userID][runID]; !ok {
		r.subscribers[userID][runID] = make(map[chan codeqa.RunEvent]struct{})
	}
	ch := make(chan codeqa.RunEvent, 32)
	r.subscribers[userID][runID][ch] = struct{}{}
	return ch
}

func (r *codeQARuntime) unsubscribeRun(userID int64, runID string, ch chan codeqa.RunEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	userSubs := r.subscribers[userID]
	if userSubs == nil {
		return
	}
	runSubs := userSubs[runID]
	if runSubs == nil {
		return
	}
	delete(runSubs, ch)
	close(ch)
	if len(runSubs) == 0 {
		delete(userSubs, runID)
	}
	if len(userSubs) == 0 {
		delete(r.subscribers, userID)
	}
}

func (r *codeQARuntime) publish(userID int64, event codeqa.RunEvent) {
	r.mu.RLock()
	runSubs := r.subscribers[userID][event.RunID]
	subs := make([]chan codeqa.RunEvent, 0, len(runSubs))
	for ch := range runSubs {
		subs = append(subs, ch)
	}
	r.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}
