// llm.go

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// ErrorData mirrors the error structure returned by the OpenAI API.
type ErrorData struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse wraps an error returned by the OpenAI API.
type ErrorResponse struct {
	Error ErrorData `json:"error"`
}

func completionsHandler(c echo.Context, config *Config) error {
	// Set up the OpenAI API client
	client := &http.Client{}

	// Read the original request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorData{
				Message: "Error reading request body: " + err.Error(),
			},
		})
	}

	// Create a new request to the default completions endpoint in the config
	openAIURL := config.Completions.DefaultHost
	req, err := http.NewRequest("POST", openAIURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorData{
				Message: "Error creating proxy request: " + err.Error(),
			},
		})
	}

	// Copy headers from the original request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Completions.APIKey)

	// Check if this is a streaming request by parsing the raw JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorData{
				Message: "Invalid JSON in request: " + err.Error(),
			},
		})
	}

	// Only set the default model if the endpoint is OpenAI's API
	if strings.Contains(openAIURL, "api.openai.com") {
		// Set the model only if model is empty and we're using OpenAI
		if model, exists := payload["model"]; !exists || model == "" {
			payload["model"] = config.Completions.CompletionsModel

			// Re-marshal the body with the updated model
			updatedBody, err := json.Marshal(payload)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorData{
						Message: "Error updating request body: " + err.Error(),
					},
				})
			}

			// Update the request with the new body
			req, err = http.NewRequest("POST", openAIURL, bytes.NewBuffer(updatedBody))
			if err != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorData{
						Message: "Error creating updated request: " + err.Error(),
					},
				})
			}

			// Re-set the headers
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+config.Completions.APIKey)
		}
	}

	// Handle streaming differently than non-streaming
	isStream := false
	if stream, exists := payload["stream"]; exists {
		if streamBool, ok := stream.(bool); ok {
			isStream = streamBool
		}
	}

	if isStream {
		// Set response headers for streaming
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")

		// Make sure the writer supports flushing
		flusher, ok := c.Response().Writer.(http.Flusher)
		if !ok {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorData{
					Message: "Streaming not supported",
				},
			})
		}

		// Make the request to OpenAI
		resp, err := client.Do(req)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorData{
					Message: "Error forwarding request to OpenAI: " + err.Error(),
				},
			})
		}
		defer resp.Body.Close()

		// If OpenAI returned an error, pass it along
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			c.Response().WriteHeader(resp.StatusCode)
			c.Response().Write(bodyBytes)
			return nil
		}

		// Stream the response back to the client
		scanner := bufio.NewScanner(resp.Body)
		// Increase the buffer size to handle larger tokens
		const maxScanTokenSize = 1024 * 1024 // 1MB buffer
		scannerBuffer := make([]byte, maxScanTokenSize)
		scanner.Buffer(scannerBuffer, maxScanTokenSize)

		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				// Make sure each line is properly formatted as a server-sent event
				// The format should be: "data: {json}\n\n"
				if !strings.HasPrefix(line, "data: ") {
					line = "data: " + line
				}
				// Write the SSE line with double newlines
				_, err := c.Response().Write([]byte(line + "\n\n"))
				if err != nil {
					return err
				}
				flusher.Flush()
			}
		}

		if err := scanner.Err(); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorData{
					Message: "Error reading stream from OpenAI: " + err.Error(),
				},
			})
		}

		return nil
	} else {
		// Non-streaming request
		resp, err := client.Do(req)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorData{
					Message: "Error forwarding request to OpenAI: " + err.Error(),
				},
			})
		}
		defer resp.Body.Close()

		// Read the response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorData{
					Message: "Error reading response from OpenAI: " + err.Error(),
				},
			})
		}

		// Set response status code and headers
		c.Response().WriteHeader(resp.StatusCode)
		c.Response().Header().Set("Content-Type", resp.Header.Get("Content-Type"))

		// Write the response body
		c.Response().Write(respBody)
		return nil
	}
}
