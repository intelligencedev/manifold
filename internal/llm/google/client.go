package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	genai "google.golang.org/genai"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

type Client struct {
	client      *genai.Client
	model       string
	httpOptions genai.HTTPOptions
}

func New(cfg config.GoogleConfig, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gemini-1.5-flash"
	}

	httpOpts := genai.HTTPOptions{}
	if cfg.Timeout > 0 {
		t := time.Duration(cfg.Timeout) * time.Second
		httpOpts.Timeout = &t
	}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		httpOpts.BaseURL = strings.TrimSuffix(base, "/") + "/"
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      strings.TrimSpace(cfg.APIKey),
		HTTPClient:  httpClient,
		HTTPOptions: httpOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("init google client: %w", err)
	}

	return &Client{
		client:      client,
		model:       model,
		httpOptions: httpOpts,
	}, nil
}

func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	effectiveModel := c.pickModel(model)

	// Add observability like OpenAI/Anthropic clients
	ctx, span := llm.StartRequestSpan(ctx, "Google Chat", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	contents, err := toContents(msgs)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_toContents_error")
		return llm.Message{}, err
	}

	toolDecls, toolCfg, err := adaptTools(tools)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_chat_adaptTools_error")
		return llm.Message{}, err
	}

	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Int("contents", len(contents)).Msg("google_chat_api_call_start")

	start := time.Now()
	resp, err := c.client.Models.GenerateContent(ctx, effectiveModel, contents, c.buildContentConfig(ctx, effectiveModel, toolDecls, toolCfg))
	dur := time.Since(start)

	log.Debug().Dur("duration", dur).Bool("has_response", resp != nil).Bool("has_error", err != nil).Msg("google_chat_api_call_complete")

	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Str("model", effectiveModel).Dur("duration", dur).Msg("google_chat_error")
		return llm.Message{}, err
	}

	msg, err := messageFromResponse(resp)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Dur("duration", dur).Msg("google_chat_response_parse_error")
		return llm.Message{}, err
	}

	llm.LogRedactedResponse(ctx, resp)
	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Dur("duration", dur).Int("tool_calls", len(msg.ToolCalls)).Msg("google_chat_ok")

	return msg, nil
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	effectiveModel := c.pickModel(model)

	// Add observability like OpenAI/Anthropic clients
	ctx, span := llm.StartRequestSpan(ctx, "Google ChatStream", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	contents, err := toContents(msgs)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_stream_toContents_error")
		return err
	}

	toolDecls, toolCfg, err := adaptTools(tools)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("google_stream_adaptTools_error")
		return err
	}

	start := time.Now()
	log.Debug().Str("model", effectiveModel).Int("tools", len(tools)).Int("msgs", len(msgs)).Msg("google_stream_start")

	stream := c.client.Models.GenerateContentStream(ctx, effectiveModel, contents, c.buildContentConfig(ctx, effectiveModel, toolDecls, toolCfg))

	hasContent := false
	var toolCallCount int
	var thoughtSummaryCount int
	var thoughtSummary strings.Builder
	for resp, err := range stream {
		if err != nil {
			dur := time.Since(start)
			span.RecordError(err)
			log.Error().Err(err).Dur("duration", dur).Msg("google_stream_error")
			return err
		}
		// Use streaming-aware response parser that tolerates intermediate chunks
		// with empty candidates or nil content (normal in streaming).
		msg, summaryDelta, skip, err := messageFromStreamResponse(resp)
		if err != nil {
			dur := time.Since(start)
			span.RecordError(err)
			log.Error().Err(err).Dur("duration", dur).Msg("google_stream_response_parse_error")
			return err
		}
		if summaryDelta != "" && h != nil {
			thoughtSummaryCount++
			thoughtSummary.WriteString(summaryDelta)
			log.Debug().Int("thought_count", thoughtSummaryCount).Int("summary_len", thoughtSummary.Len()).Msg("google_stream_thought_summary")
			h.OnThoughtSummary(thoughtSummary.String())
		}
		if skip {
			// Intermediate chunk with no actionable content - continue streaming
			continue
		}
		hasContent = true
		if h != nil {
			if msg.Content != "" {
				h.OnDelta(msg.Content)
			}
			for _, img := range msg.Images {
				h.OnImage(img)
			}
		}
		for _, tc := range msg.ToolCalls {
			toolCallCount++
			if h != nil {
				h.OnToolCall(tc)
			}
		}
	}

	dur := time.Since(start)
	if !hasContent {
		log.Warn().Dur("duration", dur).Int("thought_summaries", thoughtSummaryCount).Msg("google_stream_empty_response")
	} else {
		log.Debug().Dur("duration", dur).Int("tool_calls", toolCallCount).Int("thought_summaries", thoughtSummaryCount).Msg("google_stream_ok")
	}

	return nil
}

func (c *Client) pickModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return c.model
	}
	return m
}

func (c *Client) buildContentConfig(ctx context.Context, model string, tools []*genai.Tool, toolCfg *genai.ToolConfig) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{
		HTTPOptions: &c.httpOptions,
		Tools:       tools,
		ToolConfig:  toolCfg,
	}
	if shouldIncludeThoughtSummaries(model) {
		cfg.ThinkingConfig = &genai.ThinkingConfig{IncludeThoughts: true}
	}
	if opts, ok := llm.ImagePromptFromContext(ctx); ok {
		size := strings.TrimSpace(opts.Size)
		if size == "" {
			size = "1K"
		}
		cfg.ResponseModalities = []string{"IMAGE", "TEXT"}
		cfg.ImageConfig = &genai.ImageConfig{
			ImageSize: size,
		}
	}
	return cfg
}

func shouldIncludeThoughtSummaries(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if idx := strings.LastIndex(m, "/"); idx != -1 {
		m = m[idx+1:]
	}
	return strings.Contains(m, "gemini-2.5") || strings.Contains(m, "gemini-3")
}

func toContents(msgs []llm.Message) ([]*genai.Content, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("messages required")
	}

	decodeThoughtSignature := func(sig string) ([]byte, bool) {
		s := strings.TrimSpace(sig)
		if s == "" {
			return nil, false
		}
		// If this contains Unicode replacement characters, it almost certainly
		// round-tripped through a UTF-8-only path (e.g., JSON) and is corrupted.
		if strings.ContainsRune(s, '\uFFFD') {
			return nil, false
		}
		// Preferred path: signatures are stored as base64 so they survive JSON.
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			return b, true
		}
		// Backward-compatible fallback: treat as raw bytes.
		return []byte(s), true
	}

	toolNamesByID := make(map[string]string)
	var lastFuncName string
	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "", "user", "system":
			role = genai.RoleUser
		case "assistant":
			role = genai.RoleModel
			for _, tc := range m.ToolCalls {
				if tc.ID != "" && tc.Name != "" {
					toolNamesByID[tc.ID] = tc.Name
				}
				if strings.TrimSpace(tc.Name) != "" {
					lastFuncName = tc.Name
				}
			}
		case "tool":
			// Tool responses are passed back to the model as function responses.
			name := toolNamesByID[m.ToolID]
			if name == "" {
				name = lastFuncName
				if name == "" {
					name = "tool_response"
				}
			}
			respMap := map[string]any{}
			if trimmed := strings.TrimSpace(m.Content); trimmed != "" {
				if err := json.Unmarshal([]byte(trimmed), &respMap); err != nil {
					respMap = map[string]any{"output": m.Content}
				}
			}
			part := genai.NewPartFromFunctionResponse(name, respMap)
			part.FunctionResponse.ID = m.ToolID
			// IMPORTANT:
			// Do not attach ThoughtSignature to FunctionResponse parts.
			// Gemini's guidance is to echo the thought_signature back inside its original
			// Part; tool responses are user-authored function responses and attaching a
			// signature here has been observed to trigger 5xx errors from the API.
			contents = append(contents, genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser))
			continue
		default:
			return nil, fmt.Errorf("unsupported role for google provider: %s", m.Role)
		}
		text := m.Content
		if role == genai.RoleUser && strings.ToLower(strings.TrimSpace(m.Role)) == "system" {
			text = "[system] " + text
		}
		parts := []*genai.Part{}
		if strings.TrimSpace(text) != "" {
			textPart := &genai.Part{Text: text}
			// For assistant (model) messages, attach the thought signature to the text part
			// if one was captured. Per Gemini 3 docs: "Always send the thought_signature
			// back to the model inside its original Part."
			if role == genai.RoleModel {
				if sigBytes, ok := decodeThoughtSignature(m.ThoughtSignature); ok {
					textPart.ThoughtSignature = sigBytes
				}
			}
			parts = append(parts, textPart)
		}
		if role == genai.RoleModel {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if len(tc.Args) > 0 {
					_ = json.Unmarshal(tc.Args, &args)
				}
				if len(args) == 0 && len(tc.Args) > 0 {
					args = map[string]any{"input": string(tc.Args)}
				}
				p := genai.NewPartFromFunctionCall(tc.Name, args)
				if sigBytes, ok := decodeThoughtSignature(tc.ThoughtSignature); ok {
					p.ThoughtSignature = sigBytes
				}
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			continue
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}
	return contents, nil
}

// messageFromStreamResponse parses a streaming response chunk. It returns:
// - (msg, false, nil) when the chunk contains actionable content
// - (empty, true, nil) when the chunk should be skipped (empty/intermediate)
// - (empty, false, err) when the chunk contains an error condition (safety block, etc.)
//
// This is more lenient than messageFromResponse because streaming can produce
// intermediate chunks with empty candidates or nil content, which is normal.
func messageFromStreamResponse(resp *genai.GenerateContentResponse) (llm.Message, string, bool, error) {
	if resp == nil {
		// Nil response in streaming is typically end-of-stream, skip it
		return llm.Message{}, "", true, nil
	}

	// Check for blocked response due to safety or other reasons
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return llm.Message{}, "", false, fmt.Errorf("request blocked by google: %s", resp.PromptFeedback.BlockReason)
	}

	// Empty candidates in streaming is normal for intermediate chunks
	if len(resp.Candidates) == 0 {
		return llm.Message{}, "", true, nil
	}

	candidate := resp.Candidates[0]

	// Check finish reason for errors (safety, recitation, etc.)
	switch candidate.FinishReason {
	case genai.FinishReasonSafety:
		return llm.Message{}, "", false, fmt.Errorf("response blocked by safety filters")
	case genai.FinishReasonRecitation:
		return llm.Message{}, "", false, fmt.Errorf("response blocked due to recitation")
	case genai.FinishReasonMalformedFunctionCall:
		return llm.Message{}, "", false, fmt.Errorf("malformed function call generated by model")
	}

	// Content can be nil in streaming intermediate chunks - skip rather than error
	if candidate.Content == nil {
		return llm.Message{}, "", true, nil
	}

	content := candidate.Content
	var sb strings.Builder
	var summary strings.Builder
	var tcs []llm.ToolCall
	var images []llm.GeneratedImage
	// Gemini 3 may return thought signatures on ANY part type (text, thought, etc.)
	// We capture the first signature we see from non-function-call parts so it can be
	// echoed back on subsequent turns.
	var textThoughtSig string
	callIdx := 0
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		// Capture thought signature from text/thought parts (non-function-call parts)
		// Per Gemini 3 docs: "Gemini 3 models may return thought signatures for all types of parts"
		if part.FunctionCall == nil && len(part.ThoughtSignature) > 0 && textThoughtSig == "" {
			textThoughtSig = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
		}
		if part.InlineData != nil {
			images = append(images, llm.GeneratedImage{
				Data:     part.InlineData.Data,
				MIMEType: part.InlineData.MIMEType,
			})
		}
		if part.Thought {
			if part.Text != "" {
				summary.WriteString(part.Text)
			}
			continue
		}
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			callIdx++
			id := part.FunctionCall.ID
			if strings.TrimSpace(id) == "" {
				id = "call-" + strconv.Itoa(callIdx)
			}
			var sig string
			if len(part.ThoughtSignature) > 0 {
				sig = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
			}
			tcs = append(tcs, llm.ToolCall{
				Name:             part.FunctionCall.Name,
				Args:             args,
				ID:               id,
				ThoughtSignature: sig,
			})
		}
	}

	// If we have no actual content/calls/images, skip this chunk
	if sb.Len() == 0 && len(tcs) == 0 && len(images) == 0 {
		return llm.Message{}, summary.String(), true, nil
	}

	return llm.Message{
		Role:    "assistant",
		Content: sb.String(),
		ToolCalls: func() []llm.ToolCall {
			if len(tcs) == 0 {
				return nil
			}
			return tcs
		}(),
		Images: func() []llm.GeneratedImage {
			if len(images) == 0 {
				return nil
			}
			return images
		}(),
		ThoughtSignature: textThoughtSig,
	}, summary.String(), false, nil
}

func messageFromResponse(resp *genai.GenerateContentResponse) (llm.Message, error) {
	if resp == nil {
		return llm.Message{}, fmt.Errorf("nil response from google provider")
	}

	// Check for blocked response due to safety or other reasons
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return llm.Message{}, fmt.Errorf("request blocked by google: %s", resp.PromptFeedback.BlockReason)
	}

	// Handle empty candidates - may happen with safety filters or other issues
	if len(resp.Candidates) == 0 {
		return llm.Message{}, fmt.Errorf("no candidates in google response")
	}

	candidate := resp.Candidates[0]

	// Check finish reason for errors (safety, recitation, etc.)
	switch candidate.FinishReason {
	case genai.FinishReasonSafety:
		return llm.Message{}, fmt.Errorf("response blocked by safety filters")
	case genai.FinishReasonRecitation:
		return llm.Message{}, fmt.Errorf("response blocked due to recitation")
	case genai.FinishReasonMalformedFunctionCall:
		return llm.Message{}, fmt.Errorf("malformed function call generated by model")
	}

	// Content can be nil in some cases (e.g., streaming intermediate chunks)
	// Return an empty message rather than an error
	if candidate.Content == nil {
		return llm.Message{Role: "assistant"}, nil
	}

	content := candidate.Content
	var sb strings.Builder
	var tcs []llm.ToolCall
	var images []llm.GeneratedImage
	// Gemini 3 may return thought signatures on ANY part type (text, thought, etc.)
	// We capture the first signature we see from non-function-call parts so it can be
	// echoed back on subsequent turns.
	var textThoughtSig string
	callIdx := 0
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		// Capture thought signature from text/thought parts (non-function-call parts)
		// Per Gemini 3 docs: "Gemini 3 models may return thought signatures for all types of parts"
		if part.FunctionCall == nil && len(part.ThoughtSignature) > 0 && textThoughtSig == "" {
			textThoughtSig = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
		}
		if part.InlineData != nil {
			images = append(images, llm.GeneratedImage{
				Data:     part.InlineData.Data,
				MIMEType: part.InlineData.MIMEType,
			})
		}
		if part.Thought {
			continue
		}
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			callIdx++
			id := part.FunctionCall.ID
			if strings.TrimSpace(id) == "" {
				id = "call-" + strconv.Itoa(callIdx)
			}
			var sig string
			if len(part.ThoughtSignature) > 0 {
				sig = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
			}
			tcs = append(tcs, llm.ToolCall{
				Name:             part.FunctionCall.Name,
				Args:             args,
				ID:               id,
				ThoughtSignature: sig,
			})
		}
	}

	return llm.Message{
		Role:    "assistant",
		Content: sb.String(),
		ToolCalls: func() []llm.ToolCall {
			if len(tcs) == 0 {
				return nil
			}
			return tcs
		}(),
		Images: func() []llm.GeneratedImage {
			if len(images) == 0 {
				return nil
			}
			return images
		}(),
		ThoughtSignature: textThoughtSig,
	}, nil
}

func adaptTools(schemas []llm.ToolSchema) ([]*genai.Tool, *genai.ToolConfig, error) {
	if len(schemas) == 0 {
		return nil, nil, nil
	}
	fd := make([]*genai.FunctionDeclaration, 0, len(schemas))
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if strings.TrimSpace(s.Name) == "" {
			return nil, nil, fmt.Errorf("google provider: tool name required")
		}
		names = append(names, s.Name)
		fd = append(fd, &genai.FunctionDeclaration{
			Name:                 s.Name,
			Description:          s.Description,
			ParametersJsonSchema: s.Parameters,
		})
	}
	sort.Strings(names)
	// Use AUTO mode to let the model decide whether to call a function or respond with text.
	// This prevents infinite loops where the model repeatedly calls the same function.
	// Note: AllowedFunctionNames should only be set when mode is ANY, not AUTO.
	// See: https://ai.google.dev/gemini-api/docs/function-calling#function-calling-modes
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAuto,
			// AllowedFunctionNames is intentionally omitted in AUTO mode per API requirements
		},
	}
	tool := &genai.Tool{FunctionDeclarations: fd}
	return []*genai.Tool{tool}, cfg, nil
}
