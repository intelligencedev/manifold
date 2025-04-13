// git_handlers.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"manifold/internal/documents"
	"manifold/internal/sefii"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// gitFilesHandler handles requests to list Git files in a repository.
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

// gitFilesIngestHandler returns an HTTP handler for ingesting Git files into the system.
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

		setDefaultChunkValues(&req)

		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		successFiles, err := processGitFiles(ctx, req, config, conn)
		if err != nil {
			log.Printf("Error processing Git files: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process Git files"})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": fmt.Sprintf("Ingested %d files successfully", len(successFiles)),
			"files":   successFiles,
		})
	}
}

// Updated setDefaultChunkValues and processGitFiles to accept JSON-tagged struct

func setDefaultChunkValues(req *struct {
	RepoPath     string `json:"repo_path"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
}) {
	if req.ChunkSize == 0 {
		req.ChunkSize = 1000
	}
	if req.ChunkOverlap == 0 {
		req.ChunkOverlap = 100
	}
}

func processGitFiles(ctx context.Context, req struct {
	RepoPath     string `json:"repo_path"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
}, config *Config, conn *pgx.Conn) ([]string, error) {
	engine := sefii.NewEngine(conn)

	gitFiles, err := documents.GetGitFiles(req.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get git files: %w", err)
	}

	successFiles := []string{}
	for _, file := range gitFiles {
		if len(strings.TrimSpace(file.Content)) == 0 {
			continue
		}

		language := documents.DeduceLanguage(file.Path)
		if err := engine.IngestDocument(
			ctx,
			file.Content,
			string(language),
			file.Path,
			filepath.Base(file.Path),
			[]string{file.Path},
			config.Embeddings.Host,
			config.Embeddings.APIKey,
			config.Completions.DefaultHost,
			config.Completions.APIKey,
			req.ChunkSize,
			req.ChunkOverlap,
			config.Embeddings.Dimensions,
			config.Embeddings.EmbedPrefix,
		); err == nil {
			successFiles = append(successFiles, file.Path)
		} else {
			log.Printf("Failed to ingest file %s: %v", file.Path, err)
		}
	}

	return successFiles, nil
}
