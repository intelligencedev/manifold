package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"singularityio/internal/agent"
	"singularityio/internal/agent/prompts"
	"singularityio/internal/config"
	"singularityio/internal/llm"
	openai "singularityio/internal/llm/openai"
	"singularityio/internal/mcpclient"
	"singularityio/internal/observability"
	"singularityio/internal/persistence/databases"
	"singularityio/internal/specialists"
	"singularityio/internal/tools"
	"singularityio/internal/tools/cli"
	"singularityio/internal/tools/db"
	"singularityio/internal/tools/fs"
	llmtools "singularityio/internal/tools/llmtool"
	specialists_tool "singularityio/internal/tools/specialists"
	"singularityio/internal/tools/web"
	"singularityio/internal/warpp"
)

type Model struct {
	ctx      context.Context
	provider llm.Provider
	cfg      config.Config

	exec     cli.Executor
	maxSteps int

	// Engine + history
	eng     agent.Engine
	history []llm.Message

	// UI
	leftVP  viewport.Model
	rightVP viewport.Model
	input   textarea.Model

	messages            []chatMsg
	currentMessage      *chatMsg // For streaming content
	currentMessageIndex int      // Track the index of the current streaming message
	running             bool
	toolCh              chan chatMsg
	streamingDeltaCh    chan string

	// demo flags
	warppDemo bool

	// specialists
	specReg *specialists.Registry

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
	leftPanelStyle         lipgloss.Style
	rightPanelStyle        lipgloss.Style
	inputStyle             lipgloss.Style
	spinnerStyle           lipgloss.Style

	activePanel   string // "left" or "right"
	userScrolledL bool
	userScrolledR bool

	// Waiting indicator when LLM/completions endpoint is in-flight
	waitingLLM bool
	spinnerIdx int
	spinners   []string
}

type chatMsg struct {
	kind    string // user | agent | tool | info
	title   string
	content string
}

func NewModel(ctx context.Context, provider llm.Provider, cfg config.Config, exec cli.Executor, maxSteps int, warppDemo bool) *Model {
	left := viewport.New(80, 20)
	right := viewport.New(40, 20)
	// Disable horizontal scrolling; we'll wrap content instead
	left.SetHorizontalStep(0)
	right.SetHorizontalStep(0)
	in := textarea.New()
	in.Placeholder = "Ask the agent..."
	in.SetHeight(3)
	in.ShowLineNumbers = false
	in.Prompt = "› "
	in.Focus()

	userTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1).MarginRight(1)
	agentTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1).MarginRight(1)
	// Don't use a colored border for tool output in the TUI; keep simple padding instead.
	toolStyle := lipgloss.NewStyle().Padding(0, 1)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	userText := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6F0FF"))
	agentText := lipgloss.NewStyle().Foreground(lipgloss.Color("#ECE7FF"))

	// Tool registry
	registry := tools.NewRegistry()
	registry.Register(cli.NewTool(exec))
	registry.Register(web.NewTool(cfg.Web.SearXNGURL))
	registry.Register(web.NewFetchTool())
	registry.Register(fs.NewWriteTool(cfg.Workdir))

	// Database tools
	if mgr, err := databases.NewManager(ctx, cfg.Databases); err == nil {
		registry.Register(db.NewSearchIndexTool(mgr.Search))
		registry.Register(db.NewSearchQueryTool(mgr.Search))
		registry.Register(db.NewSearchRemoveTool(mgr.Search))
		registry.Register(db.NewVectorUpsertTool(mgr.Vector))
		registry.Register(db.NewVectorQueryTool(mgr.Vector))
		registry.Register(db.NewVectorDeleteTool(mgr.Vector))
		registry.Register(db.NewGraphUpsertNodeTool(mgr.Graph))
		registry.Register(db.NewGraphUpsertEdgeTool(mgr.Graph))
		registry.Register(db.NewGraphNeighborsTool(mgr.Graph))
		registry.Register(db.NewGraphGetNodeTool(mgr.Graph))
	}
	// In TUI, build a provider factory with a default HTTP client
	factory := func(baseURL string) llm.Provider {
		c2 := cfg.OpenAI
		c2.BaseURL = baseURL
		hc := observability.NewHTTPClient(nil)
		if len(cfg.OpenAI.ExtraHeaders) > 0 {
			hc = observability.WithHeaders(hc, cfg.OpenAI.ExtraHeaders)
		}
		return openai.New(c2, hc)
	}
	registry.Register(llmtools.NewTransform(provider, cfg.OpenAI.Model, factory))
	// Specialists tool available in TUI as well
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, nil, registry)
	registry.Register(specialists_tool.New(specReg))

	// MCP tools
	mcpMgr := mcpclient.NewManager()
	_ = mcpMgr.RegisterFromConfig(ctx, registry, cfg.MCP)

	// Engine setup (matches cmd/agent wiring)
	eng := agent.Engine{
		LLM:              provider,
		Tools:            registry,
		MaxSteps:         maxSteps,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	m := &Model{
		ctx:                    ctx,
		provider:               provider,
		cfg:                    cfg,
		exec:                   exec,
		maxSteps:               maxSteps,
		eng:                    eng,
		warppDemo:              warppDemo,
		specReg:                specReg,
		leftVP:                 left,
		rightVP:                right,
		input:                  in,
		messages:               []chatMsg{{kind: "info", title: "", content: "Interactive mode. Type a prompt and press Enter to run. Use Tab to switch panes, arrow keys to scroll. Ctrl+C to exit."}},
		userTag:                userTag,
		agentTag:               agentTag,
		userText:               userText,
		agentText:              agentText,
		toolStyle:              toolStyle,
		infoStyle:              infoStyle,
		inputStyle:             lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1),
		spinnerStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("#F6C34E")).Bold(true),
		dividerStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		headerStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
		leftHeaderActiveStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1),
		rightHeaderActiveStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1),
		leftPanelStyle:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1),
		// Tools pane: now styled with a rounded border for visual consistency.
		rightPanelStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7E57C2")).Padding(0, 1),
		activePanel:     "left",
		// spinner frames for waiting indicator
		spinners: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
	// Enable mouse wheel scrolling on viewports. Panel styling is applied
	// when rendering so headers (tabs) can blend into the panel borders.
	m.leftVP.MouseWheelEnabled = true
	m.rightVP.MouseWheelEnabled = true
	m.setView()
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) cleanup() {}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit
		case "tab":
			// Switch focus between left and right panes
			if m.activePanel == "left" {
				m.activePanel = "right"
			} else {
				m.activePanel = "left"
			}
			return m, nil
		case "enter":
			if m.running {
				return m, nil
			}
			q := strings.TrimSpace(m.input.Value())
			if q == "" {
				return m, nil
			}
			m.input.SetValue("")
			m.messages = append(m.messages, chatMsg{kind: "user", title: "You", content: q})
			// Ensure chat pane scrolls to the newest message immediately after user input.
			// Clear the manual-scroll flag so the view will follow new content.
			m.userScrolledL = false
			m.leftVP.GotoBottom()
			m.setView()
			m.running = true
			// start reading events and engine in parallel
			m.toolCh = make(chan chatMsg, 32)
			if m.warppDemo {
				// WARPP demo does not stream tokens; produce a final answer at once
				return m, tea.Batch(m.readNextEvent(), m.runWARPPDemo(q), m.spinnerCmd())
			}
			// Pre-dispatch routing to specialists in TUI
			if name := specialists.Route(m.cfg.SpecialistRoutes, q); name != "" {
				return m, tea.Batch(m.readNextEvent(), m.runSpecialist(name, q), m.spinnerCmd())
			}
			m.streamingDeltaCh = make(chan string, 64)
			// Initialize streaming message and track its index
			m.currentMessage = &chatMsg{kind: "agent", title: "Agent", content: ""}
			m.messages = append(m.messages, *m.currentMessage)
			// Store the index of the streamed message after appending
			m.currentMessageIndex = len(m.messages) - 1
			m.setView()
			// mark we're waiting on LLM/completions
			m.waitingLLM = true
			return m, tea.Batch(m.readNextEvent(), m.readStreamingDelta(), m.runStreamingEngine(q), m.spinnerCmd())
		case "up", "down", "pgup", "pgdn", "home", "end":
			// Handle scrolling for the active pane only
			if m.activePanel == "left" {
				var cmd tea.Cmd
				m.leftVP, cmd = m.leftVP.Update(msg)
				m.userScrolledL = true
				// Don't call setView() during scrolling to avoid rendering artifacts
				return m, cmd
			} else {
				var cmd tea.Cmd
				m.rightVP, cmd = m.rightVP.Update(msg)
				m.userScrolledR = true
				// Don't call setView() during scrolling to avoid rendering artifacts
				return m, cmd
			}
		}
	case tea.MouseMsg:
		// Handle mouse click to focus panes
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			lfW, _ := m.leftPanelStyle.GetFrameSize()
			leftOuter := m.leftVP.Width + lfW
			if msg.X < leftOuter {
				m.activePanel = "left"
			} else {
				m.activePanel = "right"
			}
			return m, nil
		}

		// Let each viewport handle its own mouse events (including wheel)
		var cmdL, cmdR tea.Cmd
		lfW, _ := m.leftPanelStyle.GetFrameSize()
		leftOuter := m.leftVP.Width + lfW
		if msg.X < leftOuter {
			// Mouse is over left pane
			m.leftVP, cmdL = m.leftVP.Update(msg)
			m.userScrolledL = true
			// Don't call setView() during scrolling to avoid rendering artifacts
			return m, cmdL
		} else {
			// Mouse is over right pane
			m.rightVP, cmdR = m.rightVP.Update(msg)
			m.userScrolledR = true
			// Don't call setView() during scrolling to avoid rendering artifacts
			return m, cmdR
		}
	case tea.WindowSizeMsg:
		// Split width 2/3 (chat) and 1/3 (tools)
		// No divider column between panels (sepCols = 0)
		sepCols := 0
		total := msg.Width - sepCols
		if total < 2 {
			total = 2
		}
		// Outer panel widths (including borders/padding)
		leftOuterW := int(float64(total) * 0.66)
		if leftOuterW < 1 {
			leftOuterW = 1
		}
		rightOuterW := total - leftOuterW
		if rightOuterW < 1 {
			rightOuterW = 1
		}

		// Height available to both panels: minus input height (including its border)
		// and two header lines (header + extra spacing added earlier)
		headerLines := 2
		inputH := m.input.Height()
		// Account for the input style frame (borders + padding) so panels don't
		// overlap the input box. GetFrameSize returns (width, height) of frame.
		_, inputFrameH := m.inputStyle.GetFrameSize()
		contentOuterH := msg.Height - inputH - inputFrameH - headerLines
		if contentOuterH < 1 {
			contentOuterH = 1
		}

		// Subtract frame size (borders + padding) so viewport inner area fits
		lfW, lfH := m.leftPanelStyle.GetFrameSize()
		rfW, rfH := m.rightPanelStyle.GetFrameSize()
		m.leftVP.Width = max(1, leftOuterW-lfW)
		m.rightVP.Width = max(1, rightOuterW-rfW)
		m.leftVP.Height = max(1, contentOuterH-lfH)
		m.rightVP.Height = max(1, contentOuterH-rfH)

		// Ensure the input area wraps to the terminal width, but account for
		// the inputStyle frame width so the right border is not cut off.
		if fw, _ := m.inputStyle.GetFrameSize(); fw > 0 {
			w := msg.Width - fw
			if w < 1 {
				w = 1
			}
			m.input.SetWidth(w)
		} else {
			m.input.SetWidth(msg.Width)
		}
		m.setView()
		return m, nil
	case streamDeltaMsg:
		// First streaming delta indicates LLM is responding; clear waiting flag
		m.waitingLLM = false
		// Update the current streaming message with new content
		if m.currentMessage != nil {
			// If this is the first content (empty), start with a newline for proper spacing
			if m.currentMessage.content == "" {
				m.currentMessage.content = "\n" + string(msg)
			} else {
				m.currentMessage.content += string(msg)
			}
			// Update the specific streaming message by index, not the last message
			if m.currentMessageIndex < len(m.messages) {
				m.messages[m.currentMessageIndex] = *m.currentMessage
			}
			// Only update the left pane since streaming content goes there
			// Store current scroll position to handle focus-aware scrolling
			oldYOffset := m.leftVP.YOffset
			m.setLeftView()
			// If left pane is focused AND user was manually scrolling and not near bottom,
			// try to maintain their position to avoid interrupting their reading
			if m.activePanel == "left" && m.userScrolledL && !m.isNearBottom(m.leftVP) && oldYOffset < m.leftVP.TotalLineCount()-m.leftVP.Height {
				m.leftVP.YOffset = oldYOffset
			}
		}
		return m, m.readStreamingDelta()
	case toolEventMsg:
		// append immediate tool/assistant events
		m.messages = append(m.messages, chatMsg(msg))
		// Only update the right pane since tool events go there
		// Store current scroll position to handle focus-aware scrolling
		oldYOffset := m.rightVP.YOffset
		m.setRightView()
		// If right pane is focused AND user was manually scrolling and not near bottom,
		// try to maintain their position to avoid interrupting their reading
		if m.activePanel == "right" && m.userScrolledR && !m.isNearBottom(m.rightVP) && oldYOffset < m.rightVP.TotalLineCount()-m.rightVP.Height {
			m.rightVP.YOffset = oldYOffset
		}
		return m, m.readNextEvent()
	case toolStreamClosed:
		return m, nil
	case runResult:
		m.running = false
		// Ensure waiting flag is cleared when the run finishes
		m.waitingLLM = false
		// tool events are already handled by the streaming mechanism via toolEventMsg
		// no need to append msg.events here as it would create duplicates
		if msg.err != nil {
			m.messages = append(m.messages, chatMsg{kind: "info", title: "", content: "error: " + msg.err.Error()})
			m.setView()
			return m, nil
		}
		if m.currentMessage == nil {
			// Non-streaming path (e.g., WARPP demo): append the final answer now
			if msg.text != "" {
				m.messages = append(m.messages, chatMsg{kind: "agent", title: "Agent", content: msg.text})
			}
		} else {
			// Streaming path: finalize the in-progress assistant message
			m.currentMessage = nil
			m.currentMessageIndex = -1
		}
		// update history
		m.history = append(m.history, llm.Message{Role: "user", Content: m.lastUserContent()}, llm.Message{Role: "assistant", Content: msg.text})
		m.setView()
		return m, nil
	case spinnerTickMsg:
		// Advance spinner if we're waiting on LLM or running; otherwise ignore
		if m.waitingLLM || (m.running && m.currentMessage != nil) {
			m.spinnerIdx = (m.spinnerIdx + 1) % len(m.spinners)
			// Force a view update to show the new spinner frame
			m.setLeftView()
			// schedule next tick
			return m, m.spinnerCmd()
		}
		return m, nil
	}

	// default: update input only (viewports are handled above for focused scrolling)
	var cmdInput tea.Cmd
	m.input, cmdInput = m.input.Update(msg)
	return m, cmdInput
}

func (m *Model) View() string {
	leftHeader := m.headerStyle.Render(" Chat ")
	rightHeader := m.headerStyle.Render(" Tools ")

	// Determine active tab and corresponding panel border color so the
	// header (tab) appears to blend into the panel's top border.
	leftPanel := m.leftPanelStyle
	rightPanel := m.rightPanelStyle
	if m.activePanel == "left" {
		leftHeader = m.leftHeaderActiveStyle.Render(" Chat ")
		leftPanel = m.leftPanelStyle.BorderForeground(lipgloss.Color("#2D7FFF"))
	} else {
		rightHeader = m.rightHeaderActiveStyle.Render(" Tools ")
		rightPanel = m.rightPanelStyle.BorderForeground(lipgloss.Color("#7E57C2"))
	}

	// Spinner indicator is now rendered inline beside the Assistant header

	// Render header inside the panel so it visually becomes a tab blending
	// into the panel's border. This keeps borders consistent and makes the
	// header look like part of the panel container.
	// Add an extra blank line after the header to increase spacing
	leftContent := leftHeader + "\n\n" + m.leftVP.View()
	rightContent := rightHeader + "\n\n" + m.rightVP.View()

	leftBlock := leftPanel.Render(leftContent)
	rightBlock := rightPanel.Render(rightContent)
	// Panels are rendered directly adjacent without a vertical divider so the
	// headers blend naturally into the rounded borders.
	top := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, rightBlock)
	// Render the input inside a bordered box so its top border is visible
	inputBlock := m.inputStyle.Render(m.input.View())
	return top + "\n" + inputBlock
}

// spinnerCmd returns a command that fires a spinnerTickMsg after a short delay.
func (m *Model) spinnerCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTickMsg{} })
}

// Non-streaming execution using the same Engine path as cmd/agent, but we stream
// events into the UI via a channel.
type runResult struct {
	text   string
	err    error
	events []chatMsg
}
type toolEventMsg chatMsg
type toolStreamClosed struct{}
type streamDeltaMsg string

type spinnerTickMsg struct{}

func (m *Model) runStreamingEngine(user string) tea.Cmd {
	return func() tea.Msg {
		events := make([]chatMsg, 0, 4)
		rec := tools.NewRecordingRegistry(m.eng.Tools, func(ev tools.DispatchEvent) {
			title := "Tool: " + ev.Name
			content := string(ev.Payload)
			if ev.Name == "run_cli" {
				var args struct {
					Command        string   `json:"command"`
					Args           []string `json:"args"`
					TimeoutSeconds int      `json:"timeout_seconds"`
					Stdin          string   `json:"stdin"`
				}
				var res cli.ExecResult
				_ = json.Unmarshal(ev.Args, &args)
				if err := json.Unmarshal(ev.Payload, &res); err == nil {
					content = formatToolPayload(args.Command, args.Args, res)
				}
			}
			cm := chatMsg{kind: "tool", title: title, content: content}
			events = append(events, cm)
			select {
			case m.toolCh <- cm:
			default:
			}
		})
		eng := m.eng
		eng.Tools = rec
		// Set up streaming delta handler
		eng.OnDelta = func(delta string) {
			select {
			case m.streamingDeltaCh <- delta:
			default:
			}
		}
		// Don't use OnAssistant for streaming since we handle deltas directly
		eng.OnAssistant = nil

		ans, err := eng.RunStream(m.ctx, user, m.history)
		// close streams after engine returns
		close(m.toolCh)
		close(m.streamingDeltaCh)
		return runResult{text: ans, err: err, events: events}
	}
}

// runSpecialist performs a direct specialist call using the pre-dispatch router
// and returns a runResult for the TUI to render.
func (m *Model) runSpecialist(name, user string) tea.Cmd {
	return func() tea.Msg {
		// Announce specialist activity in the right pane
		if m.toolCh != nil {
			cm := chatMsg{kind: "tool", title: "Specialist: " + name, content: "routing"}
			select {
			case m.toolCh <- cm:
			default:
			}
		}
		a, ok := m.specReg.Get(name)
		if !ok {
			if m.toolCh != nil {
				close(m.toolCh)
			}
			return runResult{text: "", err: fmt.Errorf("unknown specialist: %s", name)}
		}
		out, err := a.Inference(m.ctx, user, nil)
		if m.toolCh != nil {
			close(m.toolCh)
		}
		return runResult{text: out, err: err}
	}
}

// runWARPP executes the production WARPP runner using loaded workflows
// (defaults included) and posts tool outputs to the right pane and a summarized
// result to the chat pane.
func (m *Model) runWARPPDemo(user string) tea.Cmd {
	return func() tea.Msg {
		events := make([]chatMsg, 0, 4)
		rec := tools.NewRecordingRegistry(m.eng.Tools, func(ev tools.DispatchEvent) {
			title := "Tool: " + ev.Name
			content := string(ev.Payload)
			if ev.Name == "run_cli" {
				var args struct {
					Command        string   `json:"command"`
					Args           []string `json:"args"`
					TimeoutSeconds int      `json:"timeout_seconds"`
					Stdin          string   `json:"stdin"`
				}
				var res cli.ExecResult
				_ = json.Unmarshal(ev.Args, &args)
				if err := json.Unmarshal(ev.Payload, &res); err == nil {
					content = formatToolPayload(args.Command, args.Args, res)
				}
			}
			cm := chatMsg{kind: "tool", title: title, content: content}
			events = append(events, cm)
			select {
			case m.toolCh <- cm:
			default:
			}
		})

		// Build WARPP runner
		wfreg, _ := warpp.LoadFromDir("configs/workflows")
		runner := &warpp.Runner{Workflows: wfreg, Tools: rec}

		// Stage 1: intent + workflow
		intent := runner.DetectIntent(m.ctx, user)
		wf, _ := wfreg.Get(intent)
		attrs := warpp.Attrs{"utter": user}
		// Stage 2: personalization (our runner does simple inference and trimming)
		wfStar, _, attrs, err := runner.Personalize(m.ctx, wf, attrs)
		if err != nil {
			close(m.toolCh)
			return runResult{text: "", err: err, events: events}
		}
		// Build allowlist from referenced tools
		allow := map[string]bool{}
		for _, s := range wfStar.Steps {
			if s.Tool != nil {
				allow[s.Tool.Name] = true
			}
		}
		// Stage 3: execution
		finalText, err := runner.Execute(m.ctx, wfStar, allow, attrs)
		close(m.toolCh)
		return runResult{text: finalText, err: err, events: events}
	}
}

func (m *Model) readNextEvent() tea.Cmd {
	return func() tea.Msg {
		if m.toolCh == nil {
			return toolStreamClosed{}
		}
		ev, ok := <-m.toolCh
		if !ok {
			return toolStreamClosed{}
		}
		return toolEventMsg(ev)
	}
}

func (m *Model) readStreamingDelta() tea.Cmd {
	return func() tea.Msg {
		if m.streamingDeltaCh == nil {
			return toolStreamClosed{}
		}
		delta, ok := <-m.streamingDeltaCh
		if !ok {
			return toolStreamClosed{}
		}
		return streamDeltaMsg(delta)
	}
}

func (m *Model) renderChat(width int) string {
	var b strings.Builder
	cnt := 0
	for i, msg := range m.messages {
		if msg.kind == "tool" {
			continue
		}
		if cnt > 0 {
			b.WriteString("\n\n")
		}
		// showSpinner for the currently-streaming assistant message
		// Show spinner if we're waiting for LLM OR if this is the current streaming message
		showSpinner := false
		if m.currentMessage != nil && i == m.currentMessageIndex && (m.waitingLLM || m.running) {
			showSpinner = true
		}
		b.WriteString(m.renderMsg(msg, width, showSpinner))
		cnt++
	}
	return b.String()
}

func (m *Model) renderTools(width int) string {
	var b strings.Builder
	cnt := 0
	for _, msg := range m.messages {
		if msg.kind != "tool" {
			continue
		}
		if cnt > 0 {
			b.WriteString("\n\n")
		}
		// Tool messages never show the agent spinner
		b.WriteString(m.renderMsg(msg, width, false))
		cnt++
	}
	if cnt == 0 {
		return m.infoStyle.Render("No tool activity yet.")
	}
	return b.String()
}

func (m *Model) renderMsg(cm chatMsg, width int, showSpinner bool) string {
	maxw := width
	if maxw < 20 {
		maxw = 20
	}
	// Create a style with enforced max width to avoid horizontal overflow
	wrap := lipgloss.NewStyle().MaxWidth(maxw)
	switch cm.kind {
	case "user":
		header := m.userTag.Render("You")
		contentWrapped := wrapString(cm.content, maxw)
		body := m.userText.Render(wrap.Render(contentWrapped))
		// Extra blank line after header for improved vertical spacing
		return header + "\n\n" + body
	case "agent":
		header := m.agentTag.Render("Agent")
		// If requested, append a small yellow spinner to the right of the Agent header
		if showSpinner {
			spin := m.spinners[m.spinnerIdx]
			header = header + " " + m.spinnerStyle.Render(spin)
		}
		// Render Markdown to ANSI using glamour. If rendering fails, fall back
		// to raw content. We then hard-wrap the ANSI output to avoid overflowing
		// bordered containers.
		mdOut, err := glamour.Render(cm.content, "dark")
		if err != nil {
			mdOut = cm.content
		}
		contentWrapped := wrapString(mdOut, maxw)
		// Do not re-apply m.agentText styling since glamour already produces
		// ANSI styling. Just return the header and the rendered markdown body.
		return header + "\n\n" + wrap.Render(contentWrapped)
	case "tool":
		header := lipgloss.NewStyle().Bold(true).Render(cm.title)
		// Adjust wrap width to account for the right panel frame/padding so
		// tool entries don't overflow the rounded border when rendered.
		inw := maxw
		if fw, _ := m.rightPanelStyle.GetFrameSize(); fw > 0 {
			if inw-fw > 1 {
				inw = inw - fw
			} else {
				inw = 1
			}
		}
		innerWrap := lipgloss.NewStyle().MaxWidth(inw)
		contentWrapped := wrapString(cm.content, inw)
		// Add an extra blank line after the tool header for clearer separation
		return m.toolStyle.Render(header + "\n\n" + innerWrap.Render(contentWrapped))
	default:
		contentWrapped := wrapString(cm.content, maxw)
		return m.infoStyle.Render(wrap.Render(contentWrapped))
	}
}

// wrapString performs hard wrapping to the given visible width, ensuring even
// very long tokens are broken across lines. This prevents overflow inside
// bordered containers.
func wrapString(s string, width int) string {
	if width <= 0 {
		return s
	}
	// Hardwrap is ANSI-aware and counts visible cell width; it will insert
	// newlines so even long tokens don't overflow bordered containers.
	return xansi.Hardwrap(s, width, false)
}

func (m *Model) setView() {
	m.setLeftView()
	m.setRightView()
}

func (m *Model) setLeftView() {
	m.leftVP.SetContent(m.renderChat(m.leftVP.Width))
	// Auto-scroll to bottom if:
	// 1. Left pane is not focused (activePanel != "left"), OR
	// 2. User hasn't manually scrolled in this pane, OR
	// 3. User is near the bottom (to follow streaming content)
	if m.activePanel != "left" || !m.userScrolledL || m.isNearBottom(m.leftVP) {
		m.leftVP.GotoBottom()
	}
}

// isNearBottom checks if the viewport is within a few lines of the bottom
func (m *Model) isNearBottom(vp viewport.Model) bool {
	// Consider "near bottom" if within 3 lines of the actual bottom
	return vp.YOffset >= vp.TotalLineCount()-vp.Height-3
}

func (m *Model) setRightView() {
	m.rightVP.SetContent(m.renderTools(m.rightVP.Width))
	// Auto-scroll to bottom if:
	// 1. Right pane is not focused (activePanel != "right"), OR
	// 2. User hasn't manually scrolled in this pane, OR
	// 3. User is near the bottom (to follow new tool output)
	if m.activePanel != "right" || !m.userScrolledR || m.isNearBottom(m.rightVP) {
		m.rightVP.GotoBottom()
	}
}

// Helpers ---------------------------------------------------------------------

func (m *Model) lastUserContent() string {
	// find last user message we appended in the view; since we keep
	// history ourselves, we can track user content simply as the last input.
	// For simplicity we return the last chat message of kind "user".
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].kind == "user" {
			return m.messages[i].content
		}
	}
	return ""
}

func formatToolPayload(cmd string, args []string, res cli.ExecResult) string {
	var b strings.Builder
	if cmd != "" {
		b.WriteString(fmt.Sprintf("$ %s %s\n", cmd, strings.Join(args, " ")))
	}
	b.WriteString(fmt.Sprintf("exit %d | ok=%v | %dms\n", res.ExitCode, res.OK, res.Duration))
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

// max returns the maximum of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// schema adaptation moved to internal/llm/openai/schema.go and registry usage above.

// we no longer render tool payloads in the TUI; tools run inside the Engine
