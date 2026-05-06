package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"manifold/internal/config"
	"manifold/internal/llm"
)

type Client struct {
	sdk                      sdk.Client
	model                    string
	extra                    map[string]any
	policy                   responsesContextPolicy
	logPayloads              bool
	baseURL                  string
	httpClient               *http.Client
	api                      string // "completions" (default) or "responses"
	apiKey                   string // Stored for raw HTTP requests (e.g., Gemini)
	inputTokensUnsupported   atomic.Bool
	applyTemplateUnsupported atomic.Bool
	tokenizeUnsupported      atomic.Bool
}

// SupportsCompaction reports whether this OpenAI client is configured for the
// native Responses compaction endpoint. OpenAI-compatible local endpoints often
// implement /responses but not /responses/compact, so only the official OpenAI
// API is auto-enabled here.
func (c *Client) SupportsCompaction() bool {
	if c == nil || !strings.EqualFold(c.api, "responses") {
		return false
	}
	base := strings.TrimSpace(c.baseURL)
	if base == "" {
		return true
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "api.openai.com")
}

// ImageAttachment represents a single image attachment to include in a user message.
// MimeType should be a valid image MIME type, e.g., "image/png" or "image/jpeg".
// Base64Data must be the base64-encoded image bytes (without data URL prefix).
type ImageAttachment struct {
	MimeType   string
	Base64Data string
}

func New(c config.OpenAIConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// For self-hosted mlx_lm.server, wrap the transport to inject Accept: text/event-stream header
	if c.BaseURL != "" && c.BaseURL != "https://api.openai.com/v1" {
		baseURL := strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/")
		if baseURL == "" {
			baseURL = "http://localhost:8000"
		}

		innerTransport := httpClient.Transport
		if innerTransport == nil {
			innerTransport = http.DefaultTransport
		}

		wrappedTransport := &sseTransportWrapper{
			inner:      innerTransport,
			baseURL:    baseURL,
			isSelfHost: true,
		}

		httpClient.Transport = wrappedTransport
	}

	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	opts = append(opts, option.WithHTTPClient(httpClient))

	api := strings.ToLower(strings.TrimSpace(c.API))
	if api == "" {
		api = "completions"
	}
	policy := defaultResponsesContextPolicy(c.Model)
	policy.applyOverrides(llm.NormalizeExtraParams(c.ExtraParams), c.Model)
	return &Client{
		sdk:         sdk.NewClient(opts...),
		model:       c.Model,
		extra:       llm.NormalizeExtraParams(c.ExtraParams),
		policy:      policy,
		logPayloads: c.LogPayloads,
		baseURL:     c.BaseURL,
		httpClient:  httpClient,
		api:         api,
		apiKey:      c.APIKey,
	}
}

func (c *Client) applyAuthHeader(req *http.Request) {
	if c == nil || req == nil {
		return
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return
	}
	if strings.TrimSpace(req.Header.Get("Authorization")) != "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// isSelfHosted returns true when we should use the fallback /tokenize endpoint
// for counting tokens instead of relying on OpenAI usage fields.
func (c *Client) isSelfHosted() bool {
	return c.baseURL != "" && c.baseURL != "https://api.openai.com/v1"
}

// tokenizeCount calls the llama.cpp server /tokenize endpoint to obtain a
// token count for the provided text. Returns 0 on error (best-effort) so that
// metrics emission can still proceed without failing the request.
func (c *Client) tokenizeCount(ctx context.Context, text string) int {
	if !c.isSelfHosted() || strings.TrimSpace(text) == "" {
		return 0
	}
	countCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	base := strings.TrimSuffix(strings.TrimSpace(c.baseURL), "/")
	// Always attempt to trim trailing /v1 for constructing /tokenize endpoint
	base = strings.TrimSuffix(base, "/v1")
	tokenURL := base + "/tokenize"
	bodyObj := map[string]any{"content": text}
	b, _ := json.Marshal(bodyObj)
	req, err := http.NewRequestWithContext(countCtx, http.MethodPost, tokenURL, bytes.NewReader(b))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuthHeader(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}
	var parsed struct {
		Tokens []any `json:"tokens"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return 0
	}
	return len(parsed.Tokens)
}

// buildPromptText flattens chat messages into a single string for approximate
// token counting in self-hosted scenarios. This does not perfectly mirror the
// template expansion but provides a consistent input to /tokenize.
func buildPromptText(msgs []llm.Message) string {
	var sb strings.Builder
	for i, m := range msgs {
		// Include role to differentiate system/user/assistant messages
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		if i < len(msgs)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// normalizeSelfHostedChatMessages rewrites message history for strict self-hosted
// chat templates (e.g., llama.cpp) that require the system message to be first.
// It merges all system messages into a single leading system message while
// preserving the order of all non-system messages.
func normalizeSelfHostedChatMessages(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	var systemParts []string
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if c := strings.TrimSpace(m.Content); c != "" {
				systemParts = append(systemParts, c)
			}
			continue
		}
		out = append(out, m)
	}

	if len(systemParts) == 0 {
		return out
	}

	merged := llm.Message{Role: "system", Content: strings.Join(systemParts, "\n\n")}
	return append([]llm.Message{merged}, out...)
}

func (c *Client) chatCompletionMessages(msgs []llm.Message) []llm.Message {
	if !c.isSelfHosted() {
		return msgs
	}
	return normalizeSelfHostedChatMessages(msgs)
}

// Tokenizer returns the most accurate tokenizer available for the configured
// OpenAI-compatible API surface.
func (c *Client) Tokenizer(cache *llm.TokenCache) llm.Tokenizer {
	if c == nil {
		return nil
	}
	if c.api == "responses" {
		policy := c.responsesContextPolicy(c.model)
		return NewResponsesTokenizer(c, c.model, cache, policy.ToolOutputMaxChars)
	}
	if c.isSelfHosted() {
		return NewLocalTokenizer(c, c.model, cache)
	}
	return nil
}

// SupportsTokenization returns true when the client can attempt provider-backed
// token counting. Official OpenAI uses Responses input_tokens; local compatible
// servers such as llama-server can expose /apply-template and /tokenize.
func (c *Client) SupportsTokenization() bool {
	return c != nil && (c.api == "responses" || c.isSelfHosted())
}
