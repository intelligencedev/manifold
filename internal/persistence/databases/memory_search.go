package databases

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// memorySearch is a naive in-memory full text search implementation.
type memorySearch struct {
	mu   sync.RWMutex
	docs map[string]doc
}

type doc struct {
	text     string
	metadata map[string]string
}

func NewMemorySearch() FullTextSearch { return &memorySearch{docs: make(map[string]doc)} }

func (m *memorySearch) Index(_ context.Context, id, text string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[string]string, len(metadata))
	for k, v := range metadata {
		cp[k] = v
	}
	m.docs[id] = doc{text: text, metadata: cp}
	return nil
}

func (m *memorySearch) Remove(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.docs, id)
	return nil
}

func (m *memorySearch) Search(_ context.Context, query string, limit int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 10
	}
	q := strings.ToLower(query)
	terms := strings.Fields(q)
	results := make([]SearchResult, 0, limit)
	for id, d := range m.docs {
		score := 0.0
		lt := strings.ToLower(d.text)
		for _, t := range terms {
			if t == "" {
				continue
			}
			count := strings.Count(lt, t)
			if count > 0 {
				score += float64(count)
			}
		}
		if score > 0 {
			snippet := d.text
			if len(snippet) > 120 {
				snippet = snippet[:120]
			}
			results = append(results, SearchResult{
				ID:       id,
				Score:    score,
				Snippet:  snippet,
				Text:     d.text,
				Metadata: copyMap(d.metadata),
			})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *memorySearch) GetByID(_ context.Context, id string) (SearchResult, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.docs[id]
	if !ok {
		return SearchResult{}, false, nil
	}
	return SearchResult{ID: id, Text: d.text, Metadata: copyMap(d.metadata)}, true, nil
}

// SearchChunks does chunk-preferring search over docs whose IDs start with "chunk:".
// It also supports simple metadata filter matching.
func (m *memorySearch) SearchChunks(_ context.Context, query string, _ string, limit int, filter map[string]string) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 10
	}
	q := strings.ToLower(query)
	terms := strings.Fields(q)
	results := make([]SearchResult, 0, limit)
	for id, d := range m.docs {
		if !strings.HasPrefix(id, "chunk:") {
			continue
		}
		if !metaMatches(d.metadata, filter) {
			continue
		}
		score := 0.0
		lt := strings.ToLower(d.text)
		for _, t := range terms {
			if t == "" {
				continue
			}
			count := strings.Count(lt, t)
			if count > 0 {
				score += float64(count)
			}
		}
		if score > 0 {
			snippet := d.text
			if len(snippet) > 120 {
				snippet = snippet[:120]
			}
			results = append(results, SearchResult{ID: id, Score: score, Snippet: snippet, Text: d.text, Metadata: copyMap(d.metadata)})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func metaMatches(md map[string]string, f map[string]string) bool {
	if len(f) == 0 {
		return true
	}
	for k, v := range f {
		if md[k] != v {
			return false
		}
	}
	return true
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
