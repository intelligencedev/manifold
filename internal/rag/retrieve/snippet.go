package retrieve

import (
    "context"
    "strings"

    "manifold/internal/persistence/databases"
)

// SnippetOptions control how snippets are generated.
type SnippetOptions struct {
    Lang string
    Query string
}

// GenerateSnippets attempts to populate Snippet for items. It prefers database-backed
// headline generation when available (ts_headline via Postgres implementation),
// and otherwise falls back to a simple substring heuristic around the first match.
// The function updates the input slice items in place and returns it for chaining.
func GenerateSnippets(ctx context.Context, search databases.FullTextSearch, items []RetrievedItem, opt SnippetOptions) []RetrievedItem {
    if len(items) == 0 {
        return items
    }
    // If the backend supports SnippetForID (Postgres), try it first.
    type snippetProvider interface { SnippetForID(ctx context.Context, id, lang, query string) (string, bool, error) }
    sp, hasSP := search.(snippetProvider)
    // If search supports GetByID with text, we can fallback using full text.
    for i := range items {
        if items[i].Snippet != "" {
            continue
        }
        if hasSP {
            if sn, ok, _ := sp.SnippetForID(ctx, items[i].ID, opt.Lang, opt.Query); ok && sn != "" {
                items[i].Snippet = sn
                continue
            }
        }
        // If we already have text and a query, create a basic snippet.
        if items[i].Text != "" {
            items[i].Snippet = simpleSnippet(items[i].Text, opt.Query)
            continue
        }
        if search != nil {
            if doc, ok, _ := search.GetByID(ctx, items[i].ID); ok {
                items[i].Text = doc.Text
                items[i].Snippet = simpleSnippet(doc.Text, opt.Query)
            }
        }
    }
    return items
}

func simpleSnippet(text, query string) string {
    if text == "" || query == "" {
        if len(text) > 160 { return text[:160] }
        return text
    }
    lt := strings.ToLower(text)
    q := strings.ToLower(strings.TrimSpace(query))
    if q == "" {
        if len(text) > 160 { return text[:160] }
        return text
    }
    idx := strings.Index(lt, q)
    if idx == -1 {
        // try first term
        parts := strings.Fields(q)
        for _, p := range parts {
            if p == "" { continue }
            idx = strings.Index(lt, p)
            if idx != -1 { break }
        }
    }
    if idx == -1 {
        if len(text) > 160 { return text[:160] }
        return text
    }
    // build a window around idx
    start := idx - 60
    if start < 0 { start = 0 }
    end := start + 160
    if end > len(text) { end = len(text) }
    return text[start:end]
}

