package magma

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
)

type Service struct {
	store  *Store
	vector databases.VectorStore
	emb    embedder.Embedder
	queue  chan string
	cfg    ServiceConfig

	startOnce sync.Once
	closeOnce sync.Once
	workerWG  sync.WaitGroup
	cancel    context.CancelFunc

	processedTotal atomic.Uint64
	failedTotal    atomic.Uint64
	droppedTotal   atomic.Uint64
	errMu          sync.RWMutex
	lastErr        error
}

func NewService(graph databases.GraphDB, vector databases.VectorStore, emb embedder.Embedder) *Service {
	return NewServiceWithConfig(graph, vector, emb, ServiceConfig{})
}

func NewServiceWithConfig(graph databases.GraphDB, vector databases.VectorStore, emb embedder.Embedder, cfg ServiceConfig) *Service {
	if emb == nil {
		emb = embedder.NewDeterministic(64, true, 0)
	}
	cfg = normalizeServiceConfig(cfg)
	return &Service{
		store:  NewStore(graph),
		vector: vector,
		emb:    emb,
		queue:  make(chan string, cfg.QueueSize),
		cfg:    cfg,
	}
}

func (s *Service) StartConsolidationWorkers(ctx context.Context, workers int) {
	if s == nil {
		return
	}
	if workers < 0 {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	s.startOnce.Do(func() {
		workerCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		for range workers {
			s.workerWG.Add(1)
			go s.runConsolidationWorker(workerCtx)
		}
	})
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.workerWG.Wait()
	})
}

func (s *Service) Ingest(ctx context.Context, in IngestRequest) (EventIngestResponse, error) {
	if s == nil || s.store == nil {
		return EventIngestResponse{}, errors.New("magma service is not configured")
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return EventIngestResponse{}, errors.New("magma ingest text is empty")
	}
	createdAt := in.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	embedding, err := s.embed(ctx, text)
	if err != nil {
		return EventIngestResponse{}, err
	}
	event := EventNode{
		ID:        eventID(in.Tenant, in.ID),
		Tenant:    in.Tenant,
		Session:   in.SessionID,
		Text:      text,
		Embedding: embedding,
		Graphs:    append([]GraphType(nil), in.Graphs...),
		CreatedAt: createdAt,
	}
	if err := s.store.StoreEvent(ctx, event); err != nil {
		return EventIngestResponse{}, err
	}
	if s.vector != nil {
		metadata := map[string]string{"tenant": in.Tenant, "session": in.SessionID, "kind": "magma_event"}
		if err := s.vector.Upsert(ctx, event.ID, embedding, metadata); err != nil {
			return EventIngestResponse{}, err
		}
	}
	status := "queued"
	if !s.enqueue(event.ID) {
		status = "queue_full"
	}
	return EventIngestResponse{EventID: event.ID, Status: status}, nil
}

func (s *Service) Consolidate(ctx context.Context, eventID string) error {
	if s == nil || s.store == nil {
		return errors.New("magma service is not configured")
	}
	event, ok := s.store.GetEvent(ctx, eventID)
	if !ok {
		return errors.New("magma event not found")
	}
	if len(event.Embedding) == 0 {
		embedding, err := s.embed(ctx, event.Text)
		if err != nil {
			return err
		}
		event.Embedding = embedding
	}
	attrs := NormalizeTemporal(event.Text, event.CreatedAt)
	var entities []EntityMention
	if s.graphEnabledForEvent(event, GraphEntity) {
		entities = ResolveEntitiesForTenant(event.Text, event.Tenant)
	}
	edges := make([]Edge, 0, len(entities)+4)
	if s.graphEnabledForEvent(event, GraphSemantic) {
		edges = append(edges, s.semanticEdges(ctx, event)...)
	}
	if s.graphEnabledForEvent(event, GraphTemporal) {
		edges = append(edges, s.temporalEdges(ctx, event, attrs)...)
	}
	if s.graphEnabledForEvent(event, GraphCausal) {
		edges = append(edges, ExtractCausalEdges(event)...)
	}
	return s.store.BatchUpsert(ctx, BatchUpsertRequest{
		Event:         event,
		TemporalAttrs: attrs,
		Entities:      entities,
		Edges:         edges,
	})
}

func (s *Service) DrainConsolidation(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = len(s.queue)
	}
	processed := 0
	for processed < limit {
		select {
		case eventID := <-s.queue:
			if err := s.consolidateQueued(ctx, eventID); err != nil {
				return processed, err
			}
			processed++
		default:
			return processed, nil
		}
	}
	return processed, nil
}

func (s *Service) runConsolidationWorker(ctx context.Context) {
	defer s.workerWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case eventID := <-s.queue:
			_ = s.consolidateQueued(ctx, eventID)
		}
	}
}

func (s *Service) Stats() ServiceStats {
	if s == nil {
		return ServiceStats{}
	}
	s.errMu.RLock()
	var lastErr string
	if s.lastErr != nil {
		lastErr = s.lastErr.Error()
	}
	s.errMu.RUnlock()
	return ServiceStats{
		QueueDepth:     len(s.queue),
		ProcessedTotal: s.processedTotal.Load(),
		FailedTotal:    s.failedTotal.Load(),
		DroppedTotal:   s.droppedTotal.Load(),
		LastError:      lastErr,
	}
}

func (s *Service) Query(ctx context.Context, query string, opt QueryOptions) (StructuredContext, error) {
	engine := QueryEngine{Service: s}
	return engine.Query(ctx, query, opt)
}

func (s *Service) Event(ctx context.Context, id string) (EventNode, bool) {
	if s == nil || s.store == nil {
		return EventNode{}, false
	}
	return s.store.GetEvent(ctx, id)
}

func (s *Service) embed(ctx context.Context, text string) ([]float32, error) {
	if s.emb == nil {
		return nil, nil
	}
	vectors, err := s.emb.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return vectors[0], nil
}

func (s *Service) enqueue(eventID string) bool {
	select {
	case s.queue <- eventID:
		return true
	default:
		s.droppedTotal.Add(1)
		return false
	}
}

func (s *Service) consolidateQueued(ctx context.Context, eventID string) error {
	if err := s.Consolidate(ctx, eventID); err != nil {
		s.failedTotal.Add(1)
		s.setLastError(err)
		return err
	}
	s.processedTotal.Add(1)
	s.setLastError(nil)
	return nil
}

func (s *Service) setLastError(err error) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	s.lastErr = err
}

func (s *Service) graphEnabled(graphType GraphType) bool {
	if s == nil {
		return false
	}
	switch graphType {
	case GraphSemantic:
		return s.cfg.Graphs.Semantic
	case GraphTemporal:
		return s.cfg.Graphs.Temporal
	case GraphCausal:
		return s.cfg.Graphs.Causal
	case GraphEntity:
		return s.cfg.Graphs.Entity
	default:
		return false
	}
}

func (s *Service) graphEnabledForEvent(event EventNode, graphType GraphType) bool {
	if len(event.Graphs) == 0 {
		return s.graphEnabled(graphType)
	}
	for _, configured := range event.Graphs {
		if configured == graphType {
			return true
		}
	}
	return false
}

func normalizeServiceConfig(cfg ServiceConfig) ServiceConfig {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.SemanticTopK <= 0 {
		cfg.SemanticTopK = 20
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.7
	}
	if !cfg.Graphs.Semantic && !cfg.Graphs.Temporal && !cfg.Graphs.Causal && !cfg.Graphs.Entity {
		cfg.Graphs = GraphConfig{Semantic: true, Temporal: true, Causal: true, Entity: true}
	}
	return cfg
}
