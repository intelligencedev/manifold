package retrieve

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
)

func TestGenerateSnippets_FallbackBasic(t *testing.T) {
	ctx := context.Background()
	search := databases.NewMemorySearch()
	// Index a fake chunk with content
	_ = search.Index(ctx, "chunk:doc:1:0", "Alpha bravo charlie delta echo foxtrot golf hotel india juliet", map[string]string{"type": "chunk", "doc_id": "doc:1"})
	items := []RetrievedItem{{ID: "chunk:doc:1:0", Score: 1.0}}
	out := GenerateSnippets(ctx, search, items, SnippetOptions{Lang: "english", Query: "charlie delta"})
	if out[0].Snippet == "" {
		t.Fatalf("expected non-empty snippet from fallback")
	}
}
