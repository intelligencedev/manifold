package ragtool

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/memory/magma"
	"manifold/internal/persistence/databases"
)

func TestMagmaLifecycleToolPruneAndReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mgr := databases.Manager{
		Graph:  databases.NewMemoryGraph(),
		Vector: databases.NewMemoryVector(),
	}
	tool := NewMagmaLifecycleTool(mgr)
	resp, err := tool.s.MagmaService().Ingest(ctx, magma.IngestRequest{
		ID:     "tool-review",
		Tenant: "t1",
		Text:   "Melanie stayed inside because rain started.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := tool.s.MagmaService().DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	pruneRaw := json.RawMessage(`{"action":"prune","policy":{"low_confidence_threshold":0.6}}`)
	got, err := tool.Call(ctx, pruneRaw)
	if err != nil {
		t.Fatalf("Call(prune) error = %v", err)
	}
	pruneResp, ok := got.(map[string]any)
	if !ok || pruneResp["ok"] != true {
		t.Fatalf("unexpected prune response: %#v", got)
	}
	reviewRaw := json.RawMessage(`{"action":"review_edges"}`)
	got, err = tool.Call(ctx, reviewRaw)
	if err != nil {
		t.Fatalf("Call(review_edges) error = %v", err)
	}
	reviewResp, ok := got.(map[string]any)
	if !ok || reviewResp["ok"] != true {
		t.Fatalf("unexpected review response: %#v", got)
	}
	edges, ok := reviewResp["edges"].([]magma.ReviewEdge)
	if !ok || len(edges) != 1 || edges[0].Source != resp.EventID {
		t.Fatalf("expected one review edge for %s, got %#v", resp.EventID, reviewResp["edges"])
	}
}
