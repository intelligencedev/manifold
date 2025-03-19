// completions.go

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest represents the payload for the completion API.
type CompletionRequest struct {
	Model            string    `json:"model,omitempty"`
	Messages         []Message `json:"messages"`
	Temperature      float64   `json:"temperature,omitempty"`
	TopP             float64   `json:"top_p,omitempty"`
	TopK             int       `json:"top_k,omitempty"`
	FrequencyPenalty float64   `json:"frequency_penalty,omitempty"`
	MaxTokens        int       `json:"max_tokens,omitempty"`
	Stream           bool      `json:"stream,omitempty"`
}

// Choice represents a choice for the completion response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Logprobs     *bool   `json:"logprobs"` // Pointer to a boolean or nil
	FinishReason string  `json:"finish_reason"`
}

// Usage contains information about token usage in the completion response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse represents the response from the completion API.
type CompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
}

// ErrorData represents the structure of an error response from the OpenAI API.
type ErrorData struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

// ErrorResponse wraps the structure of an error when an API request fails.
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

	// Create a new request to OpenAI
	// openAIURL := "https://api.openai.com/v1/chat/completions"
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
	req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)

	// Check if this is a streaming request
	var payload CompletionRequest
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorData{
				Message: "Invalid JSON in request: " + err.Error(),
			},
		})
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
