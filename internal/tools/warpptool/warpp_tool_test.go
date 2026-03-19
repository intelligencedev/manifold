package warpptool

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"manifold/internal/flow"
	"manifold/internal/tools"
)

type fakeRunner struct {
	userID     int64
	workflowID string
	input      map[string]any
	result     map[string]any
	err        error
}

func (f *fakeRunner) ExecuteWorkflowSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error) {
	f.userID = userID
	f.workflowID = workflowID
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestSanitize(t *testing.T) {
	t.Parallel()
	if got := sanitize(" Daily Summary / QA "); got != "daily_summary_qa" {
		t.Fatalf("sanitize = %q, want daily_summary_qa", got)
	}
	if got := sanitize("***"); got != "workflow" {
		t.Fatalf("sanitize fallback = %q, want workflow", got)
	}
}

func TestSyncAllRegistersUniqueWorkflowTools(t *testing.T) {
	t.Parallel()
	reg := tools.NewRegistry()
	runner := &fakeRunner{}
	names := SyncAll(reg, runner, 42, []flow.WorkflowSummary{
		{ID: "wf-alpha", Name: "Daily Summary", Description: "Summarize things."},
		{ID: "wf-beta", Name: "Daily Summary", Description: "Second summary."},
		{ID: "wf-gamma", Name: ""},
	})
	want := []string{"warpp_wf_alpha", "warpp_wf_beta", "warpp_wf_gamma"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("SyncAll names = %#v, want %#v", names, want)
	}
	gotSchemas := tools.SchemaNames(reg)
	if !reflect.DeepEqual(gotSchemas, want) {
		t.Fatalf("registered schema names = %#v, want %#v", gotSchemas, want)
	}
	UnregisterAll(reg, names)
	if got := tools.SchemaNames(reg); len(got) != 0 {
		t.Fatalf("schema names after unregister = %#v, want empty", got)
	}
}

func TestWorkflowToolCallExecutesRunner(t *testing.T) {
	t.Parallel()
	reg := tools.NewRegistry()
	runner := &fakeRunner{result: map[string]any{"payload": "done"}}
	names := SyncAll(reg, runner, 9, []flow.WorkflowSummary{{ID: "wf-1", Name: "Research Flow"}})
	if len(names) != 1 {
		t.Fatalf("expected one registered tool, got %d", len(names))
	}
	rawResult, err := reg.Dispatch(context.Background(), names[0], json.RawMessage(`{"query":"hello world"}`))
	if err != nil {
		t.Fatalf("Dispatch error = %v", err)
	}
	if runner.userID != 9 || runner.workflowID != "wf-1" {
		t.Fatalf("runner called with user=%d workflow=%q", runner.userID, runner.workflowID)
	}
	if got := runner.input["query"]; got != "hello world" {
		t.Fatalf("runner input query = %#v, want hello world", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(rawResult, &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %#v, want true", payload["ok"])
	}
	if payload["payload"] != "done" {
		t.Fatalf("payload = %#v, want done", payload["payload"])
	}
	if payload["workflow_id"] != "wf-1" {
		t.Fatalf("workflow_id = %#v, want wf-1", payload["workflow_id"])
	}
	if payload["workflow_name"] != "wf-1" {
		t.Fatalf("workflow_name = %#v, want wf-1", payload["workflow_name"])
	}
}
