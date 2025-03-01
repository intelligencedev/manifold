package main

import (
	"fmt"
	"net/http"

	"manifold/internal/sefii"

	"github.com/labstack/echo/v4"
)

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

		// Summarize the entire doc
		summaryOut, err := summarizeContent(ctx, req.Text, config.Completions.DefaultHost, config.Completions.APIKey)
		if err != nil {
			// if summarization fails, we skip it
			summaryOut = SummarizeOutput{}
		}

		// Combine summary + original text. (Optional approach)
		finalContent := req.Text
		if summaryOut.Summary != "" {
			finalContent = fmt.Sprintf("%s\n\n---\n\n%s", summaryOut.Summary, req.Text)
		}

		// Ingest with new or existing function. We can pass doc title and summary as metadata.
		err = engine.IngestDocument(ctx, finalContent, req.Language, req.FilePath, req.DocTitle,
			summaryOut.Keywords,
			config.Embeddings.Host, config.Embeddings.APIKey,
			req.ChunkSize, req.ChunkOverlap)
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
			Query            string `json:"query"`
			FilePathFilter   string `json:"file_path_filter"`
			Limit            int    `json:"limit"`
			UseInvertedIndex bool   `json:"use_inverted_index"`
			UseVectorSearch  bool   `json:"use_vector_search"`
			MergeMode        string `json:"merge_mode"` // "union", "intersect", or "weighted"
			ReturnFullDocs   bool   `json:"return_full_docs"`
			Rerank           bool   `json:"rerank"` // new param to do cross-encoder rerank
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
			req.MergeMode,
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
