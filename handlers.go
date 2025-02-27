package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	// Import manifold/internal/web/web.go
)

func configHandler(c echo.Context) error {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}
	return c.JSON(http.StatusOK, config)
}

func getFileSystem() http.FileSystem {
	fsys, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get file system: %v", err)
	}
	return http.FS(fsys)
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

// initTracer initializes the OpenTelemetry tracer.
func initTracer(config *Config) (*sdktrace.TracerProvider, error) {
	// Load the trace endpoint from the environment variable
	traceEndpoint := config.JaegerHost
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
