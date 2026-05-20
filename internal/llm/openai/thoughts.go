package openai

import (
	"encoding/json"
	"strings"

	"manifold/internal/llm"
)

func extractThoughtSignature(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	// Handle both snake_case and camelCase
	if ec, ok := m["extra_content"].(map[string]any); ok {
		if g, ok2 := ec["google"].(map[string]any); ok2 {
			if sig, ok3 := g["thought_signature"].(string); ok3 {
				return sig
			}
		}
	}
	if ec, ok := m["extraContent"].(map[string]any); ok {
		if g, ok2 := ec["google"].(map[string]any); ok2 {
			if sig, ok3 := g["thoughtSignature"].(string); ok3 {
				return sig
			}
		}
	}
	return ""
}

const (
	selfHostedThoughtStartMarker = "<|channel>thought"
	selfHostedThoughtEndMarker   = "<channel|>"
	selfHostedThinkStartMarker   = "<think>"
	selfHostedThinkEndMarker     = "</think>"
)

type thoughtTemplate struct {
	start string
	end   string
}

var selfHostedThoughtTemplates = []thoughtTemplate{
	{start: selfHostedThoughtStartMarker, end: selfHostedThoughtEndMarker},
	{start: selfHostedThinkStartMarker, end: selfHostedThinkEndMarker},
}

type taggedThoughtParseResult struct {
	visible string
	thought string
	tagged  bool
}

type selfHostedThoughtStreamState struct {
	raw              strings.Builder
	emittedVisibleLn int
	lastThought      string
	reasoning        strings.Builder
}

func extractSelfHostedReasoningContent(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		var out strings.Builder
		for _, item := range value {
			part := extractSelfHostedReasoningContent(item)
			if part == "" {
				continue
			}
			out.WriteString(part)
		}
		return out.String()
	case map[string]any:
		for _, key := range []string{"reasoning_content", "reasoningContent", "text", "content", "value", "token"} {
			if nested, ok := value[key]; ok {
				return extractSelfHostedReasoningContent(nested)
			}
		}
	}
	return ""
}

func trimLeadingSingleLineBreak(value string) string {
	if strings.HasPrefix(value, "\r\n") {
		return value[2:]
	}
	if strings.HasPrefix(value, "\n") {
		return value[1:]
	}
	return value
}

func markerSuffixPrefixLen(value, marker string) int {
	max := min(len(marker), len(value))
	for n := max; n > 0; n-- {
		if strings.HasSuffix(value, marker[:n]) {
			return n
		}
	}
	return 0
}

func appendThoughtChunk(dst *strings.Builder, chunk string) {
	if chunk == "" {
		return
	}
	if dst.Len() > 0 {
		dst.WriteString("\n\n")
	}
	dst.WriteString(chunk)
}

func parseTaggedThoughtContent(raw string) taggedThoughtParseResult {
	remaining := raw
	var visible strings.Builder
	var thought strings.Builder
	tagged := false

	for len(remaining) > 0 {
		templateIdx := -1
		startIdx := -1
		for idx, template := range selfHostedThoughtTemplates {
			candidate := strings.Index(remaining, template.start)
			if candidate == -1 {
				continue
			}
			if startIdx == -1 || candidate < startIdx {
				startIdx = candidate
				templateIdx = idx
			}
		}
		if templateIdx == -1 {
			keep := len(remaining)
			for _, template := range selfHostedThoughtTemplates {
				prefixLen := markerSuffixPrefixLen(remaining, template.start)
				candidateKeep := len(remaining) - prefixLen
				if candidateKeep < keep {
					keep = candidateKeep
				}
			}
			if keep > 0 {
				visible.WriteString(remaining[:keep])
			}
			break
		}
		template := selfHostedThoughtTemplates[templateIdx]

		tagged = true
		if startIdx > 0 {
			visible.WriteString(remaining[:startIdx])
		}

		remaining = trimLeadingSingleLineBreak(remaining[startIdx+len(template.start):])
		endIdx := strings.Index(remaining, template.end)
		if endIdx == -1 {
			keep := len(remaining) - markerSuffixPrefixLen(remaining, template.end)
			if keep > 0 {
				appendThoughtChunk(&thought, remaining[:keep])
			}
			break
		}

		appendThoughtChunk(&thought, remaining[:endIdx])
		remaining = remaining[endIdx+len(template.end):]
	}

	return taggedThoughtParseResult{
		visible: visible.String(),
		thought: thought.String(),
		tagged:  tagged,
	}
}

func stripTaggedThoughtContent(raw string) string {
	parsed := parseTaggedThoughtContent(raw)
	if !parsed.tagged {
		return raw
	}
	return parsed.visible
}

func (s *selfHostedThoughtStreamState) emit(delta string, h llm.StreamHandler, visible *strings.Builder) {
	if delta == "" {
		return
	}
	s.raw.WriteString(delta)
	parsed := parseTaggedThoughtContent(s.raw.String())
	if parsed.thought != "" && parsed.thought != s.lastThought {
		s.lastThought = parsed.thought
		if h != nil {
			h.OnThoughtSummary(parsed.thought)
		}
	}
	if len(parsed.visible) <= s.emittedVisibleLn {
		return
	}
	chunk := parsed.visible[s.emittedVisibleLn:]
	s.emittedVisibleLn = len(parsed.visible)
	if chunk == "" {
		return
	}
	if h != nil {
		h.OnDelta(chunk)
	}
	if visible != nil {
		visible.WriteString(chunk)
	}
}

func (s *selfHostedThoughtStreamState) emitReasoning(reasoning string, h llm.StreamHandler) {
	if reasoning == "" {
		return
	}
	next := reasoning
	current := s.reasoning.String()
	if current != "" {
		switch {
		case reasoning == current:
			return
		case len(reasoning) > len(current) && strings.HasPrefix(reasoning, current):
			next = reasoning
		case len(reasoning) < len(current) && strings.HasPrefix(current, reasoning):
			return
		default:
			next = current + reasoning
		}
	}
	if next == s.lastThought {
		return
	}
	s.reasoning.Reset()
	s.reasoning.WriteString(next)
	s.lastThought = next
	if h != nil {
		h.OnThoughtSummary(next)
	}
}
