package main

// Add these imports near the top of your main.go file.
import (
	"net/http"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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
func handleAnthropicMessages(c echo.Context) error {
	// Parse the incoming JSON request.
	var req AnthropicRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	// Ensure required fields are present.
	if req.Model == "" || req.MaxTokens == 0 || len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "model, max_tokens, and messages are required"})
	}

	// Map the incoming messages to the Anthropic SDK message parameters.
	var messages []anthropic.MessageParam
	for _, msg := range req.Messages {
		// For simplicity, we treat any role other than "system" as a user message.
		// (You could expand this if Anthropic provides separate constructors for assistant messages.)
		if strings.ToLower(msg.Role) == "user" || strings.ToLower(msg.Role) == "assistant" {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		} else {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		}
	}

	// Process any system prompt messages.
	var systemBlocks []anthropic.TextBlockParam
	for _, s := range req.System {
		systemBlocks = append(systemBlocks, anthropic.NewTextBlock(s))
	}

	// Build the Anthropic API request parameters.
	params := anthropic.MessageNewParams{
		Model:     anthropic.F(req.Model),
		MaxTokens: anthropic.F(req.MaxTokens),
		Messages:  anthropic.F(messages),
	}
	if len(systemBlocks) > 0 {
		params.System = anthropic.F(systemBlocks)
	}

	// Create the Anthropic client (using an API key from the ANTHROPIC_API_KEY env var).
	client := anthropic.NewClient(option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))
	ctx := c.Request().Context()

	// Initiate the streaming call.
	stream := client.Messages.NewStreaming(ctx, params)

	// Set headers for a streaming (chunked) response.
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	// Ensure that the writer supports flushing.
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Streaming not supported"})
	}

	// Stream the output from Anthropic back to the client.
	for stream.Next() {
		event := stream.Current()
		// Check for delta content (the Anthropic SDK will send events with text deltas).
		switch delta := event.Delta.(type) {
		case anthropic.ContentBlockDeltaEventDelta:
			if delta.Text != "" {
				if _, err := c.Response().Writer.Write([]byte(delta.Text)); err != nil {
					return err
				}
				flusher.Flush()
			}
		}
	}

	// If any error occurred during streaming, return it.
	if err := stream.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return nil
}
