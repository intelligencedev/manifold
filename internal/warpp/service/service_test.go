package service

import (
	"context"
	"testing"

	"manifold/internal/warpp"
	"manifold/internal/warpp/runtime"
)

type testState struct{}

func (testState) ListWorkflowSummaries(context.Context, int64) ([]runtime.WorkflowSummary, error) {
	return nil, nil
}
func (testState) GetWorkflow(context.Context, int64, string) (warpp.Document, warpp.Canvas, bool, error) {
	return warpp.Document{}, warpp.Canvas{}, false, nil
}
func (testState) CreateRun(int64, string, map[string]any) string { return "run" }
func (testState) CreateRunWithID(int64, string, string, map[string]any) string {
	return "run"
}
func (testState) AppendRunEvent(int64, string, warpp.Event) bool { return true }

func TestServiceHelpers(t *testing.T) {
	if New(Deps{State: testState{}}) == nil {
		t.Fatal("New returned nil")
	}
	if BaseResolver() == nil {
		t.Fatal("BaseResolver returned nil")
	}
	if got := SanitizeToolName("workflow/demo"); got != "workflow_demo" {
		t.Fatalf("SanitizeToolName()=%q", got)
	}
	if got := DiagnosticSummary(nil); got != "invalid workflow" {
		t.Fatalf("DiagnosticSummary()=%q", got)
	}
	if got := PortSpecJSONSchema([]warpp.PortSpec{{Name: "topic", Type: "text", Required: true}}); len(got["required"].([]string)) != 1 {
		t.Fatalf("schema=%#v", got)
	}
	if got := JSONSchemaForType("number", "count"); got["type"] != "number" {
		t.Fatalf("number schema=%#v", got)
	}
	if got := SubflowRefs(warpp.Document{Nodes: []warpp.Node{{Type: "flow.child"}}}); len(got) != 1 || got[0] != "child" {
		t.Fatalf("SubflowRefs()=%v", got)
	}
}

func TestServiceUnavailable(t *testing.T) {
	var service *Service
	if _, err := service.ExecuteSync(context.Background(), 0, "missing", nil); err == nil {
		t.Fatal("nil service should fail")
	}
}

func TestServiceExposesWorkflowState(t *testing.T) {
	service := New(Deps{State: testState{}})
	if _, err := service.ListWorkflowSummaries(context.Background(), 42); err != nil {
		t.Fatalf("ListWorkflowSummaries() error = %v", err)
	}
}
