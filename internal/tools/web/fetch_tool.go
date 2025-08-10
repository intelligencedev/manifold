package web

import (
	"context"
	"encoding/json"
	"time"
)

type fetchTool struct{ f *Fetcher }

func NewFetchTool() *fetchTool { return &fetchTool{f: NewFetcher()} }

func (t *fetchTool) Name() string { return "web_fetch" }

func (t *fetchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Fetch a web URL over HTTP(S) and return best-effort Markdown (readability extraction when possible).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":             map[string]any{"type": "string", "description": "Absolute URL (http or https)."},
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 60, "description": "Overall timeout for the request."},
				"max_bytes":       map[string]any{"type": "integer", "minimum": 1024, "maximum": 16777216, "description": "Maximum response size to read (bytes)."},
				"prefer_readable": map[string]any{"type": "boolean", "description": "Extract main article content when available."},
				"user_agent":      map[string]any{"type": "string", "description": "Override User-Agent header."},
				"max_redirects":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum redirects to follow."},
			},
			"required": []string{"url"},
		},
	}
}

func (t *fetchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		URL            string `json:"url"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxBytes       int64  `json:"max_bytes"`
		PreferReadable bool   `json:"prefer_readable"`
		UserAgent      string `json:"user_agent"`
		MaxRedirects   int    `json:"max_redirects"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	opts := []Option{}
	if args.TimeoutSeconds > 0 {
		opts = append(opts, WithTimeout(time.Duration(args.TimeoutSeconds)*time.Second))
	}
	if args.MaxBytes > 0 {
		opts = append(opts, WithMaxBytes(args.MaxBytes))
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
	res, err := f.FetchMarkdown(ctx, args.URL)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
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
