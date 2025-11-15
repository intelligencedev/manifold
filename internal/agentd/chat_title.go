package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"manifold/internal/llm"
)

const (
	chatTitleMaxRunes = 48
	chatTitleTimeout  = 12 * time.Second
)

var defaultSessionNames = map[string]struct{}{
	"":             {},
	"new chat":     {},
	"conversation": {},
}

func isDefaultSessionName(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	_, ok := defaultSessionNames[trimmed]
	return ok
}

func (a *app) generateChatTitle(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt required")
	}

	provider := a.summaryLLM
	model := strings.TrimSpace(a.cfg.OpenAI.SummaryModel)
	if provider == nil {
		provider = a.llm
	}
	if model == "" {
		model = a.cfg.OpenAI.Model
	}

	if provider == nil {
		return fallbackChatTitle(prompt), fmt.Errorf("llm provider unavailable")
	}

	cctx, cancel := context.WithTimeout(ctx, chatTitleTimeout)
	defer cancel()

	msgs := []llm.Message{
		{
			Role: "system",
			Content: "You write concise titles (max 6 words, max 48 characters) that describe a chat topic based on the first user prompt. " +
				"Respond with only the title, no quotes or punctuation.",
		},
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(cctx, msgs, nil, model)
	if err != nil {
		return fallbackChatTitle(prompt), err
	}

	title := sanitizeGeneratedTitle(resp.Content)
	if title == "" {
		return fallbackChatTitle(prompt), fmt.Errorf("empty title from provider")
	}

	return title, nil
}

func sanitizeGeneratedTitle(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	trimmed := strings.Trim(raw, " \n\r\t\"'`“”‘’")
	trimmed = strings.Trim(trimmed, "-–—•")
	trimmed = collapseWhitespace(trimmed)
	trimmed = strings.Trim(trimmed, ".!?,")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	if utf8.RuneCountInString(trimmed) > chatTitleMaxRunes {
		trimmed = truncateRunes(trimmed, chatTitleMaxRunes)
	}
	return trimmed
}

func fallbackChatTitle(prompt string) string {
	collapsed := collapseWhitespace(prompt)
	if collapsed == "" {
		return "Conversation"
	}
	return truncateRunes(collapsed, chatTitleMaxRunes)
}

func collapseWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(runes[:max]))
}
