package databases

// Phase 3 tests for memory_search.go improvements:
//   M1 – query-aware snippet (memTextSnippet)
//   M2 – field weighting (title > url > body)
//   M3 – title/url metadata searched, not just body
//   C2 – SearchChunks strips lang from filter

import (
	"context"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// M1: memTextSnippet
// ─────────────────────────────────────────────────────────────────────────────

func TestMemTextSnippet_CentresAroundMatch(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 80) + " TARGET " + strings.Repeat("y", 80)
	snip := memTextSnippet(long, "TARGET")
	if !strings.Contains(snip, "TARGET") {
		t.Fatalf("snippet should contain the query term; got %q", snip)
	}
	// Should not return the full 168-char string
	if len(snip) >= len(long) {
		t.Fatalf("expected a shorter snippet, got length %d", len(snip))
	}
}

func TestMemTextSnippet_FallsBackToPrefix(t *testing.T) {
	t.Parallel()
	text := "Hello world this is a test"
	snip := memTextSnippet(text, "notpresent")
	// Falls back to first 160 chars — the whole string here
	if snip != text {
		t.Fatalf("expected full text fallback, got %q", snip)
	}
}

func TestMemTextSnippet_EmptyQuery(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 200)
	snip := memTextSnippet(text, "")
	if len(snip) > 160 {
		t.Fatalf("expected at most 160 chars for empty query, got %d", len(snip))
	}
}

func TestMemTextSnippet_FirstTermFallback(t *testing.T) {
	t.Parallel()
	text := "The quick brown fox jumps over the lazy dog"
	// Phrase not present, but "fox" is
	snip := memTextSnippet(text, "fox jump")
	if !strings.Contains(snip, "fox") {
		t.Fatalf("expected snippet to contain 'fox', got %q", snip)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// M2+M3: field weighting and title/url searchability
// ─────────────────────────────────────────────────────────────────────────────

func TestMemorySearch_TitleMatchRanksAboveBodyMatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	// doc1: "manifold" only in body
	_ = s.Index(ctx, "doc:body", "this document contains manifold in the body only", nil)
	// doc2: "manifold" in title metadata (no body match)
	_ = s.Index(ctx, "doc:title", "completely different body content here", map[string]string{
		"title": "manifold overview guide",
	})

	hits, err := s.Search(ctx, "manifold", 5)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected 2 hits, got %d: %#v", len(hits), hits)
	}
	// Title match (doc:title) should score higher than body match (doc:body)
	if hits[0].ID != "doc:title" {
		t.Fatalf("expected title match to rank first; got %v then %v (scores %.2f / %.2f)",
			hits[0].ID, hits[1].ID, hits[0].Score, hits[1].Score)
	}
}

func TestMemorySearch_URLMatchRanksAboveBodyMatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	_ = s.Index(ctx, "doc:body", "the term golang appears in the body text", nil)
	_ = s.Index(ctx, "doc:url", "unrelated body content", map[string]string{
		"url": "https://golang.org/doc/faq",
	})

	hits, err := s.Search(ctx, "golang", 5)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected 2 hits, got %d: %#v", len(hits), hits)
	}
	// URL match should rank above body-only match
	if hits[0].ID != "doc:url" {
		t.Fatalf("expected url match to rank first; got %v (score %.2f)", hits[0].ID, hits[0].Score)
	}
}

func TestMemorySearch_TitleOnlyDocumentIsFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	// "raretoken" appears only in title, not in body text
	_ = s.Index(ctx, "doc:titlematch", "body text has nothing relevant", map[string]string{
		"title": "raretoken special guide",
	})

	hits, err := s.Search(ctx, "raretoken", 5)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "doc:titlematch" {
		t.Fatalf("expected to find doc via title match, got: %#v", hits)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C2: SearchChunks strips lang from filter
// ─────────────────────────────────────────────────────────────────────────────

func TestMemorySearch_SearchChunksIgnoresLangFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	// Index a chunk WITHOUT a lang metadata key
	_ = s.Index(ctx, "chunk:doc:1:0", "alpha bravo charlie", map[string]string{
		"type":   "chunk",
		"doc_id": "doc:1",
	})

	// Query with lang=english in the filter — should still find the chunk
	hits, err := s.(*memorySearch).SearchChunks(ctx, "alpha", "", 5, map[string]string{
		"lang": "english",
	})
	if err != nil {
		t.Fatalf("SearchChunks() error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "chunk:doc:1:0" {
		t.Fatalf("expected chunk found despite missing lang in metadata; got %#v", hits)
	}
}

func TestMemorySearch_SearchChunksStillFiltersOtherKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	_ = s.Index(ctx, "chunk:t1:0", "alpha bravo", map[string]string{"type": "chunk", "tenant": "t1"})
	_ = s.Index(ctx, "chunk:t2:0", "alpha bravo", map[string]string{"type": "chunk", "tenant": "t2"})

	// Filter by tenant=t1 — should only return t1 chunk
	hits, err := s.(*memorySearch).SearchChunks(ctx, "alpha", "", 5, map[string]string{
		"tenant": "t1",
	})
	if err != nil {
		t.Fatalf("SearchChunks() error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "chunk:t1:0" {
		t.Fatalf("expected only t1 chunk; got %#v", hits)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Snippet integration: Search() returns query-aware snippets
// ─────────────────────────────────────────────────────────────────────────────

func TestMemorySearch_SearchReturnsQueryAwareSnippet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewMemorySearch()

	// Bury the query term deep in the text so blind truncation would miss it
	prefix := strings.Repeat("padding word ", 20) // ~260 chars
	_ = s.Index(ctx, "doc:snippet", prefix+"FINDME keyword here"+strings.Repeat(" extra", 10), nil)

	hits, err := s.Search(ctx, "FINDME", 5)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected a hit")
	}
	if !strings.Contains(strings.ToLower(hits[0].Snippet), "findme") {
		t.Fatalf("expected snippet to contain 'findme'; got %q", hits[0].Snippet)
	}
}
