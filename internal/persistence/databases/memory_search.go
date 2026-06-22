package databases

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"
)

const maxSearchLimit = 1000 // Upper bound to prevent excessive allocations.

// Field-weight multipliers matching Postgres ts_weighted (A=title, B=url, C=body).
// These keep cross-engine scoring semantics consistent.
const (
	memWeightTitle = 4.0 // matches Postgres weight A
	memWeightURL   = 2.0 // matches Postgres weight B
	memWeightBody  = 1.0 // matches Postgres weight C
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
	maps.Copy(cp, metadata)
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
		score := memScoreDoc(d, terms)
		if score > 0 {
			results = append(results, SearchResult{
				ID:       id,
				Score:    score,
				Snippet:  memTextSnippet(d.text, query),
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
	if limit > maxSearchLimit {
		limit = maxSearchLimit // cap to prevent excessive allocations
	}
	q := strings.ToLower(query)
	terms := strings.Fields(q)
	// Strip lang from the filter — lang is stored inconsistently in metadata
	// across backends. This matches SQLite's sqliteChunkSearchFilter behaviour
	// and prevents spurious misses when lang was not set at index time.
	chunkFilter := memChunkFilter(filter)
	results := make([]SearchResult, 0, limit)
	for id, d := range m.docs {
		if !strings.HasPrefix(id, "chunk:") {
			continue
		}
		if !metaMatches(d.metadata, chunkFilter) {
			continue
		}
		score := memScoreDoc(d, terms)
		if score > 0 {
			results = append(results, SearchResult{ID: id, Score: score, Snippet: memTextSnippet(d.text, query), Text: d.text, Metadata: copyMap(d.metadata)})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// memScoreDoc computes a weighted relevance score for a document.
//
// Matching terms are scored with per-field multipliers that mirror Postgres
// ts_weighted (A=title, B=url, C=body), ensuring cross-engine ranking
// consistency: title matches outrank url matches, which outrank body matches.
// Both the body text and the title/url metadata fields are searched.
func memScoreDoc(d doc, terms []string) float64 {
	lbody := strings.ToLower(d.text)
	ltitle := strings.ToLower(d.metadata["title"])
	lurl := strings.ToLower(d.metadata["url"])
	var score float64
	for _, t := range terms {
		if t == "" {
			continue
		}
		if n := strings.Count(ltitle, t); n > 0 {
			score += float64(n) * memWeightTitle
		}
		if n := strings.Count(lurl, t); n > 0 {
			score += float64(n) * memWeightURL
		}
		if n := strings.Count(lbody, t); n > 0 {
			score += float64(n) * memWeightBody
		}
	}
	return score
}

// memTextSnippet returns a query-aware excerpt from text, centred around the
// first occurrence of any query term.  When no term is found it falls back to
// the first 160 characters.  The window is ±75 bytes around the hit, capped at
// text boundaries, giving a snippet of up to ~150 characters — comparable to
// Postgres ts_headline output size.
func memTextSnippet(text, query string) string {
	const window = 75
	if text == "" {
		return ""
	}
	if query == "" {
		if len(text) > 160 {
			return text[:160]
		}
		return text
	}
	lt := strings.ToLower(text)
	lq := strings.ToLower(strings.TrimSpace(query))

	// Try the full phrase first, then fall back term-by-term.
	idx := -1
	if lq != "" {
		idx = strings.Index(lt, lq)
	}
	if idx == -1 {
		for _, term := range strings.Fields(lq) {
			if term == "" {
				continue
			}
			if i := strings.Index(lt, term); i != -1 {
				idx = i
				break
			}
		}
	}
	if idx == -1 {
		if len(text) > 160 {
			return text[:160]
		}
		return text
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := start + (window * 2)
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}

// memChunkFilter returns a copy of filter with the "lang" key removed.
// Language filtering is inconsistently populated across call sites and backends,
// so we avoid hard-rejecting chunks that were indexed without an explicit lang.
// This mirrors SQLite's sqliteChunkSearchFilter behaviour.
func memChunkFilter(filter map[string]string) map[string]string {
	if len(filter) == 0 {
		return nil
	}
	out := make(map[string]string, len(filter))
	for k, v := range filter {
		if k == "lang" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	maps.Copy(cp, m)
	return cp
}
