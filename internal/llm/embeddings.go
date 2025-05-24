// internal/llm/embeddings.go
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// EmbeddingRequest defines the request structure for generating embeddings.
type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

// EmbeddingResponse defines the response structure from the embedding service.
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
}

// Embedding represents a single embedding result.
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// GenerateEmbeddings generates embeddings for the provided text chunks.
func GenerateEmbeddings(host string, apiKey string, chunks []string) ([][]float32, error) {
	var results [][]float32

	// Process chunks individually to isolate failures
	for i, chunk := range chunks {
		// Skip if chunk is too short to be meaningful
		if len(strings.TrimSpace(chunk)) < 10 {
			log.Printf("Warning: Text at index %d too short for embedding, using zero vector", i)
			// Append a zero vector of appropriate dimension
			results = append(results, make([]float32, 768))
			continue
		}

		// Create an embedding request for a single chunk
		embeddingRequest := EmbeddingRequest{
			Input:          []string{chunk},
			Model:          "nomic-embed-text-v1.5.Q8_0",
			EncodingFormat: "float",
		}

		singleEmbedding, err := FetchEmbeddings(host, embeddingRequest, apiKey)
		if err != nil {
			log.Printf("Warning: Failed to embed chunk %d: %v", i, err)
			// Append a zero vector instead of failing
			results = append(results, make([]float32, 768))
		} else if len(singleEmbedding) > 0 {
			results = append(results, singleEmbedding[0])
		} else {
			log.Printf("Warning: Empty embedding result for chunk %d", i)
			results = append(results, make([]float32, 768))
		}
	}

	return results, nil
}

// FetchEmbeddings sends the embedding request to the specified host and parses the response.
func FetchEmbeddings(host string, request EmbeddingRequest, apiKey string) ([][]float32, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", host, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var embeddings [][]float32
	for _, item := range result["data"].([]interface{}) {
		var embedding []float32
		dataMap := item.(map[string]interface{})
		for _, v := range dataMap["embedding"].([]interface{}) {
			embedding = append(embedding, float32(v.(float64)))
		}
		embeddings = append(embeddings, embedding)
	}
	return embeddings, nil
}
