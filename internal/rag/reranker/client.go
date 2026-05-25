package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"manifold/internal/config"
	"manifold/internal/rag/retrieve"
)

// Client calls an OpenAI-compatible reranking endpoint such as llama-server's
// /v1/rerank API.
type Client struct {
	cfg        config.RerankingConfig
	httpClient *http.Client
}

type requestBody struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type responseBody struct {
	Results []responseResult `json:"results"`
	Data    []responseResult `json:"data"`
}

type responseResult struct {
	Index          int      `json:"index"`
	RelevanceScore *float64 `json:"relevance_score,omitempty"`
	Score          *float64 `json:"score,omitempty"`
}

// NewClient constructs a reranker backed by cfg.
func NewClient(cfg config.RerankingConfig) *Client {
	return &Client{cfg: cfg, httpClient: http.DefaultClient}
}

// Rerank orders items by endpoint relevance. It preserves every input item,
// appending any items omitted by the endpoint in their original order.
func (c *Client) Rerank(ctx context.Context, query string, items []retrieve.RetrievedItem) ([]retrieve.RetrievedItem, error) {
	if len(items) <= 1 {
		return items, nil
	}
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return items, fmt.Errorf("reranking baseURL is required")
	}

	documents := make([]string, len(items))
	for i, item := range items {
		documents[i] = documentText(item)
	}
	body, err := json.Marshal(requestBody{
		Model:     c.cfg.Model,
		Query:     formatQuery(query, c.cfg.Instruction),
		Documents: documents,
		TopN:      len(items),
	})
	if err != nil {
		return items, err
	}

	timeout := time.Duration(c.cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodPost, endpointURL(c.cfg), bytes.NewReader(body))
	if err != nil {
		return items, err
	}
	applyHeaders(req, c.cfg)
	req.Header.Set("Content-Type", "application/json")

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return items, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return items, fmt.Errorf("reranking error: %s: %s", resp.Status, string(b))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return items, fmt.Errorf("read reranking response: %w", err)
	}
	var parsed responseBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return items, fmt.Errorf("parse reranking response: %w", err)
	}
	results := parsed.Results
	if len(results) == 0 {
		results = parsed.Data
	}
	return applyResults(items, results), nil
}

// Ping verifies the endpoint can score a minimal request.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Rerank(ctx, "ping", []retrieve.RetrievedItem{
		{ID: "ping-a", Text: "ping"},
		{ID: "ping-b", Text: "pong"},
	})
	if err != nil {
		return fmt.Errorf("reranking endpoint reachability check failed: %w", err)
	}
	return nil
}

func documentText(item retrieve.RetrievedItem) string {
	text := strings.TrimSpace(item.Text)
	if text != "" {
		return text
	}
	text = strings.TrimSpace(item.Snippet)
	if text != "" {
		return text
	}
	if item.Doc.Title != "" || item.Doc.URL != "" {
		return strings.TrimSpace(item.Doc.Title + "\n" + item.Doc.URL)
	}
	if item.Metadata != nil {
		title := strings.TrimSpace(item.Metadata["title"])
		url := strings.TrimSpace(item.Metadata["url"])
		if title != "" || url != "" {
			return strings.TrimSpace(title + "\n" + url)
		}
	}
	return item.ID
}

func formatQuery(query, instruction string) string {
	query = strings.TrimSpace(query)
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return query
	}
	return "Instruct: " + instruction + "\nQuery: " + query
}

func applyResults(items []retrieve.RetrievedItem, results []responseResult) []retrieve.RetrievedItem {
	if len(results) == 0 {
		return items
	}
	ordered := append([]responseResult(nil), results...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return resultScore(ordered[i]) > resultScore(ordered[j])
	})

	out := make([]retrieve.RetrievedItem, 0, len(items))
	used := make([]bool, len(items))
	for _, result := range ordered {
		if result.Index < 0 || result.Index >= len(items) || used[result.Index] {
			continue
		}
		item := items[result.Index]
		originalScore := item.Score
		score := resultScore(result)
		item.Score = score
		if item.Explanation == nil {
			item.Explanation = map[string]any{}
		}
		item.Explanation["pre_rerank_score"] = originalScore
		item.Explanation["rerank_score"] = score
		item.Explanation["rerank_index"] = result.Index
		out = append(out, item)
		used[result.Index] = true
	}
	for i, item := range items {
		if !used[i] {
			out = append(out, item)
		}
	}
	return out
}

func resultScore(result responseResult) float64 {
	if result.RelevanceScore != nil {
		return *result.RelevanceScore
	}
	if result.Score != nil {
		return *result.Score
	}
	return 0
}

func endpointURL(cfg config.RerankingConfig) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = "/v1/rerank"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func applyHeaders(req *http.Request, cfg config.RerankingConfig) {
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	if _, ok := cfg.Headers["Authorization"]; ok {
		return
	}
	if cfg.APIHeader == "Authorization" && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	} else if cfg.APIHeader != "" && cfg.APIKey != "" {
		req.Header.Set(cfg.APIHeader, cfg.APIKey)
	}
}
