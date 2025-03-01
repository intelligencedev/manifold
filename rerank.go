package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"manifold/internal/sefii"
	"net/http"
	"sort"
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

// reRankChunks calls the llama.cpp reranker and reorders the chunks based on relevance.
func reRankChunks(ctx context.Context, config *Config, query string, chunks []sefii.Chunk) ([]sefii.Chunk, error) {
	// Build a list of candidate documents.
	documents := make([]string, len(chunks))
	for i, ch := range chunks {
		documents[i] = ch.Content
	}

	// Construct the rerank payload.
	rankReq := RerankRequest{
		Model:     "slide-bge-reranker-v2-m3.Q8_0.gguf",
		Query:     query,
		TopN:      len(chunks), // or a specific top_n if desired
		Documents: documents,
	}
	payload, err := json.Marshal(rankReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank payload: %w", err)
	}

	url := config.Reranker.Host
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rerank failed with status %d: %s", resp.StatusCode, string(body))
	}

	var rankResp RerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rankResp); err != nil {
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	// Map the returned scores back to the original chunks.
	// The response "Results" list has an "index" field that corresponds to the candidate document index.
	scoreMap := make(map[int]float64)
	for _, result := range rankResp.Results {
		scoreMap[result.Index] = result.RelevanceScore
	}

	// Sort chunks by relevance_score (descending order)
	sort.Slice(chunks, func(i, j int) bool {
		return scoreMap[i] > scoreMap[j]
	})

	log.Printf("Reranking complete. Top score: %v", scoreMap[0])
	return chunks, nil
}
