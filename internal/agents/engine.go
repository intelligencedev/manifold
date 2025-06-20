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
	tools "manifold/internal/tools"
	"manifold/internal/util"

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

	mcpMgr   *mcp.Manager
	mcpTools map[string]ToolInfo

	a2aClients map[string]*a2aclient.A2AClient

	fleet *Fleet // map of fleet name to Fleet instance
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
		Config:     cfg,
		DB:         db,
		HTTPClient: &http.Client{Timeout: 180 * time.Second},
		mcpMgr:     mgr,
		mcpTools:   make(map[string]ToolInfo),
		a2aClients: make(map[string]*a2aclient.A2AClient),
		fleet:      agentFleet,
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

		session, err := engine.RunSession(ctx, cfg, req)
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
			embeds, err := embeddings.GenerateEmbeddings(ae.Config.Embeddings.Host, ae.Config.Embeddings.APIKey, embedTexts)
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
	return nil
}

func (ae *AgentEngine) RunSession(ctx context.Context, cfg *configpkg.Config, req ReActRequest) (*AgentSession, error) {
	return ae.RunSessionWithHook(ctx, cfg, req, nil)
}

func (ae *AgentEngine) RunSessionWithHook(ctx context.Context, cfg *configpkg.Config, req ReActRequest, hook StepHook) (*AgentSession, error) {
	sess := &AgentSession{ID: uuid.New(), Objective: req.Objective, Created: time.Now()}

	var td []string

	// Append agent fleet
	td = append(td, "Available specialized assistant workers:")
	for _, worker := range ae.fleet.ListWorkers() {
		td = append(td, fmt.Sprintf("- %s (%s) • %s", worker.Name, worker.Role, worker.Instructions))
	}

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
		td = append(td, fmt.Sprintf("- %s • %s", t.Name, t.Description), string(schema))
	}
	td = append(td,
		"- ask_assistant_worker • get help from specialized assistant worker",
		"- code_eval    • run code in sandbox",
		"- web_search   • search the web",
		"- web_fetch    • fetch webpage content",
		"- finish       • end and output final answer",
	)

	sysPrompt := fmt.Sprintf(`You are a helpful assistant in a sandboxed environment with access to various tools. You are the ONLY assistant in your team with access to the tools. The other assistants can only think and reply to your requests for help from your team. You are the ONLY one that makes the tool calls. You are encouraged to get feedback from your team before proceeding with tasks by using the ask_assistant_worker tool.

IMPORTANT: If you get stuck, or detect a loop, ask for assistance from another expert and ensure you give them all of the information necessary for them to help you.

Objective: %s

IMPORTANT: If there is a specialized assistant worker available that can help with the task, you call it with the ask_assistant_worker tool.
Assistants are specialized workers that can help with specific tasks, such as code generation, data analysis, or document analysis.
Assistants can only take your instructions and respond using their knowledge expertise. They cannot execute code or perform actions directly.
- ask_assistant_worker • get help from specialized assistant worker
{"properties":{"name":{"description":"Name of the worker to ask","type":"string"},"model":{"description":"Optional model to use. Leave empty unless explicitly requested.","type":"string"},"msg":{"description":"Message to send to the worker. Detailed task or help query.","type":"string"}},"required":["name","msg"],"type":"object"}

Always use the manifold::cli tool to run commands, and the code_eval tool to run Python code when making your own tools.

The manifold cli tool:
- manifold::cli • Execute a raw CLI command
{"properties":{"command":{"description":"Command string to execute","type":"string"},"dir":{"description":"Optional working directory","type":"string"}},"required":["command"],"type":"object"}

The manifold web search tool:
- web_search • Search the web for information
{"properties":{"query":{"description":"Search query","type":"string"}},"required":["query"],"type":"object"}

Below is a list of available CLI commands using the manifold::cli tool schema you should use.

// ── Listing & navigation ────────────────────────────────────────────────
{"command":"ls -la PATH"}
{"command":"find PATH -type f"}
{"command":"find PATH -type d"}
{"command":"tree PATH"}

// ── Search & pattern matching ───────────────────────────────────────────
{"command":"grep -rn \"PATTERN\" PATH"}
{"command":"find PATH -name \"*.ext\" -exec grep -l \"PATTERN\" {} \\;"}
{"command":"find PATH -name \"FILENAME\""}
{"command":"find PATH -name \"*PATTERN*\""}

// ── File reading & paging ───────────────────────────────────────────────
{"command":"cat FILE"}
{"command":"head -n N FILE"}
{"command":"tail -n N FILE"}
{"command":"less FILE"}
{"command":"sed -n 'START,ENDp' FILE"}

// ── File creation & simple writes ───────────────────────────────────────
{"command":"echo \"TEXT\"  > FILE"}
{"command":"echo \"TEXT\" >> FILE"}
{"command":"cat > FILE <<'EOF'  # …content…  EOF"}
{"command":"touch FILE"}

// ── Precise non-interactive edits ───────────────────────────────────────
{"command":"sed -i 's/OLD/NEW/g' FILE"}
{"command":"sed -i 'Ns/OLD/NEW/' FILE"}
{"command":"sed -i '/PATTERN/s/OLD/NEW/g' FILE"}
{"command":"sed -i '/PATTERN/d' FILE"}
{"command":"awk '{gsub(/OLD/,\"NEW\");print}' FILE > NEWFILE"}

// ── Text manipulation utilities ────────────────────────────────────────
{"command":"cut -d'DELIM' -f N FILE"}
{"command":"awk -F'DELIM' '{print $N}' FILE"}
{"command":"sort FILE"}
{"command":"uniq FILE"}
{"command":"tr 'a-z' 'A-Z' < FILE"}

// ── Basic file operations ───────────────────────────────────────────────
{"command":"cp FILE DEST"}
{"command":"cp -r DIR DEST"}
{"command":"mv FILE DEST"}
{"command":"rm FILE"}
{"command":"rm -rf DIR"}
{"command":"mkdir -p PATH"}
{"command":"chmod +x FILE"}
{"command":"ln -s TARGET LINK"}

// ── Line/chunk utilities ────────────────────────────────────────────────
{"command":"nl FILE"}
{"command":"wc -l FILE"}
{"command":"split -l N FILE PREFIX"}

// ── Backup & versioning ────────────────────────────────────────────────
{"command":"cp FILE FILE.bak"}
{"command":"diff FILE1 FILE2"}
{"command":"patch FILE < PATCHFILE"}

// ── Process control & chaining ──────────────────────────────────────────
{"command":"COMMAND1 && COMMAND2"}
{"command":"COMMAND1 || COMMAND2"}
{"command":"COMMAND1 | COMMAND2"}

// ── File information ────────────────────────────────────────────────────
{"command":"stat FILE"}
{"command":"file FILE"}
{"command":"du -sh PATH"}

// ── Pattern-based batch operations ──────────────────────────────────────
{"command":"find PATH -name \"*.ext\" -delete"}
{"command":"for f in *.txt; do mv \"$f\" \"${f%%.txt}.bak\"; done"}

// ── Text analysis helpers ───────────────────────────────────────────────
{"command":"grep -c \"PATTERN\" FILE"}
{"command":"grep -v \"PATTERN\" FILE"}
{"command":"grep -A N -B N \"PATTERN\" FILE"}
{"command":"grep -E \"REGEX\" FILE"}
{"command":"awk '/PATTERN/{print NR\": \"$0}' FILE"}

Rules:
- Never invoke interactive editors (vim, nano, etc.).  
- Keep patches minimal; do not reformat entire files unless required.  
- Track and reference original line numbers in your reasoning.  
- For non-code text, skip language checkers but still diff/patch/verify.
- Never use search engines or external APIs directly; use the web_search tool instead.

Prefer to answer directly (with Thought + finish) for narrative tasks
such as writing, explaining, or summarising natural-language text.
Only fall back to a tool for *computational* or *programmatic*
work (e.g. data transformation, heavy math, file parsing).

Always consider using the tools first. If no tool is available that can be used to complete the task, make your own using the code_eval tool.
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

		out, err := ae.callLLM(ctx, "", model, currentMessages)
		if err != nil {
			return nil, err
		}
		thought, action, input := parseReAct(out)

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

func (ae *AgentEngine) callLLM(ctx context.Context, assistantName string, model string, msgs []completions.Message) (string, error) {

	fleet := ae.fleet
	assistant := fleet.GetWorker(assistantName)
	if assistant == nil {
		assistant = &configpkg.FleetWorker{
			Name:         "default",
			Role:         "assistant",
			Endpoint:     ae.Config.Completions.DefaultHost,
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

	var body []byte

	if ae.Config.Completions.Backend != "mlx" {
		body, _ = json.Marshal(completions.CompletionRequest{Model: model, Messages: msgs, Temperature: ae.Config.Completions.Temperature})
	} else {
		body, _ = json.Marshal(completions.CompletionRequest{Messages: msgs, Temperature: ae.Config.Completions.Temperature, MaxTokens: maxTokens})
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", assistant.Endpoint, bytes.NewBuffer(body))
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

		msg := fmt.Sprintf("%s\n\n%s", worker.Instructions, req.Msg)

		return ae.callLLM(ctx, worker.Name, worker.Model, []completions.Message{
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
		return "", fmt.Errorf(resp.Error)
	}
	return resp.Result, nil
}

func (ae *AgentEngine) runWebSearch(_ context.Context, arg string, cfg *configpkg.Config) (string, error) {
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
		urls = tools.GetSearXNGResults(cfg.Tools.Search.Endpoint, req.Query)
	} else {
		urls = tools.SearchDDG(req.Query)
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

	return tools.GetSearchResults(urls), nil
}

func (ae *AgentEngine) runWebFetch(_ context.Context, arg string) (string, error) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(arg), &req); err != nil {
		req.URL = strings.TrimSpace(arg)
	}
	if req.URL == "" {
		return "", fmt.Errorf("url required")
	}
	pg, err := tools.WebGetHandler(req.URL)
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
