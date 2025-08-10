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

	"gptagent/internal/agent"
	"gptagent/internal/agent/prompts"
	"gptagent/internal/config"
	"gptagent/internal/llm"
	"gptagent/internal/tools"
	"gptagent/internal/tools/cli"
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

	messages         []chatMsg
	currentMessage   *chatMsg // For streaming content
	running          bool
	toolCh           chan chatMsg
	streamingDeltaCh chan string

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

func NewModel(ctx context.Context, provider llm.Provider, cfg config.Config, exec cli.Executor, maxSteps int) *Model {
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
		leftVP:                 left,
		rightVP:                right,
		input:                  in,
		messages:               []chatMsg{{kind: "info", title: "", content: "Interactive mode. Type a prompt and press Enter to run. Ctrl+C to exit."}},
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
			m.streamingDeltaCh = make(chan string, 64)
			// Initialize streaming message
			m.currentMessage = &chatMsg{kind: "agent", title: "Agent", content: ""}
			m.messages = append(m.messages, *m.currentMessage)
			m.setView()
			return m, tea.Batch(m.readNextEvent(), m.readStreamingDelta(), m.runStreamingEngine(q))
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
			// Update the last message in the slice
			if len(m.messages) > 0 {
				m.messages[len(m.messages)-1] = *m.currentMessage
			}
			m.setView()
		}
		return m, m.readStreamingDelta()
	case toolEventMsg:
		// append immediate tool/assistant events
		m.messages = append(m.messages, chatMsg(msg))
		m.setView()
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
		// Finalize the streaming message
		if m.currentMessage != nil {
			m.currentMessage = nil // Reset current message
		}
		// update history (porting CLI behavior) - streaming already handled the final assistant message
		m.history = append(m.history, llm.Message{Role: "user", Content: m.lastUserContent()}, llm.Message{Role: "assistant", Content: msg.text})
		m.setView()
		return m, nil
	}

	// default: update input and both viewports
	var cmdInput, cmdL, cmdR tea.Cmd
	m.input, cmdInput = m.input.Update(msg)
	m.leftVP, cmdL = m.leftVP.Update(msg)
	m.rightVP, cmdR = m.rightVP.Update(msg)
	return m, tea.Batch(cmdInput, cmdL, cmdR)
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

func (m *Model) runEngine(user string) tea.Cmd {
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
		eng.OnAssistant = func(am llm.Message) {
			if am.Content == "" {
				return
			}
			cm := chatMsg{kind: "agent", title: "Agent", content: am.Content}
			select {
			case m.toolCh <- cm:
			default:
			}
		}
		ans, err := eng.Run(m.ctx, user, m.history)
		// close stream after engine returns
		close(m.toolCh)
		return runResult{text: ans, err: err, events: events}
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
	wrap := lipgloss.NewStyle().MaxWidth(maxw).Padding(0)
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
		innerWrap := lipgloss.NewStyle().MaxWidth(inw).Padding(0)
		return m.toolStyle.Render(header + "\n" + innerWrap.Render(cm.content))
	default:
		return m.infoStyle.Render(wrap.Render(cm.content))
	}
}

func (m *Model) setView() {
	m.leftVP.SetContent(m.renderChat(m.leftVP.Width))
	m.rightVP.SetContent(m.renderTools(m.rightVP.Width))
	if !m.userScrolledL {
		m.leftVP.GotoBottom()
	}
	if !m.userScrolledR {
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
