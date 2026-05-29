package magma

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	select {
	case s.queue <- event.ID:
	default:
	}
	return EventIngestResponse{EventID: event.ID, Status: "ingested"}, nil
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
	if s.graphEnabled(GraphEntity) {
		entities = ResolveEntities(event.Text)
	}
	edges := make([]Edge, 0, len(entities)+4)
	if s.graphEnabled(GraphSemantic) {
		edges = append(edges, s.semanticEdges(ctx, event)...)
	}
	if s.graphEnabled(GraphTemporal) {
		edges = append(edges, s.temporalEdges(ctx, event, attrs)...)
	}
	if s.graphEnabled(GraphCausal) {
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
			if err := s.Consolidate(ctx, eventID); err != nil {
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
			_ = s.Consolidate(ctx, eventID)
		}
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
