// agents.go — ReAct engine (MCP-only version)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"manifold/internal/mcp"
)

/*──────────────────────────────
   Public payloads
──────────────────────────────*/

type ReActRequest struct {
	Objective string `json:"objective"`
	MaxSteps  int    `json:"max_steps,omitempty"`
	Model     string `json:"model,omitempty"`
}

type ReActResponse struct {
	SessionID string      `json:"session_id"`
	Trace     []AgentStep `json:"trace"`
	Result    string      `json:"result"`
	Completed bool        `json:"completed"`
}

/*──────────────────────────────
   Internal structs
──────────────────────────────*/

type AgentStep struct {
	Index       int    `json:"index"`
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Observation string `json:"observation"`
}

type AgentSession struct {
	ID        uuid.UUID   `json:"id"`
	Objective string      `json:"objective"`
	Steps     []AgentStep `json:"steps"`
	Result    string      `json:"result"`
	Completed bool        `json:"completed"`
	Created   time.Time   `json:"created"`
}

/*──────────────────────────────
   Engine definition
──────────────────────────────*/

type AgentEngine struct {
	Config       *Config
	DB           *pgx.Conn
	MemoryEngine *AgenticEngine
	HTTPClient   *http.Client

	mcpMgr   *mcp.Manager
	mcpTools map[string]struct{} // set of server::tool names
}

/*──────────────────────────────
   HTTP handlers
──────────────────────────────*/

func runReActAgentHandler(cfg *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ReActRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(400, map[string]string{"error": "invalid request"})
		}
		req.Objective = strings.TrimSpace(req.Objective)
		if req.Objective == "" {
			return c.JSON(400, map[string]string{"error": "objective required"})
		}
		if req.MaxSteps <= 0 {
			req.MaxSteps = 10
		}

		ctx := c.Request().Context()
		conn, err := Connect(ctx, cfg.Database.ConnectionString)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		defer conn.Close(ctx)

		mgr, err := mcp.NewManager(ctx, "config.yaml")
		if err != nil {
			return c.JSON(500, map[string]string{"error": fmt.Sprintf("mcp manager: %v", err)})
		}

		engine := &AgentEngine{
			Config:       cfg,
			DB:           conn,
			MemoryEngine: NewAgenticEngine(conn),
			HTTPClient:   &http.Client{Timeout: 180 * time.Second},
			mcpMgr:       mgr,
			mcpTools:     make(map[string]struct{}),
		}
		if err := engine.MemoryEngine.EnsureAgenticMemoryTable(ctx, cfg.Embeddings.Dimensions); err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		if err := engine.discoverMCPTools(ctx); err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}

		session, err := engine.RunSession(ctx, req)
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}
		return c.JSON(200, ReActResponse{
			SessionID: session.ID.String(),
			Trace:     session.Steps,
			Result:    session.Result,
			Completed: session.Completed,
		})
	}
}

/*──────────────────────────────
   MCP discovery
──────────────────────────────*/

func (ae *AgentEngine) discoverMCPTools(ctx context.Context) error {
	for _, server := range ae.mcpMgr.List() {
		tools, err := ae.mcpMgr.ListTools(ctx, server)
		if err != nil {
			log.Printf("list tools %s: %v", server, err)
			continue
		}
		for _, t := range tools {
			name := extractToolName(t)
			if name != "" {
				ae.mcpTools[fmt.Sprintf("%s::%s", server, name)] = struct{}{}
			}
		}
	}
	return nil
}

func extractToolName(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		f := rv.FieldByName("Name")
		if f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

/*──────────────────────────────
   Execution loop
──────────────────────────────*/

func (ae *AgentEngine) RunSession(ctx context.Context, req ReActRequest) (*AgentSession, error) {
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	// Build prompt
	var toolsDesc []string
	for name := range ae.mcpTools {
		toolsDesc = append(toolsDesc, "- "+name)
	}
	toolsDesc = append(toolsDesc, "- finish (include the FINAL answer text in Action Input)")

	sysPrompt := fmt.Sprintf(
		"You are ReAct-Agent.\nObjective: %s\n"+
			"Use this loop:\nThought: <reasoning>\nAction: <tool>\nAction Input: <JSON or text>\n"+
			"Available MCP tools:\n%s",
		req.Objective, strings.Join(toolsDesc, "\n"))

	model := req.Model
	if model == "" {
		model = ae.Config.Completions.CompletionsModel
	}

	for i := 0; i < req.MaxSteps; i++ {
		msgs := []Message{{Role: "system", Content: sysPrompt}}
		for _, st := range sess.Steps {
			msgs = append(msgs, Message{Role: "assistant",
				Content: fmt.Sprintf("Thought: %s\nAction: %s\nAction Input: %s\nObservation: %s",
					st.Thought, st.Action, st.ActionInput, st.Observation)})
		}
		msgs = append(msgs, Message{Role: "user", Content: "Next step?"})

		resp, err := ae.callLLM(ctx, model, msgs)
		if err != nil {
			return nil, err
		}
		thought, action, input := parseReAct(resp)
		if action == "" {
			return nil, fmt.Errorf("LLM returned no Action:\n%s", resp)
		}

		obs, err := ae.execTool(ctx, action, input)
		if err != nil {
			obs = "error: " + err.Error()
		}

		step := AgentStep{Index: len(sess.Steps) + 1, Thought: thought, Action: action, ActionInput: input, Observation: obs}
		sess.Steps = append(sess.Steps, step)
		_ = ae.persistStep(ctx, step)

		if strings.EqualFold(action, "finish") {
			if step.ActionInput == "" { // safety net
				sess.Result = thought
			} else {
				sess.Result = step.ActionInput
			}
			sess.Completed = true
			break
		}
	}
	if !sess.Completed {
		sess.Result = "Max steps reached"
	}
	return sess, nil
}

/*──────────────────────────────
   LLM wrapper
──────────────────────────────*/

func (ae *AgentEngine) callLLM(ctx context.Context, model string, msgs []Message) (string, error) {
	reqBody, _ := json.Marshal(CompletionRequest{Model: model, Messages: msgs, MaxTokens: 1024, Temperature: 0.2})
	req, _ := http.NewRequestWithContext(ctx, "POST", ae.Config.Completions.DefaultHost, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ae.Config.Completions.APIKey)

	resp, err := ae.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm %d: %s", resp.StatusCode, string(b))
	}
	var cr CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

/*──────────────────────────────
   Parse LLM output
──────────────────────────────*/

func parseReAct(s string) (thought, action, input string) {
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(strings.ToLower(l), "thought:"):
			thought = strings.TrimSpace(l[len("thought:"):])
		case strings.HasPrefix(strings.ToLower(l), "action:"):
			action = strings.TrimSpace(l[len("action:"):])
		case strings.HasPrefix(strings.ToLower(l), "action input:"):
			input = strings.TrimSpace(l[len("action input:"):])
		}
	}
	return
}

/*──────────────────────────────
   Tool execution (only MCP + finish)
──────────────────────────────*/

func (ae *AgentEngine) execTool(ctx context.Context, name, arg string) (string, error) {
	if strings.EqualFold(name, "finish") {
		return arg, nil
	}
	if _, ok := ae.mcpTools[name]; !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	parts := strings.SplitN(name, "::", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("tool must be server::tool format")
	}
	var params interface{}
	if err := json.Unmarshal([]byte(arg), &params); err != nil {
		params = arg // treat as raw string
	}
	resp, err := ae.mcpMgr.CallTool(ctx, parts[0], parts[1], params)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(resp)
	return string(out), nil
}

/*──────────────────────────────
   Memory persistence
──────────────────────────────*/

func (ae *AgentEngine) persistStep(ctx context.Context, st AgentStep) error {
	text := fmt.Sprintf("Thought: %s\nAction: %s\nInput: %s\nObs: %s",
		st.Thought, st.Action, st.ActionInput, st.Observation)
	_, err := ae.MemoryEngine.IngestAgenticMemory(ctx, ae.Config, text, "react_step")
	return err
}
