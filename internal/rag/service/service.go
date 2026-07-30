package service

import (
	"context"
	"maps"
	"time"

	"manifold/internal/agent/memory/magma"
	"manifold/internal/config"
	"manifold/internal/llm"
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

	log      Logger
	metrics  Metrics
	clock    Clock
	emb      embedder.Embedder
	embCfg   config.EmbeddingConfig
	rerank   retrieve.Reranker
	magmaLLM llm.Provider
	magma    *magma.Service
	magmaCfg config.MagmaConfig
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
	}
	for _, o := range opts {
		o(s)
	}
	if mgr.Graph != nil && s.magma == nil {
		s.magma = magma.NewServiceWithConfig(mgr.Graph, mgr.Vector, s.emb, magmaServiceConfig(s.magmaCfg, s.metrics, s.magmaLLM))
	}
	return s
}

// Option configures the Service during construction.
type Option func(*Service)

// WithEmbedder sets a custom embedder implementation used during ingestion.
func WithEmbedder(e embedder.Embedder) Option { return func(s *Service) { s.emb = e } }

// WithEmbeddingConfig sets embedding configuration used for query instructions.
func WithEmbeddingConfig(cfg config.EmbeddingConfig) Option {
	return func(s *Service) { s.embCfg = cfg }
}

// WithReranker sets the optional external reranker used when a retrieval
// request enables reranking.
func WithReranker(rr retrieve.Reranker) Option {
	return func(s *Service) { s.rerank = rr }
}

// WithMagmaService overrides the optional MAGMA backend, primarily for tests
// and custom deployments.
func WithMagmaService(ms *magma.Service) Option {
	return func(s *Service) { s.magma = ms }
}

// WithMagmaConfig sets service-level defaults for opt-in MAGMA ingestion and
// retrieval. Per-request options still override non-zero request fields.
func WithMagmaConfig(cfg config.MagmaConfig) Option {
	return func(s *Service) { s.magmaCfg = cfg }
}

func WithMagmaLLM(provider llm.Provider) Option {
	return func(s *Service) { s.magmaLLM = provider }
}

func (s *Service) MagmaService() *magma.Service {
	if s == nil {
		return nil
	}
	return s.magma
}

func (s *Service) Close() {
	if s == nil || s.magma == nil {
		return
	}
	s.magma.Close()
}

// StartMagmaBackgroundWorkers starts MAGMA consolidation and lifecycle workers
// for long-lived service instances.
func (s *Service) StartMagmaBackgroundWorkers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.startMagmaWorkers(ctx)
}

// Ingest performs chunk-centric ingestion. Stubbed for Milestone 3.
func (s *Service) Ingest(ctx context.Context, in ingest.IngestRequest) (ingest.IngestResponse, error) {
	start := s.clock.Now()
	s.metrics.IncCounter("ingestion_docs_total", map[string]string{"tenant": in.Tenant})

	pre, decision, err := s.prepareIngest(ctx, in)
	if err != nil {
		return ingest.IngestResponse{}, err
	}
	if decision.Action == "skip" {
		return s.skippedIngestResponse(decision, start), nil
	}

	state, err := s.indexIngest(ctx, in, pre, decision)
	if err != nil {
		return ingest.IngestResponse{}, err
	}
	dur := s.recordIngestTotal(start, in.Tenant)
	magmaEventID, warnings, err := s.ingestMagma(ctx, in, pre)
	if err != nil {
		return ingest.IngestResponse{}, err
	}

	return ingest.IngestResponse{
		DocID:        in.ID,
		Version:      decision.Version,
		ChunkIDs:     state.chunkIDs,
		MagmaEventID: magmaEventID,
		Stats: ingest.IngestStats{
			NumChunks:     state.numChunks,
			TotalTokens:   approxTokens(pre.Text),
			VectorUpserts: state.vectorUpserts,
			Duration:      dur,
		},
		Warnings: warnings,
	}, nil
}

type ingestIndexState struct {
	chunkIDs      []string
	numChunks     int
	vectorUpserts int
}

func (s *Service) prepareIngest(ctx context.Context, in ingest.IngestRequest) (ingest.PreprocessedDoc, ingest.IdempotencyDecision, error) {
	t0 := s.clock.Now()
	pre, err := ingest.Preprocess(ctx, ingest.DefaultLanguageDetector{}, in)
	if err != nil {
		return ingest.PreprocessedDoc{}, ingest.IdempotencyDecision{}, err
	}
	s.recordIngestStage("preprocess", in.Tenant, t0)

	t0 = s.clock.Now()
	decision, err := ingest.ResolveIdempotency(ctx, nil, in.Tenant, in, pre)
	if err != nil {
		return ingest.PreprocessedDoc{}, ingest.IdempotencyDecision{}, err
	}
	s.recordIngestStage("idempotency", in.Tenant, t0)
	return pre, decision, nil
}

func (s *Service) skippedIngestResponse(decision ingest.IdempotencyDecision, start time.Time) ingest.IngestResponse {
	return ingest.IngestResponse{
		DocID:    decision.DocID,
		Version:  decision.Version,
		ChunkIDs: nil,
		Stats: ingest.IngestStats{
			Duration: s.clock.Now().Sub(start),
		},
	}
}

func (s *Service) indexIngest(ctx context.Context, in ingest.IngestRequest, pre ingest.PreprocessedDoc, decision ingest.IdempotencyDecision) (ingestIndexState, error) {
	chunks, err := s.chunkIngest(pre.Text, in)
	if err != nil {
		return ingestIndexState{}, err
	}
	chunkReq, chunkIDs, err := s.indexSearch(ctx, in, pre, decision, chunks)
	if err != nil {
		return ingestIndexState{}, err
	}
	vecUpserts, err := s.indexEmbeddings(ctx, in, chunkReq)
	if err != nil {
		return ingestIndexState{}, err
	}
	if err := s.indexGraph(ctx, in, chunkReq, pre); err != nil {
		return ingestIndexState{}, err
	}
	return ingestIndexState{chunkIDs: chunkIDs, numChunks: len(chunks), vectorUpserts: vecUpserts}, nil
}

func (s *Service) chunkIngest(text string, in ingest.IngestRequest) ([]chunker.Chunk, error) {
	t0 := s.clock.Now()
	chunks, err := (chunker.SimpleChunker{}).Chunk(text, in.Options.Chunking)
	if err != nil {
		return nil, err
	}
	s.recordIngestStage("chunk", in.Tenant, t0)
	for range chunks {
		s.metrics.IncCounter("ingestion_chunks_total", map[string]string{"tenant": in.Tenant})
	}
	return chunks, nil
}

func (s *Service) indexSearch(ctx context.Context, in ingest.IngestRequest, pre ingest.PreprocessedDoc, decision ingest.IdempotencyDecision, chunks []chunker.Chunk) (ingest.ChunkIndexRequest, []string, error) {
	t0 := s.clock.Now()
	if err := ingest.UpsertDocumentToSearch(ctx, s.search, in.ID, in, pre, decision.Version); err != nil {
		return ingest.ChunkIndexRequest{}, nil, err
	}
	s.recordIngestStage("search_document", in.Tenant, t0)

	chunkReq := ingest.ChunkIndexRequest{DocID: in.ID, Lang: pre.Language, Chunks: chunkRecords(chunks), Input: in, Version: decision.Version}
	t0 = s.clock.Now()
	chunkIDs, err := ingest.UpsertChunksToSearch(ctx, s.search, chunkReq)
	if err != nil {
		return ingest.ChunkIndexRequest{}, nil, err
	}
	s.recordIngestStage("search_chunks", in.Tenant, t0)
	return chunkReq, chunkIDs, nil
}

func chunkRecords(chunks []chunker.Chunk) []ingest.ChunkRecord {
	records := make([]ingest.ChunkRecord, 0, len(chunks))
	for _, c := range chunks {
		records = append(records, ingest.ChunkRecord{Index: c.Index, Text: c.Text})
	}
	return records
}

func (s *Service) indexEmbeddings(ctx context.Context, in ingest.IngestRequest, chunkReq ingest.ChunkIndexRequest) (int, error) {
	if !in.Options.Embedding.Enabled || s.vector == nil || s.emb == nil {
		return 0, nil
	}
	t0 := s.clock.Now()
	n, err := ingest.UpsertChunkEmbeddings(ctx, s.vector, s.emb, chunkReq)
	if err != nil {
		return 0, err
	}
	s.recordIngestStage("embedding", in.Tenant, t0)
	return n, nil
}

func (s *Service) indexGraph(ctx context.Context, in ingest.IngestRequest, chunkReq ingest.ChunkIndexRequest, pre ingest.PreprocessedDoc) error {
	if !in.Options.Graph.Enabled || s.graph == nil {
		return nil
	}
	t0 := s.clock.Now()
	if _, err := ingest.UpsertDocAndChunksGraph(ctx, s.graph, chunkReq, pre); err != nil {
		return err
	}
	s.recordIngestStage("graph", in.Tenant, t0)
	return nil
}

func (s *Service) recordIngestTotal(start time.Time, tenant string) time.Duration {
	dur := s.clock.Now().Sub(start)
	s.metrics.ObserveHistogram("ingestion_stage_ms", float64(ms(dur)), map[string]string{"stage": "total", "tenant": tenant})
	return dur
}

func (s *Service) ingestMagma(ctx context.Context, in ingest.IngestRequest, pre ingest.PreprocessedDoc) (string, []string, error) {
	magmaOpt := s.normalizeMagmaIngestOptions(in.Options.Magma)
	if !magmaOpt.Enabled || s.magma == nil {
		return "", nil, nil
	}
	t0 := s.clock.Now()
	s.startMagmaWorkers(context.Background())
	resp, err := s.magma.Ingest(ctx, magma.IngestRequest{
		ID:           in.ID,
		Tenant:       in.Tenant,
		SessionID:    magmaOpt.SessionID,
		Text:         pre.Text,
		Metadata:     in.Metadata,
		Graphs:       magmaGraphTypes(magmaOpt.Graphs),
		SemanticTopK: magmaOpt.TopSemanticK,
	})
	if err != nil {
		return "", nil, err
	}
	warnings := s.recordMagmaIngest(in.Tenant, resp.Status, t0)
	return resp.EventID, warnings, nil
}

func (s *Service) startMagmaWorkers(ctx context.Context) {
	if s == nil || s.magma == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.magma.StartConsolidationWorkers(ctx, s.magmaCfg.Consolidation.WorkerCount)
	if s.magmaCfg.Lifecycle.PruneIntervalMinutes > 0 {
		s.magma.StartLifecycleWorker(ctx, time.Duration(s.magmaCfg.Lifecycle.PruneIntervalMinutes)*time.Minute)
	}
}

func (s *Service) recordMagmaIngest(tenant string, status string, start time.Time) []string {
	warnings := []string(nil)
	if status == "queue_full" {
		warnings = append(warnings, "magma consolidation queue is full; event stored but not queued for consolidation")
		s.metrics.IncCounter("magma_consolidation_queue_dropped_total", map[string]string{"tenant": tenant})
	}
	magmaFastMS := float64(ms(s.clock.Now().Sub(start)))
	s.metrics.ObserveHistogram("ingestion_stage_ms", magmaFastMS, map[string]string{"stage": "magma_fast", "tenant": tenant})
	s.metrics.ObserveHistogram("magma_ingestion_fast_ms", magmaFastMS, map[string]string{"tenant": tenant, "status": status})
	s.metrics.IncCounter("magma_events_total", map[string]string{"tenant": tenant})
	return warnings
}

func (s *Service) recordIngestStage(stage string, tenant string, start time.Time) {
	s.metrics.ObserveHistogram("ingestion_stage_ms", float64(ms(s.clock.Now().Sub(start))), map[string]string{"stage": stage, "tenant": tenant})
}

// Retrieve executes a hybrid retrieval query. Stubbed for Milestone 3.
func (s *Service) Retrieve(ctx context.Context, q string, opt retrieve.RetrieveOptions) (retrieve.RetrieveResponse, error) {
	rStart := s.clock.Now()
	opt = s.normalizeRetrieveOptions(opt)
	opt.Magma = s.normalizeMagmaRetrieveOptions(opt.Magma)
	if opt.Magma.Enabled && s.magma != nil {
		return s.retrieveMagma(ctx, q, opt, rStart)
	}
	plan := retrieve.BuildQueryPlan(ctx, q, opt)
	qvec, instruction, instructionUsed, err := s.retrieveQueryVector(ctx, plan, opt)
	if err != nil {
		return retrieve.RetrieveResponse{}, err
	}
	ftRes, vecRes, diag, err := s.retrieveCandidates(ctx, plan, qvec)
	if err != nil {
		return retrieve.RetrieveResponse{}, err
	}
	items, fusionMS := s.fuseRetrieveItems(ftRes, vecRes, plan, opt)
	items, magmaGraphDbg, addDbg, err := s.augmentAndAssembleRetrieve(ctx, q, opt, plan, items)
	if err != nil {
		return retrieve.RetrieveResponse{}, err
	}
	items, pkgMS := s.packageRetrieveItems(ctx, plan, opt, items)
	totalMS := ms(s.clock.Now().Sub(rStart))
	s.metrics.ObserveHistogram("retrieval_stage_ms", float64(totalMS), map[string]string{"stage": "total", "tenant": plan.Tenant})
	debug := retrieveDebug(plan, diag, pkgMS, fusionMS, totalMS, instruction, instructionUsed, addDbg, magmaGraphDbg)
	return retrieve.RetrieveResponse{Query: plan.Query, Items: items, Debug: debug}, nil
}

// defaultLogger is a minimal internal logger that drops logs.
type defaultLogger struct{}

func (defaultLogger) Info(string, map[string]any)  {}
func (defaultLogger) Error(string, map[string]any) {}
func (defaultLogger) Debug(string, map[string]any) {}

// approxTokens uses a rough 4 char/token heuristic for metrics only.
func approxTokens(s string) int { return (len(s) + 3) / 4 }

func ms(d time.Duration) int64 { return int64(d / time.Millisecond) }

func (s *Service) normalizeRetrieveOptions(opt retrieve.RetrieveOptions) retrieve.RetrieveOptions {
	rerankActive := opt.Rerank && s.rerank != nil
	if rerankActive {
		return opt
	}
	opt.UseRRF = true
	if opt.FtK <= 0 && opt.VecK <= 0 && opt.Alpha == 0 {
		opt.Alpha = 0.5
	}
	return opt
}

func (s *Service) normalizeMagmaIngestOptions(opt ingest.MagmaOptions) ingest.MagmaOptions {
	if !opt.Enabled && s.magmaCfg.Enabled {
		opt.Enabled = true
	}
	if opt.ConsolidationModel == "" {
		opt.ConsolidationModel = s.magmaCfg.Consolidation.Model
	}
	if opt.TopSemanticK <= 0 {
		opt.TopSemanticK = s.magmaCfg.Graphs.Semantic.TopK
	}
	if len(opt.Graphs) == 0 && s.magmaCfg.Enabled {
		opt.Graphs = enabledMagmaGraphs(s.magmaCfg)
	}
	return opt
}

func (s *Service) normalizeMagmaRetrieveOptions(opt retrieve.MagmaRetrieveOptions) retrieve.MagmaRetrieveOptions {
	if !opt.Enabled && s.magmaCfg.Enabled {
		opt.Enabled = true
	}
	if opt.MaxHops <= 0 {
		opt.MaxHops = s.magmaCfg.Retrieval.DefaultHops
	}
	if opt.MaxNodes <= 0 {
		opt.MaxNodes = s.magmaCfg.Retrieval.DefaultMaxNodes
	}
	if opt.ContextFormat == "" {
		opt.ContextFormat = s.magmaCfg.Retrieval.ContextFormat
	}
	if opt.IntentClassification == "" {
		opt.IntentClassification = s.magmaCfg.Retrieval.IntentClassification
	}
	return opt
}

func enabledMagmaGraphs(cfg config.MagmaConfig) []string {
	graphs := []string{}
	if cfg.Graphs.Semantic.Enabled {
		graphs = append(graphs, "semantic")
	}
	if cfg.Graphs.Temporal.Enabled {
		graphs = append(graphs, "temporal")
	}
	if cfg.Graphs.Causal.Enabled {
		graphs = append(graphs, "causal")
	}
	if cfg.Graphs.Entity.Enabled {
		graphs = append(graphs, "entity")
	}
	if len(graphs) == 0 {
		return []string{"semantic", "temporal", "causal", "entity"}
	}
	return graphs
}

func magmaGraphTypes(graphs []string) []magma.GraphType {
	out := make([]magma.GraphType, 0, len(graphs))
	seen := map[magma.GraphType]bool{}
	for _, graph := range graphs {
		var graphType magma.GraphType
		switch graph {
		case "semantic":
			graphType = magma.GraphSemantic
		case "temporal":
			graphType = magma.GraphTemporal
		case "causal":
			graphType = magma.GraphCausal
		case "entity":
			graphType = magma.GraphEntity
		default:
			continue
		}
		if seen[graphType] {
			continue
		}
		seen[graphType] = true
		out = append(out, graphType)
	}
	return out
}

func magmaServiceConfig(cfg config.MagmaConfig, observer magma.Observer, provider llm.Provider) magma.ServiceConfig {
	return magma.ServiceConfig{
		QueueSize:           cfg.Consolidation.MaxQueueSize,
		BatchSize:           cfg.Consolidation.BatchSize,
		SemanticTopK:        cfg.Graphs.Semantic.TopK,
		SimilarityThreshold: cfg.Graphs.Semantic.SimilarityThreshold,
		CausalThreshold:     cfg.Graphs.Causal.LLMThreshold,
		LLM:                 provider,
		Model:               cfg.Consolidation.Model,
		Prompts: magma.PromptConfig{
			ConsolidationExtraction: cfg.Consolidation.Prompts.ConsolidationExtraction,
			IntentClassification:    cfg.Consolidation.Prompts.IntentClassification,
		},
		Observer: observer,
		Lifecycle: magma.LifecyclePolicy{
			EventTTL:               time.Duration(cfg.Lifecycle.EventTTLHours) * time.Hour,
			MaxEdgesPerSourceRel:   cfg.Lifecycle.MaxEdgesPerSourceRel,
			MinSemanticWeight:      cfg.Lifecycle.MinSemanticWeight,
			LowConfidenceThreshold: cfg.Lifecycle.LowConfidenceThreshold,
			RequireReviewApproval:  cfg.Lifecycle.RequireReviewApproval,
			ArchiveBeforeDelete:    cfg.Lifecycle.ArchiveBeforeDelete,
		},
		RequireCausalGrounding: cfg.Lifecycle.RequireCausalGrounding,
		Graphs: magma.GraphConfig{
			Semantic:    cfg.Graphs.Semantic.Enabled,
			Temporal:    cfg.Graphs.Temporal.Enabled,
			Causal:      cfg.Graphs.Causal.Enabled,
			Entity:      cfg.Graphs.Entity.Enabled,
			CoReference: cfg.Graphs.Entity.CoReference,
		},
	}
}

func (s *Service) hydrateRerankText(ctx context.Context, items []retrieve.RetrievedItem) []retrieve.RetrievedItem {
	if s.search == nil || len(items) == 0 {
		return items
	}
	out := append([]retrieve.RetrievedItem(nil), items...)
	for i := range out {
		if out[i].Text != "" || out[i].Snippet != "" {
			continue
		}
		doc, ok, err := s.search.GetByID(ctx, out[i].ID)
		if err != nil || !ok {
			continue
		}
		out[i].Text = doc.Text
		if out[i].Metadata == nil {
			out[i].Metadata = map[string]string{}
		}
		maps.Copy(out[i].Metadata, doc.Metadata)
	}
	return out
}
