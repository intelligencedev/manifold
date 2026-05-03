package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if imgOpts, ok := llm.ImagePromptFromContext(ctx); ok {
		return c.chatWithImageGeneration(ctx, msgs, model, imgOpts)
	}

	tools = c.requestTools(tools)

	if strings.EqualFold(c.api, "responses") {
		return c.chatResponses(ctx, msgs, tools, model, nil)
	}

	effectiveModel := firstNonEmpty(model, c.model)

	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(effectiveModel),
	}
	// messages
	params.Messages = AdaptMessages(string(params.Model), c.chatCompletionMessages(msgs))
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
	if len(c.extra) > 0 {
		// When no tools are provided, ensure we don't forward tool-specific
		// flags from the client extra params.
		if !actualTools {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(sanitizeExtraFields(tmp))
		} else {
			params.SetExtraFields(sanitizeExtraFields(c.extra))
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
		if c.isSelfHosted() {
			out.Content = stripTaggedThoughtContent(out.Content)
		}
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
		llm.RecordTokenMetricsFromContext(ctx, string(params.Model), promptTokens, completionTokens)
	} else {
		// Use OpenAI provided usage
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetricsFromContext(ctx, string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
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
	tools = c.requestTools(tools)

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
	params.Messages = AdaptMessages(string(params.Model), c.chatCompletionMessages(msgs))
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
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
		if !actualTools {
			delete(merged, "parallel_tool_calls")
		}
		params.SetExtraFields(sanitizeExtraFields(merged))
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
		if c.isSelfHosted() {
			out.Content = stripTaggedThoughtContent(out.Content)
		}
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
		llm.RecordTokenMetricsFromContext(ctx, string(params.Model), promptTokens, completionTokens)
	} else {
		llm.RecordTokenAttributes(span, int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens), int(comp.Usage.TotalTokens))
		llm.RecordTokenMetricsFromContext(ctx, string(params.Model), int(comp.Usage.PromptTokens), int(comp.Usage.CompletionTokens))
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
	tools = c.requestTools(tools)
	if strings.EqualFold(c.api, "responses") {
		return c.chatStreamResponses(ctx, msgs, tools, model, h)
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
	params.Messages = AdaptMessages(string(params.Model), c.chatCompletionMessages(msgs))
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
	if len(c.extra) > 0 {
		// When no tools are provided, ensure we don't forward tool-specific
		// flags from the client extra params.
		if !actualTools {
			tmp := make(map[string]any, len(c.extra))
			for k, v := range c.extra {
				tmp[k] = v
			}
			delete(tmp, "parallel_tool_calls")
			params.SetExtraFields(sanitizeExtraFields(tmp))
		} else {
			params.SetExtraFields(sanitizeExtraFields(c.extra))
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
			llm.RecordTokenMetricsFromContext(ctx, string(params.Model), promptTokens, completionTokens)
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
		"messages": AdaptMessages(model, c.chatCompletionMessages(msgs)),
		"stream":   true,
	}
	actualTools := configureChatCompletionBodyTools(body, tools, c.isSelfHosted())
	// Merge extra params, but drop tool flags if no tools
	if len(c.extra) > 0 {
		tmp := make(map[string]any, len(c.extra))
		for k, v := range c.extra {
			tmp[k] = v
		}
		if !actualTools {
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
	var thoughtStream selfHostedThoughtStreamState
	emitSelfHostedDelta := func(content string) {
		if content == "" {
			return
		}
		thoughtStream.emit(content, h, &assistantContentBuilder)
	}

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
					if reasoning := extractSelfHostedReasoningContent(delta["reasoning_content"]); reasoning != "" {
						thoughtStream.emitReasoning(reasoning, h)
					}
					if reasoning := extractSelfHostedReasoningContent(delta["reasoningContent"]); reasoning != "" {
						thoughtStream.emitReasoning(reasoning, h)
					}
					if s, ok := delta["content"].(string); ok && s != "" {
						emitSelfHostedDelta(s)
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
					if reasoning := extractSelfHostedReasoningContent(msg["reasoning_content"]); reasoning != "" {
						thoughtStream.emitReasoning(reasoning, h)
					}
					if reasoning := extractSelfHostedReasoningContent(msg["reasoningContent"]); reasoning != "" {
						thoughtStream.emitReasoning(reasoning, h)
					}
					if s, ok := msg["content"].(string); ok && s != "" {
						emitSelfHostedDelta(s)
					}
				}
			}
			continue
		}

		// mlx_lm compatibility: sometimes payload may contain {"response":"..."}
		if s, ok := m["response"].(string); ok && s != "" {
			emitSelfHostedDelta(s)
			continue
		}
		// Another possible token key
		if s, ok := m["token"].(string); ok && s != "" {
			emitSelfHostedDelta(s)
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
			llm.RecordTokenMetricsFromContext(ctx, firstNonEmpty(model, c.model), promptTokens, completionTokens)
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
