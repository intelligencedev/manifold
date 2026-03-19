package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"manifold/internal/flow"
	llmpkg "manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/tools"
)

type stubFlowV2Store struct {
	records map[int64]map[string]persist.FlowV2WorkflowRecord
}

func (s *stubFlowV2Store) Init(context.Context) error { return nil }

func (s *stubFlowV2Store) ListWorkflows(ctx context.Context, userID int64) ([]persist.FlowV2WorkflowRecord, error) {
	user := s.records[userID]
	out := make([]persist.FlowV2WorkflowRecord, 0, len(user))
	for _, record := range user {
		out = append(out, record)
	}
	return out, nil
}

func (s *stubFlowV2Store) GetWorkflow(ctx context.Context, userID int64, workflowID string) (persist.FlowV2WorkflowRecord, bool, error) {
	user := s.records[userID]
	record, ok := user[workflowID]
	return record, ok, nil
}

func (s *stubFlowV2Store) UpsertWorkflow(ctx context.Context, userID int64, record persist.FlowV2WorkflowRecord) (persist.FlowV2WorkflowRecord, bool, error) {
	return persist.FlowV2WorkflowRecord{}, false, errors.New("not implemented")
}

func (s *stubFlowV2Store) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) error {
	return errors.New("not implemented")
}

func TestExecuteWorkflowSyncReturnsFinalOutput(t *testing.T) {
	t.Parallel()
	reg := newRuntimeStubRegistry(runtimeTestTool{
		name: "test_tool",
		callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"payload": "done"}, nil
		},
	})
	wf := flow.Workflow{
		ID:   "wf-1",
		Name: "Test Workflow",
		Trigger: flow.Trigger{
			Type: flow.TriggerTypeManual,
		},
		Nodes: []flow.Node{{
			ID:   "finish",
			Name: "Finish",
			Kind: flow.NodeKindAction,
			Type: "tool",
			Tool: "test_tool",
		}},
	}
	store := &stubFlowV2Store{records: map[int64]map[string]persist.FlowV2WorkflowRecord{
		0: {
			"wf-1": {
				UserID:    0,
				Workflow:  wf,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}}
	a := &app{flowV2: newFlowV2Runtime(store), baseToolRegistry: reg, toolRegistry: reg}
	result, err := a.ExecuteWorkflowSync(context.Background(), 0, "wf-1", map[string]any{"query": "hello"})
	if err != nil {
		t.Fatalf("ExecuteWorkflowSync error = %v", err)
	}
	if result["workflow_id"] != "wf-1" {
		t.Fatalf("workflow_id = %#v, want wf-1", result["workflow_id"])
	}
	if result["final_node_id"] != "finish" {
		t.Fatalf("final_node_id = %#v, want finish", result["final_node_id"])
	}
	if result["payload"] != "done" {
		t.Fatalf("payload = %#v, want done", result["payload"])
	}
}

type schemaRegistry struct{ names []string }

func (s *schemaRegistry) Schemas() []llmpkg.ToolSchema {
	out := make([]llmpkg.ToolSchema, 0, len(s.names))
	for _, name := range s.names {
		out = append(out, llmpkg.ToolSchema{Name: name})
	}
	return out
}
func (*schemaRegistry) Dispatch(context.Context, string, json.RawMessage) ([]byte, error) {
	return nil, nil
}
func (s *schemaRegistry) Register(t tools.Tool)  { s.names = append(s.names, t.Name()) }
func (s *schemaRegistry) Unregister(name string) {}

func TestSyncWarppToolsRegistersSystemWorkflows(t *testing.T) {
	t.Parallel()
	store := &stubFlowV2Store{records: map[int64]map[string]persist.FlowV2WorkflowRecord{
		0: {
			"wf-1": {Workflow: flow.Workflow{ID: "wf-1", Name: "Daily Summary", Description: "Run daily summary."}},
		},
	}}
	reg := &schemaRegistry{}
	a := &app{flowV2: newFlowV2Runtime(store), baseToolRegistry: reg, toolRegistry: reg}
	a.syncWarppTools(context.Background())
	if len(a.warppToolNames) != 1 || a.warppToolNames[0] != "warpp_wf_1" {
		t.Fatalf("warppToolNames = %#v, want [warpp_wf_1]", a.warppToolNames)
	}
}
