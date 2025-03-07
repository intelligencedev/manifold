package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"manifold/internal/sefii"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

type EmbeddingResponse struct {
	Object string       `json:"object"`
	Data   []Embedding  `json:"data"`
	Model  string       `json:"model"`
	Usage  UsageMetrics `json:"usage"`
}

type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type SummarizeOutput struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords,omitempty"`
}

// Connect takes a connection string and returns a connection to the database
func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// GenerateEmbeddings generates embeddings for a given text
func GenerateEmbeddings(host string, apiKey string, chunks []string) ([][]float32, error) {
	embeddingRequest := EmbeddingRequest{
		Input:          chunks,
		Model:          "nomic-embed-text-v1.5.Q8_0",
		EncodingFormat: "float",
	}

	embeddings, err := FetchEmbeddings(host, embeddingRequest, apiKey)
	if err != nil {
		panic(err)
	}

	return embeddings, nil
}

func FetchEmbeddings(host string, request EmbeddingRequest, apiKey string) ([][]float32, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	// Print the request for debugging purposes.
	fmt.Println(string(b))

	req, err := http.NewRequest("POST", host, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

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
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	var embeddings [][]float32
	for _, item := range result["data"].([]interface{}) {
		var embedding []float32
		for _, v := range item.(map[string]interface{})["embedding"].([]interface{}) {
			embedding = append(embedding, float32(v.(float64)))
		}
		embeddings = append(embeddings, embedding)
	}
	return embeddings, nil
}

func sefiiIngestHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Text         string `json:"text"`
			Language     string `json:"language"`
			ChunkSize    int    `json:"chunk_size"`
			ChunkOverlap int    `json:"chunk_overlap"`
			FilePath     string `json:"file_path"`
			DocTitle     string `json:"doc_title"` // new metadata
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

		// Ingest document with chunk-level summarization
		err = engine.IngestDocument(
			ctx,
			req.Text,
			req.Language,
			req.FilePath,
			req.DocTitle,
			[]string{req.FilePath}, // file path as first keyword
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
			Alpha            float64 `json:"alpha"` // New param for vector weight
			Beta             float64 `json:"beta"`  // New param for keyword weight
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
		// Set default alpha/beta if not provided
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

		chunks, err := engine.SearchRelevantChunks(ctx,
			req.Query,
			req.FilePathFilter,
			req.Limit,
			req.UseInvertedIndex,
			req.UseVectorSearch,
			config.Embeddings.Host,
			config.Embeddings.APIKey,
			config.Embeddings.SearchPrefix,
			req.MergeMode,
			req.Alpha, // Pass alpha
			req.Beta,  // Pass beta
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// Rerank if requested
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
