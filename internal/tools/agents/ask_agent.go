package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// AskAgentTool performs a synchronous HTTP call to the local /agent/run endpoint
// to invoke another agent (optionally a named specialist) and returns its output.
// This is a simple RPC-style delegator for Phase 1 (no Kafka/bus yet).
type AskAgentTool struct {
	httpClient *http.Client
	baseURL    string // e.g., http://127.0.0.1:32180
	// defaultTimeout is applied when the parent context has no deadline and
	// the caller did not specify timeout_ms. Intended to honor
	// AGENT_RUN_TIMEOUT_SECONDS for non-stream /agent/run.
	defaultTimeout time.Duration
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
		SessionID string        `json:"session_id"`
		ProjectID string        `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(args.SessionID)
	switch {
	case sessionID == "":
		sessionID = uuid.NewString()
	default:
		if _, err := uuid.Parse(sessionID); err != nil {
			// Deterministically map non-UUID identifiers to a UUID so repeated
			// values anchor to the same chat transcript.
			sessionID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionID)).String()
		}
	}

	body := map[string]any{"prompt": args.Prompt, "session_id": sessionID}
	if len(args.History) > 0 {
		body["history"] = args.History
	}
	if pid := strings.TrimSpace(args.ProjectID); pid != "" {
		body["project_id"] = pid
	}
	b, _ := json.Marshal(body)
	// Build endpoint URL; force non-stream JSON via stream=0; include specialist when provided
	u, _ := neturl.Parse(fmt.Sprintf("%s/agent/run", t.baseURL))
	q := u.Query()
	q.Set("stream", "0")
	if args.To != "" {
		q.Set("specialist", args.To)
	}
	u.RawQuery = q.Encode()

	// Determine effective context: prefer parent deadline; otherwise, use
	// tool default timeout to honor AGENT_RUN_TIMEOUT_SECONDS for non-stream.
	runCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && t.defaultTimeout > 0 && args.TimeoutMS <= 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, t.defaultTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(runCtx, http.MethodPost, u.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := t.httpClient
	// Apply explicit per-call timeout if provided. If not provided and no
	// parent deadline, apply the tool default.
	if args.TimeoutMS > 0 {
		c := *t.httpClient
		c.Timeout = time.Duration(args.TimeoutMS) * time.Millisecond
		client = &c
	} else if _, hasDeadline := ctx.Deadline(); !hasDeadline && t.defaultTimeout > 0 {
		c := *t.httpClient
		c.Timeout = t.defaultTimeout
		client = &c
	}
	// Observability: log effective timeout and whether parent had deadline
	{
		log := observability.LoggerWithTrace(ctx)
		eff := int(client.Timeout / time.Millisecond)
		_, has := ctx.Deadline()
		log.Debug().Int("args_timeout_ms", args.TimeoutMS).Int("effective_timeout_ms", eff).Bool("parent_has_deadline", has).Str("endpoint", u.String()).Msg("ask_agent_call")
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
	// If result is a JSON-encoded string, best-effort decode it so callers
	// receive structured results rather than double-encoded payloads.
	if raw, ok := payload["result"].(string); ok && strings.HasPrefix(strings.TrimSpace(raw), "{") {
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
			payload["result"] = decoded
		}
	}
	return map[string]any{"ok": true, "to": args.To, "response": payload}, nil
}
