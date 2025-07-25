package sefii

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	configpkg "manifold/internal/config"
)

// Connect establishes a connection to the database using the provided connection string.
func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, connStr)
}

// IngestHandler returns an Echo handler that ingests a document into the SEFII engine.
func IngestHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Text             string `json:"text"`
			Language         string `json:"language"`
			ChunkSize        int    `json:"chunk_size"`
			ChunkOverlap     int    `json:"chunk_overlap"`
			FilePath         string `json:"file_path"`
			DocTitle         string `json:"doc_title"`
			GenerateSummary  *bool  `json:"generate_summary"`
			GenerateKeywords *bool  `json:"generate_keywords"`
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

		genSummary := true
		if req.GenerateSummary != nil {
			genSummary = *req.GenerateSummary
		}
		genKeywords := true
		if req.GenerateKeywords != nil {
			genKeywords = *req.GenerateKeywords
		}

		ctx := c.Request().Context()
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		// Add completions route to the config.Completions.DefaultHost
		completionsEndpoint := fmt.Sprintf("%s/chat/completions", config.Completions.DefaultHost)

		engine := NewEngine(conn.Conn())
		err = engine.IngestDocument(
			ctx,
			req.Text,
			req.Language,
			req.FilePath,
			req.DocTitle,
			[]string{req.FilePath},
			config.Embeddings.Host,
			config.Completions.CompletionsModel,
			config.Embeddings.APIKey,
			completionsEndpoint,
			config.Completions.APIKey,
			req.ChunkSize,
			req.ChunkOverlap,
			config.Embeddings.Dimensions,
			config.Embeddings.EmbedPrefix,
			genSummary,
			genKeywords,
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ingest document"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Document ingested successfully"})
	}
}

// SearchHandler returns an Echo handler that searches for chunks.
func SearchHandler(config *configpkg.Config) echo.HandlerFunc {
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
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEngine(conn.Conn())
		results, err := engine.SearchChunks(ctx, req.Query, req.FilePath, req.Limit, config.Embeddings.Host, config.Embeddings.APIKey)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to search chunks"})
		}

		return c.JSON(http.StatusOK, results)
	}
}

// CombinedRetrieveHandler returns an Echo handler that performs combined retrieval of chunks or full documents.
func CombinedRetrieveHandler(config *configpkg.Config) echo.HandlerFunc {
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
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEngine(conn.Conn())
		chunks, err := engine.SearchRelevantChunks(ctx, req.Query, req.FilePathFilter, req.Limit, req.UseInvertedIndex, req.UseVectorSearch, config.Embeddings.Host, config.Embeddings.APIKey, config.Embeddings.SearchPrefix, req.MergeMode, req.Alpha, req.Beta)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		if req.Rerank {
			chunks, _ = ReRankChunks(ctx, config, req.Query, chunks)
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

// SummarySearchHandler returns an Echo handler that performs summary-based searches.
func SummarySearchHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Query          string `json:"query"`
			FilePathFilter string `json:"file_path_filter"`
			Limit          int    `json:"limit"`
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
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEngine(conn.Conn())
		chunks, err := engine.SearchBySummary(ctx, req.Query, req.FilePathFilter, req.Limit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
	}
}
