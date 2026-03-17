package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/flow"
	persist "manifold/internal/persistence"
)

func TestFlowV2StoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewPostgresFlowV2Store(nil)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	record, created, err := store.UpsertWorkflow(ctx, 7, persist.FlowV2WorkflowRecord{
		Workflow: flow.Workflow{
			ID:          "wf_persist",
			Name:        "Persisted Flow",
			Description: "round-trip",
			Trigger: flow.Trigger{
				Type:     flow.TriggerTypeSchedule,
				Schedule: &flow.ScheduleTrigger{Cron: "*/5 * * * *"},
			},
			Nodes: []flow.Node{{
				ID:   "node_1",
				Name: "Node 1",
				Kind: flow.NodeKindAction,
				Type: "tool",
				Tool: "utility_textbox",
				Inputs: map[string]flow.InputBinding{
					"text": {Expression: "$run.input.message"},
				},
			}},
		},
		Canvas: flow.WorkflowCanvas{
			Nodes: map[string]flow.CanvasNode{
				"node_1": {X: 120, Y: 240},
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert workflow: %v", err)
	}
	if !created {
		t.Fatal("expected create on first upsert")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}

	got, found, err := store.GetWorkflow(ctx, 7, "wf_persist")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if !found {
		t.Fatal("expected workflow to exist")
	}
	if got.Workflow.Trigger.Type != flow.TriggerTypeSchedule {
		t.Fatalf("unexpected trigger type: %s", got.Workflow.Trigger.Type)
	}
	if got.Workflow.Nodes[0].Inputs["text"].Expression != "$run.input.message" {
		t.Fatalf("unexpected input binding: %+v", got.Workflow.Nodes[0].Inputs["text"])
	}
	if got.Canvas.Nodes["node_1"].X != 120 || got.Canvas.Nodes["node_1"].Y != 240 {
		t.Fatalf("unexpected canvas node: %+v", got.Canvas.Nodes["node_1"])
	}

	list, err := store.ListWorkflows(ctx, 7)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(list) != 1 || list[0].Workflow.ID != "wf_persist" {
		t.Fatalf("unexpected list result: %+v", list)
	}

	time.Sleep(time.Millisecond)
	updated, created, err := store.UpsertWorkflow(ctx, 7, persist.FlowV2WorkflowRecord{
		Workflow: flow.Workflow{
			ID:   "wf_persist",
			Name: "Persisted Flow Updated",
			Trigger: flow.Trigger{
				Type: flow.TriggerTypeManual,
			},
		},
	})
	if err != nil {
		t.Fatalf("update workflow: %v", err)
	}
	if created {
		t.Fatal("expected update on second upsert")
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("expected updated_at >= created_at: created=%s updated=%s", updated.CreatedAt, updated.UpdatedAt)
	}

	if err := store.DeleteWorkflow(ctx, 7, "wf_persist"); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	_, found, err = store.GetWorkflow(ctx, 7, "wf_persist")
	if err != nil {
		t.Fatalf("get deleted workflow: %v", err)
	}
	if found {
		t.Fatal("expected workflow to be deleted")
	}
}
