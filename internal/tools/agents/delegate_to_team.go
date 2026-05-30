package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// DelegateToTeamTool performs a synchronous HTTP call to the local /agent/run endpoint
// to invoke a team's orchestrator (via ?group=) and returns its output.
type DelegateToTeamTool struct {
	httpClient *http.Client
	baseURL    string // e.g., http://127.0.0.1:32180
	// defaultTimeout is applied when the parent context has no deadline and
	// the caller did not specify timeout_ms.
	defaultTimeout time.Duration
}

type delegateToTeamArgs struct {
	Team        string        `json:"team"`
	Prompt      string        `json:"prompt"`
	History     []llm.Message `json:"history"`
	TimeoutMS   int           `json:"timeout_ms"`
	SessionID   string        `json:"session_id"`
	ProjectID   string        `json:"project_id"`
	ObjectiveID string        `json:"objective_id"`
	RoomID      string        `json:"room_id"`
}

// NewDelegateToTeamTool constructs a DelegateToTeamTool. If defaultTimeoutSeconds > 0,
// the tool will apply that as a per-request timeout when the provided context
// does not already carry a deadline and the call does not specify timeout_ms.
func NewDelegateToTeamTool(httpClient *http.Client, baseURL string, defaultTimeoutSeconds int) *DelegateToTeamTool {
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
	return &DelegateToTeamTool{httpClient: httpClient, baseURL: baseURL, defaultTimeout: def}
}

func (t *DelegateToTeamTool) Name() string { return "delegate_to_team" }

func (t *DelegateToTeamTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Delegate a task to a team's orchestrator and wait for the response.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team": map[string]any{
					"type":        "string",
					"description": "Team name to route to (invokes the team's orchestrator).",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Prompt to send to the team's orchestrator.",
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
			"required": []string{"team", "prompt"},
		},
	}
}

func (t *DelegateToTeamTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args delegateToTeamArgs
	if len(raw) == 0 {
		return map[string]any{"ok": false, "error": "empty arguments: team and prompt are required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid arguments: %v", err)}, nil
	}
	if strings.TrimSpace(args.Team) == "" {
		return map[string]any{"ok": false, "error": "team is required"}, nil
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return map[string]any{"ok": false, "error": "prompt is required"}, nil
	}

	scope := resolveDelegatedRunScope(ctx, args.SessionID, args.ProjectID, args.ObjectiveID, args.RoomID)
	endpoint := agentRunURL(t.baseURL, map[string]string{"group": strings.TrimSpace(args.Team)})
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

	client := t.client(args)
	logDelegateToTeamCall(ctx, client, args.TimeoutMS, endpoint)
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
	payload = decodeRunPayload(data)
	return map[string]any{"ok": true, "team": args.Team, "response": payload}, nil
}

func (t *DelegateToTeamTool) runContext(ctx context.Context, args delegateToTeamArgs) (context.Context, context.CancelFunc) {
	if args.TimeoutMS > 0 {
		return context.WithTimeout(ctx, time.Duration(args.TimeoutMS)*time.Millisecond)
	}
	if t.defaultTimeout > 0 {
		return context.WithTimeout(ctx, t.defaultTimeout)
	}
	return ctx, nil
}

func (t *DelegateToTeamTool) client(args delegateToTeamArgs) *http.Client {
	c := *t.httpClient
	client := &c
	if args.TimeoutMS > 0 {
		client.Timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	} else if t.defaultTimeout > 0 {
		client.Timeout = t.defaultTimeout
	} else {
		client.Timeout = 0
	}
	return client
}

func logDelegateToTeamCall(ctx context.Context, client *http.Client, timeoutMS int, endpoint string) {
	log := observability.LoggerWithTrace(ctx)
	effective := int(client.Timeout / time.Millisecond)
	_, hasDeadline := ctx.Deadline()
	log.Debug().Int("args_timeout_ms", timeoutMS).Int("effective_timeout_ms", effective).Bool("parent_has_deadline", hasDeadline).Str("endpoint", endpoint).Msg("delegate_to_team_call")
}
