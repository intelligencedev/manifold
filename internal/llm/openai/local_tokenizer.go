package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// LocalTokenizer counts tokens for OpenAI-compatible local servers that expose
// a llama-server style /tokenize endpoint.
type LocalTokenizer struct {
	client *Client
	model  string
	cache  *llm.TokenCache
}

func NewLocalTokenizer(client *Client, model string, cache *llm.TokenCache) *LocalTokenizer {
	return &LocalTokenizer{client: client, model: model, cache: cache}
}

func (t *LocalTokenizer) CountTokens(ctx context.Context, text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	if t.cache != nil {
		if count, ok := t.cache.Get(text); ok {
			return count, nil
		}
	}
	count, err := t.tokenizeText(ctx, text)
	if err != nil {
		return 0, err
	}
	if t.cache != nil {
		t.cache.Set(text, count)
	}
	return count, nil
}

func (t *LocalTokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	if t == nil || t.client == nil || !t.client.isSelfHosted() {
		return llm.EstimateTokensForMessages(msgs), nil
	}

	prompt := buildPromptText(t.client.chatCompletionMessages(msgs))
	count, err := t.tokenizeText(ctx, prompt)
	if err != nil {
		observability.LoggerWithTrace(ctx).Debug().Err(err).Msg("local_tokenize_failed_using_heuristic")
		return llm.EstimateTokensForMessages(msgs), nil
	}
	observability.LoggerWithTrace(ctx).Debug().Int("total_tokens", count).Int("message_count", len(msgs)).Msg("local_tokens_counted")
	return count, nil
}

func (t *LocalTokenizer) tokenizeText(ctx context.Context, text string) (int, error) {
	if t.client.tokenizeUnsupported.Load() {
		return 0, fmt.Errorf("tokenize unsupported")
	}
	var parsed struct {
		Tokens []any `json:"tokens"`
	}
	status, body, err := t.postJSON(ctx, t.rootURL()+"/tokenize", map[string]any{"content": text}, &parsed)
	if err != nil {
		return 0, err
	}
	if status == http.StatusNotFound {
		t.client.tokenizeUnsupported.Store(true)
		return 0, fmt.Errorf("tokenize returned 404")
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("tokenize returned status %d: %s", status, string(body))
	}
	return len(parsed.Tokens), nil
}

func (t *LocalTokenizer) postJSON(ctx context.Context, url string, bodyObj any, out any) (int, []byte, error) {
	body, err := json.Marshal(bodyObj)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	t.client.applyAuthHeader(req)
	resp, err := t.client.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, respBody, err
		}
	}
	return resp.StatusCode, respBody, nil
}

func (t *LocalTokenizer) rootURL() string {
	base := strings.TrimSuffix(strings.TrimSpace(t.client.baseURL), "/")
	base = strings.TrimSuffix(base, "/v1")
	return base
}

var _ llm.Tokenizer = (*LocalTokenizer)(nil)
