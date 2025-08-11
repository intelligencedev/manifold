// main.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"net/http"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/ssestream"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"

	"go.opentelemetry.io/contrib/instrumentation/host"
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

// ---------- observability config ----------

type ObsConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string // e.g. http://localhost:4318
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

	// OpenTelemetry / logging defaults from env (can also be read by the SDK)
	// These mirror OTEL_* envs to make local flags explicit if desired.
	obs := ObsConfig{
		ServiceName:    firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), "singularityio"),
		ServiceVersion: strings.TrimSpace(os.Getenv("SERVICE_VERSION")),
		Environment:    firstNonEmpty(os.Getenv("ENVIRONMENT"), "dev"),
		OTLPEndpoint:   strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")), // e.g. http://localhost:4318
	}
	_ = obs // will be consumed in main()

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

// ---------- logging & observability setup ----------

func initLogger() {
	// JSON logging, include timestamps; align with structured logging best practices.
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(os.Stdout).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

// initOTel configures Resource, TracerProvider, MeterProvider, propagators, and HTTP client transport.
// It returns a shutdown function to flush providers.
func initOTel(ctx context.Context, obs ObsConfig) (func(context.Context) error, error) {
	// Build a Resource with env detection + our attributes.
	res, err := resource.New(ctx,
		resource.WithFromEnv(),      // OTEL_RESOURCE_ATTRIBUTES, OTEL_SERVICE_NAME
		resource.WithTelemetrySDK(), // SDK info
		resource.WithProcess(),      // PID, command, etc.
		resource.WithOS(),           // OS info
		resource.WithAttributes(
			semconv.ServiceName(obs.ServiceName),
			semconv.ServiceVersion(obs.ServiceVersion),
			attribute.String("deployment.environment", obs.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("init resource: %w", err)
	}

	// Trace exporter (OTLP/HTTP). Endpoint can be empty and resolved via env.
	trExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(obs.OTLPEndpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("init trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(trExp),
		sdktrace.WithResource(res),
	)

	// Metrics exporter (OTLP/HTTP) + periodic reader.
	mExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(obs.OTLPEndpoint), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("init metrics exporter: %w", err)
	}
	reader := metric.NewPeriodicReader(mExp, metric.WithInterval(10*time.Second))
	mp := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Start host metrics instrumentation
	err = host.Start(host.WithMeterProvider(mp))
	if err != nil {
		return nil, fmt.Errorf("failed to start host metrics: %w", err)
	}

	// Compose shutdown that flushes metrics and traces.
	return func(ctx context.Context) error {
		var firstErr error
		if err := mp.Shutdown(ctx); err != nil {
			firstErr = err
		}
		if err := tp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}, nil
}

// helper to create an HTTP client with otelhttp transport for outbound calls (e.g., OpenAI)
func newObservabilityHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: 60 * time.Second}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	base.Transport = otelhttp.NewTransport(rt)
	return base
}

// ---------- openai client ----------

func newOpenAIClient(cfg *Config) openai.Client {
	httpClient := newObservabilityHTTPClient(&http.Client{Timeout: 60 * time.Second})
	return openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(httpClient),
	)
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
	tracer := otel.Tracer("singularityio/cli")
	meter := otel.Meter("singularityio/cli")
	ctx, span := tracer.Start(ctx, "runCLI")
	defer span.End()

	// metrics instruments (created once per process would be ideal; kept simple here)
	cmdCounter, _ := meter.Int64Counter("cli.commands.total")
	durHist, _ := meter.Int64Histogram("cli.command.duration.ms")

	if a.Command == "" {
		if a.Command != "" {
			span.SetAttributes(attribute.String("cli.command", a.Command))
		}
		return toolResult{}, errors.New("command is required")
	}
	if isBinaryBlocked(a.Command, cfg.BlockBinaries) {
		if a.Command != "" {
			span.SetAttributes(attribute.String("cli.command", a.Command))
		}
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
	cmdCounter.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("command", a.Command)))
	durHist.Record(ctx, dur.Milliseconds(), otelmetric.WithAttributes(attribute.String("command", a.Command)))
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
	span.SetAttributes(
		attribute.String("cli.command", a.Command),
		attribute.Int("cli.exit_code", exit),
		attribute.Int64("cli.duration_ms", dur.Milliseconds()),
	)

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
	tracer := otel.Tracer("singularityio/agent")
	ctx, span := tracer.Start(ctx, "runAgent")
	defer span.End()
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

					log.Info().Str("cmd", args.Command).Interface("args", args.Args).Int("timeout_seconds", args.TimeoutSeconds).Msg("run_cli")
					res, err := runCLI(ctx, cfg, args)
					if err != nil {
						log.Error().Err(err).Msg("run_cli failed")
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

// ---------- interactive TUI (bubbletea) ----------

type tuiModel struct {
	ctx      context.Context
	client   openai.Client
	cfg      *Config
	maxSteps int

	// chat state persisted across turns
	params openai.ChatCompletionNewParams

	// UI
	leftVP    viewport.Model
	rightVP   viewport.Model
	input     textinput.Model
	streaming bool
	step      int

	// current streaming state
	stream *ssestream.Stream[openai.ChatCompletionChunk]
	acc    openai.ChatCompletionAccumulator

	// chat state for rendering
	messages     []chatMsg
	streamingIdx int

	// styles
	userTag                lipgloss.Style
	agentTag               lipgloss.Style
	userText               lipgloss.Style
	agentText              lipgloss.Style
	toolStyle              lipgloss.Style
	infoStyle              lipgloss.Style
	dividerStyle           lipgloss.Style
	headerStyle            lipgloss.Style
	leftHeaderActiveStyle  lipgloss.Style
	rightHeaderActiveStyle lipgloss.Style

	// focus + scroll state
	activePanel   string // "left" or "right"
	userScrolledL bool
	userScrolledR bool
}

type chatMsg struct {
	kind    string // user | agent | tool | info
	title   string
	content string
}

func newTUIModel(ctx context.Context, client openai.Client, cfg *Config, maxSteps int) *tuiModel {
	left := viewport.New(80, 20)
	right := viewport.New(40, 20)
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "Ask the agent..."
	in.Focus()

	// styles
	userTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1).MarginRight(1)
	agentTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1).MarginRight(1)
	toolStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("13")).Padding(0, 1)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	userText := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6F0FF"))
	agentText := lipgloss.NewStyle().Foreground(lipgloss.Color("#ECE7FF"))

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt(cfg)),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(runCLIFunctionDef(cfg)),
		},
		Model: openai.ChatModel(cfg.Model),
	}

	m := &tuiModel{
		ctx:                    ctx,
		client:                 client,
		cfg:                    cfg,
		maxSteps:               maxSteps,
		params:                 params,
		leftVP:                 left,
		rightVP:                right,
		input:                  in,
		messages:               []chatMsg{{kind: "info", title: "", content: "Interactive mode. Type a prompt and press Enter to run. Ctrl+C to exit."}},
		streamingIdx:           -1,
		userTag:                userTag,
		agentTag:               agentTag,
		userText:               userText,
		agentText:              agentText,
		toolStyle:              toolStyle,
		infoStyle:              infoStyle,
		dividerStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		headerStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
		leftHeaderActiveStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1),
		rightHeaderActiveStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1),
		activePanel:            "left",
	}
	m.setView()
	return m
}

func (m *tuiModel) Init() tea.Cmd { return nil }

type chunkMsg struct {
	delta string
}
type streamDoneMsg struct {
	acc openai.ChatCompletionAccumulator
}
type streamErrMsg struct{ err error }

func (m *tuiModel) cleanup() {
	if m.stream != nil {
		_ = m.stream.Close()
		m.stream = nil
	}
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		totalW := msg.Width
		totalH := msg.Height - 3
		if totalH < 5 {
			totalH = 5
		}
		leftW := int(float64(totalW) * 0.65)
		if leftW < 20 {
			leftW = 20
		}
		rightW := totalW - leftW - 1
		if rightW < 20 {
			rightW = 20
		}
		m.leftVP.Width = leftW
		m.rightVP.Width = rightW
		// leave one row for headers above each panel
		panelH := totalH - 1
		if panelH < 1 {
			panelH = 1
		}
		m.leftVP.Height = panelH
		m.rightVP.Height = panelH
		m.setView()
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cleanup()
			return m, tea.Quit
		case tea.KeyTab:
			if m.activePanel == "left" {
				m.activePanel = "right"
			} else {
				m.activePanel = "left"
			}
			return m, nil
		case tea.KeyPgUp:
			if m.activePanel == "left" {
				m.leftVP.PageUp()
				m.userScrolledL = true
			} else {
				m.rightVP.PageUp()
				m.userScrolledR = true
			}
			return m, nil
		case tea.KeyPgDown:
			if m.activePanel == "left" {
				m.leftVP.PageDown()
				m.userScrolledL = !m.leftVP.AtBottom()
			} else {
				m.rightVP.PageDown()
				m.userScrolledR = !m.rightVP.AtBottom()
			}
			return m, nil
		case tea.KeyUp:
			if m.activePanel == "left" {
				m.leftVP.LineUp(1)
				m.userScrolledL = true
			} else {
				m.rightVP.LineUp(1)
				m.userScrolledR = true
			}
			return m, nil
		case tea.KeyDown:
			if m.activePanel == "left" {
				m.leftVP.LineDown(1)
				if m.leftVP.AtBottom() {
					m.userScrolledL = false
				}
			} else {
				m.rightVP.LineDown(1)
				if m.rightVP.AtBottom() {
					m.userScrolledR = false
				}
			}
			return m, nil
		case tea.KeyHome:
			if m.activePanel == "left" {
				m.leftVP.GotoTop()
				m.userScrolledL = true
			} else {
				m.rightVP.GotoTop()
				m.userScrolledR = true
			}
			return m, nil
		case tea.KeyEnd:
			if m.activePanel == "left" {
				m.leftVP.GotoBottom()
				m.userScrolledL = false
			} else {
				m.rightVP.GotoBottom()
				m.userScrolledR = false
			}
			return m, nil
		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			query := strings.TrimSpace(m.input.Value())
			if query == "" {
				return m, nil
			}
			// Append user message
			m.params.Messages = append(m.params.Messages, openai.UserMessage(query))
			m.messages = append(m.messages, chatMsg{kind: "user", title: "You", content: query})
			// Append empty agent message placeholder for streaming
			m.messages = append(m.messages, chatMsg{kind: "agent", title: "Agent", content: ""})
			m.streamingIdx = len(m.messages) - 1
			m.setView()
			m.input.SetValue("")
			m.step = 0
			m.streaming = true
			return m, m.startAssistantStream()
		}
	case chunkMsg:
		if msg.delta != "" {
			if m.streamingIdx >= 0 && m.streamingIdx < len(m.messages) {
				m.messages[m.streamingIdx].content += msg.delta
			}
			m.setView()
		}
		return m, m.readNextChunk()
	case streamDoneMsg:
		// incorporate assistant message
		m.params.Messages = append(m.params.Messages, msg.acc.Choices[0].Message.ToParam())

		// check for tool calls
		if len(msg.acc.Choices) > 0 && len(msg.acc.Choices[0].Message.ToolCalls) > 0 {
			// execute tools
			for _, tc := range msg.acc.Choices[0].Message.ToolCalls {
				if tc.Type == "function" {
					name := tc.Function.Name
					argsJSON := tc.Function.Arguments
					if name == "run_cli" {
						var a runCLIArgs
						if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
							m.params.Messages = append(m.params.Messages, openai.ToolMessage("invalid run_cli args: "+err.Error(), tc.ID))
							continue
						}
						res, err := runCLI(m.ctx, m.cfg, a)
						if err != nil {
							log.Error().Err(err).Msg("run_cli failed")
						}
						payload, _ := json.MarshalIndent(res, "", "  ")
						// styled tool message in TUI
						pretty := formatToolPayload(a, res)
						m.messages = append(m.messages, chatMsg{kind: "tool", title: "Tool: run_cli", content: pretty})
						m.setView()
						m.params.Messages = append(m.params.Messages, openai.ToolMessage(string(payload), tc.ID))
					} else {
						// unknown function
						m.params.Messages = append(m.params.Messages, openai.ToolMessage("tool not implemented", tc.ID))
					}
				}
			}
			// Continue to next assistant step if under maxSteps
			m.step++
			if m.step < m.maxSteps {
				// add a new agent block to stream into
				m.messages = append(m.messages, chatMsg{kind: "agent", title: "Agent", content: ""})
				m.streamingIdx = len(m.messages) - 1
				m.setView()
				return m, m.startAssistantStream()
			}
		} else {
			// final answer (no tools)
			// already rendered
		}
		m.streaming = false
		m.streamingIdx = -1
		return m, nil
	case streamErrMsg:
		m.messages = append(m.messages, chatMsg{kind: "info", title: "", content: "stream error: " + msg.err.Error()})
		m.setView()
		m.streaming = false
		return m, nil
	}

	// default: update input and both viewports (enables mouse/keyboard scrolling)
	var cmdInput, cmdL, cmdR tea.Cmd
	m.input, cmdInput = m.input.Update(msg)
	m.leftVP, cmdL = m.leftVP.Update(msg)
	m.rightVP, cmdR = m.rightVP.Update(msg)
	return m, tea.Batch(cmdInput, cmdL, cmdR)
}

func (m *tuiModel) View() string {
	leftHeader := m.headerStyle.Render(" Chat ")
	rightHeader := m.headerStyle.Render(" Tools ")
	if m.activePanel == "left" {
		leftHeader = m.leftHeaderActiveStyle.Render(" Chat ")
	} else {
		rightHeader = m.rightHeaderActiveStyle.Render(" Tools ")
	}
	leftBlock := leftHeader + "\n" + m.leftVP.View()
	rightBlock := rightHeader + "\n" + m.rightVP.View()
	sep := m.dividerStyle.Render("│")
	top := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, sep, rightBlock)
	return top + "\n" + m.input.View()
}

func (m *tuiModel) startAssistantStream() tea.Cmd {
	// initialize a new stream and accumulator on the model
	m.acc = openai.ChatCompletionAccumulator{}
	m.stream = m.client.Chat.Completions.NewStreaming(m.ctx, m.params)
	return m.readNextChunk()
}

func (m *tuiModel) readNextChunk() tea.Cmd {
	return func() tea.Msg {
		if m.stream == nil {
			return streamErrMsg{errors.New("stream not initialized")}
		}
		if m.stream.Next() {
			ch := m.stream.Current()
			m.acc.AddChunk(ch)
			if len(ch.Choices) > 0 && ch.Choices[0].Delta.Content != "" {
				return chunkMsg{delta: ch.Choices[0].Delta.Content}
			}
			// non-content delta (e.g., tool call)
			return chunkMsg{delta: ""}
		}
		// stream finished
		err := m.stream.Err()
		_ = m.stream.Close()
		m.stream = nil
		if err != nil {
			return streamErrMsg{err}
		}
		return streamDoneMsg{acc: m.acc}
	}
}

func (m *tuiModel) renderChat(width int) string {
	var b strings.Builder
	cnt := 0
	for _, msg := range m.messages {
		if msg.kind == "tool" {
			continue
		}
		if cnt > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.renderMsg(msg, width))
		cnt++
	}
	return b.String()
}

func (m *tuiModel) renderTools(width int) string {
	var b strings.Builder
	cnt := 0
	for _, msg := range m.messages {
		if msg.kind != "tool" {
			continue
		}
		if cnt > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.renderMsg(msg, width))
		cnt++
	}
	if cnt == 0 {
		return m.infoStyle.Render("No tool activity yet.")
	}
	return b.String()
}

func (m *tuiModel) renderMsg(cm chatMsg, width int) string {
	maxw := width
	if maxw < 20 {
		maxw = 20
	}
	wrap := lipgloss.NewStyle().MaxWidth(maxw)
	switch cm.kind {
	case "user":
		header := m.userTag.Render("You")
		body := m.userText.Render(wrap.Render(cm.content))
		return header + "\n" + body
	case "agent":
		header := m.agentTag.Render("Agent")
		body := m.agentText.Render(wrap.Render(cm.content))
		return header + "\n" + body
	case "tool":
		header := lipgloss.NewStyle().Bold(true).Render(cm.title)
		return m.toolStyle.Render(header + "\n" + wrap.Render(cm.content))
	default:
		return m.infoStyle.Render(cm.content)
	}
}

func (m *tuiModel) setView() {
	m.leftVP.SetContent(m.renderChat(m.leftVP.Width))
	m.rightVP.SetContent(m.renderTools(m.rightVP.Width))
	if !m.userScrolledL {
		m.leftVP.GotoBottom()
	}
	if !m.userScrolledR {
		m.rightVP.GotoBottom()
	}
}

func formatToolPayload(args runCLIArgs, res toolResult) string {
	var b strings.Builder
	if args.Command != "" {
		b.WriteString(fmt.Sprintf("$ %s %s\n", args.Command, strings.Join(args.Args, " ")))
	}
	b.WriteString(fmt.Sprintf("exit %d | ok=%v | %dms\n", res.ExitCode, res.OK, res.DurationMS))
	if res.Truncated {
		b.WriteString("(output truncated)\n")
	}
	if strings.TrimSpace(res.Stdout) != "" {
		b.WriteString("\nstdout:\n")
		b.WriteString(res.Stdout)
	}
	if strings.TrimSpace(res.Stderr) != "" {
		b.WriteString("\nstderr:\n")
		b.WriteString(res.Stderr)
	}
	return b.String()
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
	initLogger()

	fmt.Println(`
	███████╗██╗███╗   ██╗ ██████╗ ██╗   ██╗██╗      █████╗ ██████╗ ██╗████████╗██╗   ██╗          ██╗ ██████╗
	██╔════╝██║████╗  ██║██╔════╝ ██║   ██║██║     ██╔══██╗██╔══██╗██║╚══██╔══╝╚██╗ ██╔╝          ██║██╔═══██╗
	███████╗██║██╔██╗ ██║██║  ███╗██║   ██║██║     ███████║██████╔╝██║   ██║    ╚████╔╝  ██████╗  ██║██║   ██║
	╚════██║██║██║╚██╗██║██║   ██║██║   ██║██║     ██╔══██║██╔══██╗██║   ██║     ╚██╔╝   ╚═════╝  ██║██║   ██║
	███████║██║██║ ╚████║╚██████╔╝╚██████╔╝███████╗██║  ██║██║  ██║██║   ██║      ██║             ██║╚██████╔╝
	╚══════╝╚═╝╚═╝  ╚═══╝ ╚═════╝  ╚═════╝╚══════╝╚═╝  ╚═╝╚═╝   ╚═╝╚═╝   ╚═╝      ╚═╝             ╚═╝ ╚═════╝
	`)

	query := flag.String("q", "", "User request for the agent (required unless -interactive)")
	maxSteps := flag.Int("max-steps", 8, "Max reasoning/act iterations")
	verbose := flag.Bool("v", false, "Verbose logs")
	interactive := flag.Bool("interactive", false, "Run in interactive TUI mode (streaming)")
	flag.Parse()

	if !*interactive && *query == "" {
		fmt.Fprintln(os.Stderr, "Usage: go run . -q \"List files and print README.md if present\" [-max-steps 8] or use -interactive")
		os.Exit(2)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("config error")
	}

	// Prepare observability (read from env)
	obs := ObsConfig{
		ServiceName:    firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), "singularityio"),
		ServiceVersion: strings.TrimSpace(os.Getenv("SERVICE_VERSION")),
		Environment:    firstNonEmpty(os.Getenv("ENVIRONMENT"), "dev"),
		OTLPEndpoint:   strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownOTel, err := initOTel(ctx, obs)
	if err != nil {
		log.Warn().Err(err).Msg("failed to initialize OpenTelemetry (continuing without exporters)")
	} else {
		defer func() {
			// Try to flush on exit
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownOTel(timeoutCtx); err != nil {
				log.Warn().Err(err).Msg("error during OpenTelemetry shutdown")
			}
		}()
	}

	if !*verbose {
		// minimal logging
		// log.SetOutput(os.Stderr)
		// log.SetFlags(0)
	} else {
		log.Info().Str("model", cfg.Model).Str("workdir", cfg.Workdir).Msg("startup")
		if len(cfg.BlockBinaries) > 0 {
			var keys []string
			for k := range cfg.BlockBinaries {
				keys = append(keys, k)
			}
			log.Info().Interface("block_binaries", keys).Msg("startup")
		} else {
			log.Info().Str("block_binaries", "NONE (all bare binaries allowed; still no absolute paths)").Msg("startup")
		}
		log.Info().Int("max_command_seconds", cfg.MaxCommandSeconds).Int("output_truncate_bytes", cfg.OutputTruncateByte).Msg("startup")
		if obs.OTLPEndpoint != "" {
			log.Info().Str("otlp_endpoint", obs.OTLPEndpoint).Msg("observability")
		}
	}

	client := newOpenAIClient(cfg)

	if *interactive {
		p := tea.NewProgram(newTUIModel(ctx, client, cfg, *maxSteps), tea.WithContext(ctx), tea.WithMouseAllMotion())
		if _, err := p.Run(); err != nil {
			log.Fatal().Err(err).Msg("interactive error")
		}
		return
	}

	answer, err := runAgent(ctx, client, cfg, *query, *maxSteps)
	if err != nil {
		log.Fatal().Err(err).Msg("agent error")
	}
	log.Info().Str("answer_preview", answer).Msg("agent_answer")
	fmt.Println("\n=== Agent Answer ===")
	fmt.Println(answer)
}
