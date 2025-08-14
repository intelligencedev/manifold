package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"singularityio/internal/agent"
	"singularityio/internal/agent/prompts"
	"singularityio/internal/config"
	"singularityio/internal/llm"
	openai "singularityio/internal/llm/openai"
	"singularityio/internal/mcpclient"
	"singularityio/internal/observability"
	"singularityio/internal/specialists"
	"singularityio/internal/tools"
	"singularityio/internal/tools/cli"
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
	input   textinput.Model

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

	activePanel   string // "left" or "right"
	userScrolledL bool
	userScrolledR bool
}

type chatMsg struct {
	kind    string // user | agent | tool | info
	title   string
	content string
}

func NewModel(ctx context.Context, provider llm.Provider, cfg config.Config, exec cli.Executor, maxSteps int, warppDemo bool) *Model {
	left := viewport.New(80, 20)
	right := viewport.New(40, 20)
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "Ask the agent..."
	in.Focus()

	userTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1).MarginRight(1)
	agentTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1).MarginRight(1)
	toolStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("13")).Padding(0, 1)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	userText := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6F0FF"))
	agentText := lipgloss.NewStyle().Foreground(lipgloss.Color("#ECE7FF"))

	// Tool registry
	registry := tools.NewRegistry()
	registry.Register(cli.NewTool(exec))
	registry.Register(web.NewTool(cfg.Web.SearXNGURL))
	registry.Register(web.NewFetchTool())
	registry.Register(fs.NewWriteTool(cfg.Workdir))
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
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, nil)
	registry.Register(specialists_tool.New(specReg))

	// MCP tools
	mcpMgr := mcpclient.NewManager()
	_ = mcpMgr.RegisterFromConfig(ctx, registry, cfg.MCP)

	// Engine setup (matches cmd/agent wiring)
	eng := agent.Engine{
		LLM:      provider,
		Tools:    registry,
		MaxSteps: maxSteps,
		System:   prompts.DefaultSystemPrompt(cfg.Workdir),
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
		dividerStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		headerStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
		leftHeaderActiveStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#2D7FFF")).Bold(true).Padding(0, 1),
		rightHeaderActiveStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#7E57C2")).Bold(true).Padding(0, 1),
		activePanel:            "left",
	}
	m.setView()
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) cleanup() {}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
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
			m.setView()
			m.running = true
			// start reading events and engine in parallel
			m.toolCh = make(chan chatMsg, 32)
			if m.warppDemo {
				// WARPP demo does not stream tokens; produce a final answer at once
				return m, tea.Batch(m.readNextEvent(), m.runWARPPDemo(q))
			}
			// Pre-dispatch routing to specialists in TUI
			if name := specialists.Route(m.cfg.SpecialistRoutes, q); name != "" {
				return m, tea.Batch(m.readNextEvent(), m.runSpecialist(name, q))
			}
			m.streamingDeltaCh = make(chan string, 64)
			// Initialize streaming message and track its index
			m.currentMessage = &chatMsg{kind: "agent", title: "Agent", content: ""}
			m.currentMessageIndex = len(m.messages) // Store the index before appending
			m.messages = append(m.messages, *m.currentMessage)
			m.setView()
			return m, tea.Batch(m.readNextEvent(), m.readStreamingDelta(), m.runStreamingEngine(q))
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
			if msg.X < m.leftVP.Width {
				m.activePanel = "left"
			} else {
				m.activePanel = "right"
			}
			return m, nil
		}

		// Let each viewport handle its own mouse events (including wheel)
		var cmdL, cmdR tea.Cmd
		if msg.X < m.leftVP.Width {
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
		// Split width evenly between left and right panes with a 1-col separator
		sep := 1
		total := msg.Width - sep
		if total < 2 {
			total = 2
		}
		leftW := total / 2
		rightW := total - leftW
		m.leftVP.Width = leftW
		m.rightVP.Width = rightW
		m.leftVP.Height = msg.Height - 3
		m.rightVP.Height = msg.Height - 3
		m.setView()
		return m, nil
	case streamDeltaMsg:
		// Update the current streaming message with new content
		if m.currentMessage != nil {
			m.currentMessage.content += string(msg)
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
	}

	// default: update input only (viewports are handled above for focused scrolling)
	var cmdInput tea.Cmd
	m.input, cmdInput = m.input.Update(msg)
	return m, cmdInput
}

func (m *Model) View() string {
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
		b.WriteString(m.renderMsg(msg, width))
		cnt++
	}
	if cnt == 0 {
		return m.infoStyle.Render("No tool activity yet.")
	}
	return b.String()
}

func (m *Model) renderMsg(cm chatMsg, width int) string {
	maxw := width
	if maxw < 20 {
		maxw = 20
	}
	// Create a style with proper word wrapping
	wrap := lipgloss.NewStyle().Width(maxw)
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
		// Adjust wrap width to account for border/padding frame
		inw := maxw
		if fw, _ := m.toolStyle.GetFrameSize(); fw > 0 {
			if inw-fw > 1 {
				inw = inw - fw
			} else {
				inw = 1
			}
		}
		innerWrap := lipgloss.NewStyle().Width(inw)
		return m.toolStyle.Render(header + "\n" + innerWrap.Render(cm.content))
	default:
		return m.infoStyle.Render(wrap.Render(cm.content))
	}
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

// schema adaptation moved to internal/llm/openai/schema.go and registry usage above.

// we no longer render tool payloads in the TUI; tools run inside the Engine
