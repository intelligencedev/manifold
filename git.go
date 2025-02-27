// git_handlers.go
package main

import (
	"fmt"
	"net/http"

	"manifold/internal/documents"
	"manifold/internal/sefii"

	"github.com/labstack/echo/v4"
)

func gitFilesHandler(c echo.Context) error {
	repoPath := c.QueryParam("repo_path")
	if repoPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "repo_path query parameter is required"})
	}
	files, err := documents.GetGitFiles(repoPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list Git files"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"files": files})
}

func gitFilesIngestHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			RepoPath     string `json:"repo_path"`
			ChunkSize    int    `json:"chunk_size"`
			ChunkOverlap int    `json:"chunk_overlap"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if req.RepoPath == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "repo_path is required"})
		}
		if req.ChunkSize == 0 {
			req.ChunkSize = 500
		}
		if req.ChunkOverlap == 0 {
			req.ChunkOverlap = 150
		}
		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)
		engine := sefii.NewEngine(conn)
		files, err := documents.GetGitFiles(req.RepoPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch Git files"})
		}
		successCount := 0
		filePreviews := []map[string]string{}
		for _, f := range files {
			if f.Content == "" {
				continue
			}
			lang := documents.DeduceLanguage(f.Path)
			contentPreview := f.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..."
			}
			filePreviews = append(filePreviews, map[string]string{
				"path":     f.Path,
				"language": string(lang),
				"preview":  contentPreview,
			})
			summary, err := summarizeContent(ctx, f.Content, config.Completions.DefaultHost, config.Completions.APIKey)
			if err != nil {
				summary = ""
			}
			finalContent := fmt.Sprintf("search_document: %s\n\n---\n\n%s", f.Path, f.Content)
			if summary != "" {
				finalContent = fmt.Sprintf("search_document: %s\n\n%s\n\n---\n\n%s", f.Path, summary, f.Content)
			}
			if err := engine.IngestDocument(ctx, finalContent, string(lang), f.Path, config.Embeddings.Host, config.Embeddings.APIKey, req.ChunkSize, req.ChunkOverlap); err != nil {
				continue
			}
			successCount++
		}
		msg := fmt.Sprintf("Ingested %d file(s) from %s into pgvector", successCount, req.RepoPath)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":       msg,
			"ingested":      successCount,
			"repo_path":     req.RepoPath,
			"file_previews": filePreviews,
		})
	}
}
