// completions.go

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	completions "manifold/internal/completions"
)

func completionsHandler(c echo.Context, config *Config) error {
	// Set up the OpenAI API client
	client := &http.Client{}

	// Read the original request body
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, completions.ErrorResponse{
			Error: completions.ErrorData{
				Message: "Error reading request body: " + err.Error(),
			},
		})
	}

	// Create a new request to the default completions endpoint in the config
	openAIURL := config.Completions.DefaultHost
	req, err := http.NewRequest("POST", openAIURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
			Error: completions.ErrorData{
				Message: "Error creating proxy request: " + err.Error(),
			},
		})
	}

	// Copy headers from the original request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Completions.APIKey)

	// Check if this is a streaming request
	var payload completions.CompletionRequest

	// Only set the default model if the endpoint is OpenAI's API
	if strings.Contains(openAIURL, "api.openai.com") {
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			return c.JSON(http.StatusBadRequest, completions.ErrorResponse{
				Error: completions.ErrorData{
					Message: "Invalid JSON in request: " + err.Error(),
				},
			})
		}

		// Set the model only if model is empty and we're using OpenAI
		if payload.Model == "" {
			payload.Model = config.Completions.CompletionsModel

			// Re-marshal the body with the updated model
			updatedBody, err := json.Marshal(payload)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
					Error: completions.ErrorData{
						Message: "Error updating request body: " + err.Error(),
					},
				})
			}

			// Update the request with the new body
			req, err = http.NewRequest("POST", openAIURL, bytes.NewBuffer(updatedBody))
			if err != nil {
				return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
					Error: completions.ErrorData{
						Message: "Error creating updated request: " + err.Error(),
					},
				})
			}

			// Re-set the headers
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+config.Completions.APIKey)
		}
	} else {
		// For non-OpenAI endpoints, just unmarshal to check if it's a streaming request
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			return c.JSON(http.StatusBadRequest, completions.ErrorResponse{
				Error: completions.ErrorData{
					Message: "Invalid JSON in request: " + err.Error(),
				},
			})
		}
	}

	// Handle streaming differently than non-streaming
	if payload.Stream {
		// Set response headers for streaming
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")

		// Make sure the writer supports flushing
		flusher, ok := c.Response().Writer.(http.Flusher)
		if !ok {
			return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
				Error: completions.ErrorData{
					Message: "Streaming not supported",
				},
			})
		}

		// Make the request to OpenAI
		resp, err := client.Do(req)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
				Error: completions.ErrorData{
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
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				// Write the SSE line
				_, err := c.Response().Write([]byte(line + "\n\n"))
				if err != nil {
					return err
				}
				flusher.Flush()
			}
		}

		if err := scanner.Err(); err != nil {
			return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
				Error: completions.ErrorData{
					Message: "Error reading stream from OpenAI: " + err.Error(),
				},
			})
		}

		return nil
	} else {
		// Non-streaming request
		resp, err := client.Do(req)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
				Error: completions.ErrorData{
					Message: "Error forwarding request to OpenAI: " + err.Error(),
				},
			})
		}
		defer resp.Body.Close()

		// Read the response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, completions.ErrorResponse{
				Error: completions.ErrorData{
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
