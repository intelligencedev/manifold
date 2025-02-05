package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	// Import manifold/internal/web/web.go
	web "manifold/internal/web"
)

const (
	service     = "api-gateway"
	environment = "development"
	id          = 1
)

// DatadogNodeRequest represents the structure of the incoming request from the frontend.
type DatadogNodeRequest struct {
	APIKey    string `json:"apiKey"`
	AppKey    string `json:"appKey"`
	Site      string `json:"site"`
	Operation string `json:"operation"`
	Query     string `json:"query"`
	FromTime  string `json:"fromTime"`
	ToTime    string `json:"toTime"`
}

// DatadogNodeResponse represents the structure of the response sent back to the frontend.
type DatadogNodeResponse struct {
	Result struct {
		Output interface{} `json:"output"`
	} `json:"result"`
}

func main() {
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

// PythonCodeRequest represents the structure of the incoming Python execution request.
type PythonCodeRequest struct {
	Code         string   `json:"code"`
	Dependencies []string `json:"dependencies"`
}

// PythonCodeResponse represents the structure of the response after executing Python code.
type PythonCodeResponse struct {
	ReturnCode int    `json:"return_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
}

// runEphemeralPython creates a temporary virtual environment,
// installs any requested dependencies, executes the user code,
// and returns stdout, stderr, and return_code.
func runEphemeralPython(code string, dependencies []string) (*PythonCodeResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	venvDir := filepath.Join(tempDir, "venv")

	// Determine python command
	pythonCmd := "python3"
	if _, err := exec.LookPath(pythonCmd); err != nil {
		// fallback to python if python3 is not found
		pythonCmd = "python"
	}

	// 1. Create the venv (with context)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonCmd, "-m", "venv", venvDir)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	log.Printf("Creating venv with: %s -m venv %s", pythonCmd, venvDir)
	if err := cmd.Run(); err != nil {
		log.Printf("Venv creation error:\nSTDOUT: %s\nSTDERR: %s", outBuf.String(), errBuf.String())
		return nil, fmt.Errorf("failed to create venv: %w", err)
	}
	log.Printf("Venv created successfully in %s", venvDir)

	// Set up path to pip/python inside venv
	pipPath := filepath.Join(venvDir, "bin", "pip")
	realPythonPath := filepath.Join(venvDir, "bin", "python")

	// Windows fix (if needed)
	if runtime.GOOS == "windows" {
		pipPath = filepath.Join(venvDir, "Scripts", "pip.exe")
		realPythonPath = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	// 2. Install dependencies
	for _, dep := range dependencies {
		ctxDep, cancelDep := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancelDep()

		cmdInstall := exec.CommandContext(ctxDep, pipPath, "install", dep)
		outBuf.Reset()
		errBuf.Reset()
		cmdInstall.Stdout = &outBuf
		cmdInstall.Stderr = &errBuf

		log.Printf("Installing dependency %s", dep)
		if err := cmdInstall.Run(); err != nil {
			log.Printf("Pip install error:\nSTDOUT: %s\nSTDERR: %s", outBuf.String(), errBuf.String())
			return nil, fmt.Errorf("failed to install %s: %w", dep, err)
		}
		log.Printf("Installed %s successfully.", dep)
	}

	// 3. Write code to user_code.py
	codeFilePath := filepath.Join(tempDir, "user_code.py")
	if err := os.WriteFile(codeFilePath, []byte(code), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write user code: %w", err)
	}

	// 4. Run the code
	ctxRun, cancelRun := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRun()

	cmdRun := exec.CommandContext(ctxRun, realPythonPath, codeFilePath)
	var runOutBuf, runErrBuf bytes.Buffer
	cmdRun.Stdout = &runOutBuf
	cmdRun.Stderr = &runErrBuf
	err = cmdRun.Run()

	response := &PythonCodeResponse{
		Stdout:     runOutBuf.String(),
		Stderr:     runErrBuf.String(),
		ReturnCode: 0,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			response.ReturnCode = exitErr.ExitCode()
		} else {
			// Some other error (context deadline, etc.)
			return response, fmt.Errorf("error running code: %w", err)
		}
	}

	return response, nil
}

// Helper functions to perform Datadog API operations

// getLogs fetches logs from Datadog based on the provided request parameters.
func getLogs(ctx context.Context, ddClient *datadog.APIClient, reqBody DatadogNodeRequest) (interface{}, error) {
	from, to, err := parseTimeframe(reqBody.FromTime, reqBody.ToTime)
	if err != nil {
		return nil, err
	}

	body := datadogV2.LogsListRequest{
		Filter: &datadogV2.LogsQueryFilter{
			Query: &reqBody.Query,
			From:  datadog.PtrString(from.Format(time.RFC3339)),
			To:    datadog.PtrString(to.Format(time.RFC3339)),
		},
		Page: &datadogV2.LogsListRequestPage{
			Limit: datadog.PtrInt32(100), // Default limit
		},
	}

	resp, r, err := datadogV2.NewLogsApi(ddClient).ListLogs(ctx, *datadogV2.NewListLogsOptionalParameters().WithBody(body))
	if err != nil {
		log.Printf("Error when calling `LogsApi.ListLogs`: %v\n", err)
		log.Printf("Full HTTP response: %v\n", r)
		return nil, err
	}

	// Deduplicate logs based on a combination of keys
	logs := resp.GetData()
	dedupedLogs := deduplicateLogs(logs)

	return dedupedLogs, nil
}

func deduplicateLogs(logs []datadogV2.Log) []datadogV2.Log {
	seen := make(map[string]bool)
	deduped := []datadogV2.Log{}

	for _, log := range logs {
		// Create a unique key based on relevant fields
		attributes, ok := log.GetAttributesOk()
		if !ok {
			continue // or handle the error appropriately
		}
		message, ok := attributes.GetMessageOk()
		if !ok {
			continue // or handle the error appropriately
		}
		host, ok := attributes.GetHostOk()
		if !ok {
			continue
		}
		service, ok := attributes.GetServiceOk()
		if !ok {
			continue
		}

		key := *message + *host + *service

		if _, ok := seen[key]; !ok {
			seen[key] = true
			deduped = append(deduped, log)
		}
	}

	return deduped
}

// getMetrics fetches metrics from Datadog based on the provided request parameters.
func getMetrics(ctx context.Context, ddClient *datadog.APIClient, reqBody DatadogNodeRequest) (interface{}, error) {
	from, to, err := parseTimeframe(reqBody.FromTime, reqBody.ToTime)
	if err != nil {
		return nil, err
	}

	// Convert time.Time to Unix timestamp in seconds for the query
	fromSec := from.Unix()
	toSec := to.Unix()

	// Use the v1 Metrics API to query timeseries data
	api := datadogV1.NewMetricsApi(ddClient)
	resp, r, err := api.QueryMetrics(ctx, fromSec, toSec, reqBody.Query)
	if err != nil {
		log.Printf("Error when calling `MetricsApi.QueryMetrics`: %v\n", err)
		log.Printf("Full HTTP response: %v\n", r)
		return nil, err
	}

	return resp, nil
}

// listMonitors fetches monitors from Datadog.
func listMonitors(ctx context.Context, ddClient *datadog.APIClient) (interface{}, error) {
	resp, r, err := datadogV1.NewMonitorsApi(ddClient).ListMonitors(ctx, *datadogV1.NewListMonitorsOptionalParameters())
	if err != nil {
		log.Printf("Error when calling `MonitorsApi.ListMonitors`: %v\n", err)
		log.Printf("Full HTTP response: %v\n", r)
		return nil, err
	}

	return resp, nil
}

// listIncidents fetches incidents from Datadog.
// Note: You need to implement this function based on Datadog's API for incidents.
func listIncidents(ctx context.Context, ddClient *datadog.APIClient) (interface{}, error) {
	// Example implementation (adjust based on actual API)
	resp, r, err := datadogV2.NewIncidentsApi(ddClient).ListIncidents(ctx, *datadogV2.NewListIncidentsOptionalParameters())
	if err != nil {
		log.Printf("Error when calling `IncidentsApi.ListIncidents`: %v\n", err)
		log.Printf("Full HTTP response: %v\n", r)
		return nil, err
	}

	return resp.GetData(), nil
}

// getEvents fetches events from Datadog based on the provided request parameters.
func getEvents(ctx context.Context, ddClient *datadog.APIClient, reqBody DatadogNodeRequest) (interface{}, error) {
	from, to, err := parseTimeframe(reqBody.FromTime, reqBody.ToTime)
	if err != nil {
		return nil, err
	}

	// Use the v2 Events API to query events
	optionalParams := *datadogV2.NewListEventsOptionalParameters()
	optionalParams = *optionalParams.WithFilterFrom(from.Format(time.RFC3339))
	optionalParams = *optionalParams.WithFilterTo(to.Format(time.RFC3339))
	if reqBody.Query != "" {
		optionalParams = *optionalParams.WithFilterQuery(reqBody.Query)
	}
	optionalParams = *optionalParams.WithPageLimit(100)

	resp, r, err := datadogV2.NewEventsApi(ddClient).ListEvents(ctx, optionalParams)
	if err != nil {
		log.Printf("Error when calling `EventsApi.ListEvents`: %v\n", err)
		log.Printf("Full HTTP response: %v\n", r)
		return nil, err
	}

	return resp.GetData(), nil
}

// parseTimeframe parses the 'fromTime' and 'toTime' strings into time.Time objects.
// Supports both relative (e.g., "now-15m") and absolute (ISO 8601) formats.
func parseTimeframe(fromStr, toStr string) (time.Time, time.Time, error) {
	now := time.Now()

	// Parse 'fromTime'
	var from time.Time
	var err error
	if strings.HasPrefix(fromStr, "now") {
		from, err = parseRelativeTime(fromStr, now)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'fromTime' timestamp: %v", err)
		}
	} else if fromStr != "" {
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'fromTime' timestamp: %v", err)
		}
	} else {
		// Default 'fromTime' if not provided
		from = now.Add(-15 * time.Minute)
	}

	// Parse 'toTime'
	var to time.Time
	if strings.HasPrefix(toStr, "now") {
		to, err = parseRelativeTime(toStr, now)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'toTime' timestamp: %v", err)
		}
	} else if toStr != "" {
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'toTime' timestamp: %v", err)
		}
	} else {
		// Default 'toTime' if not provided
		to = now
	}

	return from, to, nil
}

// parseRelativeTime parses relative time strings like "now-15m", "now-1h", etc.
func parseRelativeTime(relativeStr string, now time.Time) (time.Time, error) {
	if relativeStr == "now" {
		return now, nil
	}
	if !strings.HasPrefix(relativeStr, "now-") {
		return time.Time{}, fmt.Errorf("invalid relative format: %s", relativeStr)
	}

	durationStr := relativeStr[4:]
	// Extract the numeric value and the unit
	var value int
	var unit rune
	_, err := fmt.Sscanf(durationStr, "%d%c", &value, &unit)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration: %s", durationStr)
	}

	var duration time.Duration
	switch unit {
	case 'm':
		duration = time.Duration(value) * time.Minute
	case 'h':
		duration = time.Duration(value) * time.Hour
	case 'd':
		duration = time.Duration(value) * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("invalid time unit: %c", unit)
	}

	return now.Add(-duration), nil
}

// initTracer initializes the OpenTelemetry tracer.
func initTracer() (*sdktrace.TracerProvider, error) {
	// Load the trace endpoint from the environment variable
	traceEndpoint := strings.TrimSpace(os.Getenv("JAEGER_ENDPOINT"))
	if traceEndpoint == "" {
		return nil, fmt.Errorf("JAEGER_ENDPOINT environment variable not set")
	}

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(traceEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create a resource
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(service),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", environment),
			attribute.Int64("ID", id),
		),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create a tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set the global tracer provider and propagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

// Helper function to download a file
func downloadFile(url, filepath string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
