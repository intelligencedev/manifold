package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

	"manifold/internal/documents" // Import the documents package
	"manifold/internal/repoconcat"
	web "manifold/internal/web"
)

//go:embed frontend/dist
var frontendDist embed.FS

const (
	service     = "api-gateway"
	environment = "development"
	id          = 1
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
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	// Initialize OpenTelemetry
	tp, err := initTracer()
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
			Text     string `json:"text"`
			Splitter string `json:"splitter"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Text == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Text is required"})
		}

		// Default to chunk-based if splitter is not provided or invalid
		splitterType := documents.Language(req.Splitter)
		if splitterType == "" {
			splitterType = documents.DEFAULT
		}

		splitter, err := documents.FromLanguage(splitterType)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error creating splitter: " + err.Error()})
		}

		chunks := splitter.SplitText(req.Text)

		return c.JSON(http.StatusOK, map[string]interface{}{"chunks": chunks})
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

	// Define the /api/datadog proxy route
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
		if err := e.Start(":8080"); err != http.ErrServerClosed {
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
