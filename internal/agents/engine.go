// agents.go — ReAct engine w/ MCP, code_eval and robust path & tool-schema handling
package agents

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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/pterm/pterm"

	a2aclient "manifold/internal/a2a/client"
	configpkg "manifold/internal/config"
	"manifold/internal/documents"
	completions "manifold/internal/llm"
	embeddings "manifold/internal/llm"
	"manifold/internal/mcp"
	codeeval "manifold/internal/tools"
	"manifold/internal/util"

	"github.com/pgvector/pgvector-go"
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
	Config       *configpkg.Config
	DB           *pgx.Conn
	MemoryEngine MemoryEngine
	HTTPClient   *http.Client

	mcpMgr   *mcp.Manager
	mcpTools map[string]ToolInfo

	a2aClients map[string]*a2aclient.A2AClient
}

var (
	mcpToolsOnce   sync.Once
	cachedMCPTools map[string]ToolInfo
	cachedToolsErr error
)

func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}

// NewEngine constructs an AgentEngine using the provided database connection.
// The caller is responsible for closing the DB connection.
func NewEngine(ctx context.Context, cfg *configpkg.Config, db *pgx.Conn) (*AgentEngine, error) {
	mgr, err := mcp.NewManager(ctx, "config.yaml")
	if err != nil {
		return nil, fmt.Errorf("mcp manager: %w", err)
	}

	eng := &AgentEngine{
		Config:     cfg,
		DB:         db,
		HTTPClient: &http.Client{Timeout: 180 * time.Second},
		mcpMgr:     mgr,
		mcpTools:   make(map[string]ToolInfo),
		a2aClients: make(map[string]*a2aclient.A2AClient),
	}

	if cfg.AgenticMemory.Enabled {
		eng.MemoryEngine = NewAgenticEngine(db)
		if err := eng.MemoryEngine.EnsureAgenticMemoryTable(ctx, cfg.Embeddings.Dimensions); err != nil {
			return nil, err
		}
	} else {
		eng.MemoryEngine = &NilMemoryEngine{}
	}

	_ = eng.discoverMCPTools(ctx)

	if cfg.A2A.Role == "master" {
		for _, node := range cfg.A2A.Nodes {
			base := strings.TrimRight(node, "/") + "/api/a2a"
			cl := a2aclient.NewFromConfig(cfg, base)
			if err := cl.Check(ctx); err != nil {
				log.Printf("a2a node %s unreachable: %v", node, err)
				continue
			}
			eng.a2aClients[node] = cl
		}
		log.Printf("a2a discovery complete: %d nodes available", len(eng.a2aClients))
	}

	return eng, nil
}

/*──────────────────────── route ───────────────────────*/

func RunReActAgentHandler(cfg *configpkg.Config) echo.HandlerFunc {
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
			// use default max steps from config
			req.MaxSteps = cfg.Completions.ReactAgentConfig.MaxSteps
			log.Printf("max_steps not set in config, using default %d", req.MaxSteps)
			if req.MaxSteps <= 0 {
				pterm.Debug.Println("max_steps not set in config, using default 100")
				req.MaxSteps = 100
			}
		}

		ctx := c.Request().Context()
		if cfg.DBPool == nil {
			return c.JSON(500, map[string]string{"error": "database connection pool not initialized"})
		}
		poolConn, err := cfg.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(500, map[string]string{"error": "failed to acquire database connection"})
		}
		defer poolConn.Release()
		engine, err := NewEngine(ctx, cfg, poolConn.Conn())
		if err != nil {
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

/*────────────────────── MCP discovery ─────────────────*/

// ToolInfo holds metadata about an MCP tool.
type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// Update AgentEngine to use map[string]ToolInfo for mcpTools
// (You must also update the AgentEngine struct definition above to: mcpTools map[string]ToolInfo)

func (ae *AgentEngine) discoverMCPTools(ctx context.Context) error {
	mcpToolsOnce.Do(func() {
		cachedMCPTools = make(map[string]ToolInfo)
		for _, srv := range ae.mcpMgr.List() {
			ts, err := ae.mcpMgr.ListTools(ctx, srv)
			if err != nil {
				cachedToolsErr = err
				continue
			}
			b, _ := json.Marshal(ts)
			var tools []map[string]interface{}
			if err := json.Unmarshal(b, &tools); err != nil {
				cachedToolsErr = err
				continue
			}
			for _, t := range tools {
				name, _ := t["name"].(string)
				desc, _ := t["description"].(string)
				var inputSchema map[string]interface{}
				if schema, ok := t["input_schema"].(map[string]interface{}); ok {
					inputSchema = schema
				} else if schema, ok := t["inputSchema"].(map[string]interface{}); ok {
					inputSchema = schema
				}
				toolName := fmt.Sprintf("%s::%s", srv, name)
				cachedMCPTools[toolName] = ToolInfo{
					Name:        toolName,
					Description: desc,
					InputSchema: inputSchema,
				}

				// compute and store embedding for tool description
				if err := ae.ensureToolMemoryTable(ctx, ae.Config.Embeddings.Dimensions); err == nil {
					embedTxt := ae.Config.Embeddings.EmbedPrefix + desc
					embeds, err := embeddings.GenerateEmbeddings(ae.Config.Embeddings.Host, ae.Config.Embeddings.APIKey, []string{embedTxt})
					if err == nil && len(embeds) > 0 {
						vec := pgvector.NewVector(embeds[0])
						_ = ae.upsertToolMemory(ctx, toolName, desc, vec)
					}
				}
			}
		}
	})

	if cachedToolsErr != nil {
		return cachedToolsErr
	}
	if ae.mcpTools == nil {
		ae.mcpTools = make(map[string]ToolInfo)
	}
	for k, v := range cachedMCPTools {
		ae.mcpTools[k] = v
	}
	return nil
}

/*────────────────────── main loop ─────────────────────*/

func (ae *AgentEngine) RunSession(ctx context.Context, req ReActRequest) (*AgentSession, error) {
	return ae.RunSessionWithHook(ctx, req, nil)
}

func (ae *AgentEngine) RunSessionWithHook(ctx context.Context, req ReActRequest, hook StepHook) (*AgentSession, error) {
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	if ae.Config.A2A.Role == "master" {
		log.Printf("a2a cluster workers available: %d", len(ae.a2aClients))
	}

	var td []string
	toolLimit := ae.Config.Completions.ReactAgentConfig.NumTools
	if toolLimit <= 0 {
		toolLimit = len(ae.mcpTools)
	}
	relTools, err := ae.getRelevantTools(ctx, req.Objective, toolLimit)
	if err != nil || len(relTools) == 0 {
		for _, v := range ae.mcpTools {
			relTools = append(relTools, v)
		}
	}
	for _, t := range relTools {
		td = append(td, fmt.Sprintf("- %s • %s", t.Name, t.Description))
	}
	td = append(td,
		"- code_eval    • run code in sandbox",
		"- tool_help    • get tool schema and description",
		"- finish       • end and output final answer",
	)

	sysPrompt := fmt.Sprintf(`You are ReAct-Agent.
Objective: %s

You run inside a bash sandbox at /app/projects.  
Goal: read, patch, and validate text/code with deterministic CLI calls only.  
Return clear reasoning + final diff or file content.

─────────────────
CORE CLI PRIMITIVES
─────────────────
• List/scan   : ls, tree, find . -type f, grep -R --line-number PATTERN .
• Count lines : wc -l FILE
• Read slice  : sed -n 'START,ENDp' FILE   # preserves original numbering
• Show single : awk 'NR==N{print;exit}' FILE
• Diff        : diff -u OLD NEW   |   git diff --no-index
• Patch apply : patch -p1 < PATCH   |   git apply --stat PATCH
• In-place edit (single line) : ed -s FILE <<< $'LINEc\nNEW_TEXT\n.'
• Regex replace range         : sed -i 'A,B s/OLD/NEW/g' FILE
• Atomic overwrite            : printf '%%s\n' "$NEW" | sponge FILE

─────────────
LANG CHECKERS
─────────────
.py  → flake8 && mypy && python -m pytest -q (if tests exist)  
.js  → eslint . && npm test --silent  
.rs  → cargo fmt --check && cargo clippy --no-deps && cargo test --quiet  
.go  → gofmt -s -l . && golint ./... && go vet ./... && go test ./...  
.swift → swiftformat --lint . && swiftlint

─────────
WORKFLOW
─────────
1. **Locate** target lines via grep -n / regex.  
2. **Read** minimal context chunks with sed/awk for reasoning.  
3. **Plan** unified diff ( --- a/FILE\n+++ b/FILE … ).  
4. **Apply** patch atomically; abort on fuzz/hunk failures.  
5. **Validate** with language checkers; if any fail, rollback and rethink.  
6. **Output** final 'diff -u' (or full file if small) plus success note.

Rules:
• Never invoke interactive editors (vim, nano, etc.).  
• Keep patches minimal; do not reformat entire files unless required.  
• Track and reference original line numbers in your reasoning.  
• For non-code text, skip language checkers but still diff/patch/verify.

You have full sudo-less access inside the sandbox; no external network.

IMPORTANT: ALL tool calls should be generated as a single line
with no line breaks, and JSON should be formatted as a single line.

- Prefer to answer directly (with Thought + finish) for narrative tasks
  such as writing, explaining, or summarising natural-language text.
  Only fall back to a tool for *computational* or *programmatic*
  work (e.g. data transformation, heavy math, file parsing).

IMPORTANT: The working directory is always /app/projects. If a full path is not given, always assume the file is in this directory.
For example, if the file is called "foo.txt", the full path is "/app/projects/foo.txt". If the file is in a subdirectory, the 
full path is "/app/projects/subdir/foo.txt". ALL tool calls should be made with the full path. NEVER attempt to use a relative path.

Always consider using the tools first. If no tool is available that can be used to complete the task, make your own.

If a tool call fails, do not end with a final response, always attempt to correct by using a different tool or create your own using the code_eval tool.

You can use the code_eval tool with python to successfully complete the task if no other tool is suitable.
The code_eval tool supports third-party libraries, so you can include them in the dependencies array. 
The code should be valid and executable in Python. The code should always return a string with the result of the execution, 
so that it can be used for the next task.

If no dependencies are needed, the dependencies array must be empty (e.g., []).

The json object should be formatted in a single line as follows:
{"language":"python","code":"<python code>","dependencies":["<dependency1>","<dependency2>"]}

For example (using third party libraries):

{"language":"python","code":"import requests\nfrom bs4 import BeautifulSoup\nfrom markdownify import markdownify as md\n\ndef main():\n    url = 'https://en.wikipedia.org/wiki/Technological_singularity'\n    response = requests.get(url)\n    response.raise_for_status()\n\n    soup = BeautifulSoup(response.text, 'html.parser')\n    content = soup.find('div', id='mw-content-text')\n\n    # Convert HTML content to Markdown\n    markdown = md(str(content), heading_style=\"ATX\")\n    print(markdown)\n\nif __name__':\n    main()","dependencies":["requests","beautifulsoup4","markdownify"]}

IMPORTANT: NEVER omit the three headers below – the server will error out:
  Thought: …
  Action: …
  Action Input: …

ALWAYS REMEMBER: Never give up. If you fail to complete the task, try again with a different approach.
IMPORTANT: If a tool call fails always try another tool or create your own using the code_eval tool. DO NOT GIVE UP!

Format for every turn:
Thought: <reasoning>
Action:  <tool>
Action Input: <JSON | text>

Use tool_help <tool_name> to view a tool's JSON schema.

Tools:
%s`, req.Objective, strings.Join(td, "\n"))

	model := req.Model
	if model == "" {
		model = ae.Config.Completions.CompletionsModel
	}

	// Store conversation history across turns
	var conversationHistory []completions.Message

	// Add system prompt only once at the beginning
	conversationHistory = append(conversationHistory, completions.Message{Role: "system", Content: sysPrompt})

	for i := 0; i < req.MaxSteps; i++ {
		var currentMessages []completions.Message

		// Start with the existing conversation history
		currentMessages = append(currentMessages, conversationHistory...)

		// Only query memories if agentic memory is enabled
		var mems []AgenticMemory
		if ae.Config.AgenticMemory.Enabled && ae.MemoryEngine != nil {
			log.Printf("Searching for memories...")
			mems, _ = ae.MemoryEngine.SearchWithinWorkflow(ctx, ae.Config, sess.ID, req.Objective, 5)
		}

		// Add memories to the prompt if any were found
		if len(mems) > 0 {
			log.Printf("Found %d memories", len(mems))
			var memBuf strings.Builder
			memBuf.WriteString("🔎 **Session memory snippets**\n")
			for i, m := range mems {
				fmt.Fprintf(&memBuf, "%d. %s\n", i+1, truncate(m.NoteContext, 200))
			}
			// Add memory as a separate system message for this turn
			currentMessages = append(currentMessages, completions.Message{Role: "system", Content: memBuf.String()})
		} else {
			log.Printf("No memories found")
		}

		// For the current turn, add the user message
		currentMessages = append(currentMessages, completions.Message{Role: "user", Content: "Next step?"})

		// Print the prompt for debugging
		log.Println("=====================================")
		log.Println("Prompt:")
		for _, m := range currentMessages {
			if m.Role == "user" {
				log.Printf("User: %s", m.Content)
			} else {
				log.Printf("Assistant: %s", m.Content)
			}
		}
		log.Println("=====================================")

		out, err := ae.callLLM(ctx, model, currentMessages)
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

		// if obs > config.Embeddings.Dimensions, split it before ingesting
		if ae.Config.AgenticMemory.Enabled && ae.MemoryEngine != nil {
			// check if the observation is too long
			if len(obs) > 500 {
				// split the observation into chunks
				chunks := documents.SplitTextByCount(obs, 500)
				// ingest each chunk separately
				for _, chunk := range chunks {
					_, err := ae.MemoryEngine.IngestAgenticMemory(ctx, ae.Config, chunk, sess.ID)
					if err != nil {
						log.Printf("persist step: %v", err)
						// imediately exit the chunk for loop
						break
					}
				}

				// Search for similar memories to the objective
				mems, _ = ae.MemoryEngine.SearchWithinWorkflow(ctx, ae.Config, sess.ID, req.Objective, 30)

				if len(mems) > 0 {
					obs = ""
					var memBuf strings.Builder
					memBuf.WriteString("🔎 **Similar memory chunks**\n")
					for i, m := range mems {
						fmt.Fprintf(&memBuf, "%d. %s\n", i+1, m.NoteContext)
					}
					obs += "\n\n" + memBuf.String()
				}
			}
		}

		//obs = truncate(obs, 500)

		step := AgentStep{Index: len(sess.Steps) + 1, Thought: thought, Action: action, ActionInput: input, Observation: obs}
		sess.Steps = append(sess.Steps, step)
		_ = ae.persistStep(ctx, sess.ID, step)

		if hook != nil {
			hook(step)
		}

		// Add the assistant's response to conversation history
		assistantMessage := completions.Message{
			Role: "assistant",
			Content: fmt.Sprintf("Thought: %s\nAction: %s\nAction Input: %s",
				thought, action, input),
		}
		conversationHistory = append(conversationHistory, assistantMessage)

		// Add the observation as a user message in the conversation history
		userMessage := completions.Message{
			Role:    "user",
			Content: fmt.Sprintf("Observation: %s\n\nNext step?", obs),
		}
		conversationHistory = append(conversationHistory, userMessage)

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

func (ae *AgentEngine) callLLM(ctx context.Context, model string, msgs []completions.Message) (string, error) {
	// Calculate input token count (approximate)
	var promptTokens int
	for _, msg := range msgs {
		promptTokens += util.CountTokens(msg.Content)
	}

	// Get model context size (default to 4096 if unknown)
	modelCtx := 32768 // default context size

	// Calculate max tokens dynamically: modelCtx - promptTokens - buffer
	maxTokens := max(modelCtx-promptTokens-1024, 128)

	body, _ := json.Marshal(completions.CompletionRequest{Model: model, Messages: msgs, MaxTokens: maxTokens, Temperature: 0.7})
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
	var cr completions.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	// Remove <think> and </think> tags
	response := strings.ReplaceAll(cr.Choices[0].Message.Content, "<think>", "")
	response = strings.ReplaceAll(response, "</think>", "")
	return strings.TrimSpace(response), nil
}

/*────────────────────── parse helper ─────────────────*/

func parseReAct(s string) (thought, action, input string) {
	var grab bool
	var buf []string
	for _, ln := range strings.Split(s, "\n") {
		l := strings.TrimSpace(ln)

		switch {
		case strings.HasPrefix(strings.ToLower(l), "thought:"):
			thought = strings.TrimSpace(l[len("thought:"):])
			grab = false
		case strings.HasPrefix(strings.ToLower(l), "action:"):
			action = strings.TrimSpace(l[len("action:"):])
			grab = false
		case strings.HasPrefix(strings.ToLower(l), "action input:"):
			grab = true
			line := strings.TrimSpace(l[len("action input:"):])
			if line != "" {
				buf = append(buf, line)
			}
		default:
			if grab {
				low := strings.ToLower(l)
				// stop if we reach the next header
				if strings.HasPrefix(low, "thought:") ||
					strings.HasPrefix(low, "action:") ||
					strings.HasPrefix(low, "observation:") {
					grab = false
					continue
				}
				buf = append(buf, l)
			}
		}
	}
	input = strings.Join(buf, "\n")

	// strip ```json fences if present
	if strings.HasPrefix(input, "```") {
		input = strings.Trim(input, "` \n")
		if strings.HasPrefix(strings.ToLower(input), "json") {
			input = strings.TrimSpace(input[4:])
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
	case "tool_help":
		tname := strings.TrimSpace(arg)
		if info, ok := ae.mcpTools[tname]; ok {
			resp := map[string]interface{}{
				"description":  info.Description,
				"input_schema": info.InputSchema,
			}
			b, _ := json.Marshal(resp)
			return string(b), nil
		}
		var desc string
		err := ae.DB.QueryRow(ctx, `SELECT description FROM tool_memory WHERE tool_name = $1`, tname).Scan(&desc)
		if err == nil {
			resp := map[string]interface{}{"description": desc}
			b, _ := json.Marshal(resp)
			return string(b), nil
		}
		return "", fmt.Errorf("unknown tool: %s", tname)
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
		if !json.Valid([]byte(arg)) && strings.Contains(arg, "{") && strings.Contains(arg, "}") {
			// attempt salvage: grab everything between the first '{' and *last* '}'
			if start := strings.Index(arg, "{"); start >= 0 {
				if end := strings.LastIndex(arg, "}"); end > start {
					candidate := arg[start : end+1]
					if json.Valid([]byte(candidate)) {
						arg = candidate
					}
				}
			}
		}
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
	var req codeeval.CodeEvalRequest
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		return "", fmt.Errorf("code_eval expects JSON {language, code, dependencies}: %v", err)
	}
	var (
		resp *codeeval.CodeEvalResponse
		err  error
	)
	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python":
		result, err := codeeval.RunPythonRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &codeeval.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &codeeval.CodeEvalResponse{Result: result}
		}
	case "go":
		result, err := codeeval.RunGoRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &codeeval.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &codeeval.CodeEvalResponse{Result: result}
		}
	case "javascript":
		result, err := codeeval.RunNodeRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &codeeval.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &codeeval.CodeEvalResponse{Result: result}
		}
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
	// Check if agentic memory is enabled in configuration
	if !ae.Config.AgenticMemory.Enabled {
		return nil
	}

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
