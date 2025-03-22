package assistant

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// Message represents a message in a completion request.
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

// Assistant defines the methods that an assistant must implement.
type Assistant interface {
	// Completion sends a completion request to the assistant and returns the response.
	Completion(req *CompletionRequest) (*CompletionResponse, error)
	// RegisterTool registers a tool with the assistant.
	RegisterTool(name string, tool Tool)
}

// AssistantClient is a client for the assistant API and includes a registry for tools.
type AssistantClient struct {
	Endpoint string
	tools    map[string]Tool
}

// NewAssistantClient creates a new AssistantClient with the given endpoint and initializes the tool registry.
func NewAssistantClient(endpoint string) *AssistantClient {
	return &AssistantClient{
		Endpoint: endpoint,
		tools:    make(map[string]Tool),
	}
}

// RegisterTool registers a tool with the assistant by storing it in the internal registry.
func (c *AssistantClient) RegisterTool(name string, tool Tool) {
	c.tools[name] = tool
}

// ExecuteTool is an optional helper that looks up a registered tool by name and executes it using the provided JSON arguments.
// The tool must implement the Tool interface (defined in tool.go).
func (c *AssistantClient) ExecuteTool(name, jsonArgs string) (string, error) {
	tool, exists := c.tools[name]
	if !exists {
		return "", fmt.Errorf("tool %q is not registered", name)
	}
	// Create a simple ToolRequest.
	req := &ToolRequest{
		Command: name,
		Args:    []string{jsonArgs},
	}
	resp, err := tool.Execute(req)
	if err != nil {
		return "", err
	}
	return resp.Output, nil
}

// Completion sends a completion request to the assistant and returns the response.
func (c *AssistantClient) Completion(req *CompletionRequest) (*CompletionResponse, error) {
	// Marshal the request into JSON.
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create a new HTTP POST request.
	httpReq, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the HTTP request.
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check that the response status is OK.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read error response: %w", err)
		}
		return nil, fmt.Errorf("API returned non-OK status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the entire response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log the raw response for debugging
	log.Printf("Raw API response: %s", string(bodyBytes))

	// Decode the response.
	var completionResp CompletionResponse
	if err := json.Unmarshal(bodyBytes, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &completionResp, nil
}

// CompletionStream sends a completion request that returns a streamed response.
// It writes each line from the remote API to the provided writer in SSE format.
func (c *AssistantClient) CompletionStream(req *CompletionRequest, w io.Writer) error {
	// Marshal the request into JSON.
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create a new HTTP POST request.
	httpReq, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the HTTP request.
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check that the response status is OK.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-OK status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Ensure the writer supports flushing.
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Start scanning the response body.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// Write each line prefixed with "data:" and flush immediately.
		if len(line) > 0 {
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("error reading stream: %v", err)
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}
