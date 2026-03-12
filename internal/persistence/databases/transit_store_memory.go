package databases

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
	"manifold/internal/transit"
)

type memTransitStore struct {
	mu   sync.RWMutex
	data map[int64]map[string]transit.Record
}

func NewMemoryTransitStore() transit.Store {
	return &memTransitStore{data: make(map[int64]map[string]transit.Record)}
}

func (s *memTransitStore) Init(context.Context) error { return nil }

func (s *memTransitStore) Create(_ context.Context, tenantID, actorID int64, items []transit.CreateMemoryItem) ([]transit.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[tenantID] == nil {
		s.data[tenantID] = make(map[string]transit.Record)
	}
	now := time.Now().UTC()
	out := make([]transit.Record, 0, len(items))
	for _, item := range items {
		if _, exists := s.data[tenantID][item.KeyName]; exists {
			return nil, persistence.ErrRevisionConflict
		}
		record := transit.Record{
			ID:          uuid.NewString(),
			TenantID:    tenantID,
			KeyName:     item.KeyName,
			Description: item.Description,
			Value:       item.Value,
			Base64:      item.Base64 != nil && *item.Base64,
			Embed:       item.Embed == nil || *item.Embed,
			EmbedSource: transit.NormalizeEmbedSource(item.EmbedSource),
			Version:     1,
			CreatedBy:   actorID,
			UpdatedBy:   actorID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.data[tenantID][item.KeyName] = record
		out = append(out, record)
	}
	return out, nil
}

func (s *memTransitStore) Get(_ context.Context, tenantID int64, keys []string) ([]transit.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]transit.Record, 0, len(keys))
	for _, key := range keys {
		if record, ok := s.data[tenantID][key]; ok {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s *memTransitStore) Update(_ context.Context, tenantID, actorID int64, req transit.UpdateMemoryRequest) (transit.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.data[tenantID][req.KeyName]
	if !ok {
		return transit.Record{}, transit.NotFoundError(req.KeyName)
	}
	if req.IfVersion > 0 && record.Version != req.IfVersion {
		return transit.Record{}, persistence.ErrRevisionConflict
	}
	record.Value = req.Value
	if req.Base64 != nil {
		record.Base64 = *req.Base64
	}
	if req.Embed != nil {
		record.Embed = *req.Embed
	}
	if strings.TrimSpace(req.EmbedSource) != "" {
		record.EmbedSource = transit.NormalizeEmbedSource(req.EmbedSource)
	}
	record.Version++
	record.UpdatedBy = actorID
	record.UpdatedAt = time.Now().UTC()
	s.data[tenantID][req.KeyName] = record
	return record, nil
}

func (s *memTransitStore) Delete(_ context.Context, tenantID int64, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.data[tenantID], strings.TrimSpace(key))
	}
	return nil
}

func (s *memTransitStore) ListKeys(_ context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]transit.Metadata, 0, len(s.data[tenantID]))
	for _, record := range s.data[tenantID] {
		if req.Prefix != "" && !strings.HasPrefix(record.KeyName, req.Prefix) {
			continue
		}
		out = append(out, transit.MetadataFromRecord(record))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].KeyName < out[j].KeyName })
	if len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func (s *memTransitStore) ListRecent(_ context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]transit.Metadata, 0, len(s.data[tenantID]))
	for _, record := range s.data[tenantID] {
		if req.Prefix != "" && !strings.HasPrefix(record.KeyName, req.Prefix) {
			continue
		}
		out = append(out, transit.MetadataFromRecord(record))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func (s *memTransitStore) SearchText(_ context.Context, tenantID int64, req transit.SearchRequest) ([]transit.SearchCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(req.Query)))
	out := make([]transit.SearchCandidate, 0)
	for _, record := range s.data[tenantID] {
		if req.Prefix != "" && !strings.HasPrefix(record.KeyName, req.Prefix) {
			continue
		}
		if !transitMatchesTimeFilter(record, req) {
			continue
		}
		text := strings.ToLower(record.Description + "\n" + record.Value)
		score := 0.0
		if len(terms) == 0 {
			score = 1
		} else {
			for _, term := range terms {
				score += float64(strings.Count(text, term))
			}
		}
		if score <= 0 {
			continue
		}
		out = append(out, transit.SearchCandidate{Record: record, Score: score, Snippet: truncateSnippet(record.Value)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Record.UpdatedAt.After(out[j].Record.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func transitMatchesTimeFilter(record transit.Record, req transit.SearchRequest) bool {
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

func truncateSnippet(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 120 {
		return value
	}
	return value[:120]
}
