package agentd

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

type warppWorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PublishTool bool   `json:"publish_tool,omitempty"`
}

type warppRunRecord struct {
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

type warppRuntime struct {
	mu      sync.RWMutex
	store   persist.WarppWorkflowStore
	durable *durable.Client
	runs    map[string]*warppRunRecord
}

func newWarppRuntime(store persist.WarppWorkflowStore, durableClients ...*durable.Client) *warppRuntime {
	var durableClient *durable.Client
	if len(durableClients) > 0 {
		durableClient = durableClients[0]
	}
	return &warppRuntime{
		store:   store,
		durable: durableClient,
		runs:    map[string]*warppRunRecord{},
	}
}

func (s *warppRuntime) listWorkflowSummaries(ctx context.Context, userID int64) ([]warppWorkflowSummary, error) {
	if s.store == nil {
		return nil, fmt.Errorf("warpp store unavailable")
	}
	records, err := s.store.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]warppWorkflowSummary, 0, len(records))
	for _, rec := range records {
		out = append(out, warppWorkflowSummary{
			ID:          rec.Document.ID,
			Name:        rec.Document.Name,
			Description: rec.Document.Description,
			PublishTool: rec.Document.Publish.Tool,
		})
	}
	return out, nil
}

func (s *warppRuntime) getWorkflow(ctx context.Context, userID int64, workflowID string) (warpp.Document, warpp.Canvas, bool, error) {
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

func (s *warppRuntime) upsertWorkflow(ctx context.Context, userID int64, doc warpp.Document, canvas warpp.Canvas) (persist.WarppWorkflowRecord, bool, error) {
	if s.store == nil {
		return persist.WarppWorkflowRecord{}, false, fmt.Errorf("warpp store unavailable")
	}
	return s.store.UpsertWorkflow(ctx, userID, persist.WarppWorkflowRecord{
		UserID:   userID,
		Document: doc,
		Canvas:   canvas,
	})
}

func (s *warppRuntime) deleteWorkflow(ctx context.Context, userID int64, workflowID string) (bool, error) {
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

func (s *warppRuntime) createRun(userID int64, workflowID string, input map[string]any) string {
	runID := fmt.Sprintf("warpprun_%d", time.Now().UnixNano())
	s.createRunWithID(userID, workflowID, runID, input)
	return runID
}

func (s *warppRuntime) createRunWithID(userID int64, workflowID, runID string, input map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, ok := s.runs[runID]; ok && run.UserID == userID {
		return runID
	}
	now := time.Now().UTC()
	s.runs[runID] = &warppRunRecord{
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

func (s *warppRuntime) appendRunEvent(userID int64, runID string, event warpp.Event) bool {
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

func (s *warppRuntime) getRunEvents(userID int64, runID string) ([]warpp.Event, string, bool) {
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

func (s *warppRuntime) subscribeRun(userID int64, runID string) ([]warpp.Event, chan warpp.Event, bool, bool) {
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

func (s *warppRuntime) unsubscribeRun(runID string, ch chan warpp.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.Subs == nil {
		return
	}
	delete(run.Subs, ch)
}

func (s *warppRuntime) hydrateRunFromDurable(ctx context.Context, userID int64, runID string) bool {
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
		runEvent, ok := warppEventFromDurable(durableEvent)
		if !ok {
			continue
		}
		runEvents = append(runEvents, runEvent)
		if runEvent.Sequence > sequence {
			sequence = runEvent.Sequence
		}
	}
	now := time.Now().UTC()
	status := warppStatusFromDurable(task.Status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.runs[runID]; ok && existing.UserID == userID {
		return true
	}
	s.runs[runID] = &warppRunRecord{
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

func warppEventPayload(ev warpp.Event) map[string]any {
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

func warppEventFromDurable(ev durable.Event) (warpp.Event, bool) {
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

func warppStatusFromDurable(status durable.TaskStatus) string {
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

func (a *app) warppState() *warppRuntime {
	if a.warpp == nil {
		var store persist.WarppWorkflowStore
		if a.mgr != nil {
			store = a.mgr.Warpp
		}
		a.warpp = newWarppRuntime(store, a.durableClient)
	}
	return a.warpp
}
