package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Web search tool backed by SearXNG.
// This tool allows configurable SearXNG instances via environment variables.

type tool struct {
	http       *http.Client
	searxngURL string
}

// NewTool constructs the web_search tool with the given SearXNG URL.
func NewTool(searxngURL string) *tool {
	return &tool{
		http:       &http.Client{Timeout: 12 * time.Second},
		searxngURL: strings.TrimSuffix(searxngURL, "/"),
	}
}

func (t *tool) Name() string { return "web_search" }

func (t *tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Search the web using SearXNG and return top result links. Use for fact lookup and recent info.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string", "description": "Search query"},
				"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "default": 5},
				"category":    map[string]any{"type": "string", "description": "Search category (general, news, images, videos, etc.)", "default": "general"},
				"format":      map[string]any{"type": "string", "description": "Response format", "default": "json"},
			},
			"required": []string{"query"},
		},
	}
}

func (t *tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
		Category   string `json:"category"`
		Format     string `json:"format"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if args.MaxResults <= 0 || args.MaxResults > 10 {
		args.MaxResults = 5
	}
	if args.Category == "" {
		args.Category = "general"
	}
	if args.Format == "" {
		args.Format = "json"
	}

	q := strings.TrimSpace(args.Query)
	results, err := t.searchSearXNG(ctx, q, args.MaxResults, args.Category, args.Format)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "results": results}, nil
}

type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func (t *tool) searchSearXNG(ctx context.Context, query string, max int, category, format string) ([]SearchResult, error) {
	// Try JSON format first for better structured results
	results, err := t.searchSearXNGJSON(ctx, query, max, category)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	// Fallback to HTML parsing if JSON fails
	return t.searchSearXNGHTML(ctx, query, max, category)
}

// searchSearXNGJSON attempts to use SearXNG's JSON API
func (t *tool) searchSearXNGJSON(ctx context.Context, query string, max int, category string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/search", t.searxngURL)
	v := url.Values{}
	v.Set("q", query)
	v.Set("format", "json")
	v.Set("categories", category)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GPTAgent/1.0)")

	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("searxng http %d", resp.StatusCode)
	}

	var searxngResp struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searxngResp); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(searxngResp.Results))
	for i, r := range searxngResp.Results {
		if i >= max {
			break
		}
		results = append(results, SearchResult{
			Title: strings.TrimSpace(r.Title),
			URL:   r.URL,
		})
	}

	return results, nil
}

// searchSearXNGHTML parses HTML results as fallback
func (t *tool) searchSearXNGHTML(ctx context.Context, query string, max int, category string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/search", t.searxngURL)
	v := url.Values{}
	v.Set("q", query)
	v.Set("categories", category)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GPTAgent/1.0)")

	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("searxng http %d", resp.StatusCode)
	}

	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	urls, err := extractURLsFromHTML(root)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(urls))
	seen := map[string]struct{}{}

	for _, urlStr := range urls {
		if _, exists := seen[urlStr]; exists {
			continue
		}
		seen[urlStr] = struct{}{}

		// Extract a simple title from the URL for now
		title := urlStr
		if u, err := url.Parse(urlStr); err == nil && u.Host != "" {
			title = u.Host + u.Path
		}

		results = append(results, SearchResult{
			Title: title,
			URL:   urlStr,
		})

		if len(results) >= max {
			break
		}
	}

	return results, nil
}

// extractURLsFromHTML parses the HTML content and extracts URLs.
func extractURLsFromHTML(doc *html.Node) ([]string, error) {
	var urls []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.Contains(attr.Val, "http") {
					urls = append(urls, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return urls, nil
}
