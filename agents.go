// agents.go — ReAct engine w/ MCP, code_eval and robust path & tool-schema handling
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"manifold/internal/mcp"
)

/*──────────────────────── public ───────────────────────*/

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

/*──────────────────────── internal ─────────────────────*/

// StepHook is a callback function that's called whenever a new step is produced
type StepHook func(step AgentStep)

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

/*──────────────────────── engine ───────────────────────*/

type AgentEngine struct {
	Config       *Config
	DB           *pgx.Conn
	MemoryEngine *AgenticEngine
	HTTPClient   *http.Client

	mcpMgr   *mcp.Manager
	mcpTools map[string]struct{}
}

/*──────────────────────── route ───────────────────────*/

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
			req.MaxSteps = 14
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
		_ = engine.discoverMCPTools(ctx)

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

/*────────────────────── MCP discovery ─────────────────*/

func (ae *AgentEngine) discoverMCPTools(ctx context.Context) error {
	for _, srv := range ae.mcpMgr.List() {
		ts, err := ae.mcpMgr.ListTools(ctx, srv)
		if err != nil {
			log.Printf("list tools %s: %v", srv, err)
			continue
		}
		for _, t := range ts {
			if n := extractToolName(t); n != "" {
				ae.mcpTools[fmt.Sprintf("%s::%s", srv, n)] = struct{}{}
			}
		}
	}
	return nil
}

func extractToolName(v interface{}) string {
	switch vt := v.(type) {
	case string:
		return vt
	case fmt.Stringer:
		return vt.String()
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

/*────────────────────── main loop ─────────────────────*/

func (ae *AgentEngine) RunSession(ctx context.Context, req ReActRequest) (*AgentSession, error) {
	return ae.RunSessionWithHook(ctx, req, nil)
}

func (ae *AgentEngine) RunSessionWithHook(ctx context.Context, req ReActRequest, hook StepHook) (*AgentSession, error) {
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	var td []string
	for n := range ae.mcpTools {
		td = append(td, "- "+n)
	}
	td = append(td,
		"- stage_path   • copy host src into tmp area; returns JSON {host_path,sandbox_path,path}",
		"- code_eval    • run code in sandbox",
		"- finish       • end and output final answer",
	)
	// Explicit guidance for web_content
	td = append(td, `NOTE → "manifold::web_content" needs {"urls":["https://example.com", "..."]}`)

	sysPrompt := fmt.Sprintf(`You are ReAct-Agent.
Objective: %s

IMPORTANT: ALL tool calls should be generated as a single line
with no line breaks, and JSON should be formatted as a single line.

◆ Need host files?
   1. stage_path {"src":"/abs/host/path"}            (optional "dest")
   2. Use returned "path" with file-system tools.
   3. Inside code_eval use "sandbox_path".

◆ Fetching a web page?
   Use manifold::web_content with JSON {"urls":[<link1>, ...]}.

► Prefer to answer directly (with Thought + finish) for narrative tasks
  such as writing, explaining, or summarising natural-language text.
  Only fall back to a tool for *computational* or *programmatic*
  work (e.g. data transformation, heavy math, file parsing).

★ NEVER omit the three headers below – the server will error out:
  Thought: …
  Action: …
  Action Input: …

Format for every turn:
Thought: <reasoning>
Action:  <tool>
Action Input: <JSON | text>

Tools:
%s`, req.Objective, strings.Join(td, "\n"))

	model := req.Model
	if model == "" {
		model = ae.Config.Completions.CompletionsModel
	}

	for i := 0; i < req.MaxSteps; i++ {
		var msgs []Message
		msgs = append(msgs, Message{Role: "system", Content: sysPrompt})
		// ❶ pull top-N memories
		mems, _ := ae.MemoryEngine.SearchWithinWorkflow(ctx, ae.Config, sess.ID, req.Objective, 5)

		// ❷ graft them into the system prompt (or a separate “memory” message)
		if len(mems) > 0 {
			var memBuf strings.Builder
			memBuf.WriteString("🔎 **Session memory snippets**\n")
			for i, m := range mems {
				fmt.Fprintf(&memBuf, "%d. %s\n", i+1, truncate(m.NoteContext, 200))
			}
			msgs = append(msgs, Message{Role: "system", Content: memBuf.String()})
		}

		// build convo
		for _, st := range sess.Steps {
			msgs = append(msgs,
				Message{Role: "assistant",
					Content: fmt.Sprintf("Thought: %s\nAction: %s\nAction Input: %s\nObservation: %s",
						st.Thought, st.Action, st.ActionInput, st.Observation)})
		}
		msgs = append(msgs, Message{Role: "user", Content: "Next step?"})

		out, err := ae.callLLM(ctx, model, msgs)
		if err != nil {
			return nil, err
		}
		thought, action, input := parseReAct(out)

		/*──── graceful fallback ────*/
		if action == "" {
			// treat entire reply as the final answer
			step := AgentStep{
				Index:       len(sess.Steps) + 1,
				Thought:     "LLM reply lacked proper headers; treating as final answer.",
				Action:      "finish",
				ActionInput: strings.TrimSpace(out),
				Observation: "",
			}
			sess.Steps = append(sess.Steps, step)
			_ = ae.persistStep(ctx, sess.ID, step)

			if hook != nil {
				hook(step)
			}

			sess.Result = strings.TrimSpace(out)
			sess.Completed = true
			break
		}
		/*──────────────────────────*/

		obs, err := ae.execTool(ctx, action, input)
		if err != nil {
			obs = "error: " + err.Error()
		}

		step := AgentStep{Index: len(sess.Steps) + 1, Thought: thought, Action: action, ActionInput: input, Observation: obs}
		sess.Steps = append(sess.Steps, step)
		_ = ae.persistStep(ctx, sess.ID, step)

		if hook != nil {
			hook(step)
		}

		if strings.EqualFold(action, "finish") {
			if step.ActionInput == "" {
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

/*────────────────────── LLM helper ────────────────────*/

func (ae *AgentEngine) callLLM(ctx context.Context, model string, msgs []Message) (string, error) {
	body, _ := json.Marshal(CompletionRequest{Model: model, Messages: msgs, MaxTokens: 1024, Temperature: 0.2})
	req, _ := http.NewRequestWithContext(ctx, "POST", ae.Config.Completions.DefaultHost, bytes.NewBuffer(body))
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
		return "", fmt.Errorf("no choices")
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

/*────────────────────── parse helper ─────────────────*/

func parseReAct(s string) (thought, action, input string) {
	for _, ln := range strings.Split(s, "\n") {
		l := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(strings.ToLower(l), "thought:"):
			thought = strings.TrimSpace(l[len("thought:"):])
		case strings.HasPrefix(strings.ToLower(l), "action:"):
			action = strings.TrimSpace(l[len("action:"):])
		case strings.HasPrefix(strings.ToLower(l), "action input:"):
			input = strings.TrimSpace(l[len("action input:"):])
			if strings.HasPrefix(input, "```") {
				input = strings.Trim(input, "` \n")
				if strings.HasPrefix(strings.ToLower(input), "json") {
					input = strings.TrimSpace(input[4:])
				}
			}
		}
	}
	return
}

/*────────────────────── dispatcher ───────────────────*/

func (ae *AgentEngine) execTool(ctx context.Context, name, arg string) (string, error) {
	switch strings.ToLower(name) {
	case "finish":
		return arg, nil
	case "code_eval":
		return ae.runCodeEval(ctx, arg)
	case "stage_path":
		return ae.stagePath(arg)
	default:
		if _, ok := ae.mcpTools[name]; ok {
			// special-case: fix web_content when the LLM passes a bare string
			if strings.HasSuffix(name, "::web_content") && !json.Valid([]byte(arg)) {
				arg = fmt.Sprintf(`{"urls":["%s"]}`, strings.TrimSpace(arg))
			}
			norm, err := ae.normalizeMCPArg(arg)
			if err != nil {
				return "", err
			}
			return ae.callMCP(ctx, name, norm)
		}
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

/*────────────────────── arg normalizer ───────────────*/

func (ae *AgentEngine) normalizeMCPArg(arg string) (string, error) {
	hostPrefix := filepath.Join(ae.Config.DataPath, "tmp") + "/"
	sandboxPrefix := "/mnt/tmp/"

	if !json.Valid([]byte(arg)) { // plain text payload
		return strings.ReplaceAll(arg, sandboxPrefix, hostPrefix), nil
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(arg), &m); err != nil {
		return "", err
	}
	for k, v := range m {
		if s, ok := v.(string); ok && strings.HasPrefix(s, sandboxPrefix) {
			m[k] = strings.Replace(s, sandboxPrefix, hostPrefix, 1)
		}
	}
	if _, ok := m["path"]; !ok { // convenience alias
		if hp, ok := m["host_path"]; ok {
			m["path"] = hp
		}
	}
	b, _ := json.Marshal(m)
	return string(b), nil
}

/*────────────────────── stage_path ───────────────────*/

func (ae *AgentEngine) stagePath(arg string) (string, error) {
	var p struct {
		Src  string `json:"src"`
		Dest string `json:"dest,omitempty"`
	}
	if err := json.Unmarshal([]byte(arg), &p); err != nil {
		return "", fmt.Errorf("stage_path expects JSON {src,dest?}: %v", err)
	}
	if !filepath.IsAbs(p.Src) {
		return "", fmt.Errorf("src must be absolute")
	}
	if p.Dest == "" {
		p.Dest = filepath.Base(p.Src)
	}

	hostDst := filepath.Join(ae.Config.DataPath, "tmp", p.Dest)
	_ = os.RemoveAll(hostDst)

	if err := copyRecursive(p.Src, hostDst); err != nil {
		return "", err
	}
	resp := map[string]string{
		"host_path":    hostDst,
		"sandbox_path": "/mnt/tmp/" + p.Dest,
		"path":         hostDst,
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

func copyRecursive(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			return copyFile(p, target)
		})
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

/*────────────────────── code_eval ────────────────────*/

func (ae *AgentEngine) runCodeEval(_ context.Context, arg string) (string, error) {
	var req CodeEvalRequest
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		return "", fmt.Errorf("code_eval expects JSON {language, code, dependencies}: %v", err)
	}
	var (
		resp *CodeEvalResponse
		err  error
	)
	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python":
		resp, err = runPythonInContainer(req.Code, req.Dependencies)
	case "go":
		resp, err = runGoInContainer(req.Code, req.Dependencies)
	case "javascript":
		resp, err = runNodeInContainer(req.Code, req.Dependencies)
	default:
		return "", fmt.Errorf("unsupported language: %s", req.Language)
	}
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf(resp.Error)
	}
	return resp.Result, nil
}

/*────────────────────── MCP call ─────────────────────*/

func (ae *AgentEngine) callMCP(ctx context.Context, fq, arg string) (string, error) {
	parts := strings.SplitN(fq, "::", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid MCP tool name")
	}
	var params interface{}
	if json.Valid([]byte(arg)) {
		_ = json.Unmarshal([]byte(arg), &params)
	} else {
		params = arg
	}
	resp, err := ae.mcpMgr.CallTool(ctx, parts[0], parts[1], params)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

/*────────────────────── memory ───────────────────────*/

func (ae *AgentEngine) persistStep(ctx context.Context, workflowID uuid.UUID, st AgentStep) error {
	txt := fmt.Sprintf("Thought: %s\nAction: %s\nInput: %s\nObs: %s",
		st.Thought, st.Action, st.ActionInput, st.Observation)

	_, err := ae.MemoryEngine.IngestAgenticMemory(ctx, ae.Config, txt, workflowID)
	if err != nil {
		log.Printf("persist step: %v", err)
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
