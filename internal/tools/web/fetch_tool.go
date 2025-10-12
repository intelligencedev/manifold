package web

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"manifold/internal/persistence/databases"
)

type fetchTool struct {
	f      *Fetcher
	search databases.FullTextSearch // optional; if nil, indexing is disabled
}

// NewFetchTool constructs the web_fetch tool. If a FullTextSearch backend is
// provided, successfully fetched content will be indexed by default.
func NewFetchTool(search databases.FullTextSearch) *fetchTool { return &fetchTool{f: NewFetcher(), search: search} }

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
	var args struct {
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
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	opts := []Option{}
	if args.TimeoutSeconds > 0 {
		opts = append(opts, WithTimeout(time.Duration(args.TimeoutSeconds)*time.Second))
	}
	if args.MaxBytes > 0 {
		// Enforce minimum max_bytes of 1MB (1,000,000 bytes)
		if args.MaxBytes < 1000000 {
			args.MaxBytes = 1000000
		}
		opts = append(opts, WithMaxBytes(args.MaxBytes))
	} else {
		// If max_bytes is not provided or is 0, set to minimum of 1MB
		opts = append(opts, WithMaxBytes(1000000))
	}
	if args.PreferReadable {
		opts = append(opts, WithPreferReadable(true))
	} else {
		opts = append(opts, WithPreferReadable(false))
	}
	if args.UserAgent != "" {
		opts = append(opts, WithUserAgent(args.UserAgent))
	}
	if args.MaxRedirects > 0 {
		opts = append(opts, WithMaxRedirects(args.MaxRedirects))
	}

	f := NewFetcher(opts...)

	// default index=true
	index := true
	if args.Index != nil {
		index = *args.Index
	}

	// Single URL legacy path
	if args.URL != "" && len(args.URLs) == 0 {
		// Cache lookup by exact ID (URL)
		if t.search != nil {
			if cached, ok, _ := t.search.GetByID(ctx, args.URL); ok {
				// Best-effort decode metadata
				usedReadable := cached.Metadata["used_readable"] == "true"
				var fetchedAt time.Time
				if ts := cached.Metadata["fetched_at"]; ts != "" {
					if t0, err := time.Parse(time.RFC3339, ts); err == nil {
						fetchedAt = t0
					}
				}
				return map[string]any{
					"ok":            true,
					"input_url":     args.URL,
					"final_url":     cached.ID,
					"status":        200,
					"content_type":  cached.Metadata["content_type"],
					"charset":       cached.Metadata["charset"],
					"title":         cached.Metadata["title"],
					"markdown":      cached.Text,
					"used_readable": usedReadable,
					"fetched_at":    fetchedAt,
				}, nil
			}
		}
		res, err := f.FetchMarkdown(ctx, args.URL)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
		if index && t.search != nil {
			md := map[string]string{
				"input_url":     res.InputURL,
				"final_url":     res.FinalURL,
				"status":        fmt.Sprintf("%d", res.Status),
				"content_type":  res.ContentType,
				"charset":       res.Charset,
				"title":         res.Title,
				"used_readable": fmt.Sprintf("%v", res.UsedReadable),
				"fetched_at":    res.FetchedAt.Format(time.RFC3339),
			}
			_ = t.search.Index(ctx, idFor(res), res.Markdown, md)
		}
		return map[string]any{
			"ok":            true,
			"input_url":     res.InputURL,
			"final_url":     res.FinalURL,
			"status":        res.Status,
			"content_type":  res.ContentType,
			"charset":       res.Charset,
			"title":         res.Title,
			"markdown":      res.Markdown,
			"used_readable": res.UsedReadable,
			"fetched_at":    res.FetchedAt,
		}, nil
	}

	// Multi-URL path
	urls := make([]string, 0, 1+len(args.URLs))
	if args.URL != "" {
		urls = append(urls, args.URL)
	}
	if len(args.URLs) > 0 {
		urls = append(urls, args.URLs...)
	}
	if len(urls) == 0 {
		return map[string]any{"ok": false, "error": "missing url(s)"}, nil
	}
	conc := args.Concurrent
	if conc <= 0 {
		conc = 3
	}
	if conc > 64 {
		conc = 64
	}

	type out struct {
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
		FetchedAt    time.Time `json:"fetched_at,omitempty"`
	}

	results := make([]out, len(urls))
	var g errgroup.Group
	g.SetLimit(conc)
	for i, u := range urls {
		i, u := i, u
		g.Go(func() error {
			// Try cache first when search backend is available
			if t.search != nil {
				if cached, ok, _ := t.search.GetByID(ctx, u); ok {
					// populate from cache and short-circuit
					var fetchedAt time.Time
					if ts := cached.Metadata["fetched_at"]; ts != "" {
						if t0, err := time.Parse(time.RFC3339, ts); err == nil {
							fetchedAt = t0
						}
					}
					results[i] = out{
						OK:           true,
						InputURL:     u,
						FinalURL:     cached.ID,
						Status:       200,
						ContentType:  cached.Metadata["content_type"],
						Charset:      cached.Metadata["charset"],
						Title:        cached.Metadata["title"],
						Markdown:     cached.Text,
						UsedReadable: cached.Metadata["used_readable"] == "true",
						FetchedAt:    fetchedAt,
					}
					return nil
				}
			}
			r, err := f.FetchMarkdown(ctx, u)
			if err != nil {
				results[i] = out{OK: false, Error: err.Error()}
				return nil
			}
			results[i] = out{
				OK:           true,
				InputURL:     r.InputURL,
				FinalURL:     r.FinalURL,
				Status:       r.Status,
				ContentType:  r.ContentType,
				Charset:      r.Charset,
				Title:        r.Title,
				Markdown:     r.Markdown,
				UsedReadable: r.UsedReadable,
				FetchedAt:    r.FetchedAt,
			}
			if index && t.search != nil {
				md := map[string]string{
					"input_url":     r.InputURL,
					"final_url":     r.FinalURL,
					"status":        fmt.Sprintf("%d", r.Status),
					"content_type":  r.ContentType,
					"charset":       r.Charset,
					"title":         r.Title,
					"used_readable": fmt.Sprintf("%v", r.UsedReadable),
					"fetched_at":    r.FetchedAt.Format(time.RFC3339),
				}
				_ = t.search.Index(ctx, idFor(r), r.Markdown, md)
			}
			return nil
		})
	}
	_ = g.Wait()
	return map[string]any{"ok": true, "results": results}, nil
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
