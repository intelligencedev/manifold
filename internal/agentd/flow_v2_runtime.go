package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"manifold/internal/durable"
	"manifold/internal/flow"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
)

type flowV2RunRecord struct {
	ID         string
	UserID     int64
	WorkflowID string
	Status     string
	Input      map[string]any
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Sequence   int64
	Events     []flow.RunEvent
	Subs       map[chan flow.RunEvent]struct{}
}

type flowV2Runtime struct {
	mu      sync.RWMutex
	store   persist.FlowV2WorkflowStore
	durable *durable.Client
	runs    map[string]*flowV2RunRecord
}

type flowNodeResult struct {
	nodeID  string
	output  map[string]any
	err     error
	skipped bool
}

func newFlowV2Runtime(store persist.FlowV2WorkflowStore, durableClients ...*durable.Client) *flowV2Runtime {
	if store == nil {
		store = databases.NewPostgresFlowV2Store(nil)
	}
	var durableClient *durable.Client
	if len(durableClients) > 0 {
		durableClient = durableClients[0]
	}
	return &flowV2Runtime{
		store:   store,
		durable: durableClient,
		runs:    map[string]*flowV2RunRecord{},
	}
}

func (s *flowV2Runtime) listWorkflowSummaries(ctx context.Context, userID int64) ([]flow.WorkflowSummary, error) {
	records, err := s.store.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []flow.WorkflowSummary{}, nil
	}
	out := make([]flow.WorkflowSummary, 0, len(records))
	for _, rec := range records {
		out = append(out, flow.WorkflowSummary{
			ID:          rec.Workflow.ID,
			Name:        rec.Workflow.Name,
			Description: rec.Workflow.Description,
		})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].ID) < strings.ToLower(out[j-1].ID); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *flowV2Runtime) getWorkflow(ctx context.Context, userID int64, workflowID string) (flow.Workflow, flow.WorkflowCanvas, bool, error) {
	rec, ok, err := s.store.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return flow.Workflow{}, flow.WorkflowCanvas{}, false, err
	}
	if !ok {
		return flow.Workflow{}, flow.WorkflowCanvas{}, false, nil
	}
	return cloneWorkflow(rec.Workflow), cloneCanvas(rec.Canvas), true, nil
}

func (s *flowV2Runtime) upsertWorkflow(ctx context.Context, userID int64, wf flow.Workflow, canvas flow.WorkflowCanvas) (persist.FlowV2WorkflowRecord, bool, error) {
	return s.store.UpsertWorkflow(ctx, userID, persist.FlowV2WorkflowRecord{
		UserID:   userID,
		Workflow: cloneWorkflow(wf),
		Canvas:   cloneCanvas(canvas),
	})
}

func (s *flowV2Runtime) deleteWorkflow(ctx context.Context, userID int64, workflowID string) (bool, error) {
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

func (s *flowV2Runtime) createRun(userID int64, workflowID string, input map[string]any) string {
	runID := fmt.Sprintf("flowrun_%d", time.Now().UnixNano())
	s.createRunWithID(userID, workflowID, runID, input)
	return runID
}

func (s *flowV2Runtime) createRunWithID(userID int64, workflowID, runID string, input map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, ok := s.runs[runID]; ok && run.UserID == userID {
		return runID
	}
	now := time.Now().UTC()
	s.runs[runID] = &flowV2RunRecord{
		ID:         runID,
		UserID:     userID,
		WorkflowID: workflowID,
		Status:     "running",
		Input:      cloneMap(input),
		CreatedAt:  now,
		UpdatedAt:  now,
		Events:     make([]flow.RunEvent, 0, 32),
		Subs:       map[chan flow.RunEvent]struct{}{},
	}
	return runID
}

func (s *flowV2Runtime) appendRunEvent(userID int64, runID string, event flow.RunEvent) bool {
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
	if event.Output != nil {
		event.Output = cloneMap(event.Output)
	}
	run.Events = append(run.Events, event)
	run.UpdatedAt = event.OccurredAt
	switch event.Type {
	case flow.RunEventTypeRunCompleted:
		run.Status = "completed"
	case flow.RunEventTypeRunFailed:
		run.Status = "failed"
		if strings.TrimSpace(event.Error) != "" {
			run.Error = event.Error
		}
	case flow.RunEventTypeRunCancelled:
		run.Status = "cancelled"
	}
	subs := make([]chan flow.RunEvent, 0, len(run.Subs))
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

func (s *flowV2Runtime) getRunEvents(userID int64, runID string) ([]flow.RunEvent, string, bool) {
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
	out := make([]flow.RunEvent, len(run.Events))
	copy(out, run.Events)
	return out, run.Status, true
}

func (s *flowV2Runtime) subscribeRun(userID int64, runID string) ([]flow.RunEvent, chan flow.RunEvent, bool, bool) {
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
	snapshot := make([]flow.RunEvent, len(run.Events))
	copy(snapshot, run.Events)
	done := run.Status != "running"
	if done {
		return snapshot, nil, true, true
	}
	ch := make(chan flow.RunEvent, 64)
	run.Subs[ch] = struct{}{}
	return snapshot, ch, false, true
}

func (s *flowV2Runtime) hydrateRunFromDurable(ctx context.Context, userID int64, runID string) bool {
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
	runEvents := make([]flow.RunEvent, 0, len(events))
	var sequence int64
	for _, durableEvent := range events {
		runEvent, ok := flowEventFromDurable(durableEvent)
		if !ok {
			continue
		}
		runEvents = append(runEvents, runEvent)
		if runEvent.Sequence > sequence {
			sequence = runEvent.Sequence
		}
	}
	now := time.Now().UTC()
	status := flowStatusFromDurable(task.Status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.runs[runID]; ok && existing.UserID == userID {
		return true
	}
	s.runs[runID] = &flowV2RunRecord{
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
		Subs:       map[chan flow.RunEvent]struct{}{},
	}
	return true
}

func (s *flowV2Runtime) unsubscribeRun(runID string, ch chan flow.RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.Subs == nil {
		return
	}
	delete(run.Subs, ch)
}

func flowEventPayload(ev flow.RunEvent) map[string]any {
	payload := map[string]any{
		"type":    string(ev.Type),
		"status":  ev.Status,
		"message": ev.Message,
		"error":   ev.Error,
	}
	if strings.TrimSpace(ev.NodeID) != "" {
		payload["node_id"] = ev.NodeID
	}
	if ev.Output != nil {
		payload["output"] = cloneMap(ev.Output)
	}
	return payload
}

func flowEventFromDurable(ev durable.Event) (flow.RunEvent, bool) {
	if !strings.HasPrefix(ev.Name, "flow.") {
		return flow.RunEvent{}, false
	}
	eventType, _ := ev.Payload["type"].(string)
	if strings.TrimSpace(eventType) == "" {
		eventType = strings.TrimPrefix(ev.Name, "flow.")
	}
	nodeID, _ := ev.Payload["node_id"].(string)
	status, _ := ev.Payload["status"].(string)
	message, _ := ev.Payload["message"].(string)
	errText, _ := ev.Payload["error"].(string)
	output, _ := ev.Payload["output"].(map[string]any)
	return flow.RunEvent{
		RunID:      ev.TaskID,
		Sequence:   ev.Sequence,
		Type:       flow.RunEventType(eventType),
		NodeID:     nodeID,
		Status:     status,
		Message:    message,
		Output:     cloneMap(output),
		Error:      errText,
		OccurredAt: ev.OccurredAt,
	}, true
}

func flowStatusFromDurable(status durable.TaskStatus) string {
	switch status {
	case durable.TaskStatusCompleted:
		return "completed"
	case durable.TaskStatusFailed:
		return "failed"
	case durable.TaskStatusCancelled:
		return "cancelled"
	default:
		return "running"
	}
}

func loadDurableNodeCheckpoint(ctx context.Context, nodeID string) (map[string]any, bool, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok || tc.Store == nil {
		return nil, false, nil
	}
	raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, "node:"+nodeID)
	if err != nil || !found {
		return nil, found, err
	}
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, true, err
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, true, nil
}
