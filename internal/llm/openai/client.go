package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"intelligence.dev/internal/config"
	"intelligence.dev/internal/llm"
	"intelligence.dev/internal/observability"
)

type Client struct {
	sdk         sdk.Client
	model       string
	extra       map[string]any
	logPayloads bool
	baseURL     string
	httpClient  *http.Client
}

func New(c config.OpenAIConfig, httpClient *http.Client) *Client {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{sdk: sdk.NewClient(opts...), model: c.Model, extra: c.ExtraParams, logPayloads: c.LogPayloads, baseURL: c.BaseURL, httpClient: httpClient}
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

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// messages
	params.Messages = AdaptMessages(msgs)
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

// ChatWithOptions is like Chat but allows callers to:
//   - omit tools entirely by passing a nil or empty tools slice
//   - inject provider-specific extra fields (e.g., reasoning_effort)
//     via params.WithExtraField.
func (c *Client) ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatWithOptions", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	params.Messages = AdaptMessages(msgs)
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
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// Start tracing and log prompt
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatStream", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	// messages
	params.Messages = AdaptMessages(msgs)
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
	// Ask the API to include a final usage chunk so we can log token counts
	params.StreamOptions.IncludeUsage = sdk.Bool(true)

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
		for i, tc := range delta.ToolCalls {
			if toolCalls[i] == nil {
				toolCalls[i] = &llm.ToolCall{
					ID: tc.ID,
				}
			}

			// Accumulate function name
			if tc.Function.Name != "" {
				toolCalls[i].Name = tc.Function.Name
			}

			// Accumulate function arguments
			if tc.Function.Arguments != "" {
				if toolCalls[i].Args == nil {
					toolCalls[i].Args = json.RawMessage(tc.Function.Arguments)
				} else {
					// Append new arguments to existing ones
					existing := string(toolCalls[i].Args)
					toolCalls[i].Args = json.RawMessage(existing + tc.Function.Arguments)
				}
			}
		}

		// Check if we're done with this step (finish_reason indicates completion)
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" && !toolCallsFlushed {
			// Send all accumulated tool calls
			for _, tc := range toolCalls {
				if tc != nil && tc.Name != "" && len(tc.Args) > 0 {
					h.OnToolCall(*tc)
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
	adaptedMsgs := AdaptMessages(msgs)
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
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case sdk.ChatCompletionMessageFunctionToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Function.Name,
				Args: json.RawMessage(v.Function.Arguments),
				ID:   v.ID,
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
