package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	// Import manifold/internal/web/web.go
)

func getFileSystem() http.FileSystem {
	fsys, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get file system: %v", err)
	}
	return http.FS(fsys)
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
