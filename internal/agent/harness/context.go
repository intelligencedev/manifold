package harness

import (
	"fmt"
	"sort"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/llm/budget"
)

// CompactResult describes the deterministic harness-history compaction pass.
type CompactResult struct {
	Messages                   []HarnessMessage
	Changed                    bool
	Phase                      int
	TokensBefore               int
	TokensAfter                int
	BudgetTokens               int
	DroppedNudges              int
	TruncatedToolResults       int
	CompactedToolResults       int
	CompactedAssistantMessages int
	InsertedHints              int
}

// CompactMessages applies deterministic, metadata-aware context compaction.
// Workflow progress remains authoritative in StepTracker; the injected hint is
// only model-visible context, never enforcement state.
func CompactMessages(messages []HarnessMessage, cfg CompactConfig, tracker *StepTracker) CompactResult {
	cfg = cfg.normalized()
	out, removedHints := stripCompactHints(messages)
	result := CompactResult{
		Messages:      out,
		Changed:       removedHints > 0,
		TokensBefore:  budget.EstimateTokens(SerializeMessages(messages)),
		BudgetTokens:  compactBudgetTokens(cfg),
		InsertedHints: 0,
	}
	if !cfg.Enabled || len(out) == 0 {
		result.TokensAfter = budget.EstimateTokens(SerializeMessages(out))
		return result
	}

	result.Phase = compactPhase(result.TokensBefore, result.BudgetTokens, cfg.PhaseThresholds)
	if result.Phase == 0 {
		result.TokensAfter = budget.EstimateTokens(SerializeMessages(out))
		return result
	}

	protected := protectedMessageIndexes(out)
	recent := recentStepIndexes(out, cfg.KeepRecentSteps)
	out, result.DroppedNudges = dropOldNudges(out, protected, recent)
	if result.DroppedNudges > 0 {
		result.Changed = true
	}
	protected = protectedMessageIndexes(out)
	recent = recentStepIndexes(out, cfg.KeepRecentSteps)

	out, result.TruncatedToolResults = truncateOldToolResults(out, protected, recent, cfg.PerMessageRunes)
	if result.TruncatedToolResults > 0 {
		result.Changed = true
	}

	if result.Phase >= 2 {
		out, result.CompactedToolResults = compactOldToolResults(out, protected, recent)
		if result.CompactedToolResults > 0 {
			result.Changed = true
		}
	}

	if result.Phase >= 3 {
		out, result.CompactedAssistantMessages = compactOldAssistantMessages(out, protected, recent)
		if result.CompactedAssistantMessages > 0 {
			result.Changed = true
		}
	}

	if result.BudgetTokens > 0 {
		var budgetChanged bool
		out, budgetChanged = enforceCompactBudget(out, result.BudgetTokens, cfg.PerMessageRunes, protected, recent)
		if budgetChanged {
			result.Changed = true
		}
	}

	if result.Changed || len(tracker.Completed()) > 0 {
		out = insertCompactHint(out, result.Phase, tracker)
		result.InsertedHints = 1
		result.Changed = true
	}

	result.Messages = out
	result.TokensAfter = budget.EstimateTokens(SerializeMessages(out))
	return result
}

func normalizeThresholds(thresholds []float64) []float64 {
	out := make([]float64, 0, len(thresholds))
	for _, threshold := range thresholds {
		if threshold <= 0 {
			continue
		}
		if threshold > 1 {
			threshold = 1
		}
		out = append(out, threshold)
	}
	sort.Float64s(out)
	for len(out) < 3 {
		out = append(out, 1)
	}
	return out[:3]
}

func compactBudgetTokens(cfg CompactConfig) int {
	if cfg.ContextWindowTokens <= 0 {
		return 0
	}
	reserve := cfg.ReserveTokens
	if reserve < 0 {
		reserve = 0
	}
	if reserve >= cfg.ContextWindowTokens {
		reserve = cfg.ContextWindowTokens / 2
	}
	budgetTokens := cfg.ContextWindowTokens - reserve
	if budgetTokens <= 0 {
		budgetTokens = cfg.ContextWindowTokens / 2
	}
	return budgetTokens
}

func compactPhase(tokens, budgetTokens int, thresholds []float64) int {
	if tokens <= 0 || budgetTokens <= 0 {
		return 0
	}
	ratio := float64(tokens) / float64(budgetTokens)
	phase := 0
	for i, threshold := range thresholds {
		if ratio >= threshold {
			phase = i + 1
		}
	}
	return phase
}

func stripCompactHints(messages []HarnessMessage) ([]HarnessMessage, int) {
	out := make([]HarnessMessage, 0, len(messages))
	removed := 0
	for _, message := range messages {
		if message.Meta.Type == MessageTypeCompact {
			removed++
			continue
		}
		out = append(out, cloneHarnessMessage(message))
	}
	return out, removed
}

func protectedMessageIndexes(messages []HarnessMessage) map[int]struct{} {
	protected := make(map[int]struct{})
	if len(messages) == 0 {
		return protected
	}
	if messages[0].Message.Role == "system" {
		protected[0] = struct{}{}
	}
	lastPromptUser := -1
	for i, message := range messages {
		if message.Meta.Type == MessageTypePrompt && message.Message.Role == "user" {
			lastPromptUser = i
		}
	}
	if lastPromptUser >= 0 {
		protected[lastPromptUser] = struct{}{}
	}
	return protected
}

func recentStepIndexes(messages []HarnessMessage, keepRecentSteps int) map[int]struct{} {
	recent := make(map[int]struct{})
	if keepRecentSteps <= 0 {
		return recent
	}
	maxStep := -1
	for _, message := range messages {
		if !isStepMessage(message.Meta.Type) {
			continue
		}
		if message.Meta.StepIndex > maxStep {
			maxStep = message.Meta.StepIndex
		}
	}
	if maxStep < 0 {
		return recent
	}
	minStep := maxStep - keepRecentSteps + 1
	for i, message := range messages {
		if !isStepMessage(message.Meta.Type) {
			continue
		}
		if message.Meta.StepIndex >= minStep {
			recent[i] = struct{}{}
		}
	}
	return recent
}

func isStepMessage(messageType MessageType) bool {
	switch messageType {
	case MessageTypeAssistant, MessageTypeTool, MessageTypeNudge:
		return true
	default:
		return false
	}
}

func canCompactIndex(i int, protected, recent map[int]struct{}) bool {
	if _, ok := protected[i]; ok {
		return false
	}
	if _, ok := recent[i]; ok {
		return false
	}
	return true
}

func dropOldNudges(messages []HarnessMessage, protected, recent map[int]struct{}) ([]HarnessMessage, int) {
	out := make([]HarnessMessage, 0, len(messages))
	dropped := 0
	for i, message := range messages {
		if canCompactIndex(i, protected, recent) && message.Meta.Type == MessageTypeNudge {
			dropped++
			continue
		}
		out = append(out, message)
	}
	return out, dropped
}

func truncateOldToolResults(messages []HarnessMessage, protected, recent map[int]struct{}, perMessageRunes int) ([]HarnessMessage, int) {
	if perMessageRunes <= 0 {
		perMessageRunes = budget.DefaultPerMsgRunes
	}
	out := cloneHarnessMessages(messages)
	truncated := 0
	for i := range out {
		if !canCompactIndex(i, protected, recent) || out[i].Meta.Type != MessageTypeTool {
			continue
		}
		next := truncateRunes(out[i].Message.Content, perMessageRunes)
		if next != out[i].Message.Content {
			out[i].Message.Content = next
			truncated++
		}
	}
	return out, truncated
}

func compactOldToolResults(messages []HarnessMessage, protected, recent map[int]struct{}) ([]HarnessMessage, int) {
	out := cloneHarnessMessages(messages)
	compacted := 0
	for i := range out {
		if !canCompactIndex(i, protected, recent) || out[i].Meta.Type != MessageTypeTool {
			continue
		}
		marker := compactedToolResult(out[i])
		if out[i].Message.Content == marker {
			continue
		}
		out[i].Message.Content = marker
		compacted++
	}
	return out, compacted
}

func compactOldAssistantMessages(messages []HarnessMessage, protected, recent map[int]struct{}) ([]HarnessMessage, int) {
	out := cloneHarnessMessages(messages)
	compacted := 0
	for i := range out {
		if !canCompactIndex(i, protected, recent) || out[i].Meta.Type != MessageTypeAssistant {
			continue
		}
		if len(out[i].Message.ToolCalls) > 0 {
			if strings.TrimSpace(out[i].Message.Content) == "" {
				continue
			}
			out[i].Message.Content = ""
			compacted++
			continue
		}
		marker := "[COMPACTED assistant message]"
		if out[i].Message.Content == marker {
			continue
		}
		out[i].Message.Content = marker
		compacted++
	}
	return out, compacted
}

func enforceCompactBudget(messages []HarnessMessage, budgetTokens, perMessageRunes int, protected, recent map[int]struct{}) ([]HarnessMessage, bool) {
	out := cloneHarnessMessages(messages)
	changed := false
	if perMessageRunes <= 0 {
		perMessageRunes = budget.DefaultPerMsgRunes
	}
	for i := range out {
		if out[i].Message.Role == "system" {
			continue
		}
		next := truncateRunes(out[i].Message.Content, perMessageRunes)
		if next != out[i].Message.Content {
			out[i].Message.Content = next
			changed = true
		}
	}
	for budget.EstimateTokens(SerializeMessages(out)) > budgetTokens {
		changedThisPass := false
		for i := range out {
			if !canCompactIndex(i, protected, recent) {
				continue
			}
			switch out[i].Meta.Type {
			case MessageTypeTool:
				marker := compactedToolResult(out[i])
				if out[i].Message.Content != marker {
					out[i].Message.Content = marker
					changedThisPass = true
				}
			case MessageTypeAssistant:
				if len(out[i].Message.ToolCalls) > 0 && strings.TrimSpace(out[i].Message.Content) != "" {
					out[i].Message.Content = ""
					changedThisPass = true
				} else if len(out[i].Message.ToolCalls) == 0 && out[i].Message.Content != "[COMPACTED assistant message]" {
					out[i].Message.Content = "[COMPACTED assistant message]"
					changedThisPass = true
				}
			case MessageTypeNudge:
				if out[i].Message.Content != "[COMPACTED nudge]" {
					out[i].Message.Content = "[COMPACTED nudge]"
					changedThisPass = true
				}
			}
			if changedThisPass {
				changed = true
				break
			}
		}
		if !changedThisPass {
			break
		}
	}
	return out, changed
}

func insertCompactHint(messages []HarnessMessage, phase int, tracker *StepTracker) []HarnessMessage {
	hint := HarnessMessage{
		Message: llm.Message{
			Role:    "user",
			Content: compactHintContent(phase, tracker),
		},
		Meta: MessageMeta{Type: MessageTypeCompact, StepIndex: -1},
	}
	insertAt := 0
	if len(messages) > 0 && messages[0].Message.Role == "system" {
		insertAt = 1
	}
	for i, message := range messages {
		if message.Meta.Type == MessageTypePrompt && message.Message.Role == "user" {
			insertAt = i + 1
		}
	}
	out := make([]HarnessMessage, 0, len(messages)+1)
	out = append(out, messages[:insertAt]...)
	out = append(out, hint)
	out = append(out, messages[insertAt:]...)
	return out
}

func compactHintContent(phase int, tracker *StepTracker) string {
	parts := []string{fmt.Sprintf("[Harness compacted context: phase %d]", phase)}
	completed := tracker.Completed()
	if len(completed) == 0 {
		return strings.Join(parts, "\n")
	}
	names := make([]string, 0, len(completed))
	for _, step := range completed {
		if step.ToolName == "" {
			continue
		}
		if step.ToolCallID != "" {
			names = append(names, fmt.Sprintf("%s(%s)", step.ToolName, step.ToolCallID))
			continue
		}
		names = append(names, step.ToolName)
	}
	if len(names) > 0 {
		parts = append(parts, "Steps completed: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "\n")
}

func compactedToolResult(message HarnessMessage) string {
	tool := strings.TrimSpace(message.Meta.ToolName)
	if tool == "" {
		tool = "unknown"
	}
	id := strings.TrimSpace(message.Meta.ToolCallID)
	if id == "" {
		return fmt.Sprintf("[COMPACTED tool result: %s]", tool)
	}
	return fmt.Sprintf("[COMPACTED tool result: %s id=%s]", tool, id)
}

func truncateRunes(content string, limit int) string {
	if limit <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	marker := []rune("\n[TRUNCATED]\n")
	if limit <= len(marker)+4 {
		return string(runes[:limit])
	}
	available := limit - len(marker)
	head := max(available*6/10, 1)
	tail := available - head
	if tail < 1 {
		tail = 1
		head = available - tail
	}
	return string(runes[:head]) + string(marker) + string(runes[len(runes)-tail:])
}

func cloneHarnessMessages(messages []HarnessMessage) []HarnessMessage {
	out := make([]HarnessMessage, len(messages))
	for i, message := range messages {
		out[i] = cloneHarnessMessage(message)
	}
	return out
}

func cloneHarnessMessage(message HarnessMessage) HarnessMessage {
	return HarnessMessage{
		Message: cloneLLMMessage(message.Message),
		Meta:    message.Meta,
	}
}

func cloneLLMMessage(message llm.Message) llm.Message {
	out := message
	if len(message.ToolCalls) > 0 {
		out.ToolCalls = make([]llm.ToolCall, len(message.ToolCalls))
		for i, call := range message.ToolCalls {
			out.ToolCalls[i] = call
			out.ToolCalls[i].Args = append([]byte(nil), call.Args...)
		}
	}
	if len(message.Images) > 0 {
		out.Images = append([]llm.GeneratedImage(nil), message.Images...)
	}
	return out
}
