// Package main provides the main entry point for the application and defines handlers for SEFII operations.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"manifold/internal/sefii"
)

// EmbeddingRequest represents a request to generate embeddings.
type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

// EmbeddingResponse represents the response from an embedding generation request.
type EmbeddingResponse struct {
	Object string       `json:"object"`
	Data   []Embedding  `json:"data"`
	Model  string       `json:"model"`
	Usage  UsageMetrics `json:"usage"`
}

// Embedding represents a single embedding vector.
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// UsageMetrics provides token usage statistics for an embedding request.
type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// SummarizeOutput represents the output of a summarization operation.
type SummarizeOutput struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords,omitempty"`
}

// Connect establishes a connection to the database using the provided connection string.
func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}

// GenerateEmbeddings generates embeddings for the given text chunks.
func GenerateEmbeddings(host, apiKey string, chunks []string) ([][]float32, error) {
	embeddingRequest := EmbeddingRequest{
		Input:          chunks,
		Model:          "nomic-embed-text-v1.5.Q8_0",
		EncodingFormat: "float",
	}

	return FetchEmbeddings(host, embeddingRequest, apiKey)
}

// FetchEmbeddings sends a request to the embedding service and parses the response.
func FetchEmbeddings(host string, request EmbeddingRequest, apiKey string) ([][]float32, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequest("POST", host, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var embeddings [][]float32
	for _, item := range result.Data {
		var embedding []float32
		for _, v := range item.Embedding {
			embedding = append(embedding, float32(v))
		}
		embeddings = append(embeddings, embedding)
	}
	return embeddings, nil
}

// sefiiIngestHandler handles document ingestion requests.
func sefiiIngestHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Text         string `json:"text"`
			Language     string `json:"language"`
			ChunkSize    int    `json:"chunk_size"`
			ChunkOverlap int    `json:"chunk_overlap"`
			FilePath     string `json:"file_path"`
			DocTitle     string `json:"doc_title"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Text == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Text is required"})
		}
		if req.Language == "" {
			req.Language = "DEFAULT"
		}
		if req.ChunkSize == 0 {
			req.ChunkSize = 1200
		}
		if req.ChunkOverlap == 0 {
			req.ChunkOverlap = 100
		}

		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)
		err = engine.IngestDocument(
			ctx,
			req.Text,
			req.Language,
			req.FilePath,
			req.DocTitle,
			[]string{req.FilePath},
			config.Embeddings.Host,
			config.Embeddings.APIKey,
			config.Completions.DefaultHost,
			config.Completions.APIKey,
			req.ChunkSize,
			req.ChunkOverlap,
			config.Embeddings.Dimensions,
			config.Embeddings.EmbedPrefix,
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ingest document"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Document ingested successfully"})
	}
}

// sefiiSearchHandler handles search requests for document chunks.
func sefiiSearchHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Query    string `json:"query"`
			FilePath string `json:"file_path"`
			Limit    int    `json:"limit"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Query == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Query is required"})
		}
		if req.Limit == 0 {
			req.Limit = 10
		}

		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)
		results, err := engine.SearchChunks(ctx, req.Query, req.FilePath, req.Limit, config.Embeddings.Host, config.Embeddings.APIKey)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to search chunks"})
		}

		return c.JSON(http.StatusOK, results)
	}
}

// sefiiCombinedRetrieveHandler handles combined retrieval requests for document chunks and full documents.
func sefiiCombinedRetrieveHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Query            string  `json:"query"`
			FilePathFilter   string  `json:"file_path_filter"`
			Limit            int     `json:"limit"`
			UseInvertedIndex bool    `json:"use_inverted_index"`
			UseVectorSearch  bool    `json:"use_vector_search"`
			MergeMode        string  `json:"merge_mode"`
			ReturnFullDocs   bool    `json:"return_full_docs"`
			Rerank           bool    `json:"rerank"`
			Alpha            float64 `json:"alpha"`
			Beta             float64 `json:"beta"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.Limit == 0 {
			req.Limit = 10
		}
		if req.MergeMode == "" {
			req.MergeMode = "union"
		}
		if req.Alpha == 0 {
			req.Alpha = 0.7
		}
		if req.Beta == 0 {
			req.Beta = 0.3
		}

		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)
		chunks, err := engine.SearchRelevantChunks(ctx, req.Query, req.FilePathFilter, req.Limit, req.UseInvertedIndex, req.UseVectorSearch, config.Embeddings.Host, config.Embeddings.APIKey, config.Embeddings.SearchPrefix, req.MergeMode, req.Alpha, req.Beta)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		if req.Rerank {
			chunks, _ = reRankChunks(ctx, config, req.Query, chunks)
		}

		if len(chunks) == 0 {
			return c.JSON(http.StatusOK, map[string]interface{}{"results": []string{}})
		}
		if req.ReturnFullDocs {
			var chunkIDs []int64
			for _, ch := range chunks {
				chunkIDs = append(chunkIDs, ch.ID)
			}
			docsMap, err := engine.RetrieveDocumentsForChunks(ctx, chunkIDs)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusOK, map[string]interface{}{"documents": docsMap})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
	}
}
