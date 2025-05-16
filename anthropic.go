package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// AnthropicRequest defines the expected payload for our /v1/anthropic/messages endpoint.
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int64              `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    []string           `json:"system,omitempty"`
}

// AnthropicMessage represents an individual message.
type AnthropicMessage struct {
	Role string `json:"role"` // e.g., "user", "assistant"
	Text string `json:"text"`
}

// handleAnthropicMessages handles incoming requests, proxies them to Anthropic's streaming API,
// and writes out the response as a stream.
func handleAnthropicMessages(c echo.Context, config *Config) error {
	// Parse the incoming JSON request.
	var req AnthropicRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	// Ensure required fields are present.
	if req.Model == "" || req.MaxTokens == 0 || len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "model, max_tokens, and messages are required"})
	}

	// Map the incoming messages to the Anthropic API format with proper content blocks
	var messages []map[string]interface{}
	for _, msg := range req.Messages {
		role := strings.ToLower(msg.Role)
		if role == "user" || role == "assistant" {
			messages = append(messages, map[string]interface{}{
				"role": role,
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": msg.Text,
					},
				},
			})
		} else {
			// Default to user for any other role
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": msg.Text,
					},
				},
			})
		}
	}

	// Process any system prompt messages.
	var system string
	if len(req.System) > 0 {
		system = strings.Join(req.System, "\n")
	}

	// Build the Anthropic API request parameters.
	requestBody := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
		"stream":     true,
	}
	if system != "" {
		requestBody["system"] = system
	}

	// Marshal the request parameters as JSON
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to marshal request parameters"})
	}

	// Create a new HTTP request to the Anthropic API directly
	httpReq, err := http.NewRequestWithContext(
		c.Request().Context(),
		"POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create HTTP request"})
	}

	// Set the necessary headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", config.AnthropicKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Send the request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to send request to Anthropic API: " + err.Error()})
	}
	defer resp.Body.Close()

	// Check for error responses
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Anthropic API returned an error: %d %s", resp.StatusCode, string(body)),
		})
	}

	// Set headers for a streaming (chunked) response.
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	// Ensure that the writer supports flushing.
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Streaming not supported"})
	}

	// Stream the response directly to the client
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, err := c.Response().Writer.Write(buf[:n]); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write response: " + err.Error()})
			}
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error reading from Anthropic API: " + err.Error()})
			}
			break
		}
	}

	return nil
}
