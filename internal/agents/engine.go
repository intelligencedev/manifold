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
	"manifold/internal/documents"
	llm "manifold/internal/llm"
	"manifold/internal/mcp"
	tools "manifold/internal/tools"
	"manifold/internal/util"

	openai "github.com/sashabaranov/go-openai"

	"github.com/pgvector/pgvector-go"
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
}

var (
	mcpToolsOnce      sync.Once
	cachedMCPTools    map[string]ToolInfo
	cachedToolsErr    error
	cachedServerTools map[string][]ToolInfo
	cachedServerCfgs  map[string]mcp.ServerConfig
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
	eng.addToolAgents()

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
	mcpToolsOnce.Do(func() {
		cachedMCPTools = make(map[string]ToolInfo)
		cachedServerTools = make(map[string][]ToolInfo)
		cachedServerCfgs = make(map[string]mcp.ServerConfig)
		var toolsToEmbed []struct {
			name        string
			description string
		}

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

				// Collect tools for embedding
				toolsToEmbed = append(toolsToEmbed, struct {
					name        string
					description string
				}{name: toolName, description: desc})
			}
		}

		// Generate embeddings in a single batch if tool memory table exists
		if len(toolsToEmbed) > 0 && ae.ensureToolMemoryTable(ctx, ae.Config.Embeddings.Dimensions) == nil {
			// Prepare text for embedding
			var embedTexts []string
			for _, tool := range toolsToEmbed {
				embedTexts = append(embedTexts, ae.Config.Embeddings.EmbedPrefix+tool.description)
			}

			// Generate all embeddings in one call
			embeds, err := llm.GenerateEmbeddings(ae.Config.Embeddings.Host, ae.Config.Embeddings.APIKey, embedTexts)
			if err == nil && len(embeds) == len(toolsToEmbed) {
				// Use a wait group to handle concurrent inserts
				var wg sync.WaitGroup
				// Use a semaphore to limit concurrency to avoid overwhelming the database
				sem := make(chan struct{}, 10) // Allow up to 10 concurrent operations

				for i, tool := range toolsToEmbed {
					wg.Add(1)
					sem <- struct{}{} // Acquire semaphore

					go func(index int, toolName, desc string, embedding []float32) {
						defer wg.Done()
						defer func() { <-sem }() // Release semaphore

						vec := pgvector.NewVector(embedding)
						_ = ae.upsertToolMemory(ctx, toolName, desc, vec)
					}(i, tool.name, tool.description, embeds[i])
				}

				wg.Wait() // Wait for all insertions to complete
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
	if ae.serverTools == nil {
		ae.serverTools = make(map[string][]ToolInfo)
	}
	for srv, tools := range cachedServerTools {
		ae.serverTools[srv] = tools
	}
	if ae.serverCfgs == nil {
		ae.serverCfgs = make(map[string]mcp.ServerConfig)
	}
	for srv, cfg := range cachedServerCfgs {
		ae.serverCfgs[srv] = cfg
	}
	return nil
}

// addToolAgents creates a fleet worker for each MCP server summarizing its tools.
func (ae *AgentEngine) addToolAgents() {
	for srv, tools := range ae.serverTools {
		if len(tools) == 0 {
			continue
		}
		cfg, _ := ae.serverCfgs[srv]
		name := cfg.AgentName
		if name == "" {
			name = srv
		}
		var sb strings.Builder
		if cfg.Instructions != "" {
			sb.WriteString(cfg.Instructions)
			if !strings.HasSuffix(cfg.Instructions, "\n") {
				sb.WriteString("\n")
			}
		}
		sb.WriteString("Available tools on this server (with schemas):\n")
		for _, t := range tools {
			schema, err := json.MarshalIndent(t.InputSchema, "  ", "  ")
			if err != nil {
				schema = []byte("{}")
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n  schema: %s\n", t.Name, t.Description, string(schema)))
		}

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

	var prompt []string

	prompt = append(prompt, `
	You are a helpful assistant in a sandboxed environment with access to various tools:

	- code_eval: run python code in sandbox

	You can use the code_eval tool with python to successfully complete the task if no other tool is suitable.
	The code_eval tool supports third-party libraries, so you can include them in the dependencies array.
	The code should be valid and executable in Python. The code should always return a string with the results.

	If no dependencies are needed, the dependencies array must be empty (e.g., []).

	The json object should be formatted in a single line as follows:
		{
			"language": "python",
			"code": "<python code>",
			"dependencies": ["<dependency1>", "<dependency2>"]
		}

	For example (using third party libraries):
		{
			"language": "python",
			"code": "import requests\nfrom bs4 import BeautifulSoup\nfrom markdownify import markdownify as md\n\ndef main():\n    url = 'https://en.wikipedia.org/wiki/Technological_singularity'\n    response = requests.get(url)\n    response.raise_for_status()\n\n    soup = BeautifulSoup(response.text, 'html.parser')\n    content = soup.find('div', id='mw-content-text')\n\n    # Convert HTML content to Markdown\n    markdown = md(str(content), heading_style=\"ATX\")\n    print(markdown)\n\nif __name__':\n    main()",
			"dependencies": ["requests", "beautifulsoup4", "markdownify"]
		}

	- web_search: search the web/internet for information
	  schema:
		{
			"query": {
				"description": "The search query",
				"type": "string"
			}
		}
		You can use the web_search tool to search the web for information related to the task.
		You should never use a search engine directly, always use the web_search tool.
		When using the web_search tool, input the ideal search query related to the task.
		Always retrieve the content of the first 3 results, unless the task requires more.
		You can use the web_fetch tool to fetch the content of a webpage from a URL.

	- web_fetch: fetch webpage content from a URL
	  schema:
	  	{
			"url": {
				"description": "The URL of the webpage to fetch",
				"type": "string"
			}
		}
		You can use the web_fetch tool to fetch the content of a webpage.

	- finish: end and output final answer directly responding to the user

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

	`)

	// Append agent fleet
	assistantWorkers := "Available specialized assistant workers:\n"
	for _, worker := range ae.fleet.ListWorkers() {
		assistantWorkers += fmt.Sprintf("- %s (%s) • %s\n", worker.Name, worker.Role, worker.Instructions)
	}

	prompt = append(prompt, assistantWorkers)

	prompt = append(prompt, `
	If there is a specialized assistant worker available that can help with the task,
	you call it with the ask_assistant_worker tool. If you get stuck, or detect a loop,
	ask for assistance from another worker and ensure you give them all of the information
	necessary for them to help you.

	Rules:
	- Never invoke interactive editors (vim, nano, etc).
	- Keep patches minimal; do not reformat entire files unless required.
	- Track and reference original line numbers in your reasoning.
	- For non-code text, skip language checkers but still diff/patch/verify.

	IMPORTANT: If no tool is available that can be used to complete the task, make your own using the code_eval tool.

	IMPORTANT: If a tool call fails, do not end with a final response, always attempt to correct by using a different tool or create your own using the code_eval tool.

	IMPORTANT: NEVER omit the three headers below – the server will error out:
	Thought: …
	Action: …
	Action Input: …

	ALWAYS REMEMBER: Never give up. If you fail to complete the task, try again with a different approach. Before returning your final response, always check if the task is complete and if not, continue working on it.

	Format for every turn:
	Thought: <reasoning>
	Action:  <tool>
	Action Input: <JSON | text>

	Tools:
%s`, strings.Join(prompt, "\n"))

	useToolList := len(ae.serverTools) == 0
	if useToolList {
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
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input schema: %v", err)
			}
			prompt = append(prompt, fmt.Sprintf("- %s • %s", t.Name, t.Description), string(schema))
		}
	}

	// Append the user's objective to the system prompt
	sysPrompt := fmt.Sprintf("%s\nObjective: %s", strings.Join(prompt, "\n"), req.Objective)

	model := req.Model
	if model == "" {
		model = ae.Config.Completions.CompletionsModel
	}

	// Store conversation history across turns
	var conversationHistory []openai.ChatCompletionMessage

	// Add system prompt only once at the beginning
	conversationHistory = append(conversationHistory, openai.ChatCompletionMessage{Role: "system", Content: sysPrompt})

	for i := 0; i < req.MaxSteps; i++ {
		var currentMessages []openai.ChatCompletionMessage

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
			currentMessages = append(currentMessages, openai.ChatCompletionMessage{Role: "system", Content: memBuf.String()})
		} else {
			log.Printf("No memories found")
		}

		// For the current turn, add the user message
		currentMessages = append(currentMessages, openai.ChatCompletionMessage{Role: "user", Content: "Next step?"})

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

		out, err := ae.callLLM(ctx, "", model, currentMessages)
		if err != nil {
			return nil, err
		}
		thought, action, input := parseReAct(out)

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

		obs, err := ae.execTool(ctx, cfg, action, input)
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

		step := AgentStep{Index: len(sess.Steps) + 1, Thought: thought, Action: action, ActionInput: input, Observation: obs}
		sess.Steps = append(sess.Steps, step)
		_ = ae.persistStep(ctx, sess.ID, step)

		if hook != nil {
			hook(step)
		}

		// Add the assistant's response to conversation history
		assistantMessage := openai.ChatCompletionMessage{
			Role: "assistant",
			Content: fmt.Sprintf("Thought: %s\nAction: %s\nAction Input: %s",
				thought, action, input),
		}
		conversationHistory = append(conversationHistory, assistantMessage)

		// Add the observation as a user message in the conversation history
		userMessage := openai.ChatCompletionMessage{
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

func (ae *AgentEngine) callLLM(ctx context.Context, assistantName string, model string, msgs []openai.ChatCompletionMessage) (string, error) {

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

func (ae *AgentEngine) execTool(ctx context.Context, cfg *configpkg.Config, name, arg string) (string, error) {
	switch strings.ToLower(name) {
	case "finish":
		return arg, nil
	case "ask_assistant_worker":
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
			// Try to find the server this tool-agent represents
			var serverName string
			// Prefer exact match by AgentName, fallback to worker.Name
			for srv, cfg := range ae.serverCfgs {
				if cfg.AgentName == worker.Name || srv == worker.Name {
					serverName = srv
					break
				}
			}
			if serverName == "" {
				// fallback: try to match by worker.Name
				serverName = worker.Name
			}

			// Shallow copy the engine and restrict to only this server's tools
			subEngine := *ae
			subEngine.mcpTools = make(map[string]ToolInfo)
			subEngine.serverTools = make(map[string][]ToolInfo)
			subEngine.serverCfgs = make(map[string]mcp.ServerConfig)

			// Copy only the relevant tools/configs
			for _, t := range ae.serverTools[serverName] {
				subEngine.mcpTools[t.Name] = t
			}
			subEngine.serverTools[serverName] = ae.serverTools[serverName]
			if cfg, ok := ae.serverCfgs[serverName]; ok {
				subEngine.serverCfgs[serverName] = cfg
			}

			// Run a sub-agent session
			subReq := ReActRequest{
				Objective: req.Msg,
				MaxSteps:  5, // or configurable
				Model:     worker.Model,
			}
			subSession, err := subEngine.RunSessionWithHook(ctx, subEngine.Config, subReq, nil)
			if err != nil {
				return "", err
			}
			return subSession.Result, nil
		}

		// Otherwise, fallback to LLM call as before
		msg := fmt.Sprintf("%s\n\n%s", worker.Instructions, req.Msg)
		return ae.callLLM(ctx, worker.Name, worker.Model, []openai.ChatCompletionMessage{
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
