package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	cfg "manifold/internal/config"
)

// Request defines the expected payload for the /anthropic/messages endpoint.
type Request struct {
	Model     string    `json:"model"`
	MaxTokens int64     `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	System    []string  `json:"system,omitempty"`
}

// Message represents an individual message.
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// HandleMessages proxies requests to Anthropic's streaming API.
func HandleMessages(c echo.Context, config *cfg.Config) error {
	var req Request
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Model == "" || req.MaxTokens == 0 || len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "model, max_tokens, and messages are required"})
	}

	var messages []map[string]interface{}
	for _, msg := range req.Messages {
		role := strings.ToLower(msg.Role)
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": []map[string]interface{}{{"type": "text", "text": msg.Text}},
		})
	}

	var system string
	if len(req.System) > 0 {
		system = strings.Join(req.System, "\n")
	}

	requestBody := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   messages,
		"stream":     true,
	}
	if system != "" {
		requestBody["system"] = system
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to marshal request parameters"})
	}

	httpReq, err := http.NewRequestWithContext(c.Request().Context(), "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create HTTP request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", config.AnthropicKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to send request to Anthropic API: " + err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Anthropic API returned an error: %d %s", resp.StatusCode, string(body))})
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Streaming not supported"})
	}

	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := c.Response().Writer.Write(buf[:n]); wErr != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to write response: " + wErr.Error()})
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
