// Package runtime owns the in-memory execution state for WARP workflows.
package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"manifold/internal/durable"
	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
)

// WorkflowSummary is the small workflow representation needed by catalogs and
// subflow resolvers.
type WorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PublishTool bool   `json:"publish_tool,omitempty"`
}

type runRecord struct {
	ID         string
	UserID     int64
	WorkflowID string
	Status     string
	Input      map[string]any
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Sequence   int64
	Events     []warpp.Event
	Subs       map[chan warpp.Event]struct{}
}

// Runtime manages workflow definitions and live run state. A Runtime is safe
// for concurrent use; workflow persistence remains behind the supplied store.
type Runtime struct {
	mu      sync.RWMutex
	store   persist.WarppWorkflowStore
	durable *durable.Client
	runs    map[string]*runRecord
}

// New constructs a workflow runtime backed by store. The optional durable
// client lets the runtime hydrate runs that were created by another process.
func New(store persist.WarppWorkflowStore, durableClients ...*durable.Client) *Runtime {
	var durableClient *durable.Client
	if len(durableClients) > 0 {
		durableClient = durableClients[0]
	}
	return &Runtime{
		store:   store,
		durable: durableClient,
		runs:    map[string]*runRecord{},
	}
}

// ListWorkflowSummaries returns the workflow catalog visible to userID.
func (s *Runtime) ListWorkflowSummaries(ctx context.Context, userID int64) ([]WorkflowSummary, error) {
	if s.store == nil {
		return nil, fmt.Errorf("warpp store unavailable")
	}
	records, err := s.store.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowSummary, 0, len(records))
	for _, rec := range records {
		out = append(out, WorkflowSummary{
			ID:          rec.Document.ID,
			Name:        rec.Document.Name,
			Description: rec.Document.Description,
			PublishTool: rec.Document.Publish.Tool,
		})
	}
	return out, nil
}

// GetWorkflow loads one workflow visible to userID.
func (s *Runtime) GetWorkflow(ctx context.Context, userID int64, workflowID string) (warpp.Document, warpp.Canvas, bool, error) {
	if s.store == nil {
		return warpp.Document{}, warpp.Canvas{}, false, fmt.Errorf("warpp store unavailable")
	}
	rec, ok, err := s.store.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return warpp.Document{}, warpp.Canvas{}, false, err
	}
	if !ok {
		return warpp.Document{}, warpp.Canvas{}, false, nil
	}
	return rec.Document, rec.Canvas, true, nil
}

// UpsertWorkflow stores a workflow definition for userID.
func (s *Runtime) UpsertWorkflow(ctx context.Context, userID int64, doc warpp.Document, canvas warpp.Canvas) (persist.WarppWorkflowRecord, bool, error) {
	if s.store == nil {
		return persist.WarppWorkflowRecord{}, false, fmt.Errorf("warpp store unavailable")
	}
	return s.store.UpsertWorkflow(ctx, userID, persist.WarppWorkflowRecord{
		UserID:   userID,
		Document: doc,
		Canvas:   canvas,
	})
}

// DeleteWorkflow removes a workflow definition and reports whether it existed.
func (s *Runtime) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) (bool, error) {
	if s.store == nil {
		return false, fmt.Errorf("warpp store unavailable")
	}
	_, found, err := s.store.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := s.store.DeleteWorkflow(ctx, userID, workflowID); err != nil {
		return false, err
	}
	return true, nil
}

// CreateRun creates an in-memory run and returns its generated ID.
func (s *Runtime) CreateRun(userID int64, workflowID string, input map[string]any) string {
	runID := fmt.Sprintf("warpprun_%d", time.Now().UnixNano())
	s.CreateRunWithID(userID, workflowID, runID, input)
	return runID
}

// CreateRunWithID creates or attaches to an in-memory run with a stable ID.
func (s *Runtime) CreateRunWithID(userID int64, workflowID, runID string, input map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, ok := s.runs[runID]; ok && run.UserID == userID {
		return runID
	}
	now := time.Now().UTC()
	s.runs[runID] = &runRecord{
		ID:         runID,
		UserID:     userID,
		WorkflowID: workflowID,
		Status:     warpp.StatusRunning,
		Input:      cloneMap(input),
		CreatedAt:  now,
		UpdatedAt:  now,
		Events:     make([]warpp.Event, 0, 32),
		Subs:       map[chan warpp.Event]struct{}{},
	}
	return runID
}

// AppendRunEvent records an event for a user's run and publishes it to live
// subscribers. Events with an existing sequence are treated as idempotent.
func (s *Runtime) AppendRunEvent(userID int64, runID string, event warpp.Event) bool {
	s.mu.Lock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		s.mu.Unlock()
		return false
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.Sequence <= 0 {
		run.Sequence++
		event.Sequence = run.Sequence
	} else {
		for _, existing := range run.Events {
			if existing.Sequence == event.Sequence {
				s.mu.Unlock()
				return true
			}
		}
		if event.Sequence > run.Sequence {
			run.Sequence = event.Sequence
		}
	}
	event.RunID = runID
	run.Events = append(run.Events, event)
	run.UpdatedAt = event.OccurredAt
	switch event.Type {
	case warpp.EventRunCompleted:
		run.Status = event.Status
		if run.Status == "" {
			run.Status = warpp.StatusCompleted
		}
	case warpp.EventRunFailed:
		run.Status = warpp.StatusFailed
		if strings.TrimSpace(event.Error) != "" {
			run.Error = event.Error
		}
	case warpp.EventRunCancelled:
		run.Status = warpp.StatusCancelled
	}
	subs := make([]chan warpp.Event, 0, len(run.Subs))
	for ch := range run.Subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
	return true
}

// GetRunEvents returns a snapshot of a user's run events and status.
func (s *Runtime) GetRunEvents(userID int64, runID string) ([]warpp.Event, string, bool) {
	s.mu.RLock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		s.mu.RUnlock()
		if !s.hydrateRunFromDurable(context.Background(), userID, runID) {
			return nil, "", false
		}
		s.mu.RLock()
		run, ok = s.runs[runID]
		if !ok || run.UserID != userID {
			s.mu.RUnlock()
			return nil, "", false
		}
	}
	defer s.mu.RUnlock()
	out := make([]warpp.Event, len(run.Events))
	copy(out, run.Events)
	return out, run.Status, true
}

// SubscribeRun returns a snapshot and, while a run is active, a channel for
// subsequent events. A completed run returns done=true and no channel.
func (s *Runtime) SubscribeRun(userID int64, runID string) ([]warpp.Event, chan warpp.Event, bool, bool) {
	s.mu.Lock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		s.mu.Unlock()
		if !s.hydrateRunFromDurable(context.Background(), userID, runID) {
			return nil, nil, false, false
		}
		s.mu.Lock()
		run, ok = s.runs[runID]
		if !ok || run.UserID != userID {
			s.mu.Unlock()
			return nil, nil, false, false
		}
	}
	defer s.mu.Unlock()
	snapshot := make([]warpp.Event, len(run.Events))
	copy(snapshot, run.Events)
	done := run.Status != warpp.StatusRunning
	if done {
		return snapshot, nil, true, true
	}
	ch := make(chan warpp.Event, 64)
	run.Subs[ch] = struct{}{}
	return snapshot, ch, false, true
}

// UnsubscribeRun removes a live event subscriber.
func (s *Runtime) UnsubscribeRun(runID string, ch chan warpp.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.Subs == nil {
		return
	}
	delete(run.Subs, ch)
}

func (s *Runtime) hydrateRunFromDurable(ctx context.Context, userID int64, runID string) bool {
	if s == nil || s.durable == nil {
		return false
	}
	store := s.durable.Store()
	if store == nil {
		return false
	}
	task, ok, err := store.GetTask(ctx, userID, runID)
	if err != nil || !ok {
		return false
	}
	events, _, ok, err := s.durable.ListEvents(ctx, userID, runID, 0)
	if err != nil || !ok {
		return false
	}
	workflowID, _ := task.Params["workflow_id"].(string)
	input, _ := task.Params["input"].(map[string]any)
	runEvents := make([]warpp.Event, 0, len(events))
	var sequence int64
	for _, durableEvent := range events {
		runEvent, ok := eventFromDurable(durableEvent)
		if !ok {
			continue
		}
		runEvents = append(runEvents, runEvent)
		if runEvent.Sequence > sequence {
			sequence = runEvent.Sequence
		}
	}
	now := time.Now().UTC()
	status := statusFromDurable(task.Status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.runs[runID]; ok && existing.UserID == userID {
		return true
	}
	s.runs[runID] = &runRecord{
		ID:         runID,
		UserID:     userID,
		WorkflowID: workflowID,
		Status:     status,
		Input:      cloneMap(input),
		Error:      task.Error,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  now,
		Sequence:   sequence,
		Events:     runEvents,
		Subs:       map[chan warpp.Event]struct{}{},
	}
	return true
}

func EventPayload(ev warpp.Event) map[string]any {
	payload := map[string]any{
		"type":    string(ev.Type),
		"status":  ev.Status,
		"message": ev.Message,
		"error":   ev.Error,
	}
	if strings.TrimSpace(ev.NodePath) != "" {
		payload["node_path"] = ev.NodePath
	}
	if ev.Outputs != nil {
		payload["outputs"] = ev.Outputs
	}
	return payload
}

func eventFromDurable(ev durable.Event) (warpp.Event, bool) {
	if !strings.HasPrefix(ev.Name, "warpp.") {
		return warpp.Event{}, false
	}
	eventType, _ := ev.Payload["type"].(string)
	if strings.TrimSpace(eventType) == "" {
		eventType = strings.TrimPrefix(ev.Name, "warpp.")
	}
	nodePath, _ := ev.Payload["node_path"].(string)
	status, _ := ev.Payload["status"].(string)
	message, _ := ev.Payload["message"].(string)
	errText, _ := ev.Payload["error"].(string)
	outputs, _ := ev.Payload["outputs"].(map[string]any)
	return warpp.Event{
		RunID:      ev.TaskID,
		Sequence:   ev.Sequence,
		Type:       warpp.EventType(eventType),
		NodePath:   nodePath,
		Status:     status,
		Message:    message,
		Outputs:    outputs,
		Error:      errText,
		OccurredAt: ev.OccurredAt,
	}, true
}

func statusFromDurable(status durable.TaskStatus) string {
	switch status {
	case durable.TaskStatusCompleted:
		return warpp.StatusCompleted
	case durable.TaskStatusFailed:
		return warpp.StatusFailed
	case durable.TaskStatusCancelled:
		return warpp.StatusCancelled
	default:
		return warpp.StatusRunning
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}
