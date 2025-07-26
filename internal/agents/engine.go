// agents.go — ReAct engine w/ MCP, code_eval and robust path & tool-schema handling
package agents

import (
	"context"
	"encoding/json"
	"fmt"
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
	documentsv1 "manifold/internal/documents/v1deprecated"
	llm "manifold/internal/llm"
	"manifold/internal/mcp"
	tools "manifold/internal/tools"
	"manifold/internal/util"
)

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
}

var (
	mcpToolsOnce      sync.Once
	cachedMCPTools    map[string]ToolInfo
	cachedToolsErr    error
	cachedServerTools map[string][]ToolInfo
	cachedServerCfgs  map[string]mcp.ServerConfig
	lastCacheTime     time.Time
	cacheTTL          = 5 * time.Minute // TTL for schema cache
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

	_ = eng.discoverMCPTools(ctx)
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
		engine, err := NewEngine(ctx, cfg, poolConn.Conn())
		if err != nil {
			return c.JSON(500, map[string]string{"error": err.Error()})
		}

		session, err := engine.RunSessionWithHook(ctx, cfg, req, func(st AgentStep) {
			// Optional: log or process each step as it is generated
			log.Printf("Step %d: %s | Action: %s | Input: %s", st.Index, st.Thought, st.Action, st.ActionInput)
		})
		if err != nil {
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
	// Check if cache is still valid
	if time.Since(lastCacheTime) < cacheTTL && cachedMCPTools != nil {
		// Use cached data
		if ae.mcpTools == nil {
			ae.mcpTools = make(map[string]ToolInfo)
		}
		if ae.serverTools == nil {
			ae.serverTools = make(map[string][]ToolInfo)
		}
		if ae.serverCfgs == nil {
			ae.serverCfgs = make(map[string]mcp.ServerConfig)
		}

		// Only populate if this is not an isolated engine or if it matches the isolated server
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
		return cachedToolsErr
	}

	// Refresh cache
	mcpToolsOnce.Do(func() {
		cachedMCPTools = make(map[string]ToolInfo)
		cachedServerTools = make(map[string][]ToolInfo)
		cachedServerCfgs = make(map[string]mcp.ServerConfig)
		lastCacheTime = time.Now()

		// First, collect all tools and their info
		for _, srv := range ae.mcpMgr.List() {
			ts, err := ae.mcpMgr.ListTools(ctx, srv)
			if err != nil {
				cachedToolsErr = err
				continue
			}
			if cfg, ok := ae.mcpMgr.Config(srv); ok {
				cachedServerCfgs[srv] = cfg
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
				info := ToolInfo{
					Name:        toolName,
					Description: desc,
					InputSchema: inputSchema,
				}
				cachedMCPTools[toolName] = info
				cachedServerTools[srv] = append(cachedServerTools[srv], info)
			}
		}
	})

	if cachedToolsErr != nil {
		return cachedToolsErr
	}

	// Populate this engine's maps based on isolation settings
	if ae.mcpTools == nil {
		ae.mcpTools = make(map[string]ToolInfo)
	}
	for k, v := range cachedMCPTools {
		if ae.isolatedToServer == "" || strings.HasPrefix(k, ae.isolatedToServer+"::") {
			ae.mcpTools[k] = v
		}
	}
	if ae.serverTools == nil {
		ae.serverTools = make(map[string][]ToolInfo)
	}
	for srv, tools := range cachedServerTools {
		if ae.isolatedToServer == "" || srv == ae.isolatedToServer {
			ae.serverTools[srv] = tools
		}
	}
	if ae.serverCfgs == nil {
		ae.serverCfgs = make(map[string]mcp.ServerConfig)
	}
	for srv, cfg := range cachedServerCfgs {
		if ae.isolatedToServer == "" || srv == ae.isolatedToServer {
			ae.serverCfgs[srv] = cfg
		}
	}

	// Orchestrator (isolatedToServer == "") must NOT have direct access to MCP tools
	// This ensures the orchestrator can only route through ask_assistant_worker
	if ae.isolatedToServer == "" {
		ae.mcpTools = make(map[string]ToolInfo)
	}

	return nil
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
		cfg, _ := ae.serverCfgs[srv]
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

func (ae *AgentEngine) RunSession(ctx context.Context, cfg *configpkg.Config, req ReActRequest) (*AgentSession, error) {
	return ae.RunSessionWithHook(ctx, cfg, req, nil)
}

func (ae *AgentEngine) RunSessionWithHook(ctx context.Context, cfg *configpkg.Config, req ReActRequest, hook StepHook) (*AgentSession, error) {
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	var sysPromptBuilder strings.Builder

	// Only show generic tools and agent fleet for non-isolated engines
	if ae.isolatedToServer == "" {
		sysPromptBuilder.WriteString(`
You are the *orchestrator* agent.
You have access to basic tools and can delegate specialized tasks to tool-agents.

Available tools:
- code_eval: run python code in sandbox
  schema:
	{
		"language": {
			"description": "Programming language (python, go, javascript, sh, shell, bash)",
			"type": "string"
		},
		"code": {
			"description": "Code to execute",
			"type": "string"
		},
		"dependencies": {
			"description": "List of dependencies to install",
			"type": "array",
			"items": {"type": "string"}
		}
	}

- web_search: search the web/internet for information
  schema:
	{
		"query": {
			"description": "The search query",
			"type": "string"
		}
	}

- web_fetch: fetch webpage content from a URL
  schema:
	{
		"url": {
			"description": "The URL of the webpage to fetch",
			"type": "string"
		}
	}

- stage_path: stage files/directories for tool access
  schema:
	{
		"src": {
			"description": "Source file or directory path",
			"type": "string"
		},
		"dest": {
			"description": "Optional destination name",
			"type": "string"
		}
	}

- ask_assistant_worker: get help from specialized assistant worker
  schema:
	{
	  "properties": {
		"name": {
		  "description": "Name of the worker to ask",
		  "type": "string"
		},
		"model": {
		  "description": "Optional model to use. Leave empty unless explicitly requested.",
		  "type": "string"
		},
		"msg": {
		  "description": "Message to send to the worker. Detailed task or help query.",
		  "type": "string"
		}
	  },
	  "required": ["name", "msg"],
	  "type": "object"
	}

- finish: end and output final answer directly responding to the user

⭐ For specialized tasks requiring MCP tools, delegate to tool-agents via ask_assistant_worker.

IMPORTANT: When you delegate a task to a worker and receive a successful result in the observation, 
you should immediately finish with that result. Do not ask the user what to do next - provide the 
complete answer you received from the worker.

Example workflow:
1. User asks: "List files in /tmp"
2. You delegate: ask_assistant_worker with name "file_browser" and msg "List files in /tmp"
3. Worker returns: "Files in /tmp: file1.txt, file2.log, folder1/"
4. You finish immediately: "Files in /tmp: file1.txt, file2.log, folder1/"

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
IMPORTANT: The workers listed above are NOT direct tools. You MUST use ask_assistant_worker to invoke them.

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
		// For isolated tool-agents, provide focused instructions
		sysPromptBuilder.WriteString(`
You are a specialized tool-agent. Use ONLY the tools listed below.
When the task is done reply exactly:

Thought: <your reasoning>
Action: finish
Action Input: <concise result text>

Available tools:
- code_eval: run python code in sandbox
  schema:
	{
		"language": {
			"description": "Programming language (python, go, javascript, sh, shell, bash)",
			"type": "string"
		},
		"code": {
			"description": "Code to execute",
			"type": "string"
		},
		"dependencies": {
			"description": "List of dependencies to install",
			"type": "array",
			"items": {"type": "string"}
		}
	}
- finish: end and output final answer directly responding to the user
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var currentMessages []llm.ChatCompletionMessage

		// Start with the existing conversation history
		currentMessages = append(conversationHistory)

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
		userContent := "Next step?"
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

		if action == "" {
			// treat entire reply as the final answer
			step := AgentStep{
				Index:       len(sess.Steps) + 1,
				Thought:     "Responding...",
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
		userMessage := llm.ChatCompletionMessage{
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

	// Calculate max tokens dynamically: modelCtx - promptTokens - buffer
	maxTokens := max(ae.Config.Completions.CtxSize-promptTokens-1024, 1024)

	log.Printf("Calling LLM %s (%s) with %d tokens", assistant.Name, assistant.Model, promptTokens)

	// Check if this is an MLX backend and handle parameters accordingly
	endpoint := assistant.Endpoint
	isMLX := strings.Contains(strings.ToLower(endpoint), "mlx") ||
		ae.Config.Completions.Backend == "mlx" ||
		strings.Contains(strings.ToLower(model), "mlx")

	var response string
	var err error
	if isMLX {
		// For MLX backends, use the MLX-specific parameter formatting
		response, err = llm.CallMLX(ctx, endpoint, ae.Config.Completions.APIKey, msgs, maxTokens, ae.Config.Completions.Temperature)
	} else {
		response, err = llm.CallLLM(ctx, assistant.Endpoint, ae.Config.Completions.APIKey, model, msgs, maxTokens, ae.Config.Completions.Temperature)
	}
	if err != nil {
		return "", err
	}

	response = strings.ReplaceAll(response, "<think>", "")
	response = strings.ReplaceAll(response, "</think>", "")
	return strings.TrimSpace(response), nil
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
			"stage_path":           true,
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

	switch strings.ToLower(name) {
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
		if err := json.Unmarshal([]byte(arg), &req); err != nil {
			return "", fmt.Errorf("ask_assistant_worker expects JSON {worker, args}: %v", err)
		}

		worker := ae.fleet.GetWorker(req.Name)
		if worker == nil {
			return "", fmt.Errorf("unknown worker: %s", req.Name)
		}

		// If this is a tool-agent, run a sub-agent session with only that server's tools
		if worker.Role == "tool-agent" {
			// Create isolated engine for this specific server/tool-agent
			isolatedEngine := ae.newIsolatedToolEngine(worker.Name)

			// Create a new ReActRequest for the sub-agent session with proper context
			subReq := ReActRequest{
				Objective: fmt.Sprintf("[SYSTEM] You have been invoked by another agent to accomplish the following objective. You must respond with Thought/Action/Action Input as usual.\n\nObjective: %s", req.Msg),
				Model:     req.Model,
				MaxSteps:  cfg.Completions.ReactAgentConfig.MaxSteps,
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
				return subSession.Result, nil
			}
			return "", fmt.Errorf("sub-agent session did not complete: %s", subSession.Result)
		}

		// Otherwise, fallback to LLM call as before
		msg := fmt.Sprintf("%s\n\n%s", worker.Instructions, req.Msg)
		return ae.callLLM(ctx, worker.Name, worker.Model, []llm.ChatCompletionMessage{
			{Role: "user", Content: msg},
		})
	case "code_eval":
		return ae.runCodeEval(ctx, arg)
	case "web_search":
		return ae.runWebSearch(ctx, arg, cfg)
	case "web_fetch":
		return ae.runWebFetch(ctx, arg)
	case "stage_path":
		return ae.stagePath(arg)
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

func (ae *AgentEngine) runCodeEval(_ context.Context, arg string) (string, error) {
	var req tools.CodeEvalRequest
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		return "", fmt.Errorf("code_eval expects JSON {language, code, dependencies}: %v", err)
	}

	// Default to Python if language is not specified
	if req.Language == "" {
		req.Language = "python"
	}

	var (
		resp *tools.CodeEvalResponse
		err  error
	)
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
	case "sh", "shell", "bash":
		// Simple shell execution - wrap the code in a basic shell runner
		result, err := tools.RunPythonRaw(ae.Config, fmt.Sprintf(`
import subprocess
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
`, req.Code), []string{})
		if err != nil {
			resp = &tools.CodeEvalResponse{Error: err.Error()}
		} else {
			resp = &tools.CodeEvalResponse{Result: result}
		}
	default:
		return "", fmt.Errorf("unsupported language: %s", req.Language)
	}
	if err != nil {
		return "", err
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
	resp, err := ae.mcpMgr.CallTool(ctx, parts[0], parts[1], params)
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
	mcpToolsOnce = sync.Once{}
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
