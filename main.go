package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"manifold/internal/documents"
	"manifold/internal/repoconcat"
	"manifold/internal/sefii"
	web "manifold/internal/web"
)

//go:embed frontend/dist
var frontendDist embed.FS

const (
	service     = "api-gateway"
	environment = "development"
	id          = 1
	imagePath   = "/Users/art/Documents/code/manifold/frontend/public/mlx_out.png"
)

func main() {
	// Load the configuration
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Configuration loaded: %+v", config)
	}

	// Create a new Echo instance
	e := echo.New()

	// Use middleware for logging
	e.Use(middleware.Logger())

	// Add CORS middleware to allow all origins
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"}, // Allow all origins
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-File-Path"},
	}))

	// Initialize OpenTelemetry
	tp, err := initTracer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// Add OpenTelemetry instrumentation for Echo
	e.Use(otelecho.Middleware("api-gateway", otelecho.WithTracerProvider(tp)))

	e.GET("/*", echo.WrapHandler(http.FileServer(getFileSystem())))

	// SEFII: Ingest endpoint.
	e.POST("/api/sefii/ingest", func(c echo.Context) error {
		var req struct {
			Text         string `json:"text"`
			Language     string `json:"language"`
			ChunkSize    int    `json:"chunk_size"`
			ChunkOverlap int    `json:"chunk_overlap"`
			FilePath     string `json:"file_path"`
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
			req.ChunkSize = 1000
		}
		if req.ChunkOverlap == 0 {
			req.ChunkOverlap = 100
		}

		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		ctx := c.Request().Context()
		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)
		if err := engine.IngestDocument(ctx, req.Text, req.Language, req.FilePath, embeddingsHost, apiKey, req.ChunkSize, req.ChunkOverlap); err != nil {
			log.Printf("SEFII ingestion error: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ingest document"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Document ingested successfully"})
	})

	// SEFII: Search endpoint.
	e.POST("/api/sefii/search", func(c echo.Context) error {
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

		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		ctx := c.Request().Context()
		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)
		results, err := engine.SearchChunks(ctx, req.Query, req.FilePath, req.Limit, embeddingsHost, apiKey)
		if err != nil {
			log.Printf("SEFII search error: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to search chunks"})
		}
		return c.JSON(http.StatusOK, results)
	})

	e.POST("/api/sefii/combined-retrieve", func(c echo.Context) error {
		var req struct {
			Query            string `json:"query"`
			FilePathFilter   string `json:"file_path_filter"`
			Limit            int    `json:"limit"`
			UseInvertedIndex bool   `json:"use_inverted_index"`
			UseVectorSearch  bool   `json:"use_vector_search"`
			MergeMode        string `json:"merge_mode"` // "union" or "intersect"
			ReturnFullDocs   bool   `json:"return_full_docs"`
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
		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)

		// Retrieve the relevant chunks using our hybrid search method
		chunks, err := engine.SearchRelevantChunks(
			ctx,
			req.Query,
			req.FilePathFilter,
			req.Limit,
			req.UseInvertedIndex,
			req.UseVectorSearch,
			embeddingsHost,
			apiKey,
			req.MergeMode,
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		if len(chunks) == 0 {
			return c.JSON(http.StatusOK, map[string]interface{}{"results": []string{}})
		}

		// If full documents are requested, reassemble them from chunks by file_path
		if req.ReturnFullDocs {
			var chunkIDs []int64
			for _, ch := range chunks {
				chunkIDs = append(chunkIDs, ch.ID)
			}
			docsMap, err := engine.RetrieveDocumentsForChunks(ctx, chunkIDs)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusOK, map[string]interface{}{
				"documents": docsMap,
			})
		}

		// Otherwise, return just the chunk-level results
		return c.JSON(http.StatusOK, map[string]interface{}{
			"chunks": chunks,
		})
	})

	e.GET("/api/git-files", func(c echo.Context) error {
		repoPath := c.QueryParam("repo_path")
		if repoPath == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "repo_path query parameter is required"})
		}

		// Use the shared GetGitFiles function
		files, err := documents.GetGitFiles(repoPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list Git files"})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"files": files})
	})

	e.POST("/api/git-files/ingest", func(c echo.Context) error {
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
			req.ChunkSize = 1000
		}
		if req.ChunkOverlap == 0 {
			req.ChunkOverlap = 100
		}

		ctx := c.Request().Context()
		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		engine := sefii.NewEngine(conn)

		// Ensure the required tables exist.
		if err := engine.EnsureTable(ctx); err != nil {
			return err
		}
		if err := engine.EnsureInvertedIndexTable(ctx); err != nil {
			return err
		}

		// Fetch Git-tracked files directly (no HTTP call)
		files, err := documents.GetGitFiles(req.RepoPath)
		if err != nil {
			log.Printf("Error fetching Git files: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch Git files"})
		}

		successCount := 0
		filePreviews := []map[string]string{}

		log.Println("SEFII Ingestion: Preparing to ingest files...")

		for _, f := range files {
			if f.Content == "" {
				continue
			}
			lang := documents.DeduceLanguage(f.Path)

			// Print file details before ingestion
			contentPreview := f.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..." // Truncate long files for logging
			}

			log.Printf("[INGEST] File: %s | Language: %s | Preview: %s\n",
				f.Path, lang, strings.ReplaceAll(contentPreview, "\n", " "))

			filePreviews = append(filePreviews, map[string]string{
				"path":     f.Path,
				"language": string(lang),
				"preview":  contentPreview,
			})

			// Call the /v1/chat/completions to get a summary of the content
			summary, err := summarizeContent(ctx, f.Content)
			if err != nil {
				log.Printf("Error summarizing file %s: %v", f.Path, err)
				// Continue without a summary if the call fails
				summary = ""
			} else {
				// Log the summary for debugging purposes.
				log.Printf("[DEBUG] Summary for file %s: %s", f.Path, summary)
			}

			// Prepend the summary above the file content with proper delimiters
			finalContent := f.Content
			if summary != "" {
				finalContent = fmt.Sprintf("%s\n\n---\n\n%s", summary, f.Content)
			}

			// Actually call IngestDocument with the modified content
			if err := engine.IngestDocument(
				ctx,
				finalContent,
				string(lang),
				f.Path,
				embeddingsHost,
				apiKey,
				req.ChunkSize,
				req.ChunkOverlap,
			); err != nil {
				log.Printf("Error ingesting file %s: %v", f.Path, err)
				continue
			}
			successCount++
		}

		msg := fmt.Sprintf("Ingested %d file(s) from %s into pgvector", successCount, req.RepoPath)
		log.Println(msg)

		// Return ingestion summary with file previews
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":       msg,
			"ingested":      successCount,
			"repo_path":     req.RepoPath,
			"file_previews": filePreviews,
		})
	})

	e.POST("/api/documents/ingest", func(c echo.Context) error {
		var req ProcessTextRequest
		if err := c.Bind(&req); err != nil {
			log.Printf("Error binding request: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		// Validate required fields
		if req.Text == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Text is required"})
		}
		if req.Language == "" {
			req.Language = "en" // default value
		}
		if req.ChunkSize == 0 {
			req.ChunkSize = 1500 // default chunk size
		}
		if req.ChunkOverlap == 0 {
			req.ChunkOverlap = 100 // default overlap
		}

		// If FilePath is not provided in the JSON payload, attempt to get it from the header.
		filePath := req.FilePath
		if filePath == "" {
			filePath = c.Request().Header.Get("X-File-Path")
		}

		// Get DB connection string, embeddings host, and API key from the config
		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		// Establish a database connection (using the request context)
		ctx := c.Request().Context()
		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)

		// Process the document, passing the filePath so that each chunk is prefixed accordingly.
		err = ProcessDocument(ctx, conn, embeddingsHost, apiKey, req.Text, req.Language, req.ChunkSize, req.ChunkOverlap, filePath)
		if err != nil {
			log.Printf("Error processing document: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process document"})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Document processed successfully"})
	})

	e.POST("/api/documents/retrieve", func(c echo.Context) error {
		// Get the prompt from the request body
		var req struct {
			Prompt string `json:"prompt"`
			Limit  int    `json:"limit"`
		}

		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		// Validate the prompt
		if req.Prompt == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Prompt is required"})
		}

		// Get DB connection string, embeddings host, and API key from the config
		connStr := config.Database.ConnectionString
		embeddingsHost := config.Embeddings.Host
		apiKey := config.Embeddings.APIKey

		// Establish a database connection
		ctx := context.Background() // Or use c.Request().Context() for request-scoped context
		conn, err := Connect(ctx, connStr)
		if err != nil {
			log.Printf("Error connecting to database: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx) // Ensure the connection is closed

		// Retrieve the most similar document to the content
		docs, err := RetrieveDocuments(ctx, conn, embeddingsHost, apiKey, req.Prompt, req.Limit)
		if err != nil {
			log.Printf("Error retrieving documents: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve documents"})
		}

		return c.JSON(http.StatusOK, map[string]string{"documents": docs})
	})

	e.POST("/api/run-fmlx", func(c echo.Context) error {
		var req FMLXRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Model == "" || req.Prompt == "" || req.Steps == 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		}

		// Delete existing file if it exists
		if _, err := os.Stat(req.Output); err == nil {
			if err := os.Remove(req.Output); err != nil {
				log.Printf("Error removing existing file: %v", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to remove existing file"})
			}
		}

		// Call FMLX with the given parameters
		args := []string{
			"--model", req.Model,
			"--prompt", req.Prompt,
			"--steps", fmt.Sprintf("%d", req.Steps),
			"--seed", fmt.Sprintf("%d", req.Seed),
			"-q", fmt.Sprintf("%d", req.Quality),
			"--output", req.Output,
		}

		cmd := exec.Command("/Users/art/Documents/code/manifold/mflux/.venv/bin/mflux-generate", args...)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("Error creating stdout pipe: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stdout pipe"})
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("Error creating stderr pipe: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stderr pipe"})
		}

		if err := cmd.Start(); err != nil {
			log.Printf("Error starting mflux command: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start mflux command"})
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				log.Printf("[mflux stdout] %s", scanner.Text())
			}
		}()

		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				log.Printf("[mflux stderr] %s", scanner.Text())
			}
		}()

		err = cmd.Wait()
		wg.Wait()

		if err != nil {
			log.Printf("Error executing mflux command: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to execute mflux command: %v", err)})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "FMLX command executed successfully"})
	})

	e.GET("/mlx_out.png", func(c echo.Context) error {
		// Open the image file.
		file, err := os.Open(imagePath)
		if err != nil {
			// Handle file not found or other errors appropriately.
			if os.IsNotExist(err) {
				return c.String(http.StatusNotFound, "Image not found")
			}
			return c.String(http.StatusInternalServerError, "Error opening image")
		}
		defer file.Close()

		// Set the Content-Type header.  This is *crucial*.
		c.Response().Header().Set(echo.HeaderContentType, "image/png")

		// Copy the file content to the response.
		_, err = io.Copy(c.Response().Writer, file)
		if err != nil {
			// Log the error, but don't return an error to the client, as headers have likely already been sent
			log.Printf("Error copying image to response: %v", err)
		}
		return nil
	})

	e.POST("/api/run-sd", func(c echo.Context) error {
		var req SDRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.DiffusionModel == "" || req.Type == "" || req.ClipL == "" || req.T5xxl == "" || req.VAE == "" || req.Prompt == "" || req.Output == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		}

		args := []string{
			"--diffusion-model", req.DiffusionModel,
			"--type", req.Type,
			"--clip_l", req.ClipL,
			"--t5xxl", req.T5xxl,
			"--vae", req.VAE,
			"--cfg-scale", fmt.Sprintf("%.1f", req.CfgScale),
			"--steps", fmt.Sprintf("%d", req.Steps),
			"--sampling-method", req.SamplingMethod,
			"-H", fmt.Sprintf("%d", req.Height),
			"-W", fmt.Sprintf("%d", req.Width),
			"--seed", fmt.Sprintf("%d", req.Seed),
			"-p", req.Prompt,
			"--output", req.Output,
		}
		if req.Threads > 0 {
			args = append(args, "-t", fmt.Sprintf("%d", req.Threads))
		}
		if req.NegativePrompt != "" {
			args = append(args, "-n", req.NegativePrompt)
		}

		if req.StyleRatio > 0 {
			args = append(args, "--style-ratio", fmt.Sprintf("%.1f", req.StyleRatio))
		}

		if req.ControlStrength > 0 {
			args = append(args, "--control-strength", fmt.Sprintf("%.1f", req.ControlStrength))
		}

		if req.ClipSkip > 0 {
			args = append(args, "--clip-skip", fmt.Sprintf("%d", req.ClipSkip))
		}

		if req.SLGScale > 0 {
			args = append(args, "--slg-scale", fmt.Sprintf("%.1f", req.SLGScale))
		}

		if len(req.SkipLayers) > 0 {
			for _, v := range req.SkipLayers {
				args = append(args, "--skip-layers", fmt.Sprintf("%d", v))
			}
		}

		if req.SkipLayerStart > 0 {
			args = append(args, "--skip-layer-start", fmt.Sprintf("%.3f", req.SkipLayerStart))
		}

		if req.SkipLayerEnd > 0 {
			args = append(args, "--skip-layer-end", fmt.Sprintf("%.3f", req.SkipLayerEnd))
		}

		cmd := exec.Command("./sd", args...)

		cmd.Dir = "/Users/art/Downloads/sd-master--bin-Darwin-macOS-14.7.2-arm64/stable-diffusion.cpp/build/bin"

		// Create pipes for stdout and stderr.
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("Error creating stdout pipe: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stdout pipe"})
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Printf("Error creating stderr pipe: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stderr pipe"})
		}

		// Start the command.
		if err := cmd.Start(); err != nil {
			log.Printf("Error starting sd command: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start sd command"})
		}

		// Create a WaitGroup to wait for the goroutines to finish.
		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine to read from stdout and log.
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				log.Printf("[sd stdout] %s", scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				log.Printf("Error reading from sd stdout: %s", err)
			}
		}()

		// Goroutine to read from stderr and log.
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				log.Printf("[sd stderr] %s", scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				log.Printf("Error reading from sd stderr: %s", err)
			}
		}()

		// Wait for the command to finish *and* for the output to be processed.
		err = cmd.Wait()
		wg.Wait() // Wait for the goroutines to finish reading stdout/stderr

		if err != nil {
			log.Printf("Error executing sd command: %v", err)
			// The error is already logged by cmd.Wait and the stderr scanner.
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to execute sd command: %v", err)})
		}

		// Return a success response.  You might want to return the path to the generated image.
		return c.JSON(http.StatusOK, map[string]string{"message": "Stable Diffusion command executed successfully"})
	})

	e.POST("/api/repoconcat", func(c echo.Context) error {
		var req RepoConcatRequest
		log.Printf("Received request on /api/repoconcat")

		if err := c.Bind(&req); err != nil {
			log.Printf("Error binding request: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		log.Printf("Request body: %+v", req)

		if len(req.Paths) == 0 || len(req.Types) == 0 {
			log.Printf("Paths or Types are empty")
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Paths and types are required"})
		}
		// Instantiate RepoConcat
		rc := repoconcat.NewRepoConcat()

		// Call Concatenate
		result, err := rc.Concatenate(req.Paths, req.Types, req.Recursive, req.IgnorePattern)
		log.Printf("Concatenate result: %s, error: %v", result, err)

		if err != nil {
			log.Printf("Error from Concatenate: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// Return the result
		return c.String(http.StatusOK, result)
	})

	e.POST("/api/split-text", func(c echo.Context) error {
		var req struct {
			Text      string `json:"text"`
			Splitter  string `json:"splitter"`
			ChunkSize int    `json:"chunk_size,omitempty"` // omitempty for optional field
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Text == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Text is required"})
		}

		splitterType := documents.Language(req.Splitter)
		if splitterType == "" {
			splitterType = documents.DEFAULT
		}

		splitter, err := documents.FromLanguage(splitterType)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// If it's the DEFAULT splitter, override the ChunkSize with the request's value (if provided)
		if splitterType == documents.DEFAULT && req.ChunkSize > 0 {
			splitter.ChunkSize = req.ChunkSize
		}

		chunks := splitter.SplitText(req.Text)
		return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
	})

	e.POST("/api/save-file", func(c echo.Context) error {
		// Define a struct to bind the incoming JSON or form data.
		type SaveFileRequest struct {
			Filepath string `json:"filepath" form:"filepath"`
			Content  string `json:"content" form:"content"`
		}

		var req SaveFileRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Filepath == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Parameter 'filepath' is required"})
		}

		// Ensure the target directory exists.
		dir := filepath.Dir(req.Filepath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to create directory '%s': %v", dir, err),
			})
		}

		// Write the content to the specified file.
		if err := os.WriteFile(req.Filepath, []byte(req.Content), 0644); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to save file '%s': %v", req.Filepath, err),
			})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "File saved successfully"})
	})

	e.POST("/api/open-file", func(c echo.Context) error {
		var req struct {
			Filepath string `json:"filepath"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Filepath == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Filepath is required"})
		}

		content, err := os.ReadFile(req.Filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to read file '%s': %v", req.Filepath, err),
			})
		}

		// Return the file content as plain text.  Important!
		return c.String(http.StatusOK, string(content))
	})

	e.GET("/api/web-content", func(c echo.Context) error {
		urlsParam := c.QueryParam("urls")
		if urlsParam == "" {
			return c.String(http.StatusBadRequest, "URLs are required")
		}

		// Split the comma-separated URLs into a slice
		urls := strings.Split(urlsParam, ",")

		// Log the URLs being accessed
		log.Printf("Attempting to extract content from URLs: %s", strings.Join(urls, ", "))

		// Use a WaitGroup to wait for all goroutines to finish
		var wg sync.WaitGroup
		// Use a mutex to protect concurrent writes to the results map
		var mu sync.Mutex

		// Create a map to store the results, keyed by URL
		results := make(map[string]interface{})

		// Create a channel for timeout
		done := make(chan bool)

		go func() {
			for _, pageURL := range urls {
				// Increment the WaitGroup counter
				wg.Add(1)
				// Launch a goroutine to fetch the content
				go func(url string) {
					// Decrement the WaitGroup counter when the goroutine completes
					defer wg.Done()

					// Extract content using the WebGetHandler
					content, err := web.WebGetHandler(url)

					mu.Lock()
					defer mu.Unlock()

					if err != nil {
						log.Printf("Error extracting web content for %s: %v", url, err)
						results[url] = map[string]string{"error": fmt.Sprintf("Error extracting web content: %v", err)}
					} else {
						results[url] = content
					}
				}(pageURL)
			}
			wg.Wait()
			done <- true
		}()

		// Wait for either completion or timeout
		select {
		case <-done:
			return c.JSON(http.StatusOK, results)
		case <-time.After(60 * time.Second):
			return c.JSON(http.StatusOK, results)
		}
	})

	e.GET("/api/web-search", func(c echo.Context) error {
		query := c.QueryParam("query")
		if query == "" {
			return c.String(http.StatusBadRequest, "Query is required")
		}

		// Parse result_size parameter
		resultSize := 3
		if size := c.QueryParam("result_size"); size != "" {
			if parsedSize, err := strconv.Atoi(size); err == nil {
				resultSize = parsedSize
			}
		}

		// Parse search_backend parameter
		searchBackend := c.QueryParam("search_backend")
		if searchBackend == "" {
			searchBackend = "ddg"
		}

		// Validate sxng_url if search_backend is sxng
		var results []string
		if searchBackend == "sxng" {
			sxngURL := c.QueryParam("sxng_url")
			if sxngURL == "" {
				return c.String(http.StatusBadRequest, "sxng_url is required when search_backend is sxng")
			}
			results = web.GetSearXNGResults(sxngURL, query)
		} else {
			results = web.SearchDDG(query)
		}

		if results == nil {
			return c.String(http.StatusInternalServerError, "Error performing web search")
		}

		// Limit the number of results to result_size
		if len(results) > resultSize {
			results = results[:resultSize]
		}

		// Return the search results
		return c.JSON(http.StatusOK, results)
	})

	e.POST("/api/executePython", func(c echo.Context) error {
		// Parse incoming JSON
		var req PythonCodeRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid request body",
			})
		}

		// Run ephemeral Python environment
		result, err := runEphemeralPython(req.Code, req.Dependencies)
		if err != nil {
			// If there's an internal error setting up or running code, return 500
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
		}

		// If everything is fine, return JSON with stdout/stderr/returnCode
		return c.JSON(http.StatusOK, result)
	})

	e.POST("/api/datadog", func(c echo.Context) error {
		// Start a new span for this request
		span := trace.SpanFromContext(c.Request().Context())
		defer span.End()

		// 1. Parse the request body from DatadogNode.vue
		var reqBody DatadogNodeRequest
		if err := c.Bind(&reqBody); err != nil {
			span.RecordError(err)
			return c.String(http.StatusBadRequest, "Invalid request body")
		}

		// 2. Validate required parameters (you might want more validation)
		if reqBody.APIKey == "" || reqBody.AppKey == "" {
			return c.String(http.StatusBadRequest, "Missing API key or Application key")
		}

		// 3. Prepare the Datadog API context
		ctxDD := context.WithValue(
			c.Request().Context(),
			datadog.ContextAPIKeys,
			map[string]datadog.APIKey{
				"apiKeyAuth": {
					Key: reqBody.APIKey,
				},
				"appKeyAuth": {
					Key: reqBody.AppKey,
				},
			},
		)

		// 4. Create Datadog API client configuration
		config := datadog.NewConfiguration()
		config.SetUnstableOperationEnabled("v2.ListLogs", true)
		config.SetUnstableOperationEnabled("v2.QueryTimeseriesData", true)
		config.SetUnstableOperationEnabled("v2.ListIncidents", true)
		if reqBody.Site != "" {
			config.Servers = datadog.ServerConfigurations{
				{
					URL: "https://api." + reqBody.Site,
				},
			}
		}
		ddClient := datadog.NewAPIClient(config)

		// 5. Perform the requested Datadog operation
		var apiResponse interface{}
		var err error

		switch reqBody.Operation {
		case "getLogs":
			apiResponse, err = getLogs(ctxDD, ddClient, reqBody)
		case "getMetrics":
			apiResponse, err = getMetrics(ctxDD, ddClient, reqBody)
		case "listMonitors":
			apiResponse, err = listMonitors(ctxDD, ddClient)
		case "listIncidents":
			apiResponse, err = listIncidents(ctxDD, ddClient)
		case "getEvents":
			apiResponse, err = getEvents(ctxDD, ddClient, reqBody)
		default:
			err = fmt.Errorf("unsupported operation: %s", reqBody.Operation)
		}

		if err != nil {
			log.Printf("Error calling Datadog API: %v", err)
			span.RecordError(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Error calling Datadog API: %v", err),
			})
		}

		// 6. Format the response for DatadogNode.vue
		response := DatadogNodeResponse{}
		response.Result.Output = apiResponse

		span.SetAttributes(attribute.String("datadog.operation", reqBody.Operation))

		return c.JSON(http.StatusOK, response)
	})

	e.POST("/api/download-llama", func(c echo.Context) error {
		cudaVersion := c.FormValue("cuda")
		osArch := c.FormValue("osarch")

		// Validate inputs
		if cudaVersion == "" || osArch == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Both 'cuda' and 'osarch' parameters are required.",
			})
		}

		if cudaVersion != "cu11" && cudaVersion != "cu12" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid 'cuda' parameter. Supported values are 'cu11' and 'cu12'.",
			})
		}

		validArchs := map[string]bool{
			"macos-arm64":         true,
			"ubuntu-x64":          true,
			"win-cuda-cu11.7-x64": true,
			"win-cuda-cu12.4-x64": true,
		}
		if !validArchs[osArch] {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid 'osarch' parameter. Supported values are 'macos-arm64', 'ubuntu-x64', 'win-cuda-cu11.7-x64', 'win-cuda-cu12.4-x64'.",
			})
		}

		// Get latest release info from GitHub API
		resp, err := http.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to fetch latest release info from GitHub.",
			})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("GitHub API request failed with status: %s", resp.Status),
			})
		}

		var release map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to decode GitHub API response.",
			})
		}

		assets, ok := release["assets"].([]interface{})
		if !ok {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to find assets in the release info.",
			})
		}

		// Determine download URLs based on inputs
		var cudartDownloadURL, llamaDownloadURL string
		var releaseVersion string

		// Extract release version from tag name (e.g., "b4604")
		if tag, ok := release["tag_name"].(string); ok {
			releaseVersion = strings.TrimPrefix(tag, "b")
		}

		for _, asset := range assets {
			assetMap, ok := asset.(map[string]interface{})
			if !ok {
				continue
			}
			name, ok := assetMap["name"].(string)
			if !ok {
				continue
			}
			downloadURL, ok := assetMap["browser_download_url"].(string)
			if !ok {
				continue
			}

			if strings.Contains(name, "cudart-llama-bin-win-"+cudaVersion) && strings.HasSuffix(name, ".zip") {
				cudartDownloadURL = downloadURL
			}

			if releaseVersion != "" && strings.Contains(name, "llama-b"+releaseVersion+"-bin-"+osArch) && strings.HasSuffix(name, ".zip") {
				llamaDownloadURL = downloadURL
			}

		}

		if cudartDownloadURL == "" || llamaDownloadURL == "" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Could not find download URLs for the specified 'cuda' and 'osarch'.",
			})
		}

		// Create a temporary directory for downloads
		tempDir, err := os.MkdirTemp("", "llama-downloads")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to create temporary directory.",
			})
		}
		defer os.RemoveAll(tempDir)

		// Download files
		cudartFilePath := filepath.Join(tempDir, "cudart.zip")
		llamaFilePath := filepath.Join(tempDir, "llama.zip")

		if err := downloadFile(cudartDownloadURL, cudartFilePath); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to download cudart: %v", err),
			})
		}

		if err := downloadFile(llamaDownloadURL, llamaFilePath); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to download llama: %v", err),
			})
		}

		// Return success response
		return c.JSON(http.StatusOK, map[string]string{
			"message":          "Successfully downloaded llama.cpp release files.",
			"cudart_file_path": cudartFilePath,
			"llama_file_path":  llamaFilePath,
		})
	})

	// Start the server in a separate goroutine
	go func() {
		port := fmt.Sprintf(":%d", config.Port)
		if err := e.Start(port); err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// Shutdown OpenTelemetry
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down tracer provider: %v", err)
	}

	// Shutdown Echo
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}

	log.Println("Server gracefully stopped")
}
