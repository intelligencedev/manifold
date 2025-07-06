package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// CallLLM sends a chat completion request using the OpenAI Go SDK.
func CallLLM(ctx context.Context, endpoint, apiKey, model string, msgs []openai.ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	cfg := openai.DefaultConfig(apiKey)
	if endpoint != "" {
		cfg.BaseURL = endpoint
	}
	client := openai.NewClientWithConfig(cfg)

	req := openai.ChatCompletionRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: float32(temperature),
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// CallMLX sends a chat completion request specifically formatted for MLX backends.
// MLX backends require different parameter formatting - they omit the model field and include maxTokens directly.
func CallMLX(ctx context.Context, endpoint, apiKey string, msgs []openai.ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	// Use custom HTTP client for MLX backends as they may have different endpoint structures
	return callMLXWithHTTP(ctx, endpoint, apiKey, msgs, maxTokens, temperature)
}

// GetEndpointModels retrieves the available models using the OpenAI client.
func GetEndpointModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	cfg := openai.DefaultConfig(apiKey)
	if endpoint != "" {
		cfg.BaseURL = endpoint
	}
	client := openai.NewClientWithConfig(cfg)

	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(models.Models))
	for _, m := range models.Models {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// MLX-specific request/response structures
type mlxMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mlxRequest struct {
	Messages    []mlxMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float64      `json:"temperature"`
	// Model field is intentionally omitted for MLX backends
}

type mlxChoice struct {
	Index   int        `json:"index"`
	Message mlxMessage `json:"message"`
}

type mlxResponse struct {
	Choices []mlxChoice `json:"choices"`
}

// callMLXWithHTTP makes a raw HTTP request to MLX backends with proper parameter formatting
func callMLXWithHTTP(ctx context.Context, endpoint, apiKey string, msgs []openai.ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	// Convert OpenAI messages to MLX format
	mlxMessages := make([]mlxMessage, len(msgs))
	for i, msg := range msgs {
		mlxMessages[i] = mlxMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Create MLX request
	req := mlxRequest{
		Messages:    mlxMessages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("error marshaling MLX request: %w", err)
	}

	// Ensure the endpoint has the correct path for chat completions
	mlxEndpoint := endpoint
	if !strings.HasSuffix(endpoint, "/chat/completions") && !strings.HasSuffix(endpoint, "/v1/chat/completions") {
		// If endpoint doesn't already include the chat completions path, we need to construct it properly
		if strings.HasSuffix(endpoint, "/v1") {
			mlxEndpoint = endpoint + "/chat/completions"
		} else if strings.Contains(endpoint, "/v1/") {
			// Endpoint already has the full path
			mlxEndpoint = endpoint
		} else {
			// Assume it's a base URL and append the full path
			if strings.HasSuffix(endpoint, "/") {
				mlxEndpoint = endpoint + "v1/chat/completions"
			} else {
				mlxEndpoint = endpoint + "/v1/chat/completions"
			}
		}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", mlxEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("error creating HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	// Make request
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("error making MLX request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading MLX response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MLX API error: code: %d, status: %s, body: %s",
			resp.StatusCode, resp.Status, string(respBody))
	}

	// Parse response
	var mlxResp mlxResponse
	if err := json.Unmarshal(respBody, &mlxResp); err != nil {
		return "", fmt.Errorf("error parsing MLX response: %w", err)
	}

	// Extract content
	if len(mlxResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in MLX response")
	}

	return mlxResp.Choices[0].Message.Content, nil
}
