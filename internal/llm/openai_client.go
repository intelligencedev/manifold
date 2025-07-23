package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// ChatCompletionMessage represents a message compatible with the old API
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// isThinkingModel returns true if the model matches the "o<int>-*" pattern (e.g., o4-mini, o1-pro)
func isThinkingModel(model string) bool {
	model = strings.ToLower(model)
	if strings.HasPrefix(model, "o") {
		// Check for o<int>-* pattern
		rest := model[1:]
		i := 0
		for ; i < len(rest) && rest[i] >= '0' && rest[i] <= '9'; i++ {
		}
		if i > 0 && i < len(rest) && rest[i] == '-' {
			return true
		}
	}
	return false
}

// CallLLM sends a chat completion request using the OpenAI Go SDK.
func CallLLM(ctx context.Context, endpoint, apiKey, model string, msgs []ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	// Initialize client with API key and optional base URL
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if endpoint != "" {
		opts = append(opts, option.WithBaseURL(endpoint))
	}
	client := openai.NewClient(opts...)

	// Convert old SDK messages to new SDK params
	var newMsgs []openai.ChatCompletionMessageParamUnion
	for _, m := range msgs {
		switch strings.ToLower(m.Role) {
		case "system":
			newMsgs = append(newMsgs, openai.SystemMessage(m.Content))
		case "user":
			newMsgs = append(newMsgs, openai.UserMessage(m.Content))
		case "assistant":
			newMsgs = append(newMsgs, openai.AssistantMessage(m.Content))
		default:
			newMsgs = append(newMsgs, openai.UserMessage(m.Content))
		}
	}

	// Prepare parameters
	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(model),
		Messages:    newMsgs,
		Temperature: param.NewOpt(temperature),
	}
	if isThinkingModel(model) {
		params.MaxCompletionTokens = param.NewOpt(int64(maxTokens))
	} else {
		params.MaxTokens = param.NewOpt(int64(maxTokens))
	}

	// Call the ChatCompletion endpoint
	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

// CallMLX sends a chat completion request specifically formatted for MLX backends.
func CallMLX(ctx context.Context, endpoint, apiKey string, msgs []ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	// Use custom HTTP client for MLX backends as they may have different endpoint structures
	return callMLXWithHTTP(ctx, endpoint, apiKey, msgs, maxTokens, temperature)
}

// GetEndpointModels retrieves the available models using the OpenAI client.
func GetEndpointModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if endpoint != "" {
		opts = append(opts, option.WithBaseURL(endpoint))
	}
	client := openai.NewClient(opts...)

	models, err := client.Models.List(ctx)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, m := range models.Data {
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
func callMLXWithHTTP(ctx context.Context, endpoint, apiKey string, msgs []ChatCompletionMessage, maxTokens int, temperature float64) (string, error) {
	// Convert ChatCompletionMessage to MLX format
	mlxMessages := make([]mlxMessage, len(msgs))
	for i, m := range msgs {
		mlxMessages[i] = mlxMessage{
			Role:    m.Role,
			Content: m.Content,
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
