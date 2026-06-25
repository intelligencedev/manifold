# Code Mode Implementation Plan for Manifold

## Executive Summary

Code mode replaces the traditional tool-calling pattern — where every tool's JSON schema is loaded into the LLM context window and each tool call round-trips through the model — with a single `execute_code` meta-tool. The model writes Python code against a generated typed SDK representing all available tools. A sandboxed supervisor executes that code, dispatching tool calls through Manifold's existing `tools.Registry` and returning only the final result to the LLM.

This saves context tokens (one tool schema instead of N), eliminates intermediate tool-result messages from the conversation, enables multi-step tool composition without LLM round-trips, and keeps secrets out of the model's reach.

---

## How Manifold Currently Works (Relevant Parts)

### Agent Engine Loop (`internal/agent/engine_loop.go`)

```
for each step:
    schemas = e.Tools.Schemas()           // ← ALL tool schemas, every step
    msg = e.LLM.Chat(ctx, msgs, schemas)   // ← schemas consume context tokens
    if msg has tool calls:
        results = e.dispatchToolsAtStep(toolCalls)  // ← each result appended to msgs
        // results stay in context for next LLM call
    else:
        return msg.Content  // final answer
```

**Problem**: With N MCP tools, each schema is ~200-500 tokens. 50 tools = 10K-25K tokens of schema overhead per step. Tool results also accumulate in context.

### Tool Registry (`internal/tools/types.go`)

```go
type Tool interface {
    Name() string
    JSONSchema() map[string]any
    Call(ctx context.Context, raw json.RawMessage) (any, error)
}

type Registry interface {
    Schemas() []llm.ToolSchema
    Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error)
    Register(t Tool)
    Unregister(name string)
}
```

Implementations: `defaultRegistry`, `filteredRegistry`, `overlayRegistry`, `DiscoverableRegistry`.

### MCP Client (`internal/mcpclient/`)

- `Manager` connects to MCP servers (stdio or Streamable HTTP)
- Each MCP tool is wrapped as `mcpTool` implementing `tools.Tool`
- Tool names: `<server>_<tool_name>` (sanitized)
- `JSONSchema()` converts MCP `InputSchema` → OpenAI-compatible schema
- `Call()` invokes `session.CallTool()` and normalizes results
- `MCPServerPool` manages shared + per-user (path-dependent) sessions

### LLM Provider (`internal/llm/provider.go`)

```go
type Provider interface {
    Chat(ctx context.Context, msgs []Message, tools []ToolSchema, model string) (Message, error)
    ChatStream(ctx context.Context, msgs []Message, tools []ToolSchema, model string, h StreamHandler) error
}
```

### Command Execution & Sandbox (`internal/commandexec/`, `internal/sandbox/`)

- Policy-based approval: `allow`, `ask`, `deny` decisions per command
- Platform sandboxing: `sandbox_darwin.go` (seatbelt), `sandbox_linux.go` (seccomp)
- Path sanitization via `sandbox.SanitizeArg`
- Environment isolation: `secureEnv()` strips secrets, sets minimal PATH
- Workdir confinement via `sandbox.ResolveBaseDir`

### Tool Discovery (`internal/tools/discovery/`)

`DiscoverableRegistry` already partially addresses context bloat:
- Exposes only bootstrap tools + a `tool_search` meta-tool
- Promotes additional tools on demand when the model searches for them
- Caps total discovered tools via `maxDiscovered`

This is adjacent to code mode but still uses the JSON tool-call protocol.

### Flow System (`internal/flow/`)

DAG workflow execution that composes multiple tool calls without LLM round-trips. Nodes dispatch tools, results flow through edges. Saved workflows become tools themselves (`warpptool`). This is conceptually similar to what code mode achieves with imperative code.

### Config (`internal/config/config.go`)

Relevant fields: `EnableTools`, `ToolAllowList`, `AutoDiscover`, `MaxDiscoveredTools`, `MCP MCPConfig`, `Exec ExecConfig`.

---

## Implementation Plan

### New Package: `internal/codemode/`

#### 1. SDK Generator (`sdkgen.go`)

Generates a Python SDK stub from the tool registry's schemas. Each tool becomes a typed Python function with docstrings and type annotations derived from the JSON schema.

**Input**: `[]llm.ToolSchema` from `Registry.Schemas()`
**Output**: Python source code string (the SDK module)

```python
# Generated SDK example:
"""
Manifold Tool SDK — auto-generated.
All calls are dispatched through the Manifold supervisor.
Secrets are injected at call time; never hardcode them.
"""

from typing import Any
import manifold_rpc

def brave_brave_web_search(query: str, count: int = 10) -> dict[str, Any]:
    """Performs web searches using the Brave Search API.
    
    Args:
        query: Search query (max 400 chars, 50 words)
        count: Number of results (1-20, default 10)
    """
    return manifold_rpc.call("brave_brave_web_search", {
        "query": query,
        "count": count,
    })

def file_read(path: str, paths: list[str] | None = None) -> dict[str, Any]:
    """Read one or more files from the project workspace."""
    return manifold_rpc.call("file_read", {
        "path": path,
        "paths": paths,
    })

# ... one function per registered tool
```

**Design decisions**:
- Python as the primary language (most common for LLM code generation, rich typing)
- `manifold_rpc` is a minimal shim module provided by the executor (see below)
- Secrets/tokens are never in the SDK — the supervisor injects them at dispatch time
- Function names match the existing tool names (e.g., `brave_brave_web_search`)
- JSON Schema → Python type mapping: `string`→`str`, `integer`→`int`, `number`→`float`, `boolean`→`bool`, `array`→`list`, `object`→`dict`
- Optional properties get default `None`
- Required properties have no default
- Descriptions become docstrings

#### 2. Code Executor (`executor.go`)

The supervisor that runs the model's Python code and dispatches tool calls.

**Architecture**:

```
┌─────────────────────────────────────────────────────────┐
│  Manifold Engine (Go)                                   │
│                                                         │
│  ┌──────────┐     ┌──────────────────────────────────┐  │
│  │  Engine  │────▶│  CodeModeExecutor                │  │
│  │  Loop    │     │                                  │  │
│  │          │     │  1. Generate SDK from schemas    │  │
│  │          │     │  2. Start Python subprocess      │  │
│  │          │     │     (sandboxed via commandexec)  │  │
│  │          │     │  3. Write SDK + user code         │  │
│  │          │     │  4. Read JSON-RPC calls on stdout│  │
│  │          │     │  5. Dispatch to tools.Registry   │  │
│  │          │     │  6. Write results to stdin       │  │
│  │          │     │  7. Collect stdout = result      │  │
│  │          │◀────│  8. Return result to engine      │  │
│  └──────────┘     └──────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**JSON-RPC Protocol** (over stdin/stdout pipes):

```jsonl
// Python → Go (stdout): tool call request
{"jsonrpc":"2.0","id":1,"method":"call","params":{"name":"brave_brave_web_search","args":{"query":"test"}}}

// Go → Python (stdin): tool call response
{"jsonrpc":"2.0","id":1,"result":{"ok":true,"text":"...","structured":{...}}}
```

**`manifold_rpc` shim** (embedded in the Python runtime):

```python
# manifold_rpc.py — provided by executor, not generated
import json, sys

def call(tool_name: str, args: dict) -> dict:
    request = {
        "jsonrpc": "2.0",
        "id": _next_id(),
        "method": "call",
        "params": {"name": tool_name, "args": args}
    }
    sys.stdout.write(json.dumps(request) + "\n")
    sys.stdout.flush()
    line = sys.stdin.readline()
    response = json.loads(line)
    if "error" in response:
        raise RuntimeError(response["error"]["message"])
    return response["result"]
```

**Executor struct**:

```go
type Executor struct {
    Registry    tools.Registry   // real registry for dispatching tool calls
    PolicyEnforcer policy.Enforcer  // reuse existing policy enforcement
    Workdir     string           // sandboxed workdir for code execution
    PythonPath  string           // path to python binary
    Timeout     time.Duration    // execution timeout
    MaxOutputBytes int           // cap stdout capture
    
    // Reuses commandexec infrastructure
    ExecConfig  config.ExecConfig
}

func (ex *Executor) Execute(ctx context.Context, code string, env map[string]string) (*ExecutionResult, error)
```

**ExecutionResult**:

```go
type ExecutionResult struct {
    Stdout   string        // captured stdout (final result or print output)
    Stderr   string        // captured stderr (errors, warnings)
    ExitCode int
    ToolCalls []ToolCallRecord  // log of all tool calls made during execution
    Duration  time.Duration
}
```

**Key behaviors**:
- Reuses `commandexec.Prepare()` + sandbox wrapping for the Python subprocess
- Secrets from MCP server configs are injected into the Go process's dispatch context, never into the Python environment
- Tool calls go through `Registry.Dispatch()` which applies the same policy enforcement as normal tool calls
- Timeout enforcement via context cancellation + process kill
- Output truncation via `MaxOutputBytes`

#### 3. Code Mode Registry (`registry.go`)

Wraps the real registry and presents `execute_code` plus passthrough schemas for excluded tools (e.g., delegation tools) to the LLM.

```go
type CodeModeRegistry struct {
    real     tools.Registry     // underlying registry with all real tools
    executor *Executor
    config   CodeModeConfig
    
    // Cached SDK source (regenerated when tools change)
    sdkCache     string
    sdkCacheMu   sync.RWMutex
    sdkVersion   int64
}

func NewCodeModeRegistry(real tools.Registry, executor *Executor, cfg CodeModeConfig) *CodeModeRegistry

// Schemas returns execute_code plus passthrough schemas for excluded tools
// Schemas returns execute_code plus passthrough schemas for excluded tools
// (e.g., agent_call, ask_agent, delegate_to_team) that must remain callable
// as direct tool calls by the model.
func (r *CodeModeRegistry) Schemas() []llm.ToolSchema {
    schemas := []llm.ToolSchema{{
        Name: "execute_code",
        Description: "Execute Python code to interact with available tools. " +
            "The code has access to a typed SDK for all registered tools. " +
            "Tool calls are dispatched through the Manifold supervisor. " +
            "Write code that calls the SDK functions and return results via print() or the last expression.",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "code": map[string]any{
                    "type": "string",
                    "description": "Python code to execute. Use the SDK functions to call tools.",
                },
            },
            "required": []string{"code"},
        },
    }}
    // Append schemas for excluded tools so the model can call them directly.
    // This is required because the engine only sees schemas returned by Schemas();
    // if a tool's schema is absent, the model cannot invoke it — even if Dispatch
    // would pass it through. Delegation tools in particular are intercepted by the
    // engine (executeToolCallPayload → canHandleNestedDelegation) BEFORE reaching
    // Registry.Dispatch, so their schemas must be surfaced here.
    for _, s := range r.real.Schemas() {
        if r.isExcluded(s.Name) {
            schemas = append(schemas, s)
        }
    }
    return schemas
}

// Dispatch handles execute_code calls by running through the executor
func (r *CodeModeRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
    if name == "execute_code" {
        return r.executeCode(ctx, raw)
    }
    // Pass through to real registry for non-code tools (e.g., agent_call, ask_agent).
    // Note: delegation tools (agent_call, ask_agent, delegate_to_team) are intercepted
    // by the engine via canHandleNestedDelegation BEFORE Dispatch is called, so this
    // passthrough is a safety net for any other excluded tools that are not delegation.
    return r.real.Dispatch(ctx, name, raw)
}

// Register and Unregister delegate to the real registry so CodeModeRegistry
// satisfies the full tools.Registry interface.
func (r *CodeModeRegistry) Register(t tools.Tool)      { r.real.Register(t) }
func (r *CodeModeRegistry) Unregister(name string)     { r.real.Unregister(name) }

// isExcluded returns true if a tool should remain as a direct tool call
// (not wrapped in the code SDK). Delegation tools are always excluded.
func (r *CodeModeRegistry) isExcluded(name string) bool {
    for _, ex := range r.config.ExcludeTools {
        if ex == name {
            return true
        }
    }
    // Default exclusion: agent delegation tools require engine-level handling
    // (nested engine loops via canHandleNestedDelegation) and cannot be invoked
    // from Python code.
    switch name {
    case "agent_call", "ask_agent", "delegate_to_team":
        return true
    }
    return false
}
```

**Important**: Agent delegation tools (`agent_call`, `ask_agent`, `delegate_to_team`) must remain as direct tool calls, not go through code execution. They require engine-level handling (nested engine loops via `canHandleNestedDelegation`) that cannot be done from Python code. Their schemas are included in `Schemas()` output (see above) so the model can invoke them directly alongside `execute_code`. The engine intercepts delegation tool calls in `executeToolCallPayload` via `canHandleNestedDelegation(tc.Name)` **before** `Registry.Dispatch` is reached, so `Dispatch` passthrough for these tools is a safety net, not the primary path.

> **Future consideration**: The existing `NestedToolDispatcher` hook (`tools.WithNestedToolDispatcher` / `internal/tools/multitool/parallel.go`) provides a mechanism that could allow delegation-from-code if wired into the executor's dispatch loop. This is out of scope for the initial implementation but should be noted as a potential enhancement path.

#### 4. Prompt Builder (`prompt.go`)

Generates the system prompt section that instructs the model to use code mode.

```go
func BuildCodeModePrompt(sdkSource string, cfg CodeModeConfig) string
```

The prompt includes:
- The generated SDK source (so the model knows the available API)
- Instructions: "Write Python code using the provided SDK functions. All tool calls go through the supervisor."
- Constraint: "Do not attempt to access secrets, tokens, or environment variables directly."
- Guidance: "Use print() to return results. The last printed value or expression result is captured."
- Error handling: "If a tool call fails, handle the error in your code (try/except) and decide what to do."

**Token savings**: The SDK source is much smaller than N JSON schemas because:
- Python function signatures are more compact than JSON Schema objects
- No nested `properties`, `types`, `required` arrays
- Docstrings are shorter than full JSON Schema descriptions
- One `execute_code` schema replaces N individual schemas in the LLM request

#### 5. Secret Injection (`secrets.go`)

Handles injecting credentials into tool calls at dispatch time.

```go
type SecretInjector struct {
    mcpConfig config.MCPConfig
    envVars   map[string]string
}

// InjectSecretsIntoContext attaches MCP server credentials to the dispatch
// context so that mcpTool.Call() can use them. The Python code never sees
// these values.
func (si *SecretInjector) InjectSecretsIntoContext(ctx context.Context, toolName string) context.Context
```

This reuses the existing MCP client's env injection — the `mcpTool` already gets its credentials from the MCP server config, not from the tool call args. The Python code simply calls `brave_brave_web_search(query="...")` and the Go-side dispatch handles authentication.

---

### Config Addition (`internal/config/config.go`)

```go
type CodeModeConfig struct {
    Enabled       bool   `yaml:"enabled" json:"enabled"`
    Language      string `yaml:"language" json:"language"`           // "python" (default), future: "javascript"
    PythonPath    string `yaml:"pythonPath" json:"pythonPath"`       // override python binary path
    TimeoutSeconds int   `yaml:"timeoutSeconds" json:"timeoutSeconds"` // default: 30
    MaxOutputBytes int   `yaml:"maxOutputBytes" json:"maxOutputBytes"` // default: 1MB
    MaxToolCalls   int   `yaml:"maxToolCalls" json:"maxToolCalls"`     // cap tool calls per execution, default: 50
    // ExcludeTools lists tool names that should remain as direct tool calls
    // (not available in the code SDK). Useful for agent_call, ask_agent, etc.
    ExcludeTools []string `yaml:"excludeTools" json:"excludeTools"`
    // IncludeTools explicitly lists which tools to expose in the SDK.
    // If empty, all non-excluded tools are included.
    IncludeTools []string `yaml:"includeTools" json:"includeTools"`
    // AutoAllowInterpreter, if true, instructs the CodeModeExecutor to register
    // a policy rule at startup that auto-allows the configured interpreter binary
    // (PythonPath or default "python"). Without this, the exec policy may block
    // every execute_code call with an "ask" or "deny" decision.
    AutoAllowInterpreter bool `yaml:"autoAllowInterpreter" json:"autoAllowInterpreter"`
}
```

Add to `Config`:
```go
type Config struct {
    // ... existing fields ...
    CodeMode CodeModeConfig `yaml:"codeMode" json:"codeMode"`
}
```

---

### Engine Integration (`internal/agent/`)

#### Engine struct addition (`engine.go`):

```go
type Engine struct {
    // ... existing fields ...
    CodeMode *codemode.CodeModeRegistry  // nil = traditional tool calling
}
```

#### Engine loop modification (`engine_loop.go`):

The core change is in `runLoop` and `runStreamStep`:

```go
// Before (existing):
schemas := e.Tools.Schemas()

// After (code mode):
var schemas []llm.ToolSchema
if e.CodeMode != nil {
    schemas = e.CodeMode.Schemas()  // returns [execute_code] + excluded tool schemas
} else {
    schemas = e.Tools.Schemas()     // traditional: all tool schemas
}
```

When the model calls `execute_code`, the existing `dispatchToolsAtStep` → `executeToolCallPayload` → `Tools.Dispatch` flow works unchanged because `CodeModeRegistry` implements `tools.Registry` and intercepts `execute_code`.

**Minimal engine change**: The engine doesn't need to know about code mode internally. It sees a registry that exposes `execute_code` plus any excluded (passthrough) tool schemas. The `CodeModeRegistry` handles the rest.

#### agentd wiring (`cmd/agentd/main.go` or wherever the engine is assembled):

```go
if cfg.CodeMode.Enabled {
    executor := codemode.NewExecutor(realRegistry, cfg.Exec, cfg.CodeMode)
    codeModeReg := codemode.NewCodeModeRegistry(realRegistry, executor, cfg.CodeMode)
    engine.CodeMode = codeModeReg
    engine.Tools = codeModeReg  // replace the registry the engine sees
}
```

---

### Streaming Considerations

Code mode doesn't fundamentally change streaming. The model still streams an assistant message that contains a tool call to `execute_code`. The difference is:
- The tool call args contain Python code (potentially long)
- Execution may take longer (multiple internal tool calls)
- `OnToolStart` fires for `execute_code` (not for individual internal calls)
- `OnTool` fires when execution completes with the full result

For streaming progress during code execution, the executor can emit incremental events (tool call started/completed) through a callback that maps to `OnDelta` or a new `OnCodeExecutionProgress` callback.

---

### Policy Enforcement

The existing `PolicyEnforcer` is applied in `executeToolCallPayload` → `evaluateToolPolicy`. With code mode:

1. The `execute_code` meta-tool itself is evaluated by policy (can be allowed/denied)
2. Each internal tool call dispatched by the executor also goes through `evaluateToolPolicy` because `CodeModeRegistry.Dispatch` calls `real.Dispatch` which applies policy

This means policies are enforced at two levels:
- Macro: "is this agent allowed to execute code?"
- Micro: "is this agent allowed to call `brave_brave_web_search`?"

**⚠️ Interpreter policy**: Starting the Python subprocess goes through `commandexec.Prepare()`, which calls the policy enforcer on the command (`python` or the configured `PythonPath`). The policy decision (`allow`, `ask`, `deny`) is evaluated per-command via `validateBinaryName` and the configured `ExecConfig` policy rules. **If the default policy does not explicitly `allow` the Python interpreter, code mode will block on every `execute_code` call** — the user will be prompted (in `ask` mode) or denied (in `deny` mode).

**Required configuration**: When code mode is enabled, the exec policy MUST explicitly allow the configured interpreter binary. This can be done by:
- Adding the interpreter path to the exec allow-list in `ExecConfig`
- Or setting a policy rule that auto-allows the code-mode interpreter binary
- Or the `CodeModeExecutor` pre-validates policy at startup and fails fast with a clear error if the interpreter is not allowed

Without this, the "fall back to traditional mode" mitigation in the risks table is insufficient — the system would silently degrade or hang on every tool call.

---

### Checkpoint/Resume Compatibility

The existing checkpoint system (`engine_checkpoint.go`) checkpoints tool results per tool-call using `toolCheckpointKey(step, tc.ID)`. With code mode:
- The assistant message containing `execute_code` is checkpointed normally
- The tool result (execution output) is checkpointed normally under the `execute_code` call ID
- Internal tool calls within the code execution are **NOT checkpointed individually** — they are encapsulated in the single `execute_code` result

**⚠️ Known regression vs traditional checkpointing**: In traditional mode, each tool call has its own checkpoint (`toolCheckpointKey(step, tc.ID)` per call). If a run crashes and resumes, only the un-checkpointed tool call is replayed. With code mode, a single `execute_code` call is one checkpoint — if a crash occurs mid-execution, the **entire** code block re-runs from scratch, replaying all internal tool calls. This is **not safe for non-idempotent tools**: `file_write`, web POST requests, `agent_call` (if invoked from code), and any tool with side effects will execute again on resume.

**Mitigations** (must be addressed before production use):
1. **Idempotency tracking**: The executor should log all internal tool calls with their results. On resume, if the checkpointed `execute_code` result exists, skip re-execution entirely (the cached result is replayed). This works because the checkpoint is keyed at the `execute_code` level — if the checkpoint exists, execution completed successfully.
2. **Partial execution recovery**: If the checkpoint does NOT exist (crash during execution), the executor must either:
   - Re-run the full code block (accepting potential duplicate side effects), or
   - Detect which internal calls already completed (via an internal execution log) and skip them.
3. **Recommendation**: For Phase 1 (MVP), document that a crash during `execute_code` execution will replay all internal calls. Phase 2 should implement internal call logging to enable selective replay.
4. **Safe-by-default tools**: The SDK should annotate which tools are read-only (idempotent) vs side-effecting, so the executor can warn the model or enforce ordering constraints.

---

### Security Model

1. **Secrets isolation**: MCP server credentials (API keys, bearer tokens) are stored in the Go process's `MCPServerConfig` and injected into MCP sessions at connection time. The Python code never sees them — it calls SDK functions that dispatch to the Go-side registry, which uses the pre-authenticated MCP sessions.

2. **Sandbox reuse**: The Python subprocess runs through `commandexec.Prepare()` which applies:
   - Seatbelt (macOS) / seccomp (Linux) sandboxing
   - Path confinement to the project workdir
   - Environment isolation (no secrets in env)
   - Network restrictions (configurable)

3. **Resource limits**:
   - Execution timeout (default 30s)
   - Max output bytes (default 1MB)
   - Max tool calls per execution (default 50)
   - Max code size (enforced by LLM output limits)

4. **Policy enforcement**: Every tool call within the code goes through the same `PolicyEnforcer` as normal tool calls.

---

### Relationship to Existing Systems

| System | Relationship |
|--------|-------------|
| **Tool Discovery** | Code mode supersedes discovery for context management. When code mode is enabled, the SDK includes all tools (or a configured subset). Discovery's lazy-loading is unnecessary because the SDK is compact. However, discovery can still be used to limit which tools appear in the SDK. |
| **Flow System** | Flows and code mode are complementary. Flows are visual, pre-defined DAGs. Code mode is imperative, model-generated. Both avoid LLM round-trips for multi-step tool composition. A flow node could theoretically use code mode for dynamic sub-steps. |
| **WARPP Tools** | WARPP tools (saved workflows as tools) work normally in code mode — they appear as SDK functions and dispatch through the registry like any other tool. |
| **Agent Delegation** | `agent_call`, `ask_agent`, `delegate_to_team` are excluded from the code SDK and remain as direct tool calls. They require engine-level handling (`canHandleNestedDelegation` → nested engine loops) that cannot be done from Python code. Their schemas are surfaced by `CodeModeRegistry.Schemas()` alongside `execute_code` so the model can invoke them directly. The engine intercepts delegation calls in `executeToolCallPayload` **before** `Registry.Dispatch` is reached, so `Dispatch` passthrough is a safety net, not the primary path. |

---

### Implementation Phases

#### Phase 1: Core (MVP)
1. `internal/codemode/sdkgen.go` — SDK generator from tool schemas
2. `internal/codemode/executor.go` — Python subprocess executor with JSON-RPC dispatch
3. `internal/codemode/registry.go` — CodeModeRegistry wrapping real registry
4. `internal/codemode/prompt.go` — System prompt builder
5. Config addition: `CodeModeConfig`
6. agentd wiring: conditionally use CodeModeRegistry
7. Tests: unit tests for SDK generation, executor dispatch, registry passthrough

#### Phase 2: Hardening
1. `internal/codemode/secrets.go` — Explicit secret injection verification
2. Policy enforcement integration tests (including interpreter auto-allow verification)
3. Streaming progress callbacks (`OnCodeExecutionProgress`)
4. Error handling: Python tracebacks → structured error messages for the LLM
5. Timeout enforcement with process cleanup
6. Output truncation and formatting
7. **Internal call logging for checkpoint/resume**: Log each internal tool call with its result during `execute_code` execution. On resume after a crash (checkpoint missing), use the log to skip already-completed calls and avoid duplicate side effects on non-idempotent tools.
8. **Idempotency annotations**: Annotate SDK functions as read-only vs side-effecting so the executor can warn or enforce ordering.
1. SDK caching (only regenerate when tools change)
2. Pre-warm Python subprocess pool
3. SDK size optimization: group related tools, omit unused properties
4. Metrics: track token savings, execution time, tool call count per execution
5. Support for additional languages (JavaScript/TypeScript via Node.js)

#### Phase 4: Advanced
1. Persistent Python runtime (avoid subprocess startup cost)
2. Stateful code execution (variables persist across `execute_code` calls within a run)
3. Direct MCP protocol bridge (Python code speaks MCP directly, bypassing Go dispatch)
4. Visual code execution viewer in the UI

---

### File Inventory

New files:
```
internal/codemode/
├── sdkgen.go           # SDK generator
├── sdkgen_test.go      # SDK generation tests
├── executor.go         # Python subprocess executor
├── executor_test.go    # Executor tests
├── registry.go         # CodeModeRegistry
├── registry_test.go    # Registry tests
├── prompt.go           # System prompt builder
├── prompt_test.go      # Prompt builder tests
├── secrets.go          # Secret injection helpers
├── types.go            # Shared types (ExecutionResult, ToolCallRecord, etc.)
├── rpc_shim.py         # Embedded manifold_rpc.py shim
└── sdk_template.py     # SDK module template
```

Modified files:
```
internal/config/config.go          # Add CodeModeConfig
internal/agent/engine.go           # Add CodeMode field
cmd/agentd/main.go                 # Wire CodeModeRegistry when enabled
config.yaml.example                # Document codeMode section
docs/codemode.md                   # User documentation
```

---

### Token Savings Estimate

> **⚠️ Illustrative only**: The following figures are unverified estimates. They have not been measured against the actual Manifold codebase and should not be cited as facts. Token consumption varies widely based on schema complexity, LLM provider, result sizes, and model behavior. Real measurements should be taken during Phase 2 hardening.

| Scenario | Traditional (illustrative) | Code Mode (illustrative) | Savings |
|----------|------------|-----------|---------|
| 10 tools, 5 steps | ~50K tokens (schemas × steps) + results | ~3K (SDK once) + ~5K (5 execute_code calls) | ~80% |
| 50 tools, 10 steps | ~250K tokens (schemas × steps) + results | ~8K (SDK once) + ~10K (10 execute_code calls) | ~90% |
| 100 tools, 20 steps | ~1M tokens (schemas × steps) + results | ~15K (SDK once) + ~20K (20 execute_code calls) | ~95% |

Note: These are illustrative estimates only, not measured values. Actual savings depend on schema complexity, result sizes, and how many internal tool calls each `execute_code` invocation makes (which would otherwise each require an LLM round-trip). Phase 2 should include instrumentation to collect real token-usage data.
---

### Risks and Mitigations

| Risk | Mitigation |
| Python not available in deployment | Check at startup, fall back to traditional mode with a warning. Docker image includes Python. **Additionally, the exec policy must explicitly `allow` the configured interpreter binary** — see Policy Enforcement section. Without this, code mode will block even if Python is installed. |
| Code execution timeout kills long-running operations | Configurable timeout, default 30s. Model learns to write shorter code blocks. |
| Model writes invalid Python | Executor returns structured error with traceback. Model retries on next step. |
| Tool call abuse (model calls tools in infinite loop) | MaxToolCalls cap (default 50). Executor stops and returns error. |
| Secrets leak through error messages | Executor sanitizes error output. MCP errors don't expose credentials. |
| Sandbox escape | Reuses proven commandexec sandbox (seatbelt/seccomp). No new attack surface. |
| LLM provider doesn't support tool calling well | Code mode uses a single tool (`execute_code`), which is the simplest possible tool call. Works with all providers. |
| Checkpoint/resume regression: internal tool calls not individually checkpointed | `execute_code` results are checkpointed at the tool-call level (`toolCheckpointKey(step, tc.ID)`). If the checkpoint exists, resume skips re-execution (cached result is replayed). **However, if a crash occurs DURING execution (before the checkpoint is written), all internal tool calls replay on resume — this is a regression vs traditional per-call checkpointing.** Non-idempotent tools (file_write, web POST, delegation) will have duplicate side effects. Mitigation: Phase 2 should implement internal call logging for selective replay. See Checkpoint/Resume Compatibility section for details. |
| Checkpoint/resume breaks | `execute_code` results are checkpointed as single tool results. Re-execution is safe and idempotent at the checkpoint level. |
