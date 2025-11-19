package ingest

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
)

func TestUpsertChunkEmbeddings_MemoryVector(t *testing.T) {
	ctx := context.Background()
	vec := databases.NewMemoryVector()
	emb := embedder.NewDeterministic(8, true, 42)
	in := IngestRequest{ID: "doc:acme:1", Tenant: "acme", Source: "test"}
	chunks := []ChunkRecord{{Index: 0, Text: "hello world"}, {Index: 1, Text: "goodbye"}}

	n, err := UpsertChunkEmbeddings(ctx, vec, emb, in.ID, "english", chunks, in, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 upserts, got %d", n)
	}

	// Query by similarity roughly recovers inserted IDs when using one of the texts as query.
	qemb, err := emb.EmbedBatch(ctx, []string{"hello world"})
	if err != nil {
		t.Fatalf("embed error: %v", err)
	}
	res, err := vec.SimilaritySearch(ctx, qemb[0], 5, map[string]string{"tenant": "acme", "doc_id": in.ID})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("expected some results")
	}
	if res[0].ID != "chunk:"+in.ID+":0" {
		t.Fatalf("expected top result chunk 0, got %s", res[0].ID)
	}
}
