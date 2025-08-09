// main.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

// ---------- configuration ----------

type Config struct {
	APIKey             string
	Model              string
	Workdir            string
	BlockBinaries      map[string]struct{} // empty => block none
	MaxCommandSeconds  int
	OutputTruncateByte int
}

func loadConfig() (*Config, error) {
	// Load .env if present; do not hard-fail if missing (env vars may already be set).
	_ = godotenv.Load()

	cfg := &Config{
		APIKey:             strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		Model:              firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_MODEL")), "gpt-4o-mini"),
		Workdir:            strings.TrimSpace(os.Getenv("WORKDIR")),
		MaxCommandSeconds:  intFromEnv("MAX_COMMAND_SECONDS", 30),
		OutputTruncateByte: intFromEnv("OUTPUT_TRUNCATE_BYTES", 64*1024),
	}

	if cfg.APIKey == "" {
		return nil, errors.New("OPENAI_API_KEY is required (set in .env or environment)")
	}
	if cfg.Workdir == "" {
		return nil, errors.New("WORKDIR is required (set in .env or environment)")
	}

	// Resolve and validate workdir
	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return nil, fmt.Errorf("resolve WORKDIR: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return nil, fmt.Errorf("stat WORKDIR: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("WORKDIR must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD

	// Parse optional blocklist
	blockStr := strings.TrimSpace(os.Getenv("BLOCK_BINARIES"))
	if blockStr != "" {
		cfg.BlockBinaries = make(map[string]struct{})
		for _, b := range strings.Split(blockStr, ",") {
			b = strings.TrimSpace(b)
			if b == "" {
				continue
			}
			if strings.Contains(b, "/") || strings.Contains(b, "\\") {
				return nil, fmt.Errorf("BLOCK_BINARIES must contain bare binary names only (no paths): %q", b)
			}
			cfg.BlockBinaries[b] = struct{}{}
		}
	}

	return cfg, nil
}

// ---------- openai client ----------

func newOpenAIClient(cfg *Config) openai.Client {
	// option.WithAPIKey defaults to env lookup but we set explicitly from cfg.
	return openai.NewClient(option.WithAPIKey(cfg.APIKey))
}

// ---------- CLI tool schema ----------

type runCLIArgs struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	Stdin          string   `json:"stdin,omitempty"`
}

type toolResult struct {
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
	Truncated  bool   `json:"truncated"`
}

// Build the JSON schema for the run_cli tool.
func runCLIFunctionDef(cfg *Config) openai.FunctionDefinitionParam {
	max := cfg.MaxCommandSeconds
	if max <= 0 {
		max = 30
	}
	return openai.FunctionDefinitionParam{
		Name:        "run_cli",
		Description: openai.String("Execute a CLI command in a restricted working directory (no shell, no absolute paths)."),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Binary to execute (bare name only, e.g., 'ls', 'git', 'go'). No absolute/relative path allowed.",
				},
				"args": map[string]any{
					"type":        "array",
					"description": "Optional command arguments. Any path-like arg must be relative to WORKDIR.",
					"items": map[string]any{
						"type": "string",
					},
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Max seconds to allow the command to run (1..%d).", max),
					"minimum":     1,
					"maximum":     max,
				},
				"stdin": map[string]any{
					"type":        "string",
					"description": "Optional standard input to pass to the command.",
				},
			},
			"required": []string{"command"},
		},
	}
}

// ---------- command execution (no shell) ----------

func isPathTraversal(p string) bool {
	clean := filepath.Clean(p)
	return strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || clean == ".."
}

func isAbsoluteOrDrive(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	// Windows drive path like C:\ or C:/...
	if runtime.GOOS == "windows" {
		if len(p) >= 2 && p[1] == ':' {
			return true
		}
	}
	return false
}

// sanitizeArg returns a safe, cleaned argument if it looks like a path.
// It rejects absolute paths and traversal, and ensures the final path
// would remain under WORKDIR when joined.
func sanitizeArg(workdir, arg string) (string, error) {
	// Only attempt to sanitize path-like args.
	if !(strings.Contains(arg, "/") || strings.Contains(arg, `\`) || strings.HasPrefix(arg, ".")) {
		return arg, nil
	}
	if isAbsoluteOrDrive(arg) {
		return "", fmt.Errorf("absolute paths not allowed in args: %q", arg)
	}
	if isPathTraversal(arg) {
		return "", fmt.Errorf("path traversal not allowed in args: %q", arg)
	}
	rel := filepath.Clean(arg)
	target := filepath.Join(workdir, rel)
	target = filepath.Clean(target)

	// Ensure target stays under WORKDIR
	workdirWithSep := workdir
	if !strings.HasSuffix(workdirWithSep, string(os.PathSeparator)) {
		workdirWithSep += string(os.PathSeparator)
	}
	if !(target == workdir || strings.HasPrefix(target, workdirWithSep)) {
		return "", fmt.Errorf("arg escapes WORKDIR: %q", arg)
	}
	// return the cleaned relative path
	return rel, nil
}

func isBinaryBlocked(cmd string, block map[string]struct{}) bool {
	if strings.Contains(cmd, "/") || strings.Contains(cmd, "\\") {
		return true // disallow path-based execution; bare names only
	}
	if len(block) == 0 {
		return false // nothing is blocked
	}
	_, blocked := block[cmd]
	return blocked
}

func runCLI(ctx context.Context, cfg *Config, a runCLIArgs) (toolResult, error) {
	if a.Command == "" {
		return toolResult{}, errors.New("command is required")
	}
	if isBinaryBlocked(a.Command, cfg.BlockBinaries) {
		return toolResult{}, fmt.Errorf("binary is blocked or invalid: %q (adjust BLOCK_BINARIES to permit)", a.Command)
	}
	// Sanitize args that look like paths.
	safeArgs := make([]string, 0, len(a.Args))
	for _, arg := range a.Args {
		s, err := sanitizeArg(cfg.Workdir, arg)
		if err != nil {
			return toolResult{}, err
		}
		safeArgs = append(safeArgs, s)
	}

	// Resolve timeout
	tout := a.TimeoutSeconds
	if tout <= 0 || tout > cfg.MaxCommandSeconds {
		tout = cfg.MaxCommandSeconds
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(tout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.Command, safeArgs...)
	cmd.Dir = cfg.Workdir
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if a.Stdin != "" {
		cmd.Stdin = strings.NewReader(a.Stdin)
	}

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			exit = 124 // conventional timeout code
		} else {
			exit = 1
		}
	}

	// Truncate potentially huge outputs to keep token usage sane
	outS := stdout.String()
	errS := stderr.String()
	trunc := false
	if cfg.OutputTruncateByte > 0 {
		if len(outS) > cfg.OutputTruncateByte {
			outS = outS[:cfg.OutputTruncateByte] + "\n[TRUNCATED]"
			trunc = true
		}
		if len(errS) > cfg.OutputTruncateByte {
			errS = errS[:cfg.OutputTruncateByte] + "\n[TRUNCATED]"
			trunc = true
		}
	}

	return toolResult{
		OK:         err == nil,
		ExitCode:   exit,
		Stdout:     outS,
		Stderr:     errS,
		DurationMS: dur.Milliseconds(),
		Truncated:  trunc,
	}, nil
}

// ---------- agent loop (tool calling via Chat Completions) ----------

func systemPrompt(cfg *Config) string {
	return fmt.Sprintf(`You are a helpful build/ops agent that can execute CLI commands via a single tool: run_cli.

Rules:
- Never assume you have a shell; you cannot use pipelines or redirects. Use command + args only.
- Treat any path-like argument as relative to the locked working directory: %s
- Never use absolute paths or attempt to escape the working directory.
- Prefer short, deterministic commands (avoid interactive prompts).
- After tool calls, summarize actions and results clearly.

When you need to act, call run_cli with:
  { "command": "<binary>", "args": ["<arg1>", "..."], "timeout_seconds": 10 }

Be cautious with destructive operations. If a command could modify files, consider listing files first.`, cfg.Workdir)
}

func runAgent(ctx context.Context, client openai.Client, cfg *Config, userQuery string, maxSteps int) (string, error) {
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt(cfg)),
			openai.UserMessage(userQuery),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(runCLIFunctionDef(cfg)),
		},
		// Use model string from env; the type is an alias so casting is OK.
		Model: openai.ChatModel(cfg.Model),
		// Optional: keep temperature low for determinism
		// Temperature: openai.Float(0.2),
	}

	var finalText string

	for step := 0; step < maxSteps; step++ {
		comp, err := client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("openai call failed on step %d: %w", step, err)
		}
		if len(comp.Choices) == 0 {
			return "", fmt.Errorf("no choices returned at step %d", step)
		}
		assistant := comp.Choices[0].Message
		// Append the assistant message so the tool call has a parent
		params.Messages = append(params.Messages, assistant.ToParam())

		// If tool-calls exist, handle them.
		if len(assistant.ToolCalls) > 0 {
			for _, tc := range assistant.ToolCalls {
				switch tc.Function.Name {
				case "run_cli":
					var args runCLIArgs
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						toolErr := fmt.Sprintf("invalid run_cli args: %v", err)
						params.Messages = append(params.Messages, openai.ToolMessage(toolErr, tc.ID))
						continue
					}

					log.Printf("[run_cli] cmd=%q args=%v timeout=%ds", args.Command, args.Args, args.TimeoutSeconds)
					res, err := runCLI(ctx, cfg, args)
					if err != nil {
						log.Printf("[run_cli] error: %v", err)
					}

					payload, _ := json.MarshalIndent(res, "", "  ")
					params.Messages = append(params.Messages, openai.ToolMessage(string(payload), tc.ID))
				default:
					// Unknown tool: report politely
					msg := fmt.Sprintf("tool %q is not implemented", tc.Function.Name)
					params.Messages = append(params.Messages, openai.ToolMessage(msg, tc.ID))
				}
			}
			// Continue to allow the model to observe the tool outputs and produce a final answer or more tool calls.
			continue
		}

		// No tool calls => final text (or intermediate reasoning). Return it.
		finalText = assistant.Content
		break
	}

	if finalText == "" {
		finalText = "(no final text returned — increase -max-steps or check logs)"
	}
	return finalText, nil
}

// ---------- helpers ----------

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func intFromEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := parseInt(v); err == nil {
			return n
		}
	}
	return def
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ---------- main / flags ----------

func main() {
	query := flag.String("q", "", "User request for the agent (required)")
	maxSteps := flag.Int("max-steps", 8, "Max reasoning/act iterations")
	verbose := flag.Bool("v", false, "Verbose logs")
	flag.Parse()

	if *query == "" {
		fmt.Fprintln(os.Stderr, "Usage: go run . -q \"List files and print README.md if present\" [-max-steps 8]")
		os.Exit(2)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	if !*verbose {
		// minimal logging
		log.SetOutput(os.Stderr)
		log.SetFlags(0)
	} else {
		log.Printf("Model=%s WORKDIR=%s", cfg.Model, cfg.Workdir)
		if len(cfg.BlockBinaries) > 0 {
			var keys []string
			for k := range cfg.BlockBinaries {
				keys = append(keys, k)
			}
			log.Printf("BlockBinaries=%v", keys)
		} else {
			log.Printf("BlockBinaries=NONE (all bare binaries allowed; still no absolute paths)")
		}
		log.Printf("MaxCommandSeconds=%d OutputTruncateBytes=%d", cfg.MaxCommandSeconds, cfg.OutputTruncateByte)
	}

	client := newOpenAIClient(cfg)

	ctx := context.Background()
	answer, err := runAgent(ctx, client, cfg, *query, *maxSteps)
	if err != nil {
		log.Fatalf("agent error: %v", err)
	}

	fmt.Println("\n=== Agent Answer ===")
	fmt.Println(answer)
}
