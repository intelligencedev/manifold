package runtime_test

import (
	"context"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
	"manifold/internal/warpp/runtime"
)

type fakeStore struct {
	records map[string]persist.WarppWorkflowRecord
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: map[string]persist.WarppWorkflowRecord{}}
}

func (f *fakeStore) key(userID int64, workflowID string) string {
	return string(rune(userID)) + "/" + workflowID
}

func (f *fakeStore) Init(context.Context) error { return nil }

func (f *fakeStore) ListWorkflows(_ context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	var out []persist.WarppWorkflowRecord
	for _, record := range f.records {
		if record.UserID == userID {
			out = append(out, record)
		}
	}
	return out, nil
}

func (f *fakeStore) GetWorkflow(_ context.Context, userID int64, workflowID string) (persist.WarppWorkflowRecord, bool, error) {
	record, ok := f.records[f.key(userID, workflowID)]
	return record, ok, nil
}

func (f *fakeStore) UpsertWorkflow(_ context.Context, userID int64, record persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	_, existed := f.records[f.key(userID, record.Document.ID)]
	f.records[f.key(userID, record.Document.ID)] = record
	return record, !existed, nil
}

func (f *fakeStore) DeleteWorkflow(_ context.Context, userID int64, workflowID string) error {
	delete(f.records, f.key(userID, workflowID))
	return nil
}

func TestRuntimeRunLifecycle(t *testing.T) {
	rt := runtime.New(newFakeStore())
	runID := rt.CreateRun(1, "wf", map[string]any{"a": 1})
	if !rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning}) {
		t.Fatal("append to own run failed")
	}
	if rt.AppendRunEvent(2, runID, warpp.Event{Type: warpp.EventRunStarted}) {
		t.Fatal("cross-user append must fail")
	}
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventNodeCompleted, NodePath: "n", Outputs: map[string]any{"text": "x"}})
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompletedWithSkips})
	events, status, ok := rt.GetRunEvents(1, runID)
	if !ok || status != warpp.StatusCompletedWithSkips || len(events) != 3 {
		t.Fatalf("events=%d status=%s ok=%v", len(events), status, ok)
	}
	if events[0].Sequence != 1 || events[2].Sequence != 3 {
		t.Fatalf("sequence assignment wrong: %+v", events)
	}
}

func TestRuntimeSubscribe(t *testing.T) {
	rt := runtime.New(newFakeStore())
	runID := rt.CreateRun(1, "wf", nil)
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning})
	snapshot, ch, done, ok := rt.SubscribeRun(1, runID)
	if !ok || done || len(snapshot) != 1 || ch == nil {
		t.Fatalf("subscribe: %v %v %d", ok, done, len(snapshot))
	}
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompleted})
	if event := <-ch; event.Type != warpp.EventRunCompleted {
		t.Fatalf("live event: %+v", event)
	}
	rt.UnsubscribeRun(runID, ch)
	_, ch2, done2, ok2 := rt.SubscribeRun(1, runID)
	if !ok2 || !done2 || ch2 != nil {
		t.Fatalf("finished-run subscribe: %v %v", ok2, done2)
	}
}

func TestRuntimeWorkflowCRUD(t *testing.T) {
	rt := runtime.New(newFakeStore())
	ctx := context.Background()
	doc := warpp.Document{ID: "w", Name: "W", Publish: warpp.Publish{Tool: true}}
	if _, created, err := rt.UpsertWorkflow(ctx, 1, doc, warpp.Canvas{}); err != nil || !created {
		t.Fatalf("upsert: created=%v err=%v", created, err)
	}
	summaries, err := rt.ListWorkflowSummaries(ctx, 1)
	if err != nil || len(summaries) != 1 || !summaries[0].PublishTool {
		t.Fatalf("summaries: %v %+v", err, summaries)
	}
	got, _, found, err := rt.GetWorkflow(ctx, 1, "w")
	if err != nil || !found || got.Name != "W" {
		t.Fatalf("get: %v %v", err, found)
	}
	deleted, err := rt.DeleteWorkflow(ctx, 1, "w")
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", err, deleted)
	}
}

func TestEventPayload(t *testing.T) {
	payload := runtime.EventPayload(warpp.Event{
		Type:     warpp.EventNodeCompleted,
		Status:   warpp.StatusCompleted,
		NodePath: "node",
		Outputs:  map[string]any{"value": 1},
	})
	if payload["type"] != string(warpp.EventNodeCompleted) || payload["node_path"] != "node" {
		t.Fatalf("payload: %#v", payload)
	}
}
