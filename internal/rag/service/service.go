package service

import (
    "context"
    "time"

    "manifold/internal/persistence/databases"
    "manifold/internal/rag/chunker"
    "manifold/internal/rag/embedder"
    "manifold/internal/rag/ingest"
    "manifold/internal/rag/retrieve"
)

// Service provides high-level RAG operations backed by Search, Vector, and Graph.
type Service struct {
    search databases.FullTextSearch
    vector databases.VectorStore
    graph  databases.GraphDB

    log     Logger
    metrics Metrics
    clock   Clock
    emb     embedder.Embedder
    rerank  retrieve.Reranker
}

// New constructs a Service from a databases.Manager and optional observability.
func New(mgr databases.Manager, opts ...Option) *Service {
    s := &Service{
        search:  mgr.Search,
        vector:  mgr.Vector,
        graph:   mgr.Graph,
        log:     defaultLogger{},
        metrics: NoopMetrics{},
        clock:   SystemClock{},
        emb:     embedder.NewDeterministic(64, true, 0),
        rerank:  retrieve.NoopReranker{},
    }
    for _, o := range opts {
        o(s)
    }
    return s
}

// Option configures the Service during construction.
type Option func(*Service)

// WithLogger sets a custom logger.
func WithLogger(l Logger) Option { return func(s *Service) { s.log = l } }

// WithMetrics sets a custom metrics collector.
func WithMetrics(m Metrics) Option { return func(s *Service) { s.metrics = m } }

// WithClock sets a custom clock implementation.
func WithClock(c Clock) Option { return func(s *Service) { s.clock = c } }

// WithEmbedder sets a custom embedder implementation used during ingestion.
func WithEmbedder(e embedder.Embedder) Option { return func(s *Service) { s.emb = e } }

// WithReranker sets a reranker implementation used during retrieval.
func WithReranker(r retrieve.Reranker) Option { return func(s *Service) { s.rerank = r } }

// Ingest performs chunk-centric ingestion. Stubbed for Milestone 3.
func (s *Service) Ingest(ctx context.Context, in ingest.IngestRequest) (ingest.IngestResponse, error) {
    start := s.clock.Now()
    // Step 1: preprocess (normalize, language, hash)
    pre, err := ingest.Preprocess(ctx, ingest.DefaultLanguageDetector{}, in)
    if err != nil {
        return ingest.IngestResponse{}, err
    }
    // Step 2: idempotency resolution (using Search as lookup proxy when possible)
    // We adapt the FullTextSearch interface to our DocumentLookup if it provides GetByID on doc hash key.
    // For now, rely on a nil lookup path which returns create if unknown.
    decision, err := ingest.ResolveIdempotency(ctx, nil, in.Tenant, in, pre)
    if err != nil {
        return ingest.IngestResponse{}, err
    }
    if decision.Action == "skip" {
        return ingest.IngestResponse{
            DocID:    decision.DocID,
            Version:  decision.Version,
            ChunkIDs: nil,
            Stats: ingest.IngestStats{
                NumChunks:     0,
                TotalTokens:   0,
                VectorUpserts: 0,
                Duration:      s.clock.Now().Sub(start),
            },
        }, nil
    }

    // Step 3: chunking
    ch := chunker.SimpleChunker{}
    chunks, err := ch.Chunk(pre.Text, in.Options.Chunking)
    if err != nil {
        return ingest.IngestResponse{}, err
    }

    // Step 4: index into Search (documents and chunks) with fallback path
    if err := ingest.UpsertDocumentToSearch(ctx, s.search, in.ID, in, pre, decision.Version); err != nil {
        return ingest.IngestResponse{}, err
    }
    // adapt chunker.Chunk to ingest.ChunkRecord
    crecs := make([]ingest.ChunkRecord, 0, len(chunks))
    for _, c := range chunks {
        crecs = append(crecs, ingest.ChunkRecord{Index: c.Index, Text: c.Text})
    }
    chunkIDs, err := ingest.UpsertChunksToSearch(ctx, s.search, in.ID, pre.Language, crecs, in, decision.Version)
    if err != nil {
        return ingest.IngestResponse{}, err
    }

    // Step 5: embeddings (optional)
    vecUpserts := 0
    if in.Options.Embedding.Enabled && s.vector != nil {
        n, err := ingest.UpsertChunkEmbeddings(ctx, s.vector, s.emb, in.ID, pre.Language, crecs, in, decision.Version)
        if err != nil {
            return ingest.IngestResponse{}, err
        }
        vecUpserts = n
    }

    // Step 6: graph upserts (optional)
    if in.Options.Graph.Enabled && s.graph != nil {
        if _, err := ingest.UpsertDocAndChunksGraph(ctx, s.graph, in.ID, pre, in, crecs, decision.Version); err != nil {
            return ingest.IngestResponse{}, err
        }
    }

    dur := s.clock.Now().Sub(start)
    return ingest.IngestResponse{
        DocID:    in.ID,
        Version:  decision.Version,
        ChunkIDs: chunkIDs,
        Stats: ingest.IngestStats{
            NumChunks:     len(chunks),
            TotalTokens:   approxTokens(pre.Text),
            VectorUpserts: vecUpserts,
            Duration:      dur,
        },
        Warnings: nil,
    }, nil
}

// Retrieve executes a hybrid retrieval query. Stubbed for Milestone 3.
func (s *Service) Retrieve(ctx context.Context, q string, opt retrieve.RetrieveOptions) (retrieve.RetrieveResponse, error) {
    // Plan query
    plan := retrieve.BuildQueryPlan(ctx, q, opt)
    // For now, we reuse deterministic embedder to get a query vector when vector store is present.
    var qvec []float32
    if s.vector != nil && s.emb != nil && plan.VecK > 0 {
        emb, err := s.emb.EmbedBatch(ctx, []string{plan.Query})
        if err != nil {
            return retrieve.RetrieveResponse{}, err
        }
        if len(emb) > 0 {
            qvec = emb[0]
        }
    }

    // Run parallel candidates
    ftRes, vecRes, diag, err := retrieve.ParallelCandidates(ctx, s.search, s.vector, plan, qvec)
    if err != nil {
        return retrieve.RetrieveResponse{}, err
    }

    // Fusion: use RRF (with optional diversification) when requested, else simple concat.
    var items []retrieve.RetrievedItem
    if opt.UseRRF {
        items = retrieve.FuseAndDiversify(ftRes, vecRes, plan, opt)
    } else {
        items = make([]retrieve.RetrievedItem, 0, len(ftRes)+len(vecRes))
        for _, r := range ftRes {
            items = append(items, retrieve.RetrievedItem{ID: r.ID, Score: r.Score, Snippet: r.Snippet, Text: r.Text, Metadata: r.Metadata})
        }
        for _, r := range vecRes {
            items = append(items, retrieve.RetrievedItem{ID: r.ID, Score: r.Score, Metadata: r.Metadata})
        }
        // Cap to K
        k := opt.K
        if k <= 0 { k = 10 }
        if len(items) > k { items = items[:k] }
    }
    // Graph augment + optional rerank + final prune
    items, addDbg, err := retrieve.AssembleResults(ctx, s.graph, s.rerank, plan, opt, items)
    if err != nil {
        return retrieve.RetrieveResponse{}, err
    }

    // Package results: snippets, optional full text, doc metadata, and explanations
    if opt.IncludeSnippet {
        items = retrieve.GenerateSnippets(ctx, s.search, items, retrieve.SnippetOptions{Lang: plan.Lang, Query: plan.Query})
    }
    if opt.IncludeText && s.search != nil {
        // ensure Text present for items lacking it
        for i := range items {
            if items[i].Text != "" { continue }
            if doc, ok, _ := s.search.GetByID(ctx, items[i].ID); ok {
                items[i].Text = doc.Text
            }
        }
    }
    // Attach doc metadata (title, url)
    items = retrieve.AttachDocMetadata(ctx, s.search, items)

    // Add basic per-item explanations when available from fusion diagnostics in metadata
    for i := range items {
        if items[i].Explanation == nil {
            items[i].Explanation = map[string]any{}
        }
        // Carry doc_id for transparency
        if items[i].DocID == "" {
            items[i].DocID = retrieve.DeriveDocIDPublic(items[i].ID, items[i].Metadata)
        }
    }

    debug := map[string]any{
        "plan": map[string]any{"lang": plan.Lang, "ftK": plan.FtK, "vecK": plan.VecK},
        "diagnostics": map[string]any{"ft_ms": ms(diag.FtLatency), "vec_ms": ms(diag.VecLatency), "ft_n": diag.FtCount, "vec_n": diag.VecCount},
    }
    for k, v := range addDbg { debug[k] = v }
    return retrieve.RetrieveResponse{Query: plan.Query, Items: items, Debug: debug}, nil
}

// defaultLogger is a minimal internal logger that drops logs.
type defaultLogger struct{}

func (defaultLogger) Info(string, map[string]any)  {}
func (defaultLogger) Error(string, map[string]any) {}
func (defaultLogger) Debug(string, map[string]any) {}

// approxTokens uses a rough 4 char/token heuristic for metrics only.
func approxTokens(s string) int { return (len(s) + 3) / 4 }

// small inline itoa to avoid strconv import at this layer.
func itoa(i int) string { return fmtInt(i) }

// fmtInt implements a small integer to string conversion for non-negative ints.
func fmtInt(i int) string {
    if i == 0 {
        return "0"
    }
    // allocate a small buffer; int is non-negative
    var b [20]byte
    pos := len(b)
    for i > 0 {
        pos--
        b[pos] = byte('0' + (i % 10))
        i /= 10
    }
    return string(b[pos:])
}

func ms(d time.Duration) int64 { return int64(d / time.Millisecond) }

