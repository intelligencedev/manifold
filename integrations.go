// manifold/integrations.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
