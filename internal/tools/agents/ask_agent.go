package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"manifold/internal/llm"
)

// AskAgentTool performs a synchronous HTTP call to the local /agent/run endpoint
// to invoke another agent (optionally a named specialist) and returns its output.
// This is a simple RPC-style delegator for Phase 1 (no Kafka/bus yet).
type AskAgentTool struct {
	httpClient *http.Client
	baseURL    string // e.g., http://127.0.0.1:32180
}

func NewAskAgentTool(httpClient *http.Client, baseURL string) *AskAgentTool {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:32180"
	}
	return &AskAgentTool{httpClient: httpClient, baseURL: baseURL}
}

func (t *AskAgentTool) Name() string { return "ask_agent" }

func (t *AskAgentTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Synchronously ask another agent/specialist via the local HTTP API (/agent/run).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Optional specialist name to route to (query param ?specialist=).",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Prompt to send to the target agent.",
				},
				"history": map[string]any{
					"type":        "array",
					"description": "Optional conversation history as [{role, content}]",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"role":    map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"role", "content"},
					},
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in milliseconds for the HTTP call.",
				},
			},
			"required": []string{"prompt"},
		},
	}
}

func (t *AskAgentTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		To        string        `json:"to"`
		Prompt    string        `json:"prompt"`
		History   []llm.Message `json:"history"`
		TimeoutMS int           `json:"timeout_ms"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	body := map[string]any{"prompt": args.Prompt}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/agent/run", t.baseURL)
	if args.To != "" {
		url = url + "?specialist=" + args.To
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := t.httpClient
	if args.TimeoutMS > 0 {
		client = &http.Client{Timeout: time.Duration(args.TimeoutMS) * time.Millisecond}
	}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	if resp.StatusCode >= 400 {
		return map[string]any{"ok": false, "status": resp.StatusCode, "error": string(data)}, nil
	}
	// Expect {"result": "..."}
	return map[string]any{"ok": true, "to": args.To, "response": payload}, nil
}
