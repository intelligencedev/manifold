package tui

import (
    "context"
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

    messages    []chatMsg
    running     bool

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
            if m.running { return m, nil }
            q := strings.TrimSpace(m.input.Value())
            if q == "" { return m, nil }
            m.input.SetValue("")
            m.messages = append(m.messages, chatMsg{kind: "user", title: "You", content: q})
            m.setView()
            m.running = true
            return m, m.runAgentCmd(q)
        }
    case tea.WindowSizeMsg:
        m.leftVP.Width = msg.Width - 42
        m.leftVP.Height = msg.Height - 3
        m.rightVP.Width = 40
        m.rightVP.Height = msg.Height - 3
        m.setView()
        return m, nil
    case runResult:
        m.running = false
        if msg.err != nil {
            m.messages = append(m.messages, chatMsg{kind: "info", title: "", content: "error: " + msg.err.Error()})
            m.setView()
            return m, nil
        }
        // append assistant output and update history (porting CLI behavior)
        m.messages = append(m.messages, chatMsg{kind: "agent", title: "Agent", content: msg.text})
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
    if m.activePanel == "left" { leftHeader = m.leftHeaderActiveStyle.Render(" Chat ") } else { rightHeader = m.rightHeaderActiveStyle.Render(" Tools ") }
    leftBlock := leftHeader + "\n" + m.leftVP.View()
    rightBlock := rightHeader + "\n" + m.rightVP.View()
    sep := m.dividerStyle.Render("│")
    top := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, sep, rightBlock)
    return top + "\n" + m.input.View()
}

// Non-streaming execution using the same Engine path as cmd/agent
type runResult struct{ text string; err error }

func (m *Model) runAgentCmd(q string) tea.Cmd {
    // capture q for history update
    user := q
    return func() tea.Msg {
        ans, err := m.eng.Run(m.ctx, user, m.history)
        return runResult{text: ans, err: err}
    }
}

func (m *Model) renderChat(width int) string {
    var b strings.Builder
    cnt := 0
    for _, msg := range m.messages {
        if msg.kind == "tool" { continue }
        if cnt > 0 { b.WriteString("\n\n") }
        b.WriteString(m.renderMsg(msg, width))
        cnt++
    }
    return b.String()
}

func (m *Model) renderTools(width int) string {
    var b strings.Builder
    cnt := 0
    for _, msg := range m.messages {
        if msg.kind != "tool" { continue }
        if cnt > 0 { b.WriteString("\n\n") }
        b.WriteString(m.renderMsg(msg, width))
        cnt++
    }
    if cnt == 0 { return m.infoStyle.Render("No tool activity yet.") }
    return b.String()
}

func (m *Model) renderMsg(cm chatMsg, width int) string {
    maxw := width
    if maxw < 20 { maxw = 20 }
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

func (m *Model) setView() {
    m.leftVP.SetContent(m.renderChat(m.leftVP.Width))
    m.rightVP.SetContent(m.renderTools(m.rightVP.Width))
    if !m.userScrolledL { m.leftVP.GotoBottom() }
    if !m.userScrolledR { m.rightVP.GotoBottom() }
}

// Helpers ---------------------------------------------------------------------

func (m *Model) lastUserContent() string {
    // find last user message we appended in the view; since we keep
    // history ourselves, we can track user content simply as the last input.
    // For simplicity we return the last chat message of kind "user".
    for i := len(m.messages) - 1; i >= 0; i-- {
        if m.messages[i].kind == "user" { return m.messages[i].content }
    }
    return ""
}

// schema adaptation moved to internal/llm/openai/schema.go and registry usage above.

// we no longer render tool payloads in the TUI; tools run inside the Engine
