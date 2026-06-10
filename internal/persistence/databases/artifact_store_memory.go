package databases

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/memory/artifact"

	"github.com/google/uuid"
)

// NewMemoryArtifactStore returns an in-memory artifact store for local development and tests.
func NewMemoryArtifactStore() artifact.Store {
	return &memArtifactStore{artifacts: map[string]artifact.Artifact{}}
}

type memArtifactStore struct {
	mu        sync.RWMutex
	artifacts map[string]artifact.Artifact
}

func (s *memArtifactStore) Init(context.Context) error { return nil }

func (s *memArtifactStore) UpsertArtifact(_ context.Context, item artifact.Artifact) (artifact.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item = prepareArtifact(item)
	for id, existing := range s.artifacts {
		if existing.TenantID == item.TenantID && existing.Kind == item.Kind && existing.ExternalID == item.ExternalID && existing.ContentHash == item.ContentHash {
			item.ID = id
			item.CapturedAt = existing.CapturedAt
			s.artifacts[id] = cloneArtifact(item)
			return cloneArtifact(item), nil
		}
	}
	s.artifacts[item.ID] = cloneArtifact(item)
	return cloneArtifact(item), nil
}

func (s *memArtifactStore) GetArtifact(_ context.Context, tenantID int64, id string) (artifact.Artifact, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.artifacts[strings.TrimSpace(id)]
	if !ok || item.TenantID != normalizeTenantID(tenantID) {
		return artifact.Artifact{}, false, nil
	}
	return cloneArtifact(item), true, nil
}

func (s *memArtifactStore) FindByExternalID(_ context.Context, tenantID int64, kind artifact.ArtifactKind, externalID string) ([]artifact.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []artifact.Artifact{}
	for _, item := range s.artifacts {
		if item.TenantID == normalizeTenantID(tenantID) && item.Kind == kind && item.ExternalID == strings.TrimSpace(externalID) {
			out = append(out, cloneArtifact(item))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt.Before(out[j].CapturedAt) })
	return out, nil
}

func (s *memArtifactStore) SearchArtifacts(_ context.Context, query artifact.SearchQuery) ([]artifact.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	tenantID := normalizeTenantID(query.TenantID)
	kindSet := map[artifact.ArtifactKind]bool{}
	for _, kind := range query.Kinds {
		kindSet[kind] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	out := make([]artifact.SearchResult, 0, limit)
	for _, item := range s.artifacts {
		if item.TenantID != tenantID {
			continue
		}
		if len(kindSet) > 0 && !kindSet[item.Kind] {
			continue
		}
		text := strings.ToLower(item.Title + " " + item.Excerpt + " " + item.ExternalID)
		if needle != "" && !strings.Contains(text, needle) {
			continue
		}
		score := 1.0
		if needle != "" {
			score = 1.1
		}
		out = append(out, artifact.SearchResult{Artifact: cloneArtifact(item), Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Artifact.CapturedAt.After(out[j].Artifact.CapturedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func prepareArtifact(item artifact.Artifact) artifact.Artifact {
	item.TenantID = normalizeTenantID(item.TenantID)
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	item.ExternalID = strings.TrimSpace(item.ExternalID)
	item.URI = strings.TrimSpace(item.URI)
	item.Title = strings.TrimSpace(item.Title)
	item.Excerpt = strings.TrimSpace(item.Excerpt)
	if item.ContentHash == "" {
		sum := sha256.Sum256([]byte(item.Title + "\n" + item.Excerpt))
		item.ContentHash = fmt.Sprintf("%x", sum)
	}
	if item.CapturedAt.IsZero() {
		item.CapturedAt = time.Now().UTC()
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item
}

func cloneArtifact(item artifact.Artifact) artifact.Artifact {
	item.Metadata = maps.Clone(item.Metadata)
	return item
}
