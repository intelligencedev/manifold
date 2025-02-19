// manifold/main.go

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
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Configuration loaded: %+v", config)

	// Initialize application (create data directory, etc.).
	if err := InitializeApplication(config); err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Create Echo instance with middleware.
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-File-Path"},
	}))

	tp, err := initTracer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	e.Use(otelecho.Middleware(service, otelecho.WithTracerProvider(tp)))

	// Register all routes.
	registerRoutes(e, config)

	// Start server.
	go func() {
		port := fmt.Sprintf(":%d", config.Port)
		if err := e.Start(port); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}
	log.Println("Server gracefully stopped")
}

func registerRoutes(e *echo.Echo, config *Config) {
	// Serve static frontend files.
	e.GET("/*", echo.WrapHandler(http.FileServer(getFileSystem())))

	api := e.Group("/api")
	api.GET("/config", configHandler)

	// SEFII endpoints.
	sefiiGroup := api.Group("/sefii")
	sefiiGroup.POST("/ingest", sefiiIngestHandler(config))
	sefiiGroup.POST("/search", sefiiSearchHandler(config))
	sefiiGroup.POST("/combined-retrieve", sefiiCombinedRetrieveHandler(config))

	// Git-related endpoints.
	api.GET("/git-files", gitFilesHandler)
	api.POST("/git-files/ingest", gitFilesIngestHandler(config))

	api.POST("/run-fmlx", runFMLXHandler)
	e.GET("/mlx_out.png", imageHandler)
	api.POST("/run-sd", runSDHandler)

	api.POST("/repoconcat", repoconcatHandler)
	api.POST("/split-text", splitTextHandler)
	api.POST("/save-file", saveFileHandler)
	api.POST("/open-file", openFileHandler)

	api.GET("/web-content", webContentHandler)
	api.GET("/web-search", webSearchHandler)
	api.POST("/executePython", executePythonHandler)
	api.POST("/datadog", datadogHandler)
	api.POST("/download-llama", downloadLlamaHandler)
}

func configHandler(c echo.Context) error {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}
	return c.JSON(http.StatusOK, config)
}

func sefiiIngestHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
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

		ctx := c.Request().Context()
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)
		engine := sefii.NewEngine(conn)
		summary, err := summarizeContent(ctx, req.Text, config.Completions.DefaultHost, config.Completions.APIKey)
		if err != nil {
			summary = ""
		}
		finalContent := req.Text
		if summary != "" {
			finalContent = fmt.Sprintf("%s\n\n---\n\n%s", summary, req.Text)
		}
		if err := engine.IngestDocument(ctx, finalContent, req.Language, req.FilePath, config.Embeddings.Host, config.Embeddings.APIKey, req.ChunkSize, req.ChunkOverlap); err != nil {
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
			MergeMode        string `json:"merge_mode"`
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
		conn, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to connect to database"})
		}
		defer conn.Close(ctx)
		engine := sefii.NewEngine(conn)
		chunks, err := engine.SearchRelevantChunks(ctx, req.Query, req.FilePathFilter, req.Limit, req.UseInvertedIndex, req.UseVectorSearch, config.Embeddings.Host, config.Embeddings.APIKey, req.MergeMode)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
			finalContent := f.Content
			if summary != "" {
				finalContent = fmt.Sprintf("%s\n\n%s\n\n---\n\n%s", f.Path, summary, f.Content)
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

func runFMLXHandler(c echo.Context) error {
	var req FMLXRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Model == "" || req.Prompt == "" || req.Steps == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
	}
	if _, err := os.Stat(req.Output); err == nil {
		if err := os.Remove(req.Output); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to remove existing file"})
		}
	}
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stdout pipe"})
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stderr pipe"})
	}
	if err := cmd.Start(); err != nil {
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
	if err := cmd.Wait(); err != nil {
		wg.Wait()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to execute mflux command: %v", err)})
	}
	wg.Wait()
	return c.JSON(http.StatusOK, map[string]string{"message": "FMLX command executed successfully"})
}

func imageHandler(c echo.Context) error {
	file, err := os.Open(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.String(http.StatusNotFound, "Image not found")
		}
		return c.String(http.StatusInternalServerError, "Error opening image")
	}
	defer file.Close()
	c.Response().Header().Set(echo.HeaderContentType, "image/png")
	if _, err := io.Copy(c.Response().Writer, file); err != nil {
		log.Printf("Error copying image to response: %v", err)
	}
	return nil
}

func runSDHandler(c echo.Context) error {
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
	for _, v := range req.SkipLayers {
		args = append(args, "--skip-layers", fmt.Sprintf("%d", v))
	}
	if req.SkipLayerStart > 0 {
		args = append(args, "--skip-layer-start", fmt.Sprintf("%.3f", req.SkipLayerStart))
	}
	if req.SkipLayerEnd > 0 {
		args = append(args, "--skip-layer-end", fmt.Sprintf("%.3f", req.SkipLayerEnd))
	}
	cmd := exec.Command("./sd", args...)
	cmd.Dir = "/Users/art/Downloads/sd-master--bin-Darwin-macOS-14.7.2-arm64/stable-diffusion.cpp/build/bin"
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stdout pipe"})
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create stderr pipe"})
	}
	if err := cmd.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start sd command"})
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Printf("[sd stdout] %s", scanner.Text())
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[sd stderr] %s", scanner.Text())
		}
	}()
	if err := cmd.Wait(); err != nil {
		wg.Wait()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to execute sd command: %v", err)})
	}
	wg.Wait()
	return c.JSON(http.StatusOK, map[string]string{"message": "Stable Diffusion command executed successfully"})
}

func repoconcatHandler(c echo.Context) error {
	var req RepoConcatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if len(req.Paths) == 0 || len(req.Types) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Paths and types are required"})
	}
	rc := repoconcat.NewRepoConcat()
	result, err := rc.Concatenate(req.Paths, req.Types, req.Recursive, req.IgnorePattern)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.String(http.StatusOK, result)
}

func splitTextHandler(c echo.Context) error {
	var req struct {
		Text      string `json:"text"`
		Splitter  string `json:"splitter"`
		ChunkSize int    `json:"chunk_size,omitempty"`
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
	if splitterType == documents.DEFAULT && req.ChunkSize > 0 {
		splitter.ChunkSize = req.ChunkSize
	}
	chunks := splitter.SplitText(req.Text)
	return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
}

func saveFileHandler(c echo.Context) error {
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
	dir := filepath.Dir(req.Filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to create directory '%s': %v", dir, err)})
	}
	if err := os.WriteFile(req.Filepath, []byte(req.Content), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to save file '%s': %v", req.Filepath, err)})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "File saved successfully"})
}

func openFileHandler(c echo.Context) error {
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to read file '%s': %v", req.Filepath, err)})
	}
	return c.String(http.StatusOK, string(content))
}

func webContentHandler(c echo.Context) error {
	urlsParam := c.QueryParam("urls")
	if urlsParam == "" {
		return c.String(http.StatusBadRequest, "URLs are required")
	}
	urls := strings.Split(urlsParam, ",")
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]interface{})
	done := make(chan bool)
	go func() {
		for _, pageURL := range urls {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				content, err := web.WebGetHandler(url)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					results[url] = map[string]string{"error": fmt.Sprintf("Error extracting web content: %v", err)}
				} else {
					results[url] = content
				}
			}(pageURL)
		}
		wg.Wait()
		done <- true
	}()
	select {
	case <-done:
		return c.JSON(http.StatusOK, results)
	case <-time.After(60 * time.Second):
		return c.JSON(http.StatusOK, results)
	}
}

func webSearchHandler(c echo.Context) error {
	query := c.QueryParam("query")
	if query == "" {
		return c.String(http.StatusBadRequest, "Query is required")
	}
	resultSize := 3
	if size := c.QueryParam("result_size"); size != "" {
		if parsedSize, err := strconv.Atoi(size); err == nil {
			resultSize = parsedSize
		}
	}
	searchBackend := c.QueryParam("search_backend")
	if searchBackend == "" {
		searchBackend = "ddg"
	}
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
	if len(results) > resultSize {
		results = results[:resultSize]
	}
	return c.JSON(http.StatusOK, results)
}

func executePythonHandler(c echo.Context) error {
	var req PythonCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	result, err := runEphemeralPython(req.Code, req.Dependencies)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, result)
}

func datadogHandler(c echo.Context) error {
	span := trace.SpanFromContext(c.Request().Context())
	defer span.End()

	var reqBody DatadogNodeRequest
	if err := c.Bind(&reqBody); err != nil {
		span.RecordError(err)
		return c.String(http.StatusBadRequest, "Invalid request body")
	}
	if reqBody.APIKey == "" || reqBody.AppKey == "" {
		return c.String(http.StatusBadRequest, "Missing API key or Application key")
	}

	ctxDD := context.WithValue(c.Request().Context(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: reqBody.APIKey},
		"appKeyAuth": {Key: reqBody.AppKey},
	})
	configDD := datadog.NewConfiguration()
	configDD.SetUnstableOperationEnabled("v2.ListLogs", true)
	configDD.SetUnstableOperationEnabled("v2.QueryTimeseriesData", true)
	configDD.SetUnstableOperationEnabled("v2.ListIncidents", true)
	if reqBody.Site != "" {
		configDD.Servers = datadog.ServerConfigurations{{URL: "https://api." + reqBody.Site}}
	}
	ddClient := datadog.NewAPIClient(configDD)

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
		span.RecordError(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Error calling Datadog API: %v", err)})
	}

	response := DatadogNodeResponse{}
	response.Result.Output = apiResponse
	span.SetAttributes(attribute.String("datadog.operation", reqBody.Operation))
	return c.JSON(http.StatusOK, response)
}

func downloadLlamaHandler(c echo.Context) error {
	cudaVersion := c.FormValue("cuda")
	osArch := c.FormValue("osarch")
	if cudaVersion == "" || osArch == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Both 'cuda' and 'osarch' parameters are required."})
	}
	if cudaVersion != "cu11" && cudaVersion != "cu12" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid 'cuda' parameter. Supported values are 'cu11' and 'cu12'."})
	}
	validArchs := map[string]bool{
		"macos-arm64":         true,
		"ubuntu-x64":          true,
		"win-cuda-cu11.7-x64": true,
		"win-cuda-cu12.4-x64": true,
	}
	if !validArchs[osArch] {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid 'osarch' parameter. Supported values are 'macos-arm64', 'ubuntu-x64', 'win-cuda-cu11.7-x64', 'win-cuda-cu12.4-x64'."})
	}
	resp, err := http.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch latest release info from GitHub."})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("GitHub API request failed with status: %s", resp.Status)})
	}
	var release map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to decode GitHub API response."})
	}
	assets, ok := release["assets"].([]interface{})
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to find assets in the release info."})
	}
	var cudartDownloadURL, llamaDownloadURL string
	var releaseVersion string
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
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Could not find download URLs for the specified 'cuda' and 'osarch'."})
	}
	tempDir, err := os.MkdirTemp("", "llama-downloads")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create temporary directory."})
	}
	defer os.RemoveAll(tempDir)
	cudartFilePath := filepath.Join(tempDir, "cudart.zip")
	llamaFilePath := filepath.Join(tempDir, "llama.zip")
	if err := downloadFile(cudartDownloadURL, cudartFilePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to download cudart: %v", err)})
	}
	if err := downloadFile(llamaDownloadURL, llamaFilePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to download llama: %v", err)})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"message":          "Successfully downloaded llama.cpp release files.",
		"cudart_file_path": cudartFilePath,
		"llama_file_path":  llamaFilePath,
	})
}

// func getFileSystem() http.FileSystem {
// 	fs, err := fs.Sub(frontendDist, "frontend/dist")
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	return http.FS(fs)
// }
