// internal/llm/completions.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// choices: A list of outputs. Each output is a dictionary containing the fields:

// index: The index in the list.
// logprobs: A dictionary containing the fields:
// token_logprobs: A list of the log probabilities for the generated tokens.
// tokens: A list of the generated token ids.
// top_logprobs: A list of lists. Each list contains the logprobs top tokens (if requested) with their corresponding probabilities.

type Logprobs struct {
	TokenLogprobs []float64            `json:"token_logprobs,omitempty"`
	Tokens        []int                `json:"tokens,omitempty"`
	TopLogprobs   []map[string]float64 `json:"top_logprobs,omitempty"`
}

// Message represents a message in a conversation.
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
	Index        int       `json:"index"`
	Message      Message   `json:"message"`
	Logprobs     *Logprobs `json:"logprobs,omitempty"`
	FinishReason string    `json:"finish_reason"`
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

// CallLLM makes a call to a language model API using the specified model and messages.
// It returns the content of the first choice in the response.
func CallLLM(ctx context.Context, endpoint, apiKey, model string, msgs []Message, maxTokens int, temperature float64) (string, error) {
	client := &http.Client{}

	body, err := json.Marshal(CompletionRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	})
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return "", fmt.Errorf("error parsing error response: %w (status: %d)", err, resp.StatusCode)
		}
		return "", fmt.Errorf("API error: %s", errResp.Error.Message)
	}

	var completion CompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return "", fmt.Errorf("error parsing completion response: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no choices in completion response")
	}

	return completion.Choices[0].Message.Content, nil
}

// GetEndpointModels returns a list of available models from the API endpoint.
func GetEndpointModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("error parsing error response: %w (status: %d)", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
	}

	var models []string
	if err := json.Unmarshal(respBody, &models); err != nil {
		return nil, fmt.Errorf("error parsing models response: %w", err)
	}

	return models, nil
}
