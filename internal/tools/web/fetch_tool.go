package web

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"manifold/internal/persistence/databases"

	"golang.org/x/sync/errgroup"
)

type fetchTool struct {
	f      *Fetcher
	search databases.FullTextSearch // optional; if nil, indexing is disabled
}

type fetchToolArgs struct {
	URL            string   `json:"url"`
	URLs           []string `json:"urls"`
	Concurrent     int      `json:"concurrent"`
	Index          *bool    `json:"index"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxBytes       int64    `json:"max_bytes"`
	PreferReadable bool     `json:"prefer_readable"`
	UserAgent      string   `json:"user_agent"`
	MaxRedirects   int      `json:"max_redirects"`
}

type fetchToolResult struct {
	OK           bool      `json:"ok"`
	Error        string    `json:"error,omitempty"`
	InputURL     string    `json:"input_url,omitempty"`
	FinalURL     string    `json:"final_url,omitempty"`
	Status       int       `json:"status,omitempty"`
	ContentType  string    `json:"content_type,omitempty"`
	Charset      string    `json:"charset,omitempty"`
	Title        string    `json:"title,omitempty"`
	Markdown     string    `json:"markdown,omitempty"`
	UsedReadable bool      `json:"used_readable,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// NewFetchTool constructs the web_fetch tool. If a FullTextSearch backend is
// provided, successfully fetched content will be indexed by default.
func NewFetchTool(search databases.FullTextSearch) *fetchTool {
	return &fetchTool{f: NewFetcher(), search: search}
}

func (t *fetchTool) Name() string { return "web_fetch" }

func (t *fetchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Fetch a web URL over HTTP(S) and return best-effort Markdown (readability extraction when possible).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":             map[string]any{"type": "string", "description": "Absolute URL (http or https)."},
				"urls":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of absolute URLs to fetch."},
				"concurrent":      map[string]any{"type": "integer", "minimum": 1, "description": "When fetching multiple URLs, maximum number of concurrent fetches."},
				"index":           map[string]any{"type": "boolean", "description": "If true (default), index successfully fetched content into the documents table using the final URL as the document ID."},
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60, "description": "Overall timeout for the request."},
				"max_bytes":       map[string]any{"type": "integer", "minimum": 1000000, "maximum": 16777216, "description": "Maximum response size to read (bytes)."},
				"prefer_readable": map[string]any{"type": "boolean", "description": "Extract main article content when available."},
				"user_agent":      map[string]any{"type": "string", "description": "Override User-Agent header."},
				"max_redirects":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum redirects to follow."},
			},
			// allow either url or urls
		},
	}
}

func (t *fetchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args fetchToolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	f := NewFetcher(fetchOptions(args)...)
	index := shouldIndex(args)
	if isSingleURLFetch(args) {
		return t.fetchSingle(ctx, f, args.URL, index), nil
	}

	urls := requestedURLs(args)
	if len(urls) == 0 {
		return map[string]any{"ok": false, "error": "missing url(s)"}, nil
	}
	results := t.fetchMany(ctx, f, urls, concurrency(args), index)
	return map[string]any{"ok": true, "results": results}, nil
}

func fetchOptions(args fetchToolArgs) []Option {
	opts := []Option{WithMaxBytes(normalizeMaxBytes(args.MaxBytes))}
	if args.TimeoutSeconds > 0 {
		opts = append(opts, WithTimeout(time.Duration(args.TimeoutSeconds)*time.Second))
	}
	opts = append(opts, WithPreferReadable(args.PreferReadable))
	if args.UserAgent != "" {
		opts = append(opts, WithUserAgent(args.UserAgent))
	}
	if args.MaxRedirects > 0 {
		opts = append(opts, WithMaxRedirects(args.MaxRedirects))
	}
	return opts
}

func normalizeMaxBytes(maxBytes int64) int64 {
	if maxBytes < 1000000 {
		return 1000000
	}
	return maxBytes
}

func shouldIndex(args fetchToolArgs) bool {
	if args.Index == nil {
		return true
	}
	return *args.Index
}

func isSingleURLFetch(args fetchToolArgs) bool {
	return args.URL != "" && len(args.URLs) == 0
}

func requestedURLs(args fetchToolArgs) []string {
	urls := make([]string, 0, 1+len(args.URLs))
	if args.URL != "" {
		urls = append(urls, args.URL)
	}
	return append(urls, args.URLs...)
}

func concurrency(args fetchToolArgs) int {
	conc := args.Concurrent
	if conc <= 0 {
		return 3
	}
	if conc > 64 {
		return 64
	}
	return conc
}

func (t *fetchTool) fetchSingle(ctx context.Context, f *Fetcher, url string, index bool) map[string]any {
	if cached, ok := t.cachedResult(ctx, url); ok {
		return fetchResultMap(cached)
	}
	res, err := f.FetchMarkdown(ctx, url)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	t.indexResult(ctx, res, index)
	return fetchResultMap(resultToOutput(res))
}

func (t *fetchTool) fetchMany(ctx context.Context, f *Fetcher, urls []string, conc int, index bool) []fetchToolResult {
	results := make([]fetchToolResult, len(urls))
	var g errgroup.Group
	g.SetLimit(conc)
	for i, u := range urls {
		g.Go(func() error {
			results[i] = t.fetchOne(ctx, f, u, index)
			return nil
		})
	}
	_ = g.Wait()
	return results
}

func (t *fetchTool) fetchOne(ctx context.Context, f *Fetcher, url string, index bool) fetchToolResult {
	if cached, ok := t.cachedResult(ctx, url); ok {
		return cached
	}
	res, err := f.FetchMarkdown(ctx, url)
	if err != nil {
		return fetchToolResult{OK: false, Error: err.Error()}
	}
	t.indexResult(ctx, res, index)
	return resultToOutput(res)
}

func (t *fetchTool) cachedResult(ctx context.Context, url string) (fetchToolResult, bool) {
	if t.search == nil {
		return fetchToolResult{}, false
	}
	cached, ok, _ := t.search.GetByID(ctx, url)
	if !ok {
		return fetchToolResult{}, false
	}
	fetchedAt := parseFetchedAt(cached.Metadata["fetched_at"])
	return fetchToolResult{
		OK:           true,
		InputURL:     url,
		FinalURL:     cached.ID,
		Status:       200,
		ContentType:  cached.Metadata["content_type"],
		Charset:      cached.Metadata["charset"],
		Title:        cached.Metadata["title"],
		Markdown:     cached.Text,
		UsedReadable: cached.Metadata["used_readable"] == "true",
		FetchedAt:    fetchedAt,
	}, true
}

func parseFetchedAt(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	fetchedAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return fetchedAt
}

func (t *fetchTool) indexResult(ctx context.Context, res *Result, index bool) {
	if !index || t.search == nil {
		return
	}
	_ = t.search.Index(ctx, idFor(res), res.Markdown, resultMetadata(res))
}

func resultMetadata(res *Result) map[string]string {
	return map[string]string{
		"input_url":     res.InputURL,
		"final_url":     res.FinalURL,
		"status":        fmt.Sprintf("%d", res.Status),
		"content_type":  res.ContentType,
		"charset":       res.Charset,
		"title":         res.Title,
		"used_readable": fmt.Sprintf("%v", res.UsedReadable),
		"fetched_at":    res.FetchedAt.Format(time.RFC3339),
	}
}

func resultToOutput(res *Result) fetchToolResult {
	return fetchToolResult{
		OK:           true,
		InputURL:     res.InputURL,
		FinalURL:     res.FinalURL,
		Status:       res.Status,
		ContentType:  res.ContentType,
		Charset:      res.Charset,
		Title:        res.Title,
		Markdown:     res.Markdown,
		UsedReadable: res.UsedReadable,
		FetchedAt:    res.FetchedAt,
	}
}

func fetchResultMap(res fetchToolResult) map[string]any {
	return map[string]any{
		"ok":            res.OK,
		"input_url":     res.InputURL,
		"final_url":     res.FinalURL,
		"status":        res.Status,
		"content_type":  res.ContentType,
		"charset":       res.Charset,
		"title":         res.Title,
		"markdown":      res.Markdown,
		"used_readable": res.UsedReadable,
		"fetched_at":    res.FetchedAt,
	}
}

func idFor(r *Result) string {
	if r == nil {
		return ""
	}
	if r.FinalURL != "" {
		return r.FinalURL
	}
	return r.InputURL
}
