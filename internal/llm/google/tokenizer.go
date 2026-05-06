package google

import (
	"context"
	"strings"

	genai "google.golang.org/genai"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// Tokenizer counts tokens with Google's Models.CountTokens API.
type Tokenizer struct {
	client *Client
	model  string
	cache  *llm.TokenCache
}

func NewTokenizer(client *Client, model string, cache *llm.TokenCache) *Tokenizer {
	return &Tokenizer{client: client, model: model, cache: cache}
}

func (t *Tokenizer) CountTokens(ctx context.Context, text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	if t.cache != nil {
		if count, ok := t.cache.Get(text); ok {
			return count, nil
		}
	}
	count, err := t.CountMessagesTokens(ctx, []llm.Message{{Role: "user", Content: text}})
	if err != nil {
		return 0, err
	}
	if t.cache != nil {
		t.cache.Set(text, count)
	}
	return count, nil
}

func (t *Tokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	if t == nil || t.client == nil || t.client.client == nil {
		return llm.EstimateTokensForMessages(msgs), nil
	}
	contents, err := toContents(msgs)
	if err != nil {
		return 0, err
	}
	httpOpts := t.client.httpOptions
	cfg := &genai.CountTokensConfig{HTTPOptions: &httpOpts}
	resp, err := t.client.client.Models.CountTokens(ctx, t.client.pickModel(t.model), contents, cfg)
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("model", t.client.pickModel(t.model)).Int("messages", len(msgs)).Msg("google_count_tokens_error")
		return 0, err
	}
	count := int(resp.TotalTokens)
	observability.LoggerWithTrace(ctx).Debug().Int("total_tokens", count).Int("message_count", len(msgs)).Msg("google_count_tokens_ok")
	return count, nil
}

func (c *Client) Tokenizer(cache *llm.TokenCache) llm.Tokenizer {
	if c == nil {
		return nil
	}
	return NewTokenizer(c, c.model, cache)
}

func (c *Client) SupportsTokenization() bool { return c != nil }

var _ llm.Tokenizer = (*Tokenizer)(nil)
