package harness

import (
	"strings"

	"manifold/internal/llm"
)

// Mode selects how strictly the harness validates model output.
type Mode string

const (
	ModeLegacy      Mode = "legacy"
	ModeGuardedChat Mode = "guarded_chat"
	ModeWorkflow    Mode = "workflow"
)

const (
	defaultMaxRetriesPerStep = 3
	defaultMaxToolErrors     = 2
)

// RunConfig contains the harness controls needed for a single guarded model step.
type RunConfig struct {
	Mode              Mode
	RescueEnabled     bool
	MaxRetriesPerStep int
	MaxToolErrors     int
	Workflow          WorkflowConfig
	Compact           CompactConfig
}

// WorkflowConfig describes Forge-style loop enforcement without replacing Flow v2.
type WorkflowConfig struct {
	RequiredSteps     []string
	TerminalTools     []string
	ToolPrerequisites map[string][]Prerequisite
}

// CompactConfig controls metadata-aware context compaction inside harness loops.
type CompactConfig struct {
	Enabled             bool
	KeepRecentSteps     int
	PhaseThresholds     []float64
	ContextWindowTokens int
	ReserveTokens       int
	PerMessageRunes     int
}

// Prerequisite requires a prior successful call before a tool can run.
// When MatchArg is set, the prior call must have the same JSON argument value.
type Prerequisite struct {
	Tool     string
	MatchArg string
}

// MessageType records harness-only control-flow metadata.
type MessageType string

const (
	MessageTypePrompt    MessageType = "prompt"
	MessageTypeAssistant MessageType = "assistant"
	MessageTypeTool      MessageType = "tool"
	MessageTypeNudge     MessageType = "nudge"
	MessageTypeCompact   MessageType = "compact"
)

// MessageMeta is never sent to providers. It preserves control-flow state across
// future compaction and engine integration work.
type MessageMeta struct {
	Type       MessageType
	StepIndex  int
	ToolName   string
	ToolCallID string
}

// HarnessMessage wraps a provider message with metadata used only by the harness.
type HarnessMessage struct {
	Message llm.Message
	Meta    MessageMeta
}

// DefaultRunConfig returns the safe default: existing legacy behavior.
func DefaultRunConfig() RunConfig {
	return RunConfig{
		Mode:              ModeLegacy,
		MaxRetriesPerStep: defaultMaxRetriesPerStep,
		MaxToolErrors:     defaultMaxToolErrors,
	}
}

// NormalizeMode maps empty and unknown modes to legacy so opt-in remains explicit.
func NormalizeMode(mode Mode) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case ModeGuardedChat:
		return ModeGuardedChat
	case ModeWorkflow:
		return ModeWorkflow
	case ModeLegacy:
		return ModeLegacy
	default:
		return ModeLegacy
	}
}

func normalizeRunConfig(cfg RunConfig) RunConfig {
	cfg.Mode = NormalizeMode(cfg.Mode)
	if cfg.MaxRetriesPerStep < 0 {
		cfg.MaxRetriesPerStep = 0
	}
	if cfg.MaxRetriesPerStep == 0 {
		cfg.MaxRetriesPerStep = defaultMaxRetriesPerStep
	}
	if cfg.MaxToolErrors < 0 {
		cfg.MaxToolErrors = 0
	}
	if cfg.MaxToolErrors == 0 {
		cfg.MaxToolErrors = defaultMaxToolErrors
	}
	cfg.Workflow = cfg.Workflow.normalized()
	cfg.Compact = cfg.Compact.normalized()
	return cfg
}

func (c CompactConfig) normalized() CompactConfig {
	if c.KeepRecentSteps <= 0 {
		c.KeepRecentSteps = 4
	}
	if len(c.PhaseThresholds) == 0 {
		c.PhaseThresholds = []float64{0.60, 0.75, 0.90}
	}
	c.PhaseThresholds = normalizeThresholds(c.PhaseThresholds)
	return c
}

func (w WorkflowConfig) normalized() WorkflowConfig {
	out := WorkflowConfig{
		RequiredSteps:     normalizeNames(w.RequiredSteps),
		TerminalTools:     normalizeNames(w.TerminalTools),
		ToolPrerequisites: make(map[string][]Prerequisite, len(w.ToolPrerequisites)),
	}
	for tool, prereqs := range w.ToolPrerequisites {
		tool = normalizeName(tool)
		if tool == "" {
			continue
		}
		for _, prereq := range prereqs {
			prereq.Tool = normalizeName(prereq.Tool)
			prereq.MatchArg = strings.TrimSpace(prereq.MatchArg)
			if prereq.Tool == "" {
				continue
			}
			out.ToolPrerequisites[tool] = append(out.ToolPrerequisites[tool], prereq)
		}
	}
	if len(out.ToolPrerequisites) == 0 {
		out.ToolPrerequisites = nil
	}
	return out
}

func (w WorkflowConfig) isTerminalTool(name string) bool {
	name = normalizeName(name)
	if name == "" {
		return false
	}
	for _, terminal := range w.TerminalTools {
		if terminal == name {
			return true
		}
	}
	return false
}

// IsTerminalTool reports whether name is configured as a workflow terminal tool.
func (w WorkflowConfig) IsTerminalTool(name string) bool {
	return w.normalized().isTerminalTool(name)
}

func normalizeNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = normalizeName(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func normalizeName(name string) string {
	return strings.TrimSpace(name)
}

// SerializeMessages strips harness metadata before a provider request.
func SerializeMessages(messages []HarnessMessage) []llm.Message {
	out := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.Message)
	}
	return out
}

// WrapMessages annotates existing provider messages as prompt/history messages.
func WrapMessages(messages []llm.Message) []HarnessMessage {
	out := make([]HarnessMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, HarnessMessage{
			Message: message,
			Meta:    MessageMeta{Type: MessageTypePrompt},
		})
	}
	return out
}
