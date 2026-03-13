package transit

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/persistence"
)

type EmbedFunc func(ctx context.Context, cfg config.EmbeddingConfig, texts []string) ([][]float32, error)

type ServiceConfig struct {
	Store              Store
	Search             SearchIndexer
	Vector             VectorIndexer
	EmbeddingConfig    config.EmbeddingConfig
	EmbedFn            EmbedFunc
	DefaultSearchLimit int
	DefaultListLimit   int
	MaxBatchSize       int
	EnableVectorSearch bool
}

type Service struct {
	store              Store
	search             SearchIndexer
	vector             VectorIndexer
	embeddingConfig    config.EmbeddingConfig
	embedFn            EmbedFunc
	defaultSearchLimit int
	defaultListLimit   int
	maxBatchSize       int
	enableVectorSearch bool
}

func NewService(cfg ServiceConfig) *Service {
	embedFn := cfg.EmbedFn
	if embedFn == nil {
		embedFn = embedding.EmbedText
	}
	searchLimit := cfg.DefaultSearchLimit
	if searchLimit <= 0 {
		searchLimit = defaultSearchLimit
	}
	listLimit := cfg.DefaultListLimit
	if listLimit <= 0 {
		listLimit = defaultListLimit
	}
	maxBatchSize := cfg.MaxBatchSize
	if maxBatchSize <= 0 {
		maxBatchSize = defaultBatchSize
	}
	return &Service{
		store:              cfg.Store,
		search:             cfg.Search,
		vector:             cfg.Vector,
		embeddingConfig:    cfg.EmbeddingConfig,
		embedFn:            embedFn,
		defaultSearchLimit: searchLimit,
		defaultListLimit:   listLimit,
		maxBatchSize:       maxBatchSize,
		enableVectorSearch: cfg.EnableVectorSearch,
	}
}

func (s *Service) CreateMemory(ctx context.Context, tenantID, actorID int64, items []CreateMemoryItem) ([]Record, error) {
	if s.store == nil {
		return nil, fmt.Errorf("transit store is not configured")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("items are required")
	}
	if len(items) > s.maxBatchSize {
		return nil, fmt.Errorf("batch exceeds max size of %d", s.maxBatchSize)
	}
	seen := make(map[string]struct{}, len(items))
	normalized := make([]CreateMemoryItem, 0, len(items))
	for _, item := range items {
		item = ApplyCreateDefaults(item)
		if err := ValidateKey(item.KeyName); err != nil {
			return nil, err
		}
		item.KeyName = strings.TrimSpace(item.KeyName)
		if _, ok := seen[item.KeyName]; ok {
			return nil, fmt.Errorf("duplicate keyName in request: %s", item.KeyName)
		}
		seen[item.KeyName] = struct{}{}
		normalized = append(normalized, item)
	}
	records, err := s.store.Create(ctx, tenantID, actorID, normalized)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		_ = s.indexRecord(ctx, record)
	}
	return records, nil
}

func (s *Service) GetMemory(ctx context.Context, tenantID int64, keys []string) ([]Record, error) {
	if s.store == nil {
		return nil, fmt.Errorf("transit store is not configured")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("keys are required")
	}
	if len(keys) > s.maxBatchSize {
		return nil, fmt.Errorf("batch exceeds max size of %d", s.maxBatchSize)
	}
	for index := range keys {
		if err := ValidateKey(keys[index]); err != nil {
			return nil, err
		}
		keys[index] = strings.TrimSpace(keys[index])
	}
	return s.store.Get(ctx, tenantID, keys)
}

func (s *Service) UpdateMemory(ctx context.Context, tenantID, actorID int64, req UpdateMemoryRequest) (Record, error) {
	if s.store == nil {
		return Record{}, fmt.Errorf("transit store is not configured")
	}
	if err := ValidateKey(req.KeyName); err != nil {
		return Record{}, err
	}
	req.KeyName = strings.TrimSpace(req.KeyName)
	req.EmbedSource = NormalizeEmbedSource(req.EmbedSource)
	record, err := s.store.Update(ctx, tenantID, actorID, req)
	if err != nil {
		return Record{}, err
	}
	_ = s.indexRecord(ctx, record)
	return record, nil
}

func (s *Service) DeleteMemory(ctx context.Context, tenantID int64, keys []string) error {
	if s.store == nil {
		return fmt.Errorf("transit store is not configured")
	}
	if len(keys) == 0 {
		return fmt.Errorf("keys are required")
	}
	if len(keys) > s.maxBatchSize {
		return fmt.Errorf("batch exceeds max size of %d", s.maxBatchSize)
	}
	for _, key := range keys {
		if err := ValidateKey(key); err != nil {
			return err
		}
		_ = s.removeIndex(ctx, tenantID, strings.TrimSpace(key))
	}
	return s.store.Delete(ctx, tenantID, keys)
}

func (s *Service) ListKeys(ctx context.Context, tenantID int64, req ListRequest) ([]Metadata, error) {
	req.Limit = s.normalizeListLimit(req.Limit)
	return s.store.ListKeys(ctx, tenantID, req)
}

func (s *Service) ListRecent(ctx context.Context, tenantID int64, req ListRequest) ([]Metadata, error) {
	req.Limit = s.normalizeListLimit(req.Limit)
	return s.store.ListRecent(ctx, tenantID, req)
}

func (s *Service) SearchMemories(ctx context.Context, tenantID int64, req SearchRequest) ([]SearchHit, error) {
	req.Limit = s.normalizeSearchLimit(req.Limit)
	candidates, err := s.collectSearch(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, SearchHit{Record: candidate.Record, Score: candidate.Score, Snippet: candidate.Snippet})
	}
	return out, nil
}

func (s *Service) DiscoverMemories(ctx context.Context, tenantID int64, req SearchRequest) ([]DiscoverHit, error) {
	req.Limit = s.normalizeSearchLimit(req.Limit)
	candidates, err := s.collectSearch(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoverHit, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, DiscoverHit{Metadata: MetadataFromRecord(candidate.Record), Score: candidate.Score, Snippet: candidate.Snippet})
	}
	return out, nil
}

func (s *Service) collectSearch(ctx context.Context, tenantID int64, req SearchRequest) ([]SearchCandidate, error) {
	fallback, err := s.store.SearchText(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]SearchCandidate, len(fallback))
	for _, candidate := range fallback {
		merged[candidate.Record.KeyName] = candidate
	}

	if strings.TrimSpace(req.Query) == "" {
		return rankCandidates(merged, req.Limit), nil
	}

	if s.search != nil {
		results, searchErr := s.search.Search(ctx, req.Query, req.Limit*3)
		if searchErr == nil {
			keys := make([]string, 0, len(results))
			for _, result := range results {
				if !matchesIndexMetadata(result.Metadata, tenantID, req.Prefix) {
					continue
				}
				key := result.Metadata["key_name"]
				if key == "" {
					continue
				}
				keys = append(keys, key)
				candidate := merged[key]
				candidate.Score += result.Score
				if candidate.Snippet == "" {
					candidate.Snippet = result.Snippet
				}
				merged[key] = candidate
			}
			if len(keys) > 0 {
				records, getErr := s.store.Get(ctx, tenantID, keys)
				if getErr == nil {
					for _, record := range records {
						candidate := merged[record.KeyName]
						candidate.Record = record
						merged[record.KeyName] = candidate
					}
				}
			}
		}
	}

	if s.enableVectorSearch && s.vector != nil {
		vectors, embedErr := s.embedFn(ctx, s.embeddingConfig, []string{req.Query})
		if embedErr == nil && len(vectors) == 1 {
			filter := map[string]string{"kind": "transit", "tenant_id": strconv.FormatInt(tenantID, 10)}
			vectorResults, searchErr := s.vector.SimilaritySearch(ctx, vectors[0], req.Limit*3, filter)
			if searchErr == nil {
				keys := make([]string, 0, len(vectorResults))
				for _, result := range vectorResults {
					if !matchesIndexMetadata(result.Metadata, tenantID, req.Prefix) {
						continue
					}
					key := result.Metadata["key_name"]
					if key == "" {
						continue
					}
					keys = append(keys, key)
					candidate := merged[key]
					candidate.Score += result.Score
					merged[key] = candidate
				}
				if len(keys) > 0 {
					records, getErr := s.store.Get(ctx, tenantID, keys)
					if getErr == nil {
						for _, record := range records {
							candidate := merged[record.KeyName]
							candidate.Record = record
							merged[record.KeyName] = candidate
						}
					}
				}
			}
		}
	}

	for key, candidate := range merged {
		if candidate.Record.KeyName == "" {
			delete(merged, key)
			continue
		}
		if !matchesTimeFilter(candidate.Record, req) {
			delete(merged, key)
		}
	}
	return rankCandidates(merged, req.Limit), nil
}

func rankCandidates(merged map[string]SearchCandidate, limit int) []SearchCandidate {
	out := make([]SearchCandidate, 0, len(merged))
	for _, candidate := range merged {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Record.UpdatedAt.After(out[j].Record.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Service) indexRecord(ctx context.Context, record Record) error {
	id := indexID(record.TenantID, record.KeyName)
	metadata := map[string]string{
		"kind":         "transit",
		"tenant_id":    strconv.FormatInt(record.TenantID, 10),
		"key_name":     record.KeyName,
		"embed_source": record.EmbedSource,
	}
	text := strings.TrimSpace(record.Description + "\n\n" + record.Value)
	if s.search != nil {
		if err := s.search.Index(ctx, id, text, metadata); err != nil {
			return err
		}
	}
	if s.enableVectorSearch && s.vector != nil && record.Embed {
		embedText := record.Value
		if record.EmbedSource == "description" {
			embedText = record.Description
		}
		vectors, err := s.embedFn(ctx, s.embeddingConfig, []string{embedText})
		if err != nil {
			return err
		}
		if len(vectors) == 1 {
			if err := s.vector.Upsert(ctx, id, vectors[0], metadata); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) removeIndex(ctx context.Context, tenantID int64, key string) error {
	id := indexID(tenantID, key)
	if s.search != nil {
		if err := s.search.Remove(ctx, id); err != nil {
			return err
		}
	}
	if s.vector != nil {
		if err := s.vector.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return s.defaultSearchLimit
	}
	if limit > s.maxBatchSize {
		return s.maxBatchSize
	}
	return limit
}

func (s *Service) normalizeListLimit(limit int) int {
	if limit <= 0 {
		return s.defaultListLimit
	}
	if limit > s.maxBatchSize {
		return s.maxBatchSize
	}
	return limit
}

func matchesIndexMetadata(metadata map[string]string, tenantID int64, prefix string) bool {
	if metadata == nil {
		return false
	}
	if metadata["kind"] != "transit" {
		return false
	}
	if metadata["tenant_id"] != strconv.FormatInt(tenantID, 10) {
		return false
	}
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(metadata["key_name"], prefix)
}

func matchesTimeFilter(record Record, req SearchRequest) bool {
	if req.WithinDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(req.WithinDays) * 24 * time.Hour)
		if record.UpdatedAt.Before(cutoff) {
			return false
		}
	}
	if req.CreatedAfter != nil && record.CreatedAt.Before(*req.CreatedAfter) {
		return false
	}
	if req.CreatedBefore != nil && record.CreatedAt.After(*req.CreatedBefore) {
		return false
	}
	if req.UpdatedAfter != nil && record.UpdatedAt.Before(*req.UpdatedAfter) {
		return false
	}
	if req.UpdatedBefore != nil && record.UpdatedAt.After(*req.UpdatedBefore) {
		return false
	}
	return true
}

func indexID(tenantID int64, key string) string {
	return fmt.Sprintf("transit:%d:%s", tenantID, key)
}

func NotFoundError(key string) error {
	return fmt.Errorf("%w: transit key %s", persistence.ErrNotFound, key)
}
