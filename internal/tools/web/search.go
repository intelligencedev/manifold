package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Web search tool backed by SearXNG.
// This tool allows configurable SearXNG instances via environment variables.

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// RequestsPerSecond controls how many requests per second are allowed
	RequestsPerSecond float64
	// BurstSize is the maximum number of requests that can be made in a burst
	BurstSize int
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// BaseDelay is the base delay for exponential backoff
	BaseDelay time.Duration
	// MaxDelay is the maximum delay for exponential backoff
	MaxDelay time.Duration
	// JitterPercent adds randomness to delays (0.0 to 1.0)
	JitterPercent float64
}

// DefaultRateLimitConfig returns sensible defaults to avoid getting banned
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 0.5,              // 1 request every 2 seconds
		BurstSize:         2,                // Allow small bursts
		MaxRetries:        3,                // Retry failed requests up to 3 times
		BaseDelay:         1 * time.Second,  // Start with 1 second delay
		MaxDelay:          30 * time.Second, // Maximum 30 second delay
		JitterPercent:     0.3,              // Add up to 30% jitter
	}
}

// tokenBucket implements a simple token bucket rate limiter
type tokenBucket struct {
	capacity   int
	tokens     int
	refillAt   time.Time
	refillRate time.Duration
	mu         sync.Mutex
}

// newTokenBucket creates a new token bucket rate limiter
func newTokenBucket(capacity int, refillRate time.Duration) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillAt:   time.Now(),
		refillRate: refillRate,
	}
}

// takeToken attempts to take a token from the bucket
// Returns true if successful, false if rate limited
func (tb *tokenBucket) takeToken() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	if now.After(tb.refillAt) {
		// Refill tokens based on elapsed time
		elapsed := now.Sub(tb.refillAt)
		tokensToAdd := int(elapsed / tb.refillRate)
		if tokensToAdd > 0 {
			tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
			tb.refillAt = tb.refillAt.Add(time.Duration(tokensToAdd) * tb.refillRate)
		}
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// waitForToken blocks until a token is available
func (tb *tokenBucket) waitForToken(ctx context.Context) error {
	for {
		if tb.takeToken() {
			return nil
		}

		// Calculate how long to wait for next refill
		tb.mu.Lock()
		waitTime := time.Until(tb.refillAt)
		tb.mu.Unlock()

		if waitTime <= 0 {
			waitTime = tb.refillRate
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}

type tool struct {
	http         *http.Client
	searxngURL   string
	rateLimiter  *tokenBucket
	rateLimitCfg RateLimitConfig
	uaList       []string
}

// NewTool constructs the web_search tool with the given SearXNG URL.
func NewTool(searxngURL string) *tool {
	cfg := DefaultRateLimitConfig()
	refillRate := time.Duration(float64(time.Second) / cfg.RequestsPerSecond)

	uaList := []string{
		// Chrome (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
		// Firefox (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:102.0) Gecko/20100101 Firefox/102.0",
		// Safari (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.1 Safari/605.1.15",
		// Edge (Windows)
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.0.0",
	}
	return &tool{
		http:         &http.Client{Timeout: 12 * time.Second},
		searxngURL:   strings.TrimSuffix(searxngURL, "/"),
		rateLimiter:  newTokenBucket(cfg.BurstSize, refillRate),
		rateLimitCfg: cfg,
		uaList:       uaList,
	}
}

// NewToolWithConfig constructs the web_search tool with custom rate limiting config.
func NewToolWithConfig(searxngURL string, cfg RateLimitConfig) *tool {
	refillRate := time.Duration(float64(time.Second) / cfg.RequestsPerSecond)

	uaList := []string{
		// Chrome (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
		// Firefox (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:102.0) Gecko/20100101 Firefox/102.0",
		// Safari (macOS)
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.1 Safari/605.1.15",
		// Edge (Windows)
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.0.0",
	}
	return &tool{
		http:         &http.Client{Timeout: 12 * time.Second},
		searxngURL:   strings.TrimSuffix(searxngURL, "/"),
		rateLimiter:  newTokenBucket(cfg.BurstSize, refillRate),
		rateLimitCfg: cfg,
		uaList:       uaList,
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

	// Apply rate limiting before making the request
	if err := t.rateLimiter.waitForToken(ctx); err != nil {
		return map[string]any{"ok": false, "error": "rate limited: " + err.Error()}, nil
	}

	// Use retry with exponential backoff and jitter
	results, err := t.searchWithRetry(ctx, q, args.MaxResults, args.Category, args.Format)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "results": results}, nil
}

type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// searchWithRetry wraps searchSearXNG with exponential backoff and jitter
func (t *tool) searchWithRetry(ctx context.Context, query string, max int, category, format string) ([]SearchResult, error) {
	var lastErr error
	cfg := t.rateLimitCfg

	for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
		results, err := t.searchSearXNG(ctx, query, max, category, format)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		lastErr = err

		// Calculate exponential backoff with jitter
		delay := cfg.BaseDelay * (1 << attempt)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
		jitter := time.Duration(float64(delay) * cfg.JitterPercent * (0.5 + randFloat64()))
		delay += jitter

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, fmt.Errorf("search failed after %d retries: %v", cfg.MaxRetries, lastErr)
}

// randFloat64 returns a random float64 between 0 and 1
func randFloat64() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
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
	// Rotate User-Agent
	ua := t.uaList[int(time.Now().UnixNano())%len(t.uaList)]
	req.Header.Set("User-Agent", ua)

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
	// Rotate User-Agent
	ua := t.uaList[int(time.Now().UnixNano())%len(t.uaList)]
	req.Header.Set("User-Agent", ua)

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
