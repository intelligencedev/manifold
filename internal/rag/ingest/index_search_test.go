package ingest_test

import (
    "context"
    "testing"

    "manifold/internal/persistence/databases"
    "manifold/internal/rag/chunker"
    ingest "manifold/internal/rag/ingest"
)

func TestUpsertDocumentAndChunks_FallbackMemory(t *testing.T) {
    ctx := context.Background()
    search := databases.NewMemorySearch()

    in := ingest.IngestRequest{
        ID:     "doc:test:1",
        Title:  "Hello",
        URL:    "https://example.com",
        Source: "test",
        Text:   "# Title\n\nPara one.\n\nPara two with more words.",
        Metadata: map[string]any{"a": 1},
        Tenant:   "t1",
        Options: ingest.IngestOptions{Version: 1},
    }
    pre, err := ingest.Preprocess(ctx, ingest.DefaultLanguageDetector{}, in)
    if err != nil { t.Fatalf("preprocess: %v", err) }

    // Document upsert should succeed
    if err := ingest.UpsertDocumentToSearch(ctx, search, in.ID, in, pre, 1); err != nil {
        t.Fatalf("doc upsert: %v", err)
    }
    // Chunk using markdown strategy (to exercise split)
    chunks, err := chunker.SimpleChunker{}.Chunk(pre.Text, ingest.ChunkingOptions{Strategy: "md", MaxTokens: 32})
    if err != nil { t.Fatalf("chunk: %v", err) }
    // adapt chunks to ChunkRecord
    recs := make([]ingest.ChunkRecord, 0, len(chunks))
    for _, c := range chunks { recs = append(recs, ingest.ChunkRecord{Index: c.Index, Text: c.Text}) }
    ids, err := ingest.UpsertChunksToSearch(ctx, search, in.ID, pre.Language, recs, in, 1)
    if err != nil { t.Fatalf("chunks upsert: %v", err) }
    if len(ids) != len(chunks) {
        t.Fatalf("expected %d chunk ids, got %d", len(chunks), len(ids))
    }
    // Ensure search can find the doc and first chunk
    if _, ok, err := search.GetByID(ctx, in.ID); err != nil || !ok {
        t.Fatalf("doc not retrievable: ok=%v err=%v", ok, err)
    }
    if _, ok, err := search.GetByID(ctx, ids[0]); err != nil || !ok {
        t.Fatalf("chunk not retrievable: id=%s ok=%v err=%v", ids[0], ok, err)
    }
}

// fakeChunkSearch implements FullTextSearch and the optional chunk capabilities.
type fakeChunkSearch struct {
    docs     map[string]databases.SearchResult
    hasTable bool
    upserts  []string
}

func (f *fakeChunkSearch) Index(_ context.Context, id, text string, metadata map[string]string) error {
    if f.docs == nil { f.docs = make(map[string]databases.SearchResult) }
    f.docs[id] = databases.SearchResult{ID: id, Text: text, Metadata: metadata}
    return nil
}
func (f *fakeChunkSearch) Remove(_ context.Context, id string) error { delete(f.docs, id); return nil }
func (f *fakeChunkSearch) Search(_ context.Context, _ string, _ int) ([]databases.SearchResult, error) { return nil, nil }
func (f *fakeChunkSearch) GetByID(_ context.Context, id string) (databases.SearchResult, bool, error) {
    r, ok := f.docs[id]
    return r, ok, nil
}
func (f *fakeChunkSearch) HasChunksTable(context.Context) (bool, error) { return f.hasTable, nil }
func (f *fakeChunkSearch) UpsertChunk(_ context.Context, chunkID, docID string, idx int, text string, metadata map[string]string, lang string) error {
    f.upserts = append(f.upserts, chunkID)
    return nil
}

func TestUpsertChunks_UsesChunkTableWhenAvailable(t *testing.T) {
    ctx := context.Background()
    fs := &fakeChunkSearch{hasTable: true, docs: map[string]databases.SearchResult{}}
    in := ingest.IngestRequest{ID: "doc:test:2", Tenant: "t2"}
    chunks := []ingest.ChunkRecord{{Index: 0, Text: "a"}, {Index: 1, Text: "b"}}
    ids, err := ingest.UpsertChunksToSearch(ctx, fs, in.ID, "english", chunks, in, 1)
    if err != nil { t.Fatalf("upsert chunks: %v", err) }
    if len(ids) != 2 { t.Fatalf("expected 2 ids, got %d", len(ids)) }
    if len(fs.upserts) != 2 { t.Fatalf("expected 2 chunk upserts via table, got %d", len(fs.upserts)) }
}


