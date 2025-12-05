package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	rs "github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

type Client struct {
	sdk         sdk.Client
	model       string
	extra       map[string]any
	logPayloads bool
	baseURL     string
	httpClient  *http.Client
	api         string // "completions" (default) or "responses"
	apiKey      string // Stored for raw HTTP requests (e.g., Gemini)
}

// ImageAttachment represents a single image attachment to include in a user message.
// MimeType should be a valid image MIME type, e.g., "image/png" or "image/jpeg".
// Base64Data must be the base64-encoded image bytes (without data URL prefix).
type ImageAttachment struct {
	MimeType   string
	Base64Data string
}

// sseTransportWrapper wraps an HTTP transport to inject the Accept: text/event-stream
// header for streaming requests to self-hosted servers like mlx_lm. This fixes
// compatibility issues where mlx_lm.server expects the SSE accept header for proper
// chunked transfer encoding of streaming responses.
type sseTransportWrapper struct {
	inner      http.RoundTripper
	baseURL    string
	isSelfHost bool
}

func (t *sseTransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only inject header for self-hosted streaming requests
	if t.isSelfHost && strings.HasPrefix(req.URL.String(), t.baseURL) {
		// Check if this is a streaming request by looking for stream=true in params or body
		isStreaming := false
		if req.URL.Query().Get("stream") == "true" {
			isStreaming = true
		} else if req.Body != nil {
			// For POST requests, we need to peek at the body to check for stream=true
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				var payload map[string]any
				if err := json.Unmarshal(bodyBytes, &payload); err == nil {
					if stream, ok := payload["stream"].(bool); ok && stream {
						isStreaming = true
					}
				}
			}
		}

		// Inject the header only for streaming requests (detected via stream:true).
		// mlx_lm.server handles streaming without the Accept header in many cases,
		// but adding it for explicit streaming improves interoperability and is benign.
		if isStreaming {
			req.Header.Set("Accept", "text/event-stream")
		}
	}

	return t.inner.RoundTrip(req)
}

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
	return &Client{
		sdk:         sdk.NewClient(opts...),
		model:       c.Model,
		extra:       c.ExtraParams,
		logPayloads: c.LogPayloads,
		baseURL:     c.BaseURL,
		httpClient:  httpClient,
		api:         api,
		apiKey:      c.APIKey,
	}
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
	base := strings.TrimSuffix(strings.TrimSpace(c.baseURL), "/")
	// Always attempt to trim trailing /v1 for constructing /tokenize endpoint
	base = strings.TrimSuffix(base, "/v1")
	tokenURL := base + "/tokenize"
	bodyObj := map[string]any{"content": text}
	b, _ := json.Marshal(bodyObj)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(b))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
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

// removeUnsupportedSchema recursively deletes keys we know llama.cpp cannot
// handle (currently: "not") and returns the cleaned map.
func removeUnsupportedSchema(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	delete(in, "not")
	for k, v := range in {
		switch tv := v.(type) {
		case map[string]any:
			in[k] = removeUnsupportedSchema(tv)
		case []any:
			for idx, elem := range tv {
				if mm, ok := elem.(map[string]any); ok {
					tv[idx] = removeUnsupportedSchema(mm)
				}
			}
			in[k] = tv
		}
	}
	return in
}

// sanitizeToolSchemas clones and cleans tool schemas for self-hosted llama.cpp.
func sanitizeToolSchemas(src []llm.ToolSchema) []llm.ToolSchema {
	if len(src) == 0 {
		return src
	}
	out := make([]llm.ToolSchema, 0, len(src))
	for _, s := range src {
		if s.Parameters != nil {
			// shallow copy map to avoid mutating original
			cp := make(map[string]any, len(s.Parameters))
			for k, v := range s.Parameters {
				cp[k] = v
			}
			cleaned := removeUnsupportedSchema(cp)
			if len(cleaned) == 0 {
				s.Parameters = nil
			} else {
				s.Parameters = cleaned
			}
		}
		out = append(out, s)
	}
	return out
}

// ensureStrictJSONSchema enforces additionalProperties:false wherever a schema
// object is present. This matches stricter validation requirements of
// the Responses API for function tool parameters.
func ensureStrictJSONSchema(in any) any {
	switch v := in.(type) {
	case map[string]any:
		// If it looks like an object schema (has properties or type==object), force additionalProperties:false
		// Also ensure type: object is present when properties exist.
		if v["type"] == "object" || v["properties"] != nil || v["required"] != nil {
			v["additionalProperties"] = false
			if _, hasType := v["type"]; !hasType && v["properties"] != nil {
				v["type"] = "object"
			}
		}
		// Recurse into known schema containers
		if props, ok := v["properties"].(map[string]any); ok {
			for k, child := range props {
				props[k] = ensureStrictJSONSchema(child)
			}
			v["properties"] = props
		}
		if items, ok := v["items"]; ok {
			v["items"] = ensureStrictJSONSchema(items)
		}
		if allOf, ok := v["allOf"].([]any); ok {
			for i, child := range allOf {
				allOf[i] = ensureStrictJSONSchema(child)
			}
			v["allOf"] = allOf
		}
		if anyOf, ok := v["anyOf"].([]any); ok {
			for i, child := range anyOf {
				anyOf[i] = ensureStrictJSONSchema(child)
			}
			v["anyOf"] = anyOf
		}
		if oneOf, ok := v["oneOf"].([]any); ok {
			for i, child := range oneOf {
				oneOf[i] = ensureStrictJSONSchema(child)
			}
			v["oneOf"] = oneOf
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = ensureStrictJSONSchema(child)
		}
		return v
	default:
		return in
	}
}

// extractReasoningEffort copies the reasoning_effort flag into the typed
// Responses.Reasoning field and removes the deprecated top-level parameter so
// it is not sent to the API.
func extractReasoningEffort(extra map[string]any) (shared.ReasoningEffort, bool) {
	if extra == nil {
		return "", false
	}
	raw, ok := extra["reasoning_effort"]
	if !ok {
		return "", false
	}
	delete(extra, "reasoning_effort")
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return shared.ReasoningEffort(s), true
}

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if imgOpts, ok := llm.ImagePromptFromContext(ctx); ok {
		return c.chatWithImageGeneration(ctx, msgs, model, imgOpts)
	}

	if strings.EqualFold(c.api, "responses") {
		return c.chatResponses(ctx, msgs, tools, model, nil)
	}

	effectiveModel := firstNonEmpty(model, c.model)
	// For Gemini 3 models, use raw HTTP request to preserve thought_signature fields
	if isGemini3Model(effectiveModel) {
		return c.chatGeminiRaw(ctx, msgs, tools, effectiveModel)
	}

	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(effectiveModel),
	}
	// messages
	params.Messages = AdaptMessages(string(params.Model), msgs)
	// tools: include only when provided to avoid sending an empty array
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			params.Tools = AdaptSchemas(tools)
		}
	}
	if len(c.extra) > 0 {
		// When no tools are provided, ensure we don't forward tool-specific
		// flags from the client extra params.
		if len(tools) == 0 {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(tmp)
		} else {
			params.SetExtraFields(c.extra)
		}
	}
	// Start a tracing span and log prompt for correlation
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Chat", string(params.Model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	f := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	f = f.Int("prompt_tokens", int(comp.Usage.PromptTokens)).
		Int("completion_tokens", int(comp.Usage.CompletionTokens)).
		Int("total_tokens", int(comp.Usage.TotalTokens))

	// Attempt to surface any nested token detail attributes that the API returned
	var usageMap map[string]any
	if b, err := json.Marshal(comp.Usage); err == nil {
		if err := json.Unmarshal(b, &usageMap); err == nil {
			if v, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
				for k, val := range v {
					if num, ok := val.(float64); ok {
						f = f.Int("prompt_tokens_details_"+k, int(num))
					}
				}
			}
			if v, ok := usageMap["completion_tokens_details"].(map[string]any); ok {
				for k, val := range v {
					if num, ok := val.(float64); ok {
						f = f.Int("completion_tokens_details_"+k, int(num))
					}
				}
			}
		}
	}

	fields := f.Logger()
	if c.logPayloads && c.extra != nil && len(c.extra) > 0 {
		if b, err := json.Marshal(c.extra); err == nil {
			fields = fields.With().RawJSON("extra", observability.RedactJSON(b)).Logger()
		}
	}
	fields.Debug().Msg("chat_completion_ok")

	// Prepare assistant message output before token fallback
	var out llm.Message
	gemini := isGemini3Model(string(params.Model))
	if len(comp.Choices) > 0 {
		msg := comp.Choices[0].Message
		out = llm.Message{Role: "assistant", Content: msg.Content}
		for _, tc := range msg.ToolCalls {
			switch v := tc.AsAny().(type) {
			case sdk.ChatCompletionMessageFunctionToolCall:
				// Skip tool calls with empty or effectively empty arguments to prevent JSON unmarshal errors
				if isEmptyArgs(v.Function.Arguments) {
					log.Warn().Str("tool", v.Function.Name).Str("id", v.ID).Msg("skipping tool call with empty arguments")
					continue
				}
				sig := ""
				if gemini {
					sig = extractThoughtSignature(v.RawJSON())
				}
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
					Name:             v.Function.Name,
					Args:             json.RawMessage(v.Function.Arguments),
					ID:               v.ID,
					ThoughtSignature: sig,
				})
			case sdk.ChatCompletionMessageCustomToolCall:
				// Skip tool calls with empty input to prevent JSON unmarshal errors
				if isEmptyArgs(v.Custom.Input) {
					log.Warn().Str("tool", v.Custom.Name).Str("id", v.ID).Msg("skipping tool call with empty input")
					continue
				}
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
					Name: v.Custom.Name,
					Args: json.RawMessage(v.Custom.Input),
					ID:   v.ID,
				})
			}
		}
	}

	// Redacted response logging
	llm.LogRedactedResponse(ctx, comp.Choices)

	if c.isSelfHosted() {
		// Override token metrics using /tokenize endpoint
		promptText := buildPromptText(msgs)
		promptTokens := c.tokenizeCount(ctx, promptText)
		completionTokens := c.tokenizeCount(ctx, out.Content)
		totalTokens := promptTokens + completionTokens
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	} else {
		// Use OpenAI provided usage
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetrics(string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
	}
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	return out, nil
}

// chatGeminiRaw makes a raw HTTP request to Gemini models via the OpenAI compatibility endpoint,
// preserving thought_signature fields in tool calls which the SDK doesn't support.
func (c *Client) chatGeminiRaw(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Chat (Gemini Raw)", model, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	// Build raw request body
	body := map[string]any{
		"model":    model,
		"messages": AdaptMessagesRaw(model, msgs),
	}
	if len(tools) > 0 {
		// Convert tools to raw JSON format
		rawTools := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			rawTools = append(rawTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = rawTools
	}
	// Merge extra params
	if len(c.extra) > 0 {
		for k, v := range c.extra {
			if k == "model" || k == "messages" || k == "tools" {
				continue
			}
			if k == "parallel_tool_calls" && len(tools) == 0 {
				continue
			}
			body[k] = v
		}
	}

	// Determine endpoint URL
	baseURL := strings.TrimSuffix(strings.TrimSpace(c.baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	url := baseURL + "/chat/completions"

	// Marshal request
	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Message{}, fmt.Errorf("marshal gemini request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return llm.Message{}, fmt.Errorf("create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Set API key header. Gemini OpenAI compatibility endpoint expects Authorization: Bearer token
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("model", model).Int("tools", len(tools)).Msg("gemini_raw_request_error")
		span.RecordError(err)
		return llm.Message{}, fmt.Errorf("gemini raw request: %w", err)
	}
	defer resp.Body.Close()
	dur := time.Since(start)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error().Int("status", resp.StatusCode).RawJSON("body", observability.RedactJSON(bodyBytes)).
			Str("model", model).Dur("duration", dur).Msg("gemini_raw_bad_status")
		return llm.Message{}, fmt.Errorf("gemini raw request: status %d", resp.StatusCode)
	}

	// Parse response
	var compResp struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID           string         `json:"id"`
					Type         string         `json:"type"`
					Function     map[string]any `json:"function"`
					ExtraContent map[string]any `json:"extra_content"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&compResp); err != nil {
		log.Error().Err(err).Str("model", model).Dur("duration", dur).Msg("gemini_raw_decode_error")
		span.RecordError(err)
		return llm.Message{}, fmt.Errorf("decode gemini response: %w", err)
	}

	// Log metrics
	log.Debug().Str("model", model).Int("tools", len(tools)).Dur("duration", dur).
		Int("prompt_tokens", compResp.Usage.PromptTokens).
		Int("completion_tokens", compResp.Usage.CompletionTokens).
		Int("total_tokens", compResp.Usage.TotalTokens).
		Msg("gemini_raw_ok")

	// Build output message
	var out llm.Message
	if len(compResp.Choices) > 0 {
		choice := compResp.Choices[0]
		out.Role = "assistant"
		out.Content = choice.Message.Content

		for _, tc := range choice.Message.ToolCalls {
			call := llm.ToolCall{
				ID: tc.ID,
			}
			if fn, ok := tc.Function["name"].(string); ok {
				call.Name = fn
			}
			if args, ok := tc.Function["arguments"].(string); ok && args != "" {
				call.Args = json.RawMessage(args)
			}
			// Extract thought_signature from extra_content
			if tc.ExtraContent != nil {
				if google, ok := tc.ExtraContent["google"].(map[string]any); ok {
					if sig, ok := google["thought_signature"].(string); ok {
						call.ThoughtSignature = sig
					}
				}
			}
			// Skip tool calls with empty arguments
			if call.Name != "" && len(call.Args) > 0 {
				out.ToolCalls = append(out.ToolCalls, call)
			} else if call.Name != "" {
				log.Warn().Str("tool", call.Name).Str("id", call.ID).Msg("skipping Gemini tool call with empty arguments")
			}
		}
	}

	llm.LogRedactedResponse(ctx, compResp.Choices)
	llm.RecordTokenAttributes(span, compResp.Usage.PromptTokens, compResp.Usage.CompletionTokens, compResp.Usage.TotalTokens)
	llm.RecordTokenMetrics(model, compResp.Usage.PromptTokens, compResp.Usage.CompletionTokens)

	return out, nil
}

// chatGeminiRawStream implements streaming for Gemini models using raw HTTP requests
// to properly handle thought_signature in extra_content fields.
func (c *Client) chatGeminiRawStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Gemini Raw Stream", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	// Build raw request body
	body := map[string]any{
		"model":    firstNonEmpty(model, c.model),
		"messages": AdaptMessagesRaw(firstNonEmpty(model, c.model), msgs),
		"stream":   true,
	}
	if len(tools) > 0 {
		// Convert tools to raw JSON format
		rawTools := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			rawTools = append(rawTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = rawTools
	}
	if len(c.extra) > 0 {
		for k, v := range c.extra {
			if k == "model" || k == "messages" || k == "stream" || k == "tools" {
				continue
			}
			body[k] = v
		}
	}

	payload, _ := json.Marshal(body)
	url := strings.TrimSuffix(c.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("gemini_raw_stream_request_error")
		span.RecordError(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		log.Error().Int("status", resp.StatusCode).RawJSON("body", observability.RedactJSON(b)).Msg("gemini_raw_stream_bad_status")
		return fmt.Errorf("gemini raw stream: status %d", resp.StatusCode)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	toolCalls := make(map[int]*llm.ToolCall)
	toolCallsFlushed := false
	var assistantContent strings.Builder
	var promptTokens, completionTokens, totalTokens int

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index        int            `json:"index"`
						ID           string         `json:"id"`
						Type         string         `json:"type"`
						Function     map[string]any `json:"function"`
						ExtraContent map[string]any `json:"extra_content"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Handle usage if present
		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
			totalTokens = chunk.Usage.TotalTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Handle content deltas
		if delta.Content != "" {
			h.OnDelta(delta.Content)
			assistantContent.WriteString(delta.Content)
		}

		// Handle tool calls
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if toolCalls[idx] == nil {
				toolCalls[idx] = &llm.ToolCall{ID: tc.ID}
			}

			if name, ok := tc.Function["name"].(string); ok && name != "" {
				toolCalls[idx].Name = name
			}
			if args, ok := tc.Function["arguments"].(string); ok && args != "" {
				if toolCalls[idx].Args == nil {
					toolCalls[idx].Args = json.RawMessage(args)
				} else {
					existing := string(toolCalls[idx].Args)
					toolCalls[idx].Args = json.RawMessage(existing + args)
				}
			}

			// Extract thought_signature from extra_content
			if tc.ExtraContent != nil && toolCalls[idx].ThoughtSignature == "" {
				if google, ok := tc.ExtraContent["google"].(map[string]any); ok {
					if sig, ok := google["thought_signature"].(string); ok {
						toolCalls[idx].ThoughtSignature = sig
					}
				}
			}
		}

		// Check for finish_reason to flush tool calls
		if chunk.Choices[0].FinishReason != "" && !toolCallsFlushed {
			for _, tc := range toolCalls {
				if tc != nil && tc.Name != "" && !isEmptyArgsBytes(tc.Args) {
					h.OnToolCall(*tc)
				}
			}
			toolCallsFlushed = true
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("gemini_raw_stream_scan_error")
		span.RecordError(err)
		return err
	}

	dur := time.Since(start)
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	if promptTokens > 0 || completionTokens > 0 {
		llm.RecordTokenMetrics(firstNonEmpty(model, c.model), promptTokens, completionTokens)
	}
	llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
	log.Debug().Dur("duration", dur).Int("prompt_tokens", promptTokens).Int("completion_tokens", completionTokens).Msg("gemini_raw_stream_ok")

	return nil
}

// ChatWithOptions is like Chat but allows callers to:
//   - omit tools entirely by passing a nil or empty tools slice
//   - inject provider-specific extra fields (e.g., reasoning_effort)
//     via params.WithExtraField.
func (c *Client) ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	if strings.EqualFold(c.api, "responses") {
		return c.chatResponses(ctx, msgs, tools, model, extra)
	}
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatWithOptions", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	params.Messages = AdaptMessages(string(params.Model), msgs)
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			params.Tools = AdaptSchemas(tools)
		}
	}
	if len(c.extra) > 0 || len(extra) > 0 {
		merged := make(map[string]any, len(c.extra)+len(extra))
		for k, v := range c.extra {
			merged[k] = v
		}
		for k, v := range extra {
			merged[k] = v
		}
		// Some provider-specific flags (e.g., parallel_tool_calls) are only
		// valid when tools are actually provided. Remove those keys when no
		// tools are present to avoid 400 errors from the API.
		if len(tools) == 0 {
			delete(merged, "parallel_tool_calls")
		}
		params.SetExtraFields(merged)
	}
	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	f := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	f = f.Int("prompt_tokens", int(comp.Usage.PromptTokens)).
		Int("completion_tokens", int(comp.Usage.CompletionTokens)).
		Int("total_tokens", int(comp.Usage.TotalTokens))

	// Attempt to surface any nested token detail attributes that the API returned
	var usageMap map[string]any
	if b, err := json.Marshal(comp.Usage); err == nil {
		if err := json.Unmarshal(b, &usageMap); err == nil {
			if v, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
				for k, val := range v {
					if num, ok := val.(float64); ok {
						f = f.Int("prompt_tokens_details_"+k, int(num))
					}
				}
			}
			if v, ok := usageMap["completion_tokens_details"].(map[string]any); ok {
				for k, val := range v {
					if num, ok := val.(float64); ok {
						f = f.Int("completion_tokens_details_"+k, int(num))
					}
				}
			}
		}
	}

	fields := f.Logger()
	if c.logPayloads && c.extra != nil && len(c.extra) > 0 {
		if b, err := json.Marshal(c.extra); err == nil {
			fields = fields.With().RawJSON("extra", observability.RedactJSON(b)).Logger()
		}
	}
	fields.Debug().Msg("chat_completion_ok")
	// Prepare assistant output first
	var out llm.Message
	gemini := isGemini3Model(string(params.Model))
	if len(comp.Choices) > 0 {
		msg := comp.Choices[0].Message
		out = llm.Message{Role: "assistant", Content: msg.Content}
		for _, tc := range msg.ToolCalls {
			switch v := tc.AsAny().(type) {
			case sdk.ChatCompletionMessageFunctionToolCall:
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
					Name: v.Function.Name,
					Args: json.RawMessage(v.Function.Arguments),
					ID:   v.ID,
					ThoughtSignature: func() string {
						if gemini {
							return extractThoughtSignature(v.RawJSON())
						}
						return ""
					}(),
				})
			case sdk.ChatCompletionMessageCustomToolCall:
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
					Name: v.Custom.Name,
					Args: json.RawMessage(v.Custom.Input),
					ID:   v.ID,
				})
			}
		}
	}

	llm.LogRedactedResponse(ctx, comp.Choices)
	if c.isSelfHosted() {
		promptTokens := c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens := c.tokenizeCount(ctx, out.Content)
		totalTokens := promptTokens + completionTokens
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	} else {
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetrics(string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
	}
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	return out, nil
}

// ChatStream implements streaming chat completions using OpenAI's streaming API.
func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	if imgOpts, ok := llm.ImagePromptFromContext(ctx); ok {
		msg, err := c.chatWithImageGeneration(ctx, msgs, model, imgOpts)
		if err != nil {
			return err
		}
		if h != nil {
			if strings.TrimSpace(msg.Content) != "" {
				h.OnDelta(msg.Content)
			}
			for _, img := range msg.Images {
				h.OnImage(img)
			}
		}
		return nil
	}
	if strings.EqualFold(c.api, "responses") {
		return c.chatStreamResponses(ctx, msgs, tools, model, h)
	}
	if isGemini3Model(firstNonEmpty(model, c.model)) {
		// Use raw streaming implementation for Gemini to properly handle thought_signature
		return c.chatGeminiRawStream(ctx, msgs, tools, model, h)
	}
	// For self-hosted backends (llama.cpp, mlx_lm.server, etc.), prefer a generic SSE reader
	// to maximize compatibility. Some servers diverge slightly from OpenAI's
	// streaming chunk schema which can cause the SDK parser to abort and close
	// the connection early (observed with mlx_lm.server BrokenPipeError).
	if c.isSelfHosted() {
		return c.chatStreamSSEFallback(ctx, msgs, tools, model, h)
	}
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// Start tracing and log prompt
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatStream", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	// messages
	params.Messages = AdaptMessages(string(params.Model), msgs)
	// tools
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			params.Tools = AdaptSchemas(tools)
		}
	}
	if len(c.extra) > 0 {
		// When no tools are provided, ensure we don't forward tool-specific
		// flags from the client extra params.
		if len(tools) == 0 {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(tmp)
		} else {
			params.SetExtraFields(c.extra)
		}
	}
	// Ask the API to include a final usage chunk so we can log token counts.
	// Some self-hosted backends (e.g., mlx_lm.server) may not support this flag
	// or may behave inconsistently, so only enable it for OpenAI cloud.
	if !c.isSelfHosted() {
		params.StreamOptions.IncludeUsage = sdk.Bool(true)
	}

	start := time.Now()
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	defer func() {
		_ = stream.Close()
	}()

	// Accumulate tool calls across chunks since they come incrementally
	toolCalls := make(map[int]*llm.ToolCall)
	// Track whether we've flushed tool calls to the handler
	toolCallsFlushed := false
	// Track token usage (filled from the final usage chunk if available)
	var promptTokens, completionTokens, totalTokens int
	// Hold any nested usage detail numeric fields so we can log them at the end
	promptDetails := make(map[string]int)
	completionDetails := make(map[string]int)

	// Collect assistant content for self-hosted tokenization fallback
	var assistantContentBuilder strings.Builder
	gemini := isGemini3Model(string(params.Model))

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			// Even if there are no choices, the final chunk may contain usage
			if chunk.JSON.Usage.Valid() && chunk.JSON.Usage.Raw() != "null" {
				promptTokens = int(chunk.Usage.PromptTokens)
				completionTokens = int(chunk.Usage.CompletionTokens)
				totalTokens = int(chunk.Usage.TotalTokens)

				// Try to extract nested detail maps from the raw JSON usage
				var usageMap map[string]any
				if raw := chunk.JSON.Usage.Raw(); raw != "" && raw != "null" {
					if err := json.Unmarshal([]byte(raw), &usageMap); err == nil {
						if v, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
							for k, val := range v {
								if num, ok := val.(float64); ok {
									promptDetails[k] = int(num)
								}
							}
						}
						if v, ok := usageMap["completion_tokens_details"].(map[string]any); ok {
							for k, val := range v {
								if num, ok := val.(float64); ok {
									completionDetails[k] = int(num)
								}
							}
						}
					}
				}
			}
			continue
		}

		delta := chunk.Choices[0].Delta

		// Handle content deltas
		if delta.Content != "" {
			h.OnDelta(delta.Content)
			assistantContentBuilder.WriteString(delta.Content)
		}

		// Handle tool calls - accumulate across chunks
		// Use tc.Index (the API-provided index) NOT the range iteration index,
		// because chunks may arrive out of order or contain only a subset of tool calls.
		for _, tc := range delta.ToolCalls {
			idx := int(tc.Index)
			if toolCalls[idx] == nil {
				toolCalls[idx] = &llm.ToolCall{
					ID: tc.ID,
				}
			}

			// Accumulate function name
			if tc.Function.Name != "" {
				toolCalls[idx].Name = tc.Function.Name
			}

			// Accumulate function arguments
			if tc.Function.Arguments != "" {
				if toolCalls[idx].Args == nil {
					toolCalls[idx].Args = json.RawMessage(tc.Function.Arguments)
				} else {
					// Append new arguments to existing ones
					existing := string(toolCalls[idx].Args)
					toolCalls[idx].Args = json.RawMessage(existing + tc.Function.Arguments)
				}
			}
			if gemini && toolCalls[idx].ThoughtSignature == "" {
				toolCalls[idx].ThoughtSignature = extractThoughtSignature(tc.RawJSON())
			}
		}

		// Check if we're done with this step (finish_reason indicates completion)
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" && !toolCallsFlushed {
			// Send all accumulated tool calls
			for _, tc := range toolCalls {
				if tc != nil && tc.Name != "" && !isEmptyArgsBytes(tc.Args) {
					h.OnToolCall(*tc)
				} else if tc != nil && tc.Name != "" {
					log.Warn().Str("tool", tc.Name).Str("id", tc.ID).Msg("skipping tool call with empty arguments in stream")
				}
			}
			toolCallsFlushed = true
			// Do not break: we may still receive a final usage chunk
		}
	}

	err := stream.Err()
	dur := time.Since(start)
	// Build base logger and include nested usage detail fields if available
	baseBuilder := log.With().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
		Dur("duration", dur).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens)

	// Append any nested prompt detail fields we captured
	for k, v := range promptDetails {
		baseBuilder = baseBuilder.Int("prompt_tokens_details_"+k, v)
	}
	for k, v := range completionDetails {
		baseBuilder = baseBuilder.Int("completion_tokens_details_"+k, v)
	}

	base := baseBuilder.Logger()
	if err != nil {
		base.Error().Err(err).Msg("chat_stream_error")
		span.RecordError(err)
	} else {
		if c.isSelfHosted() {
			// Override counts by re-tokenizing prompt and accumulated assistant content
			promptTokens = c.tokenizeCount(ctx, buildPromptText(msgs))
			completionTokens = c.tokenizeCount(ctx, assistantContentBuilder.String())
			totalTokens = promptTokens + completionTokens
		}
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
		if promptTokens > 0 || completionTokens > 0 {
			llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
		}
		base.Debug().Msg("chat_stream_ok")
	}
	return err
}

// chatStreamSSEFallback implements a tolerant SSE reader for self-hosted servers
// (mlx_lm.server, llama.cpp, etc.). It posts to /v1/chat/completions with
// stream=true, sets Accept: text/event-stream, then parses lines prefixed with
// "data: ", attempting to extract deltas from a variety of chunk shapes.
func (c *Client) chatStreamSSEFallback(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	// Start tracing and log prompt
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatStream (SSE Fallback)", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	// Build URL (ensure single /v1 prefix)
	base := strings.TrimSuffix(strings.TrimSpace(c.baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	url := base + "/chat/completions"

	// Build body
	body := map[string]any{
		"model":    firstNonEmpty(model, c.model),
		"messages": AdaptMessages(model, msgs),
		"stream":   true,
	}
	if len(tools) > 0 {
		if c.isSelfHosted() {
			body["tools"] = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			body["tools"] = AdaptSchemas(tools)
		}
	}
	// Merge extra params, but drop tool flags if no tools
	if len(c.extra) > 0 {
		tmp := make(map[string]any, len(c.extra))
		for k, v := range c.extra {
			tmp[k] = v
		}
		if len(tools) == 0 {
			delete(tmp, "parallel_tool_calls")
		}
		for k, v := range tmp {
			// do not overwrite required fields above unless explicitly provided
			if k == "model" || k == "messages" || k == "stream" {
				continue
			}
			body[k] = v
		}
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// Note: API key header handling is configured at higher layers or via server settings.

	// Allow custom headers via ExtraParams["extra_headers"] if provided by config
	// But we already support config.OpenAI.ExtraHeaders at handlers layer; keeping minimal here

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		log.Error().Int("status", resp.StatusCode).RawJSON("body", observability.RedactJSON(b)).Msg("sse_fallback_bad_status")
		return fmt.Errorf("chatStream SSE fallback: status %d", resp.StatusCode)
	}

	start := time.Now()

	// Accumulate for token metrics fallback
	var assistantContentBuilder strings.Builder
	// Tool calls accumulation
	toolCalls := make(map[int]*llm.ToolCall)
	toolCallsFlushed := false

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer in case of large JSON chunks
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		// Parse JSON payload liberally
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			// Skip invalid JSON chunks rather than aborting the stream
			continue
		}

		// Try OpenAI-style: choices[0].delta.content
		if choices, ok := m["choices"].([]any); ok && len(choices) > 0 {
			if ch, ok := choices[0].(map[string]any); ok {
				// delta.content
				if delta, ok := ch["delta"].(map[string]any); ok {
					if s, ok := delta["content"].(string); ok && s != "" {
						h.OnDelta(s)
						assistantContentBuilder.WriteString(s)
					}
					// tool calls accumulation (function.name, function.arguments)
					if tcs, ok := delta["tool_calls"].([]any); ok {
						for i, tcv := range tcs {
							if tcv == nil {
								continue
							}
							if toolCalls[i] == nil {
								toolCalls[i] = &llm.ToolCall{}
							}
							if tcm, ok := tcv.(map[string]any); ok {
								if id, ok := tcm["id"].(string); ok && id != "" {
									toolCalls[i].ID = id
								}
								if fn, ok := tcm["function"].(map[string]any); ok {
									if name, ok := fn["name"].(string); ok && name != "" {
										toolCalls[i].Name = name
									}
									if args, ok := fn["arguments"].(string); ok && args != "" {
										if toolCalls[i].Args == nil {
											toolCalls[i].Args = json.RawMessage(args)
										} else {
											existing := string(toolCalls[i].Args)
											toolCalls[i].Args = json.RawMessage(existing + args)
										}
									}
								}
							}
						}
					}
				}
				// finish_reason -> flush tool calls once
				if fr, ok := ch["finish_reason"].(string); ok && fr != "" && !toolCallsFlushed {
					for _, tc := range toolCalls {
						if tc != nil && tc.Name != "" && len(tc.Args) > 0 {
							h.OnToolCall(*tc)
						}
					}
					toolCallsFlushed = true
				}
				// Some servers send message at end; capture for completeness
				if msg, ok := ch["message"].(map[string]any); ok {
					if s, ok := msg["content"].(string); ok && s != "" {
						h.OnDelta(s)
						assistantContentBuilder.WriteString(s)
					}
				}
			}
			continue
		}

		// mlx_lm compatibility: sometimes payload may contain {"response":"..."}
		if s, ok := m["response"].(string); ok && s != "" {
			h.OnDelta(s)
			assistantContentBuilder.WriteString(s)
			continue
		}
		// Another possible token key
		if s, ok := m["token"].(string); ok && s != "" {
			h.OnDelta(s)
			assistantContentBuilder.WriteString(s)
			continue
		}
	}
	// Any scanner error is non-fatal if we received some content
	scanErr := scanner.Err()

	// Token metrics fallback using /tokenize if available
	if c.isSelfHosted() {
		promptTokens := c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens := c.tokenizeCount(ctx, assistantContentBuilder.String())
		totalTokens := promptTokens + completionTokens
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		if promptTokens > 0 || completionTokens > 0 {
			llm.RecordTokenMetrics(firstNonEmpty(model, c.model), promptTokens, completionTokens)
		}
		llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
	}

	dur := time.Since(start)
	if scanErr != nil && !errors.Is(scanErr, context.Canceled) {
		observability.LoggerWithTrace(ctx).Error().Err(scanErr).Dur("duration", dur).Msg("chat_stream_sse_fallback_error")
		span.RecordError(scanErr)
		return scanErr
	}
	observability.LoggerWithTrace(ctx).Debug().Dur("duration", dur).Msg("chat_stream_sse_fallback_ok")
	return nil
}

func (c *Client) chatWithImageGeneration(ctx context.Context, msgs []llm.Message, model string, opts llm.ImagePromptOptions) (llm.Message, error) {
	prompt := lastUserPrompt(msgs)
	if strings.TrimSpace(prompt) == "" {
		return llm.Message{}, fmt.Errorf("image generation requires a user prompt")
	}

	imgModel := c.imageModel(model)
	size := normalizeImageSize(opts.Size)

	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ImageGen", imgModel, 0, len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	params := sdk.ImageGenerateParams{
		Prompt: prompt,
		Model:  sdk.ImageModel(imgModel),
		N:      param.NewOpt[int64](1),
		Size:   sdk.ImageGenerateParamsSize(size),
	}

	start := time.Now()
	resp, err := c.sdk.Images.Generate(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", imgModel).Dur("duration", dur).Msg("image_generation_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	images := make([]llm.GeneratedImage, 0, len(resp.Data))
	for _, img := range resp.Data {
		if strings.TrimSpace(img.B64JSON) == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(img.B64JSON)
		if err != nil {
			log.Warn().Err(err).Msg("decode_generated_image")
			continue
		}
		images = append(images, llm.GeneratedImage{
			Data:     data,
			MIMEType: "image/png",
		})
	}
	log.Debug().Str("model", imgModel).Dur("duration", dur).Int("images", len(images)).Msg("image_generation_ok")

	content := "Generated image"
	if len(images) > 1 {
		content = fmt.Sprintf("Generated %d images", len(images))
	}
	return llm.Message{Role: "assistant", Content: content, Images: images}, nil
}

func lastUserPrompt(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(msgs[i].Role, "user") && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

func normalizeImageSize(raw string) string {
	r := strings.TrimSpace(raw)
	switch strings.ToUpper(r) {
	case "1K", "1024", "1024X1024":
		return "1024x1024"
	case "1024X1792", "PORTRAIT":
		return "1024x1792"
	case "1792X1024", "LANDSCAPE":
		return "1792x1024"
	default:
		if r == "" {
			return "1024x1024"
		}
		return r
	}
}

func (c *Client) imageModel(model string) string {
	m := strings.TrimSpace(firstNonEmpty(model, c.model))
	if m == "" {
		m = "gpt-image-1"
	}
	lower := strings.ToLower(m)
	if strings.Contains(lower, "gpt-image") || strings.Contains(lower, "dall-e") {
		return m
	}
	return "gpt-image-1"
}

// ChatWithImageAttachment sends a chat completion with an image attachment.
// This is a concrete method specific to the OpenAI provider.
func (c *Client) ChatWithImageAttachment(ctx context.Context, msgs []llm.Message, mimeType, base64Data string, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatWithImageAttachment", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}

	// Convert all messages except the last user message, then replace it with image content
	adaptedMsgs := AdaptMessages(model, msgs)
	if len(adaptedMsgs) > 0 {
		// Find the last user message and replace it with image content
		for i := len(adaptedMsgs) - 1; i >= 0; i-- {
			if adaptedMsgs[i].OfUser != nil {
				userMsg := adaptedMsgs[i].OfUser

				// Create content parts: text + image
				var contentParts []sdk.ChatCompletionContentPartUnionParam

				// Add text content if present
				if userMsg.Content.OfString.Valid() && userMsg.Content.OfString.Value != "" {
					contentParts = append(contentParts, sdk.ChatCompletionContentPartUnionParam{
						OfText: &sdk.ChatCompletionContentPartTextParam{
							Text: userMsg.Content.OfString.Value,
						},
					})
				}

				// Add image content part
				dataURL := "data:" + mimeType + ";base64," + base64Data
				contentParts = append(contentParts, sdk.ChatCompletionContentPartUnionParam{
					OfImageURL: &sdk.ChatCompletionContentPartImageParam{
						ImageURL: sdk.ChatCompletionContentPartImageImageURLParam{
							URL: dataURL,
						},
					},
				})

				// Replace with content parts
				newUserMsg := sdk.ChatCompletionUserMessageParam{
					Content: sdk.ChatCompletionUserMessageParamContentUnion{
						OfArrayOfContentParts: contentParts,
					},
				}
				adaptedMsgs[i] = sdk.ChatCompletionMessageParamUnion{OfUser: &newUserMsg}
				break
			}
		}
	}

	params.Messages = adaptedMsgs
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			params.Tools = AdaptSchemas(tools)
		}
	}
	if len(c.extra) > 0 {
		if len(tools) == 0 {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(tmp)
		} else {
			params.SetExtraFields(c.extra)
		}
	}

	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_with_image_error")
		span.RecordError(err)
		return llm.Message{}, err
	}

	log.Debug().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_with_image_ok")
	// Log response and token counts when available
	llm.LogRedactedResponse(ctx, comp.Choices)
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	msg := comp.Choices[0].Message
	out := llm.Message{Role: "assistant", Content: msg.Content}
	if c.isSelfHosted() {
		promptTokens := c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens := c.tokenizeCount(ctx, out.Content)
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, promptTokens+completionTokens)
		llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	} else {
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetrics(string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
	}
	gemini := isGemini3Model(firstNonEmpty(model, c.model))
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case sdk.ChatCompletionMessageFunctionToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Function.Name,
				Args: json.RawMessage(v.Function.Arguments),
				ID:   v.ID,
				ThoughtSignature: func() string {
					if gemini {
						return extractThoughtSignature(v.RawJSON())
					}
					return ""
				}(),
			})
		case sdk.ChatCompletionMessageCustomToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Custom.Name,
				Args: json.RawMessage(v.Custom.Input),
				ID:   v.ID,
			})
		}
	}
	return out, nil
}

// ChatWithImageAttachments sends a chat completion with one or more image attachments.
// The images are included as content parts alongside the user's text.
func (c *Client) ChatWithImageAttachments(ctx context.Context, msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatWithImageAttachments", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}

	// Convert messages, then replace the last user message with text+image content parts
	adaptedMsgs := AdaptMessages(model, msgs)
	if len(adaptedMsgs) > 0 {
		for i := len(adaptedMsgs) - 1; i >= 0; i-- {
			if adaptedMsgs[i].OfUser != nil {
				userMsg := adaptedMsgs[i].OfUser

				// Build content parts: optional text + N images
				var contentParts []sdk.ChatCompletionContentPartUnionParam

				if userMsg.Content.OfString.Valid() && userMsg.Content.OfString.Value != "" {
					contentParts = append(contentParts, sdk.ChatCompletionContentPartUnionParam{
						OfText: &sdk.ChatCompletionContentPartTextParam{Text: userMsg.Content.OfString.Value},
					})
				}

				for _, img := range images {
					if strings.TrimSpace(img.MimeType) == "" || strings.TrimSpace(img.Base64Data) == "" {
						continue
					}
					dataURL := "data:" + img.MimeType + ";base64," + img.Base64Data
					contentParts = append(contentParts, sdk.ChatCompletionContentPartUnionParam{
						OfImageURL: &sdk.ChatCompletionContentPartImageParam{
							ImageURL: sdk.ChatCompletionContentPartImageImageURLParam{URL: dataURL},
						},
					})
				}

				newUserMsg := sdk.ChatCompletionUserMessageParam{
					Content: sdk.ChatCompletionUserMessageParamContentUnion{OfArrayOfContentParts: contentParts},
				}
				adaptedMsgs[i] = sdk.ChatCompletionMessageParamUnion{OfUser: &newUserMsg}
				break
			}
		}
	}

	params.Messages = adaptedMsgs
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = AdaptSchemas(sanitizeToolSchemas(tools))
		} else {
			params.Tools = AdaptSchemas(tools)
		}
	}
	if len(c.extra) > 0 {
		if len(tools) == 0 {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(tmp)
		} else {
			params.SetExtraFields(c.extra)
		}
	}

	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_with_images_error")
		span.RecordError(err)
		return llm.Message{}, err
	}

	log.Debug().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_with_images_ok")
	llm.LogRedactedResponse(ctx, comp.Choices)
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	msg := comp.Choices[0].Message
	out := llm.Message{Role: "assistant", Content: msg.Content}
	if c.isSelfHosted() {
		promptTokens := c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens := c.tokenizeCount(ctx, out.Content)
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, promptTokens+completionTokens)
		llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	} else {
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetrics(string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
	}
	gemini := isGemini3Model(string(params.Model))
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case sdk.ChatCompletionMessageFunctionToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Function.Name,
				Args: json.RawMessage(v.Function.Arguments),
				ID:   v.ID,
				ThoughtSignature: func() string {
					if gemini {
						return extractThoughtSignature(v.RawJSON())
					}
					return ""
				}(),
			})
		case sdk.ChatCompletionMessageCustomToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{Name: v.Custom.Name, Args: json.RawMessage(v.Custom.Input), ID: v.ID})
		}
	}
	return out, nil
}

// isEmptyArgs reports whether the provided arguments string is effectively empty
// (blank, null, or an empty JSON object/array). This guards against ghost tool
// calls with missing payloads.
func isEmptyArgs(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" || s == "{}" || s == "[]" {
		return true
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		// If it's invalid JSON but non-empty, let the tool handle the error.
		return false
	}
	switch t := v.(type) {
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	case string:
		return strings.TrimSpace(t) == ""
	}
	return false
}

func isEmptyArgsBytes(raw []byte) bool {
	return isEmptyArgs(string(raw))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// =============== Responses API adapters and implementations ===============

// adaptResponsesTools converts llm.ToolSchema to Responses ToolUnionParam slice.
func adaptResponsesTools(schemas []llm.ToolSchema) []rs.ToolUnionParam {
	out := make([]rs.ToolUnionParam, 0, len(schemas))
	for _, s := range schemas {
		params := s.Parameters
		if params != nil {
			params = ensureStrictJSONSchema(params).(map[string]any)
		}
		fn := rs.FunctionToolParam{
			Name:       s.Name,
			Parameters: params,
			// Use non-strict mode to avoid Responses API requirement that
			// "required" must list every key in properties. This preserves
			// optional fields like args/stdin/timeout_seconds on tools such as run_cli.
			Strict:      sdk.Bool(false),
			Description: sdk.String(s.Description),
		}
		out = append(out, rs.ToolUnionParam{OfFunction: &fn})
	}
	return out
}

// adaptResponsesInput builds the Responses Input item list and returns any combined instructions.
func adaptResponsesInput(msgs []llm.Message) (items rs.ResponseInputParam, instructions string) {
	items = make([]rs.ResponseInputItemUnionParam, 0, len(msgs))
	var sys []string
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sys = append(sys, m.Content)
			}
		case "user":
			content := strings.TrimSpace(m.Content)
			if content == "" {
				content = " "
			}
			part := rs.ResponseInputContentParamOfInputText(content)
			items = append(items, rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
				Content: rs.ResponseInputMessageContentListParam{part},
				Role:    "user",
			}})
		case "assistant":
			// If the assistant provided tool calls previously, include those calls so the model has context.
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					callID := tc.ID
					args := string(tc.Args)
					items = append(items, rs.ResponseInputItemParamOfFunctionCall(args, callID, tc.Name))
				}
			}
			// We generally omit plain assistant text as an input message for Responses API.
		case "tool":
			// Map tool outputs to function_call_output items
			out := strings.TrimSpace(m.Content)
			if out == "" {
				out = "{}"
			}
			items = append(items, rs.ResponseInputItemParamOfFunctionCallOutput(m.ToolID, out))
		}
	}
	if len(sys) > 0 {
		instructions = strings.Join(sys, "\n\n")
	}
	return items, instructions
}

// chatResponses handles non-streaming chat via the Responses API.
func (c *Client) chatResponses(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses Chat", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	params := rs.ResponseNewParams{
		Model: rs.ResponsesModel(firstNonEmpty(model, c.model)),
	}
	// Build input and instructions
	input, instr := adaptResponsesInput(msgs)
	if len(input) > 0 {
		params.Input.OfInputItemList = input
	}
	if strings.TrimSpace(instr) != "" {
		params.Instructions = sdk.String(instr)
	}
	// Tools
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = adaptResponsesTools(sanitizeToolSchemas(tools))
		} else {
			params.Tools = adaptResponsesTools(tools)
		}
	}
	// Merge extra params
	if len(c.extra) > 0 || len(extra) > 0 {
		merged := make(map[string]any, len(c.extra)+len(extra))
		for k, v := range c.extra {
			merged[k] = v
		}
		for k, v := range extra {
			merged[k] = v
		}
		// Remove tool-specific flags when no tools are present
		if len(tools) == 0 {
			delete(merged, "parallel_tool_calls")
		}
		// Map reasoning_effort into the typed field to match Responses API contract.
		if effort, ok := extractReasoningEffort(merged); ok {
			params.Reasoning.Effort = effort
		}
		params.SetExtraFields(merged)
	}

	start := time.Now()
	resp, err := c.sdk.Responses.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("responses_error")
		span.RecordError(err)
		return llm.Message{}, err
	}

	// Prepare assistant output
	out := llm.Message{Role: "assistant", Content: resp.OutputText()}
	// Extract tool calls from output items if any
	for _, it := range resp.Output {
		if fn := it.AsFunctionCall(); fn.Name != "" || fn.CallID != "" || fn.Arguments != "" {
			// Skip tool calls with empty or effectively empty arguments
			if isEmptyArgs(fn.Arguments) {
				log.Warn().Str("tool", fn.Name).Str("id", fn.CallID).Msg("skipping Responses API tool call with empty arguments")
				continue
			}
			id := fn.CallID
			if id == "" {
				id = fn.ID
			}
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: fn.Name,
				Args: json.RawMessage(fn.Arguments),
				ID:   id,
			})
		}
		if ct := it.AsCustomToolCall(); ct.Name != "" || ct.CallID != "" || ct.Input != "" {
			// Skip tool calls with empty or effectively empty input
			if isEmptyArgs(ct.Input) {
				log.Warn().Str("tool", ct.Name).Str("id", ct.CallID).Msg("skipping Responses API custom tool call with empty input")
				continue
			}
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: ct.Name,
				Args: json.RawMessage(ct.Input),
				ID:   ct.CallID,
			})
		}
	}

	// Usage / token metrics
	promptTokens := int(resp.Usage.InputTokens)
	completionTokens := int(resp.Usage.OutputTokens)
	totalTokens := int(resp.Usage.TotalTokens)

	f := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	f = f.Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens)
	// Attempt to surface nested details
	f = f.Int("prompt_tokens_details_cached_tokens", int(resp.Usage.InputTokensDetails.CachedTokens)).
		Int("completion_tokens_details_reasoning_tokens", int(resp.Usage.OutputTokensDetails.ReasoningTokens))
	fields := f.Logger()
	fields.Debug().Msg("responses_ok")

	if c.isSelfHosted() {
		// Override counts by re-tokenizing prompt and assistant content
		p := c.tokenizeCount(ctx, buildPromptText(msgs))
		a := c.tokenizeCount(ctx, out.Content)
		llm.RecordTokenAttributes(span, p, a, p+a)
		llm.RecordTokenMetrics(string(params.Model), p, a)
	} else {
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	}
	llm.LogRedactedResponse(ctx, map[string]any{"output_text_len": len(out.Content), "tool_calls": len(out.ToolCalls)})

	return out, nil
}

// chatStreamResponses streams output via the Responses API.
func (c *Client) chatStreamResponses(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	// Tracing / prompt
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses ChatStream", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	params := rs.ResponseNewParams{Model: rs.ResponsesModel(firstNonEmpty(model, c.model))}
	in, instr := adaptResponsesInput(msgs)
	if len(in) > 0 {
		params.Input.OfInputItemList = in
	}
	if strings.TrimSpace(instr) != "" {
		params.Instructions = sdk.String(instr)
	}
	if len(tools) > 0 {
		if c.isSelfHosted() {
			params.Tools = adaptResponsesTools(sanitizeToolSchemas(tools))
		} else {
			params.Tools = adaptResponsesTools(tools)
		}
	}
	if len(c.extra) > 0 {
		merged := make(map[string]any, len(c.extra))
		for k, v := range c.extra {
			merged[k] = v
		}
		if len(tools) == 0 {
			delete(merged, "parallel_tool_calls")
		}
		if effort, ok := extractReasoningEffort(merged); ok {
			params.Reasoning.Effort = effort
		}
		params.SetExtraFields(merged)
	}

	start := time.Now()
	stream := c.sdk.Responses.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	// Accumulate function/custom tool call info per output index
	type callAcc struct {
		name string
		id   string // call_id preferred
		args strings.Builder
		done bool
	}
	acc := map[int64]*callAcc{}

	// Token usage
	var promptTokens, completionTokens, totalTokens int
	// Assistant content for self-hosted tokenization
	var assistantContent strings.Builder

	for stream.Next() {
		ev := stream.Current()
		switch v := ev.AsAny().(type) {
		case rs.ResponseTextDeltaEvent:
			if v.Delta != "" {
				h.OnDelta(v.Delta)
				assistantContent.WriteString(v.Delta)
			}
		case rs.ResponseOutputItemAddedEvent:
			// Capture function call metadata early
			if fn := v.Item.AsFunctionCall(); fn.Name != "" || fn.CallID != "" || fn.Arguments != "" {
				ca := acc[v.OutputIndex]
				if ca == nil {
					ca = &callAcc{}
					acc[v.OutputIndex] = ca
				}
				ca.name = fn.Name
				ca.id = fn.CallID
				if ca.id == "" {
					ca.id = fn.ID
				}
				if fn.Arguments != "" && ca.args.Len() == 0 {
					ca.args.WriteString(fn.Arguments)
				}
			}
			if ct := v.Item.AsCustomToolCall(); ct.Name != "" || ct.CallID != "" || ct.Input != "" {
				ca := acc[v.OutputIndex]
				if ca == nil {
					ca = &callAcc{}
					acc[v.OutputIndex] = ca
				}
				ca.name = ct.Name
				ca.id = ct.CallID
				if ca.id == "" {
					ca.id = ct.ID
				}
				if ct.Input != "" && ca.args.Len() == 0 {
					ca.args.WriteString(ct.Input)
				}
			}
		case rs.ResponseOutputItemDoneEvent:
			// Nothing special; metadata already handled
			_ = v
		case rs.ResponseFunctionCallArgumentsDeltaEvent:
			ca := acc[v.OutputIndex]
			if ca == nil {
				ca = &callAcc{}
				acc[v.OutputIndex] = ca
			}
			if v.Delta != "" {
				ca.args.WriteString(v.Delta)
			}
		case rs.ResponseFunctionCallArgumentsDoneEvent:
			ca := acc[v.OutputIndex]
			if ca != nil && !ca.done {
				if ca.args.Len() == 0 && v.Arguments != "" {
					ca.args.WriteString(v.Arguments)
				}
				ca.done = true
				// Skip tool calls with empty or effectively empty arguments
				argsStr := ca.args.String()
				if isEmptyArgs(argsStr) {
					log.Warn().Str("tool", ca.name).Str("id", ca.id).Msg("skipping Responses API stream tool call with empty arguments")
					continue
				}
				// Emit tool call
				h.OnToolCall(llm.ToolCall{
					Name: ca.name,
					Args: json.RawMessage(argsStr),
					ID:   ca.id,
				})
			}
		case rs.ResponseCustomToolCallInputDeltaEvent:
			ca := acc[v.OutputIndex]
			if ca == nil {
				ca = &callAcc{}
				acc[v.OutputIndex] = ca
			}
			if v.Delta != "" {
				ca.args.WriteString(v.Delta)
			}
		case rs.ResponseCustomToolCallInputDoneEvent:
			ca := acc[v.OutputIndex]
			if ca != nil && !ca.done {
				if ca.args.Len() == 0 && v.Input != "" {
					ca.args.WriteString(v.Input)
				}
				ca.done = true
				// Skip tool calls with empty or effectively empty input
				argsStr := ca.args.String()
				if isEmptyArgs(argsStr) {
					log.Warn().Str("tool", ca.name).Str("id", ca.id).Msg("skipping Responses API stream custom tool call with empty input")
					continue
				}
				h.OnToolCall(llm.ToolCall{
					Name: ca.name,
					Args: json.RawMessage(argsStr),
					ID:   ca.id,
				})
			}
		case rs.ResponseCompletedEvent:
			// Capture usage
			promptTokens = int(v.Response.Usage.InputTokens)
			completionTokens = int(v.Response.Usage.OutputTokens)
			totalTokens = int(v.Response.Usage.TotalTokens)
		}
	}

	err := stream.Err()
	dur := time.Since(start)
	base := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).
		Int("prompt_tokens", promptTokens).Int("completion_tokens", completionTokens).Int("total_tokens", totalTokens).Logger()
	if err != nil {
		base.Error().Err(err).Msg("responses_stream_error")
		span.RecordError(err)
	} else {
		if c.isSelfHosted() {
			p := c.tokenizeCount(ctx, buildPromptText(msgs))
			a := c.tokenizeCount(ctx, assistantContent.String())
			llm.RecordTokenAttributes(span, p, a, p+a)
			if p > 0 || a > 0 {
				llm.RecordTokenMetrics(string(params.Model), p, a)
			}
			llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": p, "completion_tokens": a, "total_tokens": p + a})
		} else {
			llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
			if promptTokens > 0 || completionTokens > 0 {
				llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
			}
			llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
		}
		base.Debug().Msg("responses_stream_ok")
	}
	return err
}
