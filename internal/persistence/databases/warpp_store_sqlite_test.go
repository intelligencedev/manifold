package databases

import (
	"context"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
)

func TestSQLiteWarppStoreCRUD(t *testing.T) {
	store := NewSQLiteWarppStore(openTestSQLite(t))
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	doc := warpp.Document{ID: "wf1", Name: "One", Nodes: []warpp.Node{
		{ID: "a", Type: "data.parse", Inputs: map[string]warpp.Input{
			"text": {One: &warpp.Binding{Value: "{}", HasValue: true}}}}}}
	rec, created, err := store.UpsertWorkflow(ctx, 7, persist.WarppWorkflowRecord{UserID: 7, Document: doc})
	if err != nil || !created {
		t.Fatalf("upsert: %v created=%v", err, created)
	}
	if rec.Document.ID != "wf1" {
		t.Fatalf("roundtrip doc: %+v", rec.Document)
	}
	got, found, err := store.GetWorkflow(ctx, 7, "wf1")
	if err != nil || !found || got.Document.Name != "One" {
		t.Fatalf("get: %v %v %+v", err, found, got)
	}
	if _, found, _ := store.GetWorkflow(ctx, 8, "wf1"); found {
		t.Fatal("user scoping broken")
	}
	doc.Name = "Two"
	_, created, err = store.UpsertWorkflow(ctx, 7, persist.WarppWorkflowRecord{UserID: 7, Document: doc})
	if err != nil || created {
		t.Fatalf("update should not report created: %v %v", err, created)
	}
	list, err := store.ListWorkflows(ctx, 7)
	if err != nil || len(list) != 1 || list[0].Document.Name != "Two" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := store.DeleteWorkflow(ctx, 7, "wf1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetWorkflow(ctx, 7, "wf1"); found {
		t.Fatal("delete failed")
	}
}
