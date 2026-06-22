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
	"manifold/internal/observability"
	"manifold/internal/sandbox"
)

// AskAgentTool performs a synchronous HTTP call to the local /agent/run endpoint
// to invoke another agent (optionally a named specialist) and returns its output.
// This is a simple RPC-style delegator for local, in-process orchestration.
type AskAgentTool struct {
	httpClient *http.Client
	baseURL    string // e.g., http://127.0.0.1:32180
	// defaultTimeout is applied when the parent context has no deadline and
	// the caller did not specify timeout_ms. Intended to honor
	// AGENT_RUN_TIMEOUT_SECONDS for non-stream /agent/run.
	defaultTimeout time.Duration
}

type askAgentArgs struct {
	To          string        `json:"to"`
	Prompt      string        `json:"prompt"`
	History     []llm.Message `json:"history"`
	TimeoutMS   int           `json:"timeout_ms"`
	SessionID   string        `json:"session_id"`
	ProjectID   string        `json:"project_id"`
	ObjectiveID string        `json:"objective_id"`
	RoomID      string        `json:"room_id"`
}

// NewAskAgentTool constructs an AskAgentTool. If defaultTimeoutSeconds > 0,
// the tool will apply that as a per-request timeout when the provided context
// does not already carry a deadline and the call does not specify timeout_ms.
func NewAskAgentTool(httpClient *http.Client, baseURL string, defaultTimeoutSeconds int) *AskAgentTool {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:32180"
	}
	var def time.Duration
	if defaultTimeoutSeconds > 0 {
		def = time.Duration(defaultTimeoutSeconds) * time.Second
	}
	return &AskAgentTool{httpClient: httpClient, baseURL: baseURL, defaultTimeout: def}
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
				"session_id": map[string]any{
					"type":        "string",
					"description": "Optional chat session identifier. Auto-generated when omitted.",
				},
				"project_id": map[string]any{
					"type":        "string",
					"description": "Optional project ID to scope the remote agent's sandbox (passed through to /agent/run). This must be the project ID/UUID, not the display name.",
				},
				"room_id": map[string]any{
					"type":        "string",
					"description": "Optional Matrix room ID to preserve room-scoped tools like pulse_tasks.",
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
	var args askAgentArgs
	if len(raw) == 0 {
		return map[string]any{"ok": false, "error": "empty arguments: prompt is required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid arguments: %v", err)}, nil
	}

	scope := resolveDelegatedRunScope(ctx, args.SessionID, args.ProjectID, args.ObjectiveID, args.RoomID)
	endpoint := agentRunURL(t.baseURL, map[string]string{"specialist": args.To})
	runCtx, cancel := t.runContext(ctx, args)
	if cancel != nil {
		defer cancel()
	}

	body := delegatedRunBody(args.Prompt, args.History, scope)
	req, err := http.NewRequestWithContext(runCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setDelegatedHeaders(ctx, req)

	client := t.client(args, ctx)
	logAskAgentCall(ctx, client, args.TimeoutMS, endpoint)
	resp, err := client.Do(req)
	if err != nil {
		return delegatedRunErrorPayload(err), nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return delegatedRunStatusPayload(resp.StatusCode, string(data)), nil
	}
	payload := decodeRunPayload(data)
	return map[string]any{"ok": true, "to": args.To, "response": payload}, nil
}

func (t *AskAgentTool) runContext(ctx context.Context, args askAgentArgs) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && t.defaultTimeout > 0 && args.TimeoutMS <= 0 {
		return context.WithTimeout(ctx, t.defaultTimeout)
	}
	return ctx, nil
}

func setDelegatedHeaders(ctx context.Context, req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if cookie, ok := sandbox.AuthCookieFromContext(ctx); ok {
		req.Header.Set("Cookie", cookie)
	}
}

func (t *AskAgentTool) client(args askAgentArgs, ctx context.Context) *http.Client {
	if args.TimeoutMS > 0 {
		c := *t.httpClient
		c.Timeout = time.Duration(args.TimeoutMS) * time.Millisecond
		return &c
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && t.defaultTimeout > 0 {
		c := *t.httpClient
		c.Timeout = t.defaultTimeout
		return &c
	}
	return t.httpClient
}

func logAskAgentCall(ctx context.Context, client *http.Client, timeoutMS int, endpoint string) {
	log := observability.LoggerWithTrace(ctx)
	effective := int(client.Timeout / time.Millisecond)
	_, hasDeadline := ctx.Deadline()
	log.Debug().Int("args_timeout_ms", timeoutMS).Int("effective_timeout_ms", effective).Bool("parent_has_deadline", hasDeadline).Str("endpoint", endpoint).Msg("ask_agent_call")
}
