// agents.go — ReAct engine w/ MCP, code_eval and robust path & tool-schema handling
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	documentsv1 "manifold/internal/documents/v1deprecated"
	llm "manifold/internal/llm"
	"manifold/internal/mcp"
	tools "manifold/internal/tools"
	"manifold/internal/util"
)

type ReActRequest struct {
	Objective       string `json:"objective"`
	MaxSteps        int    `json:"max_steps,omitempty"`
	Model           string `json:"model,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
	ApiKey          string `json:"api_key,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type ReActResponse struct {
	SessionID string      `json:"session_id"`
	Trace     []AgentStep `json:"trace"`
	Result    string      `json:"result"`
	Completed bool        `json:"completed"`
}

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

type AgentEngine struct {
	Config       *configpkg.Config
	DB           *pgx.Conn
	MemoryEngine MemoryEngine
	HTTPClient   *http.Client

	mcpMgr      *mcp.Manager
	mcpTools    map[string]ToolInfo
	serverTools map[string][]ToolInfo
	serverCfgs  map[string]mcp.ServerConfig

	a2aClients map[string]*a2aclient.A2AClient

	fleet *Fleet // map of fleet name to Fleet instance

	// Isolation controls - when set, this engine is restricted to specific servers
	isolatedToServer  string
	recursionDepth    int
	skipAddToolAgents bool

	// Per-request overrides for endpoint and API key (used by React agent streaming)
	overrideEndpoint        string
	overrideApiKey          string
	overrideReasoningEffort string
}

var (
	cachedMCPTools    map[string]ToolInfo
	cachedToolsErr    error
	cachedServerTools map[string][]ToolInfo
	cachedServerCfgs  map[string]mcp.ServerConfig
	lastCacheTime     time.Time
	cacheTTL          = 5 * time.Minute // TTL for schema cache
	cacheGlobalMu     sync.RWMutex
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

	agentFleet := NewFleet()
	fleetCfg := cfg.AgentFleet

	for _, worker := range fleetCfg.Workers {
		fleetWorker := configpkg.FleetWorker{
			Name:         worker.Name,
			Role:         worker.Role,
			Endpoint:     worker.Endpoint,
			Model:        worker.Model,
			CtxSize:      worker.CtxSize,
			Temperature:  worker.Temperature,
			Instructions: worker.Instructions,
		}

		agentFleet.AddWorker(fleetWorker)
	}

	eng := &AgentEngine{
		Config:      cfg,
		DB:          db,
		HTTPClient:  &http.Client{Timeout: 180 * time.Second},
		mcpMgr:      mgr,
		mcpTools:    make(map[string]ToolInfo),
		serverTools: make(map[string][]ToolInfo),
		serverCfgs:  make(map[string]mcp.ServerConfig),
		a2aClients:  make(map[string]*a2aclient.A2AClient),
		fleet:       agentFleet,
	}

	if cfg.AgenticMemory.Enabled {
		eng.MemoryEngine = NewAgenticEngine(db)
		if err := eng.MemoryEngine.EnsureAgenticMemoryTable(ctx, cfg.Embeddings.Dimensions); err != nil {
			return nil, err
		}
	} else {
		eng.MemoryEngine = &NilMemoryEngine{}
	}

	if err := eng.discoverMCPTools(ctx); err != nil {
		log.Printf("warn: discoverMCPTools: %v", err)
	}
	if !eng.skipAddToolAgents {
		eng.addToolAgents()
	}

	return eng, nil
}

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

		// Create a timeout context for engine initialization to prevent hanging on MCP discovery
		engineCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		engine, err := NewEngine(engineCtx, cfg, poolConn.Conn())
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}

		// Apply per-request overrides if provided
		if strings.TrimSpace(req.Endpoint) != "" {
			engine.overrideEndpoint = strings.TrimSpace(req.Endpoint)
		}
		if strings.TrimSpace(req.ApiKey) != "" {
			engine.overrideApiKey = strings.TrimSpace(req.ApiKey)
		}
		if strings.TrimSpace(req.ReasoningEffort) != "" {
			engine.overrideReasoningEffort = strings.ToLower(strings.TrimSpace(req.ReasoningEffort))
		}

		session, err := engine.RunSessionWithHook(ctx, cfg, req, func(st AgentStep) {
			// Optional: log or process each step as it is generated
			log.Printf("Step %d: %s | Action: %s | Input: %s", st.Index, st.Thought, st.Action, st.ActionInput)
		})
		if err != nil {
			// Log detailed error with endpoint/model context for debugging
			ep := req.Endpoint
			if strings.TrimSpace(ep) == "" {
				ep = cfg.Completions.DefaultHost
			}
			log.Printf("[react] error: %v (model=%s endpoint=%s)", err, req.Model, ep)
			return c.String(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(200, ReActResponse{
			SessionID: session.ID.String(),
			Trace:     session.Steps,
			Result:    session.Result,
			Completed: session.Completed,
		})
	}
}

// ToolInfo holds metadata about an MCP tool.
type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// Update AgentEngine to use map[string]ToolInfo for mcpTools
// (You must also update the AgentEngine struct definition above to: mcpTools map[string]ToolInfo)

func (ae *AgentEngine) discoverMCPTools(ctx context.Context) error {

	// fast-path: serve from global cache within TTL
	cacheGlobalMu.RLock()
	valid := time.Since(lastCacheTime) < cacheTTL && cachedMCPTools != nil
	cacheGlobalMu.RUnlock()

	if valid {
		populateFromCacheLocked(ae)
		return cachedToolsErr
	}

	// slow-path: refresh global cache (single writer)
	cacheGlobalMu.Lock()
	defer cacheGlobalMu.Unlock()

	// re-check after acquiring write lock to avoid thundering herd

	if time.Since(lastCacheTime) < cacheTTL && cachedMCPTools != nil {
		populateFromCacheLocked(ae)
		return cachedToolsErr
	}

	cachedMCPTools = make(map[string]ToolInfo)
	cachedServerTools = make(map[string][]ToolInfo)
	cachedServerCfgs = make(map[string]mcp.ServerConfig)
	lastCacheTime = time.Now()
	cachedToolsErr = nil

	for _, srv := range ae.mcpMgr.List() {
		ts, err := ae.mcpMgr.ListTools(ctx, srv)
		if err != nil {
			// continue but remember last error
			log.Printf("warn: ListTools(%s): %v", srv, err)
			cachedToolsErr = err
			continue
		}
		if cfg, ok := ae.mcpMgr.Config(srv); ok {
			cachedServerCfgs[srv] = cfg
		}
		b, _ := json.Marshal(ts)
		var toolsSlice []map[string]interface{}
		if err := json.Unmarshal(b, &toolsSlice); err != nil {
			log.Printf("warn: unmarshal tools %s: %v", srv, err)
			cachedToolsErr = err
			continue
		}
		for _, t := range toolsSlice {
			name, _ := t["name"].(string)
			if name == "" {
				continue
			}
			desc, _ := t["description"].(string)
			var inputSchema map[string]interface{}
			if schema, ok := t["input_schema"].(map[string]interface{}); ok {
				inputSchema = schema
			} else if schema, ok := t["inputSchema"].(map[string]interface{}); ok {
				inputSchema = schema
			}
			toolName := fmt.Sprintf("%s::%s", srv, name)
			info := ToolInfo{
				Name:        toolName,
				Description: desc,
				InputSchema: inputSchema,
			}
			cachedMCPTools[toolName] = info
			cachedServerTools[srv] = append(cachedServerTools[srv], info)
		}
	}
	populateFromCacheLocked(ae)
	return cachedToolsErr
}

// populateFromCacheLocked must be called with global cache lock held and uses global cache
func populateFromCacheLocked(ae *AgentEngine) {
	if ae.mcpTools == nil {
		ae.mcpTools = make(map[string]ToolInfo)
	}
	if ae.serverTools == nil {
		ae.serverTools = make(map[string][]ToolInfo)
	}
	if ae.serverCfgs == nil {
		ae.serverCfgs = make(map[string]mcp.ServerConfig)
	}
	// copy based on isolation
	for k, v := range cachedMCPTools {
		if ae.isolatedToServer == "" || strings.HasPrefix(k, ae.isolatedToServer+"::") {
			ae.mcpTools[k] = v
		}
	}
	for srv, tools := range cachedServerTools {
		if ae.isolatedToServer == "" || srv == ae.isolatedToServer {
			ae.serverTools[srv] = tools
		}
	}
	for srv, cfg := range cachedServerCfgs {
		if ae.isolatedToServer == "" || srv == ae.isolatedToServer {
			ae.serverCfgs[srv] = cfg
		}
	}
	// orchestrator must not access MCP tools directly
	if ae.isolatedToServer == "" {
		ae.mcpTools = make(map[string]ToolInfo)
	}
}

// addToolAgents creates a fleet worker for each MCP server summarizing its tools.
func (ae *AgentEngine) addToolAgents() {
	// Skip adding tool agents for isolated engines or when explicitly disabled
	if ae.isolatedToServer != "" || ae.skipAddToolAgents {
		return
	}

	for srv, tools := range ae.serverTools {
		if len(tools) == 0 {
			continue
		}
		cfg := ae.serverCfgs[srv]
		name := cfg.AgentName
		if name == "" {
			name = srv
		}

		// Build capability summary for better LLM matching
		var capabilities []string
		for _, tool := range tools {
			// Extract key capabilities from tool descriptions
			desc := strings.ToLower(tool.Description)
			if strings.Contains(desc, "web") || strings.Contains(desc, "http") || strings.Contains(desc, "fetch") {
				capabilities = append(capabilities, "web content")
			}
			if strings.Contains(desc, "search") {
				capabilities = append(capabilities, "search")
			}
			if strings.Contains(desc, "file") || strings.Contains(desc, "read") || strings.Contains(desc, "write") {
				capabilities = append(capabilities, "file operations")
			}
			if strings.Contains(desc, "database") || strings.Contains(desc, "sql") {
				capabilities = append(capabilities, "database")
			}
			if strings.Contains(desc, "code") || strings.Contains(desc, "execute") {
				capabilities = append(capabilities, "code execution")
			}
		}

		// Remove duplicates
		seen := make(map[string]bool)
		var uniqueCaps []string
		for _, cap := range capabilities {
			if !seen[cap] {
				uniqueCaps = append(uniqueCaps, cap)
				seen[cap] = true
			}
		}

		var sb strings.Builder
		if cfg.Instructions != "" {
			sb.WriteString(cfg.Instructions)
			if !strings.HasSuffix(cfg.Instructions, "\n") {
				sb.WriteString("\n")
			}
		}

		// Add capability summary
		if len(uniqueCaps) > 0 {
			sb.WriteString(fmt.Sprintf("I can handle: %s. ", strings.Join(uniqueCaps, ", ")))
		}
		sb.WriteString(fmt.Sprintf("I have access to %d specialized tools from the %s server. Use me for tasks related to my capabilities.", len(tools), srv))

		worker := configpkg.FleetWorker{
			Name:         name,
			Role:         "tool-agent",
			Endpoint:     ae.Config.Completions.DefaultHost,
			Model:        ae.Config.Completions.CompletionsModel,
			CtxSize:      ae.Config.Completions.CtxSize,
			Temperature:  ae.Config.Completions.Temperature,
			Instructions: sb.String(),
		}
		ae.fleet.AddWorker(worker)
	}
}

// normalizeAction makes action names robust to capitalization, spacing, and accidental code fencing.
func normalizeAction(a string) string {
	a = strings.TrimSpace(a)
	a = strings.Trim(a, "`")
	a = strings.ToLower(a)
	a = strings.TrimSpace(a)
	switch a {
	case "final", "done", "complete", "finish.", "finalize", "return":
		return "finish"
	}
	return a
}

// ensureJSON tries to ensure the action input is valid JSON if schema expects JSON.
// If it's plain text, it returns the trimmed string.
func ensureJSON(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return t, false
	}
	// fast-path
	if json.Valid([]byte(t)) {
		return t, true
	}
	// try salvage from code fences again if present
	if strings.HasPrefix(t, "```") && strings.HasSuffix(t, "```") {
		t2 := strings.TrimSuffix(strings.TrimPrefix(t, "```"), "```")
		t2 = strings.TrimSpace(t2)
		if strings.HasPrefix(strings.ToLower(t2), "json") {
			t2 = strings.TrimSpace(t2[4:])
		}
		if json.Valid([]byte(t2)) {
			return t2, true
		}
	}
	// try extracting JSON object substring
	if i := strings.Index(t, "{"); i >= 0 {
		if j := strings.LastIndex(t, "}"); j > i {
			sub := t[i : j+1]
			if json.Valid([]byte(sub)) {
				return sub, true
			}
		}
	}
	return t, false
}

func (ae *AgentEngine) RunSession(ctx context.Context, cfg *configpkg.Config, req ReActRequest) (*AgentSession, error) {
	return ae.RunSessionWithHook(ctx, cfg, req, nil)
}

func (ae *AgentEngine) RunSessionWithHook(ctx context.Context, cfg *configpkg.Config, req ReActRequest, hook StepHook) (*AgentSession, error) {
	// Print/log available tools and assistants at the start of the workflow
	if ae.isolatedToServer == "" {
		// Orchestrator: print generic tools and delegation
		log.Printf("[AgentEngine] Available generic tools:")
		log.Printf("- code_eval: run code in a sandbox")
		log.Printf("- web_search: search the web/internet for information")
		log.Printf("- web_fetch: fetch webpage content from a URL")
		log.Printf("- ask_assistant_worker: get help from specialized assistant worker")
		log.Printf("- finish: end and output final answer directly responding to the user AFTER completing the objective.")

		log.Printf("[AgentEngine] Available assistant workers:")
		for _, worker := range ae.fleet.ListWorkers() {
			log.Printf("- %s • %s", worker.Name, worker.Instructions)
		}
	} else {
		// Tool-agent: print available MCP tools
		log.Printf("[AgentEngine] Available MCP tools for this tool-agent:")
		for _, t := range ae.mcpTools {
			log.Printf("- %s • %s", t.Name, t.Description)
		}
	}
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	// Add retry cap for missing Action/Action Input guardrail
	formatRetries := 0

	var sysPromptBuilder strings.Builder
	now := time.Now().Format("2006-01-02 15:04:05 -0700 MST")
	sysPromptBuilder.WriteString("Current datetime: " + now + "\n\n")

	// Only show generic tools and agent fleet for non-isolated engines
	if ae.isolatedToServer == "" {
		sysPromptBuilder.WriteString(`
You are the *orchestrator* agent. A highly proactive and autonomous assistant. 
You think deeply, act decisively, and never leave a problem half-solved.
You have access to basic tools and can delegate specialized tasks to tool-agents.

Available tools (with usage examples):

- code_eval: run code in a sandbox
  Examples:
	Action: code_eval
	Action Input: {"language": "python", "code": "print(2 + 2)"}
	Action: code_eval
	Action Input: {"language": "go", "code": "fmt.Println(42)"}
	Action: code_eval
	Action Input: {"language": "bash", "code": "ls -l"}

- web_search: search the web/internet for information
  Examples:
	Action: web_search
	Action Input: {"query": "golang error handling best practices"}
	Action: web_search
	Action Input: {"query": "latest news on AI"}
	Action: web_search
	Action Input: {"query": "how to write unit tests in go"}

- web_fetch: fetch webpage content from a URL
  Examples:
	Action: web_fetch
	Action Input: {"url": "https://golang.org"}
	Action: web_fetch
	Action Input: {"url": "https://news.ycombinator.com"}
	Action: web_fetch
	Action Input: {"url": "https://github.com"}

- ask_assistant_worker: get help from specialized assistant worker
  Examples:
	Action: ask_assistant_worker
	Action Input: {"name": "python-expert", "msg": "How do I use decorators in Python?"}
	Action: ask_assistant_worker
	Action Input: {"name": "web-scraper", "msg": "Extract all links from https://example.com"}
	Action: ask_assistant_worker
	Action Input: {"name": "sql-helper", "msg": "Write a query to select all users"}

- finish: end and output final answer directly responding to the user AFTER completing the objective.

For specialized tasks requiring MCP tools, delegate to tool-agents via ask_assistant_worker.

IMPORTANT: When you delegate a task to a worker and receive a successful result in the observation, 
you should immediately finish with that result. Do not ask the user what to do next - provide the 
complete answer you received from the worker.

## Workflow

### 1. Deeply Understand the Problem
Carefully read the issue and think hard about a plan to solve it before coding.

### 2. Codebase Investigation
- Explore relevant files and directories
- Search for key functions, classes, or variables related to the issue
- Read and understand relevant code snippets
- Identify the root cause of the problem
- Validate and update your understanding continuously as you gather more context

### 3. Develop a Detailed Plan
- Outline a specific, simple, and verifiable sequence of steps to fix the problem
- Create a todo list in markdown format to track your progress
- Check off completed steps using [x] syntax and display the updated list to the user
- Continue working through the plan without stopping to ask what to do next

### 4. Making Code Changes
- Before editing, always read the relevant file contents or section to ensure complete context
- Make small, testable, incremental changes that logically follow from your investigation and plan

---

## How to Create a Todo List

Use the following format to create a todo list:

- [ ] Description of the first step
- [ ] Description of the second step
- [ ] Description of the third step

**Important:** Do not ever use HTML tags. Always use the markdown format shown above. Always wrap the todo list in triple backticks.

Always include the worker's actual output in your final response.

IMPORTANT: NEVER invoke ANY interactive shells, terminals, or REPLs. ALWAYS use absolute paths for file operations.
NEVER finalize the conversation with a plan or what you intend on doing. 
Your FINAL response must be a confirmation that the plan was executed and the user's query was addressed. 
You must return the results.
Always format your final response using markdown syntax. 
Use markdown syntax to stylize lists, headers, tables, code blocks, apply italics and bold text, etc.
`)

		// Append agent fleet
		sysPromptBuilder.WriteString("Available specialized assistant workers (MUST use ask_assistant_worker to invoke):\n")
		for _, worker := range ae.fleet.ListWorkers() {
			sysPromptBuilder.WriteString(fmt.Sprintf("- %s • %s\n", worker.Name, worker.Instructions))
		}

		sysPromptBuilder.WriteString(`
IMPORTANT: Never provide commentary, meta-discussion, or anything unrelated to the user's objective. Only output what is required to achieve the objective.

Example: To use any worker, call:
Action: ask_assistant_worker
Action Input: {"name": "worker_name", "msg": "your detailed request here"}

NEVER call workers as direct tools. ALWAYS use ask_assistant_worker to delegate to them.

If there is a specialized assistant worker available that can help with the task,
you call it with the ask_assistant_worker tool. If you get stuck, or detect a loop,
ask for assistance from another worker and ensure you give them all of the information
necessary for them to help you.
`)
	} else {
		// For isolated tool-agents, provide focused instructions and list all generic tools
		sysPromptBuilder.WriteString(`
You are a specialized tool-agent. Complete ONLY the specific objective requested.

CRITICAL: Do NOT exceed the scope of the objective. Do NOT take additional steps beyond what was requested.

When the task is done reply exactly:

Thought: <your reasoning about completing the objective>
Action: finish
Action Input: <concise result containing only what was requested>

Available tools:
- web_search: search the web/internet for information
  schema: {"query": "string"}
- web_fetch: fetch webpage content from a URL
  schema: {"url": "string"}
- code_eval: run code in a sandbox (python, go, bash, ...)
  schema: {"language": "string", "code": "string", "dependencies": ["string"]}
- finish: end and output the final answer
`)

	}

	sysPromptBuilder.WriteString(`
Rules:
- Never invoke interactive editors (vim, nano, python idle, etc).
- Keep patches minimal; do not reformat entire files unless required.
- Track and reference original line numbers in your reasoning.
- For non-code text, skip language checkers but still diff/patch/verify.
- NEVER invoke ANY interactive shells, terminals, or REPLs. ALWAYS use absolute paths for file operations.

IMPORTANT: If no tool is available that can be used to complete the task, make your own using the code_eval tool.

IMPORTANT: If a tool call fails, do not end with a final response, always attempt to correct by using a different tool or 
create your own using the code_eval tool.

IMPORTANT: NEVER omit the three headers below – the server will error out:
Thought: …
Action: …
Action Input: …

`)

	// Add specific guidance for tool-agents to stay focused
	if ae.isolatedToServer != "" {
		sysPromptBuilder.WriteString(`
FOCUS RULES FOR SPECIALIZED TOOL-AGENTS:
- Complete ONLY what is explicitly requested in the objective
- Do NOT suggest next steps, improvements, or additional actions
- Do NOT analyze beyond what was asked for
- Do NOT provide recommendations unless specifically requested
- When the objective is complete, immediately use "Action: finish"
- Your final response should contain ONLY the requested information

`)
	}

	sysPromptBuilder.WriteString(`
ALWAYS REMEMBER: Never give up. If you fail to complete the task, try again with a different approach. Before returning your final 
response, always check if the task is complete and if not, continue working on it.

IMPORTANT: The user cannot see your thoughts, actions, or action inputs. So you should always provide a final response that is 
clear and concise, summarizing the results of your actions returning all of the information you have gathered 
ONLY if it is relevant to the user's query.

Always format your final response using markdown syntax. 
Use markdown syntax to stylize lists, headers, tables, code blocks, apply italics and bold text, etc.

Format for every turn:
Thought: <reasoning>
Action:  <tool>
Action Input: <JSON | text>
`)

	// Only expose individual MCP tools if this is an isolated tool-agent engine
	// This prevents the main orchestrator from being overwhelmed with tool schemas
	// when specialized tool assistants are available
	useToolList := ae.isolatedToServer != "" && len(ae.mcpTools) > 0
	if useToolList {
		var relTools []ToolInfo

		// Show all available MCP tools for this isolated tool-agent
		for _, v := range ae.mcpTools {
			relTools = append(relTools, v)
		}

		sysPromptBuilder.WriteString("\nAdditional tools:\n")
		for _, t := range relTools {
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input schema: %v", err)
			}
			sysPromptBuilder.WriteString(fmt.Sprintf("- %s • %s\n", t.Name, t.Description))
			sysPromptBuilder.WriteString(string(schema) + "\n")
		}
	}

	// Append the user's objective to the system prompt
	sysPromptBuilder.WriteString(fmt.Sprintf("Objective: %s", req.Objective))

	sysPrompt := sysPromptBuilder.String()

	model := req.Model
	if model == "" {
		model = ae.Config.Completions.CompletionsModel
	}

	// Store conversation history across turns
	var conversationHistory []llm.ChatCompletionMessage

	// Add system prompt only once at the beginning
	conversationHistory = append(conversationHistory, llm.ChatCompletionMessage{Role: "system", Content: sysPrompt})

	for i := 0; i < req.MaxSteps; i++ {
		if sess.Completed {
			break // safety: never execute further steps once marked complete
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var currentMessages []llm.ChatCompletionMessage

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
			currentMessages = append(currentMessages, llm.ChatCompletionMessage{Role: "system", Content: memBuf.String()})
		} else {
			log.Printf("No memories found")
		}

		// For the current turn, add the user message
		userContent := "Is the objective complete? If not, continue to the next logical step that directly addresses the objective."
		if i == 0 {
			// For the first step, include the actual user objective
			userContent = req.Objective
		}
		currentMessages = append(currentMessages, llm.ChatCompletionMessage{Role: "user", Content: userContent})

		// Debug printing disabled except for LLM call token count

		out, err := ae.callLLM(ctx, "", model, currentMessages)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		thought, action, input := parseReAct(out)

		// If no action was parsed but the response contains "task is complete", treat as finish
		if action == "" && strings.Contains(strings.ToLower(out), "task is complete") {
			action = "finish"
			input = strings.TrimSpace(out)
		}

		// ----------------------------------------------------------------------
		// Guard-rail: the model must ALWAYS emit an Action.  If it forgets, ask
		// it to re-format instead of incorrectly finishing the workflow.
		// ----------------------------------------------------------------------
		if action == "" {
			formatRetries++
			if formatRetries >= 2 {
				// After one failed re-prompt, accept the second malformed reply as finish
				action = "finish"
				input = strings.TrimSpace(out)
			} else {
				conversationHistory = append(conversationHistory,
					llm.ChatCompletionMessage{Role: "assistant", Content: out},
					llm.ChatCompletionMessage{Role: "system", Content: `⚠️ Your last reply was missing the mandatory "Action:" and "Action Input:" fields.
Please resend using **exactly** this pattern:

Thought: <your reasoning>
Action: <tool name or "finish">\nAction Input: <json | text>

If the task is complete, use Action "finish". Otherwise pick an appropriate tool.`},
				)
				// retry the same step without advancing the loop counter
				i--
				continue
			}
		} else {
			formatRetries = 0 // reset after a well-formed step
		}

		// Create a preliminary step with thought, action, and input but no observation yet
		// This allows the frontend to display the thought immediately
		preliminaryStep := AgentStep{
			Index:       len(sess.Steps) + 1,
			Thought:     thought,
			Action:      action,
			ActionInput: input,
			Observation: "", // Will be filled in after tool execution
		}

		// Call the hook immediately with the preliminary step to send thought to frontend
		if hook != nil {
			hook(preliminaryStep)
		}

		obs, err := ae.execTool(ctx, cfg, action, input, hook)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			obs = "error: " + err.Error()
		}

		// if obs > config.Embeddings.Dimensions, split it before ingesting
		if ae.Config.AgenticMemory.Enabled && ae.MemoryEngine != nil {
			// check if the observation is too long
			if len(obs) > 500 {
				// split the observation into chunks
				chunks := documentsv1.SplitTextByCount(obs, 500)
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

		step := AgentStep{Index: len(sess.Steps) + 1, Thought: thought, Action: action, ActionInput: input, Observation: obs}
		sess.Steps = append(sess.Steps, step)
		_ = ae.persistStep(ctx, sess.ID, step)

		// Note: We don't call the hook here again since we already called it with the preliminary step
		// The hook was called immediately after LLM response to send the thought to frontend

		// Add the assistant's response to conversation history
		assistantMessage := llm.ChatCompletionMessage{
			Role: "assistant",
			Content: fmt.Sprintf("Thought: %s\nAction: %s\nAction Input: %s",
				thought, action, input),
		}
		conversationHistory = append(conversationHistory, assistantMessage)

		// Add the observation as a user message in the conversation history
		// nextPrompt := "Next step?"
		var nextPrompt string
		// If we just delegated and got a non-empty result back, assume task could be done.
		if action == "ask_assistant_worker" && strings.TrimSpace(obs) != "" {
			nextPrompt = "If the objective is complete, reply with:\n\nThought: done\nAction: finish\nAction Input: <your answer>"
		}
		userMessage := llm.ChatCompletionMessage{
			Role:    "user",
			Content: fmt.Sprintf("Observation: %s\n\n%s", obs, nextPrompt),
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
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sess.Result = "Max steps reached"
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sess, nil
}

func (ae *AgentEngine) callLLM(ctx context.Context, assistantName string, model string, msgs []llm.ChatCompletionMessage) (string, error) {

	fleet := ae.fleet
	assistant := fleet.GetWorker(assistantName)
	if assistant == nil {
		assistant = &configpkg.FleetWorker{
			Name:         "default",
			Role:         "assistant",
			Endpoint:     ae.Config.Completions.DefaultHost,
			ApiKey:       ae.Config.Completions.APIKey,
			Model:        model,
			CtxSize:      ae.Config.Completions.CtxSize,
			Temperature:  ae.Config.Completions.Temperature,
			Instructions: "",
		}
	}

	// Calculate input token count (approximate)
	var promptTokens int
	for _, msg := range msgs {
		promptTokens += util.CountTokens(msg.Content)
	}
	// if too long, summarize into ~8192 tokens
	if promptTokens > 16000 {
		summed, err := ae.summarizeConversation(ctx, msgs, model)
		if err != nil {
			return "", err
		}
		msgs = summed
		// recompute token count
		promptTokens = 0
		for _, msg := range msgs {
			promptTokens += util.CountTokens(msg.Content)
		}
	}

	// Calculate max tokens dynamically: modelCtx - promptTokens - buffer
	maxTokens := max(ae.Config.Completions.CtxSize-promptTokens-1024, 1024)

	log.Printf("Calling LLM %s (%s) with %d tokens", assistant.Name, assistant.Model, promptTokens)

	// Determine endpoint/apiKey with per-request overrides
	endpoint := assistant.Endpoint
	if strings.TrimSpace(ae.overrideEndpoint) != "" {
		endpoint = strings.TrimSpace(ae.overrideEndpoint)
	}
	apiKey := ae.Config.Completions.APIKey
	if strings.TrimSpace(ae.overrideApiKey) != "" {
		apiKey = strings.TrimSpace(ae.overrideApiKey)
	}

	log.Printf("LLM endpoint: %s", endpoint)

	// Check if this is an MLX backend and handle parameters accordingly
	isMLX := strings.Contains(strings.ToLower(endpoint), "mlx") ||
		ae.Config.Completions.Backend == "mlx" ||
		strings.Contains(strings.ToLower(model), "mlx")

	var response string
	var err error
	if isMLX {
		// For MLX backends, use the MLX-specific parameter formatting
		response, err = llm.CallMLX(ctx, endpoint, apiKey, msgs, maxTokens, ae.Config.Completions.Temperature)
	} else {
		// If a reasoning effort override is set, attach to context
		ctxLLM := ctx
		if strings.TrimSpace(ae.overrideReasoningEffort) != "" {
			ctxLLM = llm.WithReasoningEffort(ctxLLM, ae.overrideReasoningEffort)
		}
		response, err = llm.CallLLM(ctxLLM, endpoint, apiKey, model, msgs, maxTokens, ae.Config.Completions.Temperature)
	}
	if err != nil {
		log.Printf("LLM call failed (model=%s endpoint=%s): %v", model, endpoint, err)
		return "", err
	}

	response = strings.ReplaceAll(response, "<think>", "")
	response = strings.ReplaceAll(response, "</think>", "")
	return strings.TrimSpace(response), nil
}

// helper: send long conversation to summary endpoint and rebuild msgs
func (ae *AgentEngine) summarizeConversation(ctx context.Context, msgs []llm.ChatCompletionMessage, model string) ([]llm.ChatCompletionMessage, error) {
	// pick summary_worker if configured
	var (
		endpoint     string
		apiKey       string
		modelName    string
		maxTokens    int
		temperature  float64
		instructions string
	)

	// Find the first user message in msgs (should be the objective)
	var userObjective string
	for _, m := range msgs {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			userObjective = m.Content
			break
		}
	}

	if w := ae.fleet.GetWorker("summary_worker"); w != nil {
		endpoint = w.Endpoint
		apiKey = ae.Config.Completions.APIKey
		modelName = w.Model
		maxTokens = 8192
		temperature = w.Temperature
		instructions = fmt.Sprintf(`Provide a detailed but concise summary of our conversation above. 
		Focus on information that would be helpful for continuing the conversation, 
		including what we did, what we're doing, which files we're working on, 
		and what we're going to do next. You MUST include information that is relevant to the user's objective: %s`,
			userObjective)
	} else {
		endpoint = ae.Config.Completions.SummaryHost
		if endpoint == "" {
			endpoint = ae.Config.Completions.DefaultHost
		}
		apiKey = ae.Config.Completions.APIKey
		modelName = model
		maxTokens = 8192
		temperature = ae.Config.Completions.Temperature
		instructions = fmt.Sprintf(`Provide a detailed but concise summary of our conversation above. 
		Focus on information that would be helpful for continuing the conversation, 
		including what we did, what we're doing, which files we're working on, 
		and what we're going to do next. You MUST include information that is relevant to the user's objective: %s`,
			userObjective)
	}

	// log the endpoint and model being used for summarization
	log.Printf("Summarizing conversation with endpoint: %s, model: %s", endpoint, modelName)

	// merge full conversation
	var conv strings.Builder
	for _, m := range msgs {
		conv.WriteString(strings.ToUpper(m.Role) + ": " + m.Content + "\n\n")
	}
	summaryPrompt := []llm.ChatCompletionMessage{
		{Role: "system", Content: instructions},
		{Role: "user", Content: conv.String()},
	}

	// choose LLM call style
	isMLX := strings.Contains(strings.ToLower(endpoint), "mlx") ||
		ae.Config.Completions.Backend == "mlx" ||
		strings.Contains(strings.ToLower(modelName), "mlx")

	var summary string
	var err error
	if isMLX {
		summary, err = llm.CallMLX(ctx, endpoint, apiKey, summaryPrompt, maxTokens, temperature)
	} else {
		summary, err = llm.CallLLM(ctx, endpoint, apiKey, modelName, summaryPrompt, maxTokens, temperature)
	}
	if err != nil {
		return nil, fmt.Errorf("summary failed: %w", err)
	}

	// Find the original system prompt (should be the first system message in msgs)
	var origSysPrompt string
	for _, m := range msgs {
		if m.Role == "system" && strings.TrimSpace(m.Content) != "" {
			origSysPrompt = m.Content
			break
		}
	}

	// keep the last user message (for context)
	var lastUser llm.ChatCompletionMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			lastUser = msgs[i]
			break
		}
	}

	newMsgs := []llm.ChatCompletionMessage{}
	if origSysPrompt != "" {
		newMsgs = append(newMsgs, llm.ChatCompletionMessage{Role: "system", Content: origSysPrompt})
	}
	newMsgs = append(newMsgs, llm.ChatCompletionMessage{Role: "system", Content: "Conversation summary:\n" + summary})
	if lastUser.Content != "" {
		newMsgs = append(newMsgs, lastUser)
	}
	return newMsgs, nil
}

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

func (ae *AgentEngine) execTool(ctx context.Context, cfg *configpkg.Config, name, arg string, hook StepHook) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// ─────────── Hard gate for the orchestrator ───────────
	if ae.isolatedToServer == "" { // top-level orchestrator
		lname := strings.ToLower(name)
		// Allow orchestrator to call generic tools and delegation
		allowedTools := map[string]bool{
			"finish":               true,
			"ask_assistant_worker": true,
			"code_eval":            true,
			"web_search":           true,
			"web_fetch":            true,
		}

		if !allowedTools[lname] {
			// Block MCP tools and unknown tools
			if strings.Contains(name, "::") {
				return "", fmt.Errorf(
					"orchestrator cannot call MCP tool '%s' directly – delegate with ask_assistant_worker", name)
			}
			return "", fmt.Errorf(
				"orchestrator cannot call '%s' directly – use available tools or delegate with ask_assistant_worker", name)
		}
	}

	// Normalize action spelling and stray punctuation
	name = normalizeAction(name)
	// Normalize input: retain raw if not JSON
	argJSON, isJSON := ensureJSON(arg)
	switch name {
	case "finish":
		return arg, nil
	case "ask_assistant_worker":
		// Prevent isolated engines from using ask_assistant_worker to avoid recursion
		if ae.isolatedToServer != "" {
			return "", fmt.Errorf("ask_assistant_worker is not available for specialized tool agents")
		}

		// Check recursion depth to prevent infinite loops
		if ae.recursionDepth >= 2 {
			return "", fmt.Errorf("maximum recursion depth reached, cannot delegate further")
		}

		// parse the arg as a JSON object
		var req struct {
			Name  string `json:"name"`
			Model string `json:"model,omitempty"`
			Msg   string `json:"msg,omitempty"`
		}
		if !isJSON {
			return "", fmt.Errorf("ask_assistant_worker expects JSON input, got text: %q", truncate(arg, 200))
		}
		if err := json.Unmarshal([]byte(argJSON), &req); err != nil {
			return "", fmt.Errorf("ask_assistant_worker expects JSON fields {name, model, msg}: %v", err)
		}

		req.Name = strings.TrimSpace(req.Name)
		req.Msg = strings.TrimSpace(req.Msg)

		worker := ae.fleet.GetWorker(req.Name)
		if worker == nil {
			return "", fmt.Errorf("unknown worker: %s", req.Name)
		}

		// If this is a tool-agent, run a sub-agent session with only that server's tools
		if worker.Role == "tool-agent" {
			// Create isolated engine for this specific server/tool-agent
			isolatedEngine := ae.newIsolatedToolEngine(worker.Name)

			// Create a new ReActRequest for the sub-agent session with focused objective
			// Don't include worker instructions in the objective to prevent scope creep
			subReq := ReActRequest{
				Objective: fmt.Sprintf(`You are a specialized tool agent. Your task is to complete the following objective using ONLY the tools available to you. Follow your Thought / Action / Action Input pattern. Once the objective is accomplished, use Action: finish with the results.

Objective: %s`, req.Msg),
				Model:    req.Model,
				MaxSteps: cfg.Completions.ReactAgentConfig.MaxSteps,
			}

			// Run the sub-agent session with configurable timeout to prevent blocking
			// Calculate timeout based on sub-request max steps (30 seconds per step minimum)
			timeout := time.Duration(max(subReq.MaxSteps*30, 120)) * time.Second
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Print debug info
			log.Printf("Running isolated sub-agent session for worker %s with %d tools",
				worker.Name, len(isolatedEngine.mcpTools))

			// Pass the parent's hook down to the sub-agent session
			// Create a wrapper hook that adds a prefix for sub-agent thoughts
			var wrappedHook StepHook
			if hook != nil {
				wrappedHook = func(step AgentStep) {
					// Add a prefix to distinguish sub-agent thoughts
					step.Thought = fmt.Sprintf("[%s] %s", worker.Name, step.Thought)
					hook(step)
				}
			}
			subSession, err := isolatedEngine.RunSessionWithHook(ctx, cfg, subReq, wrappedHook)
			if err != nil {
				return "", fmt.Errorf("failed to run sub-agent session: %w", err)
			}
			// Return the result of the sub-agent session
			if subSession.Completed {
				// Bubble a synthetic FINISH so the outer loop exits immediately.
				// We encode it as triple-header so parseReAct sees Action == finish.
				finishPayload := fmt.Sprintf(
					"Thought: Sub-agent completed\nAction: finish\nAction Input: %q",
					subSession.Result,
				)
				return finishPayload, nil
			}
			return "", fmt.Errorf("sub-agent session did not complete: %s", subSession.Result)
		}

		// Otherwise, fallback to LLM call as before
		msg := fmt.Sprintf("%s\n\n%s", worker.Instructions, req.Msg)
		resp, err := ae.callLLM(ctx, worker.Name, worker.Model, []llm.ChatCompletionMessage{
			{Role: "user", Content: msg},
		})
		if hook != nil {
			step := AgentStep{
				Index:       0, // Index will be set by caller if needed
				Thought:     fmt.Sprintf("[%s] %s", worker.Name, resp),
				Action:      "finish",
				ActionInput: resp,
				Observation: "",
			}
			hook(step)
		}
		return resp, err
	case "code_eval", "code-eval", "execute_code":
		return ae.runCodeEval(ctx, arg)
	case "web_search":
		return ae.runWebSearch(ctx, arg, cfg)
	case "web_fetch":
		return ae.runWebFetch(ctx, arg)

	default:
		// Check if this is an MCP tool first
		if _, ok := ae.mcpTools[name]; ok {
			// Only isolated tool-agents can call MCP tools directly
			if ae.isolatedToServer == "" {
				return "", fmt.Errorf("orchestrator cannot call MCP tools directly - use ask_assistant_worker to delegate to appropriate tool-agent instead")
			}

			// special-case: fix web_content when the LLM passes a bare string
			if strings.HasSuffix(name, "::web_content") && !json.Valid([]byte(arg)) {
				arg = fmt.Sprintf(`{"urls":["%s"]}`, strings.TrimSpace(arg))
			}
			// Provide sensible defaults if schema exists but argument lacks fields
			if isJSON {
				var m map[string]any
				if err := json.Unmarshal([]byte(argJSON), &m); err == nil {
					if strings.HasSuffix(name, "::web_fetch") {
						if _, ok := m["url"]; !ok && m["urls"] == nil {
							return "", fmt.Errorf("web_fetch expects {\"url\": \"https://...\"}")
						}
					}
				}
			}
			norm, err := ae.normalizeMCPArg(arg)
			if err != nil {
				return "", err
			}
			return ae.callMCP(ctx, name, norm)
		}

		// If orchestrator tries to call any tool with "::" that's not in mcpTools, block it
		if ae.isolatedToServer == "" && strings.Contains(name, "::") {
			return "", fmt.Errorf("orchestrator cannot call MCP tools directly - use ask_assistant_worker to delegate to appropriate tool-agent instead")
		}

		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (ae *AgentEngine) normalizeMCPArg(arg string) (string, error) {
	hostPrefix := filepath.Join(ae.Config.DataPath, "tmp") + "/"
	sandboxPrefix := "/mnt/tmp/"

	// Remove stray trailing commas that easily break JSON
	arg = strings.TrimSpace(arg)
	arg = strings.TrimSuffix(arg, ",")

	// Salvage to JSON if needed
	if !json.Valid([]byte(arg)) { // plain text payload
		if strings.Contains(arg, "{") && strings.Contains(arg, "}") {
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
	// Flatten trivial {"input": "..."} wrappers sometimes produced by models
	if len(m) == 1 {
		if val, ok := m["input"]; ok {
			switch v := val.(type) {
			case string:
				if !json.Valid([]byte(v)) {
					v = strings.ReplaceAll(v, sandboxPrefix, hostPrefix)
					return v, nil
				}
			}
		}
	}
	for k, v := range m {
		if s, ok := v.(string); ok && strings.HasPrefix(s, sandboxPrefix) {
			m[k] = strings.Replace(s, sandboxPrefix, hostPrefix, 1)
		}
		// recursively convert in arrays
		if arr, ok := v.([]interface{}); ok {
			for i := range arr {
				if sv, ok := arr[i].(string); ok && strings.HasPrefix(sv, sandboxPrefix) {
					arr[i] = strings.Replace(sv, sandboxPrefix, hostPrefix, 1)
				}
			}
			m[k] = arr
		}
		// recursively convert in nested objects
		if mv, ok := v.(map[string]any); ok {
			for kk, vv := range mv {
				if sv, ok := vv.(string); ok && strings.HasPrefix(sv, sandboxPrefix) {
					mv[kk] = strings.Replace(sv, sandboxPrefix, hostPrefix, 1)
				}
			}
			m[k] = mv
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

func (ae *AgentEngine) runCodeEval(_ context.Context, arg string) (string, error) {
	var req tools.CodeEvalRequest
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		return "", fmt.Errorf("code_eval expects JSON {language, code, dependencies}: %v", err)
	}

	// Default to Python if language is not specified
	if req.Language == "" {
		req.Language = "python"
	}
	if strings.TrimSpace(req.Code) == "" {
		return "", fmt.Errorf("code_eval: code cannot be empty")
	}

	var resp *tools.CodeEvalResponse
	switch strings.ToLower(strings.TrimSpace(req.Language)) {
	case "python":
		result, err := tools.RunPythonRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	case "go":
		result, err := tools.RunGoRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	case "javascript":
		result, err := tools.RunNodeRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	case "node", "nodejs":
		result, err := tools.RunNodeRaw(ae.Config, req.Code, req.Dependencies)
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	case "sh", "shell", "bash":
		// Simple shell execution - wrap the code in a basic shell runner
		pyCode := fmt.Sprintf(`import subprocess
import sys
try:
	result = subprocess.run(%q, shell=True, capture_output=True, text=True, timeout=30)
	if result.returncode == 0:
		print(result.stdout)
	else:
		print(f"Error (exit code {result.returncode}): {result.stderr}", file=sys.stderr)
		sys.exit(result.returncode)
except subprocess.TimeoutExpired:
	print("Command timed out after 30 seconds", file=sys.stderr)
	sys.exit(1)
except Exception as e:
	print(f"Failed to execute command: {e}", file=sys.stderr)
	sys.exit(1)
`, req.Code)
		result, err := tools.RunPythonRaw(ae.Config, pyCode, []string{})
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	default:
		return "", fmt.Errorf("unsupported language: %s", req.Language)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Result, nil
}

func (ae *AgentEngine) runWebSearch(ctx context.Context, arg string, cfg *configpkg.Config) (string, error) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		req.Query = strings.TrimSpace(arg)
	}

	if req.Query == "" {
		return "", fmt.Errorf("query required")
	}

	var urls []string
	if strings.ToLower(cfg.Tools.Search.Backend) == "sxng" {
		if cfg.Tools.Search.Endpoint == "" {
			return "", fmt.Errorf("sxng_url required")
		}
		urls = tools.GetSearXNGResults(ctx, ae.DB, cfg.Tools.Search.Endpoint, req.Query)
	} else {
		urls = tools.SearchDDG(ctx, ae.DB, req.Query)
	}

	if urls == nil {
		return "", fmt.Errorf("error performing web search")
	}

	resultSize := cfg.Tools.Search.ResultSize
	if resultSize <= 0 {
		resultSize = 3
	}

	if len(urls) > resultSize {
		urls = urls[:resultSize]
	}

	// Deduplicate URLs
	return tools.GetSearchResults(ctx, ae.DB, urls), nil
}

func (ae *AgentEngine) runWebFetch(ctx context.Context, arg string) (string, error) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		req.URL = strings.TrimSpace(arg)
	}
	if req.URL == "" {
		return "", fmt.Errorf("url required")
	}
	pg, err := tools.WebGetHandler(ctx, ae.DB, req.URL)
	if err != nil {
		return "", err
	}
	if pg == nil {
		return "", fmt.Errorf("no content")
	}
	b, _ := json.Marshal(pg)
	return string(b), nil
}

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
	// add small delay budget to avoid starving tool calls in short contexts
	callCtx, cancel := context.WithTimeout(ctx, time.Until(time.Now().Add(55*time.Second)))
	defer cancel()
	resp, err := ae.mcpMgr.CallTool(callCtx, parts[0], parts[1], params)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

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

// newIsolatedToolEngine creates a new AgentEngine instance isolated to a specific MCP server
// This prevents tool-agents from accessing tools from other servers and avoids recursion
func (ae *AgentEngine) newIsolatedToolEngine(agentName string) *AgentEngine {
	clone := *ae // shallow copy is fine for read-only fields

	// Resolve the actual server key from the agent name
	server := ae.serverKeyFromAgent(agentName)

	// Create isolated maps for this server only
	clone.serverTools = make(map[string][]ToolInfo)
	clone.serverCfgs = make(map[string]mcp.ServerConfig)
	clone.mcpTools = make(map[string]ToolInfo)
	clone.fleet = NewFleet() // prevent recursive ask-assistant loops
	clone.isolatedToServer = server
	clone.recursionDepth = ae.recursionDepth + 1
	clone.skipAddToolAgents = true // prevent recursive tool agent creation

	// Copy only this server's data
	if tools, exists := ae.serverTools[server]; exists {
		clone.serverTools[server] = tools
		// Populate mcpTools with this server's tools
		for _, tool := range tools {
			clone.mcpTools[tool.Name] = tool
		}
	}
	if cfg, exists := ae.serverCfgs[server]; exists {
		clone.serverCfgs[server] = cfg
	}

	return &clone
}

// refreshCache forces a refresh of the MCP tools cache
// This can be called via an admin endpoint to reload tools without restart
func (ae *AgentEngine) refreshCache(ctx context.Context) error {
	// Reset the once and cache
	cachedMCPTools = nil
	cachedServerTools = nil
	cachedServerCfgs = nil
	lastCacheTime = time.Time{}

	return ae.discoverMCPTools(ctx)
}

// AdminRefreshCacheHandler provides an HTTP endpoint to refresh the MCP tools cache
// This can be added to an admin router to allow cache refresh without restart
// Example usage: router.POST("/admin/refresh-cache", AdminRefreshCacheHandler(cfg))
func AdminRefreshCacheHandler(cfg *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
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

		if err := engine.refreshCache(ctx); err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}

		return c.JSON(200, map[string]string{"status": "cache refreshed successfully"})
	}
}

// serverKeyFromAgent resolves the actual server key from either the agent name or server name
// This handles cases where AgentName is overridden in config
func (ae *AgentEngine) serverKeyFromAgent(name string) string {
	for srv, cfg := range ae.serverCfgs {
		if cfg.AgentName == name || srv == name {
			return srv
		}
	}
	return name // fallback to original name
}
