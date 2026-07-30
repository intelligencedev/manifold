package agentd

import (
	"context"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
)

type fakeWarppStore struct {
	recs map[string]persist.WarppWorkflowRecord
}

func newFakeWarppStore() *fakeWarppStore {
	return &fakeWarppStore{recs: map[string]persist.WarppWorkflowRecord{}}
}
func (f *fakeWarppStore) key(userID int64, id string) string {
	return string(rune(userID)) + "/" + id
}
func (f *fakeWarppStore) Init(ctx context.Context) error { return nil }
func (f *fakeWarppStore) ListWorkflows(ctx context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	var out []persist.WarppWorkflowRecord
	for _, r := range f.recs {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeWarppStore) GetWorkflow(ctx context.Context, userID int64, id string) (persist.WarppWorkflowRecord, bool, error) {
	r, ok := f.recs[f.key(userID, id)]
	return r, ok, nil
}
func (f *fakeWarppStore) UpsertWorkflow(ctx context.Context, userID int64, rec persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	_, existed := f.recs[f.key(userID, rec.Document.ID)]
	f.recs[f.key(userID, rec.Document.ID)] = rec
	return rec, !existed, nil
}
func (f *fakeWarppStore) DeleteWorkflow(ctx context.Context, userID int64, id string) error {
	delete(f.recs, f.key(userID, id))
	return nil
}

func TestWarppRuntimeRunLifecycle(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	runID := rt.CreateRun(1, "wf", map[string]any{"a": 1})
	if !rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning}) {
		t.Fatal("append to own run failed")
	}
	if rt.AppendRunEvent(2, runID, warpp.Event{Type: warpp.EventRunStarted}) {
		t.Fatal("cross-user append must fail")
	}
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventNodeCompleted, NodePath: "n",
		Outputs: map[string]any{"text": "x"}})
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompletedWithSkips})
	events, status, ok := rt.GetRunEvents(1, runID)
	if !ok || status != warpp.StatusCompletedWithSkips || len(events) != 3 {
		t.Fatalf("events=%d status=%s ok=%v", len(events), status, ok)
	}
	if events[0].Sequence != 1 || events[2].Sequence != 3 {
		t.Fatalf("sequence assignment wrong: %+v", events)
	}
}

func TestWarppRuntimeSubscribe(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	runID := rt.CreateRun(1, "wf", nil)
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning})
	snapshot, ch, done, ok := rt.SubscribeRun(1, runID)
	if !ok || done || len(snapshot) != 1 || ch == nil {
		t.Fatalf("subscribe: %v %v %d", ok, done, len(snapshot))
	}
	rt.AppendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompleted})
	ev := <-ch
	if ev.Type != warpp.EventRunCompleted {
		t.Fatalf("live event: %+v", ev)
	}
	rt.UnsubscribeRun(runID, ch)
	_, ch2, done2, ok2 := rt.SubscribeRun(1, runID)
	if !ok2 || !done2 || ch2 != nil {
		t.Fatalf("finished-run subscribe: %v %v", ok2, done2)
	}
}

func TestWarppRuntimeWorkflowCRUD(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	ctx := context.Background()
	doc := warpp.Document{ID: "w", Name: "W", Publish: warpp.Publish{Tool: true},
		Nodes: []warpp.Node{{ID: "a", Type: "data.parse",
			Inputs: map[string]warpp.Input{"text": {One: &warpp.Binding{Value: "{}", HasValue: true}}}}}}
	if _, _, err := rt.UpsertWorkflow(ctx, 1, doc, warpp.Canvas{}); err != nil {
		t.Fatal(err)
	}
	sums, err := rt.ListWorkflowSummaries(ctx, 1)
	if err != nil || len(sums) != 1 || !sums[0].PublishTool {
		t.Fatalf("summaries: %v %+v", err, sums)
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
