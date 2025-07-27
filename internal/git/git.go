// Package git contains handlers and utilities for git file ingestion.
package git

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	documentsv1 "manifold/internal/documents/v1deprecated"
	"manifold/internal/sefii"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	cfg "manifold/internal/config"
)

// FilesHandler handles requests to list Git files in a repository.
func FilesHandler(c echo.Context) error {
	repoPath := c.QueryParam("repo_path")
	if repoPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "repo_path query parameter is required"})
	}

	files, err := documentsv1.GetGitFiles(repoPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list Git files"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"files": files})
}

// FilesIngestHandler returns an HTTP handler for ingesting Git files into the system.
func FilesIngestHandler(cfg *cfg.Config) echo.HandlerFunc {
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

		// Use the connection pool instead of creating a new connection
		if cfg.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		// Get a connection from the pool
		conn, err := cfg.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		// Return the connection to the pool when done
		defer conn.Release()

		successFiles, err := processGitFiles(ctx, req, cfg, conn.Conn())
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
}, cfg *cfg.Config, conn *pgx.Conn) ([]string, error) {
	engine := sefii.NewEngine(conn)

	gitFiles, err := documentsv1.GetGitFiles(req.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get git files: %w", err)
	}

	successFiles := []string{}
	for _, file := range gitFiles {
		if len(strings.TrimSpace(file.Content)) == 0 {
			continue
		}

		language := documentsv1.DeduceLanguage(file.Path)
		summaryEndpoint := cfg.Completions.SummaryHost
		if summaryEndpoint == "" {
			summaryEndpoint = cfg.Completions.DefaultHost
		}
		keywordsEndpoint := cfg.Completions.KeywordsHost
		if keywordsEndpoint == "" {
			keywordsEndpoint = cfg.Completions.DefaultHost
		}

		if err := engine.IngestDocument(
			ctx,
			file.Content,
			string(language),
			file.Path,
			filepath.Base(file.Path),
			[]string{file.Path},
			cfg.Embeddings.Host,
			cfg.Completions.CompletionsModel,
			cfg.Embeddings.APIKey,
			summaryEndpoint,
			keywordsEndpoint,
			cfg.Completions.APIKey,
			req.ChunkSize,
			req.ChunkOverlap,
			cfg.Embeddings.Dimensions,
			cfg.Embeddings.EmbedPrefix,
			true,
			true,
			cfg.Ingestion.MaxWorkers,
		); err == nil {
			successFiles = append(successFiles, file.Path)
		} else {
			log.Printf("Failed to ingest file %s: %v", file.Path, err)
		}
	}

	return successFiles, nil
}
