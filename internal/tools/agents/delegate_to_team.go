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
	"manifold/internal/sandbox"
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
	var args struct {
		Team      string        `json:"team"`
		Prompt    string        `json:"prompt"`
		History   []llm.Message `json:"history"`
		TimeoutMS int           `json:"timeout_ms"`
		SessionID string        `json:"session_id"`
		ProjectID string        `json:"project_id"`
	}
	// Handle empty or nil JSON gracefully
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

	// Inherit session_id from context if not explicitly provided by the LLM.
	sessionID := strings.TrimSpace(args.SessionID)
	fromContext := false
	if sessionID == "" {
		if ctxSID, ok := sandbox.SessionIDFromContext(ctx); ok {
			sessionID = ctxSID
			fromContext = true
		}
	}

	switch {
	case sessionID == "":
		sessionID = uuid.NewString()
	case !fromContext:
		// Only convert non-UUID values that came from the LLM args, not from context
		if _, err := uuid.Parse(sessionID); err != nil {
			// Deterministically map non-UUID identifiers to a UUID so repeated
			// values anchor to the same chat transcript.
			sessionID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionID)).String()
		}
	}

	// Inherit project_id from context if not explicitly provided by the LLM.
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		if ctxPID, ok := sandbox.ProjectIDFromContext(ctx); ok {
			projectID = ctxPID
		}
	}

	body := map[string]any{"prompt": args.Prompt, "session_id": sessionID}
	if len(args.History) > 0 {
		body["history"] = args.History
	}
	if projectID != "" {
		body["project_id"] = projectID
	}
	b, _ := json.Marshal(body)

	// Build endpoint URL; force non-stream JSON via stream=0; include team name as group param
	u, _ := neturl.Parse(fmt.Sprintf("%s/agent/run", t.baseURL))
	q := u.Query()
	q.Set("stream", "0")
	q.Set("group", strings.TrimSpace(args.Team))
	u.RawQuery = q.Encode()

	// Team delegation is a long-running operation. Unlike other tools, we detach
	// from the parent context's deadline to allow the team to work without timeout.
	// The team's internal agent runs have their own timeout management.
	// Only apply a timeout if explicitly requested via timeout_ms argument.
	runCtx := context.WithoutCancel(ctx)
	if args.TimeoutMS > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(args.TimeoutMS)*time.Millisecond)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(runCtx, http.MethodPost, u.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Forward auth cookie from context to authenticate internal service calls.
	if cookie, ok := sandbox.AuthCookieFromContext(ctx); ok {
		req.Header.Set("Cookie", cookie)
	}

	client := t.httpClient
	if args.TimeoutMS > 0 {
		c := *t.httpClient
		c.Timeout = time.Duration(args.TimeoutMS) * time.Millisecond
		client = &c
	}
	// No default timeout for team delegation - teams are long-running workflows
	{
		log := observability.LoggerWithTrace(ctx)
		eff := int(client.Timeout / time.Millisecond)
		log.Debug().Int("args_timeout_ms", args.TimeoutMS).Int("effective_timeout_ms", eff).Str("endpoint", u.String()).Msg("delegate_to_team_call")
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
	if rawResult, ok := payload["result"].(string); ok && strings.HasPrefix(strings.TrimSpace(rawResult), "{") {
		var decoded any
		if err := json.Unmarshal([]byte(rawResult), &decoded); err == nil {
			payload["result"] = decoded
		}
	}
	return map[string]any{"ok": true, "team": args.Team, "response": payload}, nil
}
