// git_handlers.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

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
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Repo path is required"})
		}
		if req.ChunkSize == 0 {
			req.ChunkSize = 1000
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

		// Get all git files
		gitFiles, err := documents.GetGitFiles(req.RepoPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to get git files: %v", err)})
		}

		// Process each file individually
		successFiles := []string{}
		for _, file := range gitFiles {
			// Skip empty files
			if len(strings.TrimSpace(file.Content)) == 0 {
				continue
			}

			// Deduce language from file extension
			language := documents.DeduceLanguage(file.Path)

			// Ingest the file with the appropriate language
			err = engine.IngestDocument(
				ctx,
				file.Content,
				string(language),
				file.Path,
				filepath.Base(file.Path), // Use filename as doc title
				[]string{file.Path},      // Add file path as a keyword
				config.Embeddings.Host,
				config.Embeddings.APIKey,
				config.Completions.DefaultHost,
				config.Completions.APIKey,
				req.ChunkSize,
				req.ChunkOverlap,
				config.Embeddings.Dimensions,
				config.Embeddings.EmbedPrefix,
			)

			if err == nil {
				successFiles = append(successFiles, file.Path)
			} else {
				log.Printf("Failed to ingest file %s: %v", file.Path, err)
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": fmt.Sprintf("Ingested %d files successfully", len(successFiles)),
			"files":   successFiles,
		})
	}
}
