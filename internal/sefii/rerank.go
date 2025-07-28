package sefii

import (
        "bytes"
        "context"
        "encoding/json"
        "fmt"
        "io"
        logpkg "manifold/internal/logging"
        "net/http"
        "sort"

	configpkg "manifold/internal/config"
)

// RerankRequest defines the payload to send to the reranker.
type RerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	TopN      int      `json:"top_n"`
	Documents []string `json:"documents"`
}

// RerankResult represents one document's rerank score.
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// RerankResponse represents the complete response from the reranker.
type RerankResponse struct {
	Model   string         `json:"model"`
	Object  string         `json:"object"`
	Usage   interface{}    `json:"usage"`
	Results []RerankResult `json:"results"`
}

// ReRankChunks calls the llama.cpp reranker and reorders the chunks based on relevance.
func ReRankChunks(ctx context.Context, config *configpkg.Config, query string, chunks []Chunk) ([]Chunk, error) {
	documents := extractDocuments(chunks)

	rankReq, err := createRerankRequest(query, documents, len(chunks))
	if err != nil {
		return nil, fmt.Errorf("failed to create rerank request: %w", err)
	}

	resp, err := sendRerankRequest(ctx, config.Reranker.Host, rankReq)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rerank failed with status %d: %s", resp.StatusCode, string(body))
	}

	rankResp, err := parseRerankResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	scoreMap := mapScores(rankResp.Results)
	sortChunksByScore(chunks, scoreMap)

    logpkg.Log.Infof("Reranking complete. Top score: %v", scoreMap[0])
	return chunks, nil
}

func extractDocuments(chunks []Chunk) []string {
	documents := make([]string, len(chunks))
	for i, ch := range chunks {
		documents[i] = ch.Content
	}
	return documents
}

func createRerankRequest(query string, documents []string, topN int) ([]byte, error) {
	rankReq := RerankRequest{
		Model:     "slide-bge-reranker-v2-m3.Q8_0.gguf",
		Query:     query,
		TopN:      topN,
		Documents: documents,
	}
	return json.Marshal(rankReq)
}

func sendRerankRequest(ctx context.Context, url string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	return client.Do(req)
}

func parseRerankResponse(body io.Reader) (*RerankResponse, error) {
	var rankResp RerankResponse
	if err := json.NewDecoder(body).Decode(&rankResp); err != nil {
		return nil, err
	}
	return &rankResp, nil
}

func mapScores(results []RerankResult) map[int]float64 {
	scoreMap := make(map[int]float64)
	for _, result := range results {
		scoreMap[result.Index] = result.RelevanceScore
	}
	return scoreMap
}

func sortChunksByScore(chunks []Chunk, scoreMap map[int]float64) {
	sort.Slice(chunks, func(i, j int) bool {
		return scoreMap[i] > scoreMap[j]
	})
}
