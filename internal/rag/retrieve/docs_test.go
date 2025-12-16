package retrieve

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
)

func TestAttachDocMetadata_LoadsFromDocRow(t *testing.T) {
	ctx := context.Background()
	search := databases.NewMemorySearch()
	// Index a document with title and url metadata
	_ = search.Index(ctx, "doc:test:1", "doc body", map[string]string{"title": "T1", "url": "https://ex"})
	// Index a chunk without title/url
	_ = search.Index(ctx, "chunk:doc:test:1:0", "chunk body", map[string]string{"type": "chunk", "doc_id": "doc:test:1"})

	items := []RetrievedItem{{ID: "chunk:doc:test:1:0", Metadata: map[string]string{"doc_id": "doc:test:1"}}}
	out := AttachDocMetadata(ctx, search, items)
	if out[0].DocID != "doc:test:1" {
		t.Fatalf("expected DocID derived as doc:test:1, got %s", out[0].DocID)
	}
	if out[0].Doc.Title != "T1" || out[0].Doc.URL != "https://ex" {
		t.Fatalf("expected title/url from doc row, got %+v", out[0].Doc)
	}
}
