package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"strings"
	"syscall"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
	rs "github.com/openai/openai-go/v2/responses"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

const (
	maxResponsesToolOutputChars          = 24_000
	responsesToolOutputEllipsis          = "\n\n[TRUNCATED: tool output exceeded OpenAI context budget]"
	defaultResponsesToolOutputTokenLimit = 6_000
	defaultResponsesReserveOutputTokens  = 25_000
	defaultResponsesMinKeepMessages      = 8
	defaultResponsesOverflowRetryTrim    = 4
	defaultResponsesOverflowRetries      = 3
	responsesMinInputBudgetTokens        = 8_000
)

func boundedResponsesToolOutputWithLimit(content string, maxChars int) string {
	out := strings.TrimSpace(content)
	if out == "" {
		return "{}"
	}
	if maxChars <= 0 {
		maxChars = maxResponsesToolOutputChars
	}
	if len(out) <= maxChars {
		return out
	}
	limit := max(maxChars-len(responsesToolOutputEllipsis), 0)
	return out[:limit] + responsesToolOutputEllipsis
}

type responsesContextPolicy struct {
	Enabled              bool
	MaxInputTokens       int
	ReserveOutputTokens  int
	MinKeepMessages      int
	ToolOutputTokenLimit int
	ToolOutputMaxChars   int
	OverflowRetryTrim    int
	OverflowRetries      int
}

func defaultResponsesContextPolicy(model string) responsesContextPolicy {
	ctxWindow, _ := llm.ContextSize(model)
	if ctxWindow <= 0 {
		ctxWindow = 32_000
	}
	reserve := defaultResponsesReserveOutputTokens
	maxInput := max(ctxWindow-reserve, responsesMinInputBudgetTokens)
	toolLimit := defaultResponsesToolOutputTokenLimit
	toolChars := max(toolLimit*4, maxResponsesToolOutputChars)
	return responsesContextPolicy{
		Enabled:              true,
		MaxInputTokens:       maxInput,
		ReserveOutputTokens:  reserve,
		MinKeepMessages:      defaultResponsesMinKeepMessages,
		ToolOutputTokenLimit: toolLimit,
		ToolOutputMaxChars:   toolChars,
		OverflowRetryTrim:    defaultResponsesOverflowRetryTrim,
		OverflowRetries:      defaultResponsesOverflowRetries,
	}
}

func (p *responsesContextPolicy) applyOverrides(extra map[string]any, model string) {
	if p == nil {
		return
	}
	if v, ok := boolFromAny(extra["context_management_enabled"]); ok {
		p.Enabled = v
	}
	if v, ok := intFromAny(extra["context_input_tokens_limit"]); ok && v > 0 {
		p.MaxInputTokens = v
	}
	if v, ok := intFromAny(extra["context_reserve_output_tokens"]); ok && v >= 0 {
		p.ReserveOutputTokens = v
		if p.MaxInputTokens <= 0 {
			ctxWindow, _ := llm.ContextSize(model)
			if ctxWindow <= 0 {
				ctxWindow = 32_000
			}
			p.MaxInputTokens = ctxWindow - p.ReserveOutputTokens
		}
	}
	if v, ok := intFromAny(extra["context_min_keep_messages"]); ok && v > 0 {
		p.MinKeepMessages = v
	}
	if v, ok := intFromAny(extra["tool_output_token_limit"]); ok && v > 0 {
		p.ToolOutputTokenLimit = v
	}
	if v, ok := intFromAny(extra["tool_output_char_limit"]); ok && v > 0 {
		p.ToolOutputMaxChars = v
	}
	if v, ok := intFromAny(extra["context_overflow_retry_trim_messages"]); ok && v > 0 {
		p.OverflowRetryTrim = v
	}
	if v, ok := intFromAny(extra["context_overflow_retries"]); ok && v > 0 {
		p.OverflowRetries = v
	}

	if p.MaxInputTokens < responsesMinInputBudgetTokens {
		p.MaxInputTokens = responsesMinInputBudgetTokens
	}
	if p.MinKeepMessages <= 0 {
		p.MinKeepMessages = defaultResponsesMinKeepMessages
	}
	if p.ToolOutputTokenLimit <= 0 {
		p.ToolOutputTokenLimit = defaultResponsesToolOutputTokenLimit
	}
	if p.ToolOutputMaxChars <= 0 {
		p.ToolOutputMaxChars = p.ToolOutputTokenLimit * 4
	}
	if p.ToolOutputMaxChars < 1024 {
		p.ToolOutputMaxChars = 1024
	}
	if p.OverflowRetryTrim <= 0 {
		p.OverflowRetryTrim = defaultResponsesOverflowRetryTrim
	}
	if p.OverflowRetries <= 0 {
		p.OverflowRetries = defaultResponsesOverflowRetries
	}
}

func (c *Client) responsesContextPolicy(model string) responsesContextPolicy {
	p := c.policy
	if model != "" && model != c.model {
		base := defaultResponsesContextPolicy(model)
		base.applyOverrides(c.extra, model)
		return base
	}
	return p
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		var i int
		_, err := fmt.Sscanf(s, "%d", &i)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func boolFromAny(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		switch s {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func sanitizeExtraFields(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return extra
	}
	out := make(map[string]any, len(extra))
	for k, v := range extra {
		switch k {
		case "context_management_enabled",
			"context_input_tokens_limit",
			"context_reserve_output_tokens",
			"context_min_keep_messages",
			"tool_output_token_limit",
			"tool_output_char_limit",
			"context_overflow_retry_trim_messages",
			"context_overflow_retries":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

func isContextLengthExceededErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "context_length_exceeded") ||
		strings.Contains(s, "context length exceeded") ||
		strings.Contains(s, "input is too long") ||
		strings.Contains(s, "maximum context length")
}

// =============== Responses API adapters and implementations ===============

// adaptResponsesTools converts llm.ToolSchema to Responses ToolUnionParam slice.
func adaptResponsesTools(schemas []llm.ToolSchema) []rs.ToolUnionParam {
	out := make([]rs.ToolUnionParam, 0, len(schemas))
	for _, s := range schemas {
		if isNativeWebSearchSchema(s.Name) {
			out = append(out, rs.ToolParamOfWebSearch(rs.WebSearchToolTypeWebSearch))
			continue
		}
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

func adaptResponsesInputWithLimit(msgs []llm.Message, toolOutputMaxChars int) (items rs.ResponseInputParam, instructions string) {
	// The Responses API requires each function_call_output item to correspond to a
	// function_call item with the same call_id in the provided input.
	//
	// In practice, chat history may contain persisted tool outputs without the
	// original tool call message (e.g., due to compaction, legacy persistence, or
	// partial history windows). Filter these "orphan" tool outputs to avoid 400s.
	validToolCallIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				continue
			}
			validToolCallIDs[tc.ID] = struct{}{}
		}
	}

	items = make([]rs.ResponseInputItemUnionParam, 0, len(msgs))
	var sys []string
	for _, m := range msgs {
		if m.Compaction != nil {
			items = append(items, responseCompactionItemParam(*m.Compaction))
			continue
		}
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
			toolID := strings.TrimSpace(m.ToolID)
			if toolID == "" {
				continue
			}
			if _, ok := validToolCallIDs[toolID]; !ok {
				continue
			}
			out := boundedResponsesToolOutputWithLimit(m.Content, toolOutputMaxChars)
			items = append(items, rs.ResponseInputItemParamOfFunctionCallOutput(toolID, out))
		}
	}
	if len(sys) > 0 {
		instructions = strings.Join(sys, "\n\n")
	}
	return items, instructions
}
func adaptResponsesInputWithImagesAndLimit(msgs []llm.Message, images []ImageAttachment, toolOutputMaxChars int) (items rs.ResponseInputParam, instructions string) {
	if len(images) == 0 {
		return adaptResponsesInputWithLimit(msgs, toolOutputMaxChars)
	}

	validToolCallIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				continue
			}
			validToolCallIDs[tc.ID] = struct{}{}
		}
	}

	lastUserIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	items = make([]rs.ResponseInputItemUnionParam, 0, len(msgs))
	var sys []string
	for i, m := range msgs {
		if m.Compaction != nil {
			items = append(items, responseCompactionItemParam(*m.Compaction))
			continue
		}
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
			if i == lastUserIdx {
				parts := make(rs.ResponseInputMessageContentListParam, 0, 1+len(images))
				if strings.TrimSpace(content) != "" {
					parts = append(parts, rs.ResponseInputContentParamOfInputText(content))
				}
				for _, img := range images {
					if strings.TrimSpace(img.MimeType) == "" || strings.TrimSpace(img.Base64Data) == "" {
						continue
					}
					dataURL := "data:" + img.MimeType + ";base64," + img.Base64Data
					parts = append(parts, responseInputImageContentParam(dataURL))
				}
				if len(parts) == 0 {
					parts = append(parts, rs.ResponseInputContentParamOfInputText(" "))
				}
				items = append(items, rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
					Content: parts,
					Role:    "user",
				}})
				continue
			}
			part := rs.ResponseInputContentParamOfInputText(content)
			items = append(items, rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
				Content: rs.ResponseInputMessageContentListParam{part},
				Role:    "user",
			}})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					callID := tc.ID
					args := string(tc.Args)
					items = append(items, rs.ResponseInputItemParamOfFunctionCall(args, callID, tc.Name))
				}
			}
		case "tool":
			toolID := strings.TrimSpace(m.ToolID)
			if toolID == "" {
				continue
			}
			if _, ok := validToolCallIDs[toolID]; !ok {
				continue
			}
			out := boundedResponsesToolOutputWithLimit(m.Content, toolOutputMaxChars)
			items = append(items, rs.ResponseInputItemParamOfFunctionCallOutput(toolID, out))
		}
	}
	if len(sys) > 0 {
		instructions = strings.Join(sys, "\n\n")
	}
	return items, instructions
}

func responseInputImageContentParam(dataURL string) rs.ResponseInputContentUnionParam {
	return rs.ResponseInputContentUnionParam{
		OfInputImage: &rs.ResponseInputImageParam{
			Detail:   rs.ResponseInputImageDetailAuto,
			ImageURL: param.NewOpt(dataURL),
		},
	}
}

func responseCompactionItemParam(item llm.CompactionItem) rs.ResponseInputItemUnionParam {
	payload := map[string]any{
		"type":              "compaction",
		"encrypted_content": item.EncryptedContent,
	}
	if strings.TrimSpace(item.ID) != "" {
		payload["id"] = item.ID
	}
	raw, _ := json.Marshal(payload)
	return param.Override[rs.ResponseInputItemUnionParam](json.RawMessage(raw))
}

func (c *Client) chatResponsesWithImages(ctx context.Context, msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	effectiveModel := firstNonEmpty(model, c.model)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses Chat With Images", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	policy := c.responsesContextPolicy(effectiveModel)
	managedMsgs := c.trimResponsesMessagesToBudget(ctx, msgs, effectiveModel, policy)

	for overflowAttempt := 0; overflowAttempt < policy.OverflowRetries; overflowAttempt++ {
		params := rs.ResponseNewParams{Model: rs.ResponsesModel(effectiveModel)}
		input, instr := adaptResponsesInputWithImagesAndLimit(managedMsgs, images, policy.ToolOutputMaxChars)
		if len(input) > 0 {
			params.Input.OfInputItemList = input
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

		merged := map[string]any{}
		if len(c.extra) > 0 {
			merged = make(map[string]any, len(c.extra))
			maps.Copy(merged, c.extra)
		}
		if len(merged) > 0 {
			if len(tools) == 0 {
				delete(merged, "parallel_tool_calls")
			}
			if effort, ok := extractReasoningEffort(merged); ok {
				params.Reasoning.Effort = effort
			}
		}
		if summary, ok := extractReasoningSummary(merged); ok {
			params.Reasoning.Summary = summary
		}
		if len(merged) > 0 {
			params.SetExtraFields(sanitizeExtraFields(merged))
		}

		start := time.Now()
		resp, err := c.sdk.Responses.New(ctx, params)
		dur := time.Since(start)
		if err != nil {
			if policy.Enabled && isContextLengthExceededErr(err) && overflowAttempt+1 < policy.OverflowRetries {
				managedMsgs = trimOldestNonSystem(managedMsgs, policy.OverflowRetryTrim)
				policy.ToolOutputMaxChars = max(1024, policy.ToolOutputMaxChars/2)
				log.Warn().Err(err).Str("model", string(params.Model)).Int("overflow_attempt", overflowAttempt+1).Msg("responses_with_images_context_overflow_retry")
				continue
			}
			log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("responses_with_images_error")
			span.RecordError(err)
			return llm.Message{}, err
		}

		out := llm.Message{Role: "assistant", Content: resp.OutputText()}
		for _, it := range resp.Output {
			switch it.Type {
			case "function_call":
				fn := it.AsFunctionCall()
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
		}
		llm.LogRedactedResponse(ctx, map[string]any{"output_text_len": len(out.Content), "tool_calls": len(out.ToolCalls)})
		return out, nil
	}

	return llm.Message{}, fmt.Errorf("responses_with_images failed after context retries")
}

// chatResponses handles non-streaming chat via the Responses API.
func (c *Client) chatResponses(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	effectiveModel := firstNonEmpty(model, c.model)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses Chat", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	policy := c.responsesContextPolicy(effectiveModel)
	managedMsgs := c.trimResponsesMessagesToBudget(ctx, msgs, effectiveModel, policy)

	var (
		resp *rs.Response
		err  error
		dur  time.Duration
	)
	for overflowAttempt := 0; overflowAttempt < policy.OverflowRetries; overflowAttempt++ {
		params := rs.ResponseNewParams{Model: rs.ResponsesModel(effectiveModel)}
		input, instr := adaptResponsesInputWithLimit(managedMsgs, policy.ToolOutputMaxChars)
		if len(input) > 0 {
			params.Input.OfInputItemList = input
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

		merged := map[string]any{}
		if len(c.extra) > 0 || len(extra) > 0 {
			merged = make(map[string]any, len(c.extra)+len(extra))
			maps.Copy(merged, c.extra)
			maps.Copy(merged, extra)
		}
		if len(merged) > 0 {
			if len(tools) == 0 {
				delete(merged, "parallel_tool_calls")
			}
			if effort, ok := extractReasoningEffort(merged); ok {
				params.Reasoning.Effort = effort
			}
		}
		if summary, ok := extractReasoningSummary(merged); ok {
			params.Reasoning.Summary = summary
		}
		if len(merged) > 0 {
			params.SetExtraFields(sanitizeExtraFields(merged))
		}

		start := time.Now()
		resp, err = c.sdk.Responses.New(ctx, params)
		dur = time.Since(start)
		if err == nil {
			break
		}
		if policy.Enabled && isContextLengthExceededErr(err) && overflowAttempt+1 < policy.OverflowRetries {
			managedMsgs = trimOldestNonSystem(managedMsgs, policy.OverflowRetryTrim)
			policy.ToolOutputMaxChars = max(1024, policy.ToolOutputMaxChars/2)
			log.Warn().Err(err).Str("model", string(params.Model)).Int("overflow_attempt", overflowAttempt+1).Msg("responses_context_overflow_retry")
			continue
		}
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("responses_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	if err != nil {
		return llm.Message{}, err
	}

	// Prepare assistant output
	out := llm.Message{Role: "assistant", Content: resp.OutputText()}
	// Extract tool calls from output items based on their Type field.
	// The SDK's As* methods always return a struct even for non-matching types,
	// so we must check Type first to avoid creating ghost tool calls.
	for _, it := range resp.Output {
		switch it.Type {
		case "function_call":
			fn := it.AsFunctionCall()
			id := fn.CallID
			if id == "" {
				id = fn.ID
			}
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: fn.Name,
				Args: json.RawMessage(fn.Arguments),
				ID:   id,
			})
		case "mcp_call":
			// MCP/custom tool calls use the mcp_call type
			ct := it.AsMcpCall()
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: ct.Name,
				Args: json.RawMessage(ct.Arguments),
				ID:   ct.ID,
			})
		}
	}

	// Usage / token metrics
	promptTokens := int(resp.Usage.InputTokens)
	completionTokens := int(resp.Usage.OutputTokens)
	totalTokens := int(resp.Usage.TotalTokens)

	f := log.With().Str("model", effectiveModel).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(managedMsgs))
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
		p := c.tokenizeCount(ctx, buildPromptText(managedMsgs))
		a := c.tokenizeCount(ctx, out.Content)
		llm.RecordTokenAttributes(span, p, a, p+a)
		llm.RecordTokenMetricsFromContext(ctx, effectiveModel, p, a)
	} else {
		llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
		llm.RecordTokenMetricsFromContext(ctx, effectiveModel, promptTokens, completionTokens)
	}
	llm.LogRedactedResponse(ctx, map[string]any{"output_text_len": len(out.Content), "tool_calls": len(out.ToolCalls)})

	return out, nil
}

// chatStreamResponses streams output via the Responses API.
func (c *Client) chatStreamResponses(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	effectiveModel := firstNonEmpty(model, c.model)
	// Tracing / prompt
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses ChatStream", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	policy := c.responsesContextPolicy(effectiveModel)
	managedMsgs := c.trimResponsesMessagesToBudget(ctx, msgs, effectiveModel, policy)

	// Accumulate function/custom tool call info per output index
	type callAcc struct {
		name string
		id   string // call_id preferred
		args strings.Builder
		done bool
	}

	var lastErr error
	for overflowAttempt := 0; overflowAttempt < policy.OverflowRetries; overflowAttempt++ {
		params := rs.ResponseNewParams{Model: rs.ResponsesModel(effectiveModel)}
		in, instr := adaptResponsesInputWithLimit(managedMsgs, policy.ToolOutputMaxChars)
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
		merged := map[string]any{}
		if len(c.extra) > 0 {
			merged = make(map[string]any, len(c.extra))
			maps.Copy(merged, c.extra)
		}
		if len(merged) > 0 {
			if len(tools) == 0 {
				delete(merged, "parallel_tool_calls")
			}
			if effort, ok := extractReasoningEffort(merged); ok {
				params.Reasoning.Effort = effort
			}
		}
		if summary, ok := extractReasoningSummary(merged); ok {
			params.Reasoning.Summary = summary
		}
		if len(merged) > 0 {
			params.SetExtraFields(sanitizeExtraFields(merged))
		}

		streamCompleted := false
		for attempt := range 2 {
			if attempt > 0 && ctx.Err() != nil {
				lastErr = ctx.Err()
				break
			}
			start := time.Now()
			stream := c.sdk.Responses.NewStreaming(ctx, params)

			acc := map[int64]*callAcc{}
			var promptTokens, completionTokens, totalTokens int
			var assistantContent strings.Builder
			var summaryIndex int64 = -1
			var summaryText strings.Builder
			sawDelta := false
			sawToolCall := false
			sawSummary := false

			for stream.Next() {
				ev := stream.Current()
				switch v := ev.AsAny().(type) {
				case rs.ResponseTextDeltaEvent:
					if v.Delta != "" {
						sawDelta = true
						h.OnDelta(v.Delta)
						assistantContent.WriteString(v.Delta)
					}
				case rs.ResponseReasoningSummaryTextDeltaEvent:
					if summaryIndex != v.SummaryIndex {
						summaryIndex = v.SummaryIndex
						summaryText.Reset()
					}
					if v.Delta != "" {
						sawSummary = true
						summaryText.WriteString(v.Delta)
						h.OnThoughtSummary(summaryText.String())
					}
				case rs.ResponseReasoningSummaryTextDoneEvent:
					if summaryIndex != v.SummaryIndex {
						summaryIndex = v.SummaryIndex
					}
					summaryText.Reset()
					if v.Text != "" {
						sawSummary = true
						summaryText.WriteString(v.Text)
						h.OnThoughtSummary(summaryText.String())
					}
				case rs.ResponseOutputItemAddedEvent:
					switch v.Item.Type {
					case "function_call":
						fn := v.Item.AsFunctionCall()
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
					case "mcp_call":
						ct := v.Item.AsMcpCall()
						ca := acc[v.OutputIndex]
						if ca == nil {
							ca = &callAcc{}
							acc[v.OutputIndex] = ca
						}
						ca.name = ct.Name
						ca.id = ct.ID
						if ct.Arguments != "" && ca.args.Len() == 0 {
							ca.args.WriteString(ct.Arguments)
						}
					}
				case rs.ResponseOutputItemDoneEvent:
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
						sawToolCall = true
						h.OnToolCall(llm.ToolCall{Name: ca.name, Args: json.RawMessage(ca.args.String()), ID: ca.id})
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
						sawToolCall = true
						h.OnToolCall(llm.ToolCall{Name: ca.name, Args: json.RawMessage(ca.args.String()), ID: ca.id})
					}
				case rs.ResponseCompletedEvent:
					promptTokens = int(v.Response.Usage.InputTokens)
					completionTokens = int(v.Response.Usage.OutputTokens)
					totalTokens = int(v.Response.Usage.TotalTokens)
				}
			}

			err := stream.Err()
			_ = stream.Close()
			dur := time.Since(start)
			base := log.With().Str("model", effectiveModel).Int("tools", len(tools)).Dur("duration", dur).
				Int("prompt_tokens", promptTokens).Int("completion_tokens", completionTokens).Int("total_tokens", totalTokens).Logger()
			if err != nil {
				lastErr = err
				if attempt == 0 && isConnectionReset(err) && !sawDelta && !sawToolCall && !sawSummary && ctx.Err() == nil {
					base.Warn().Err(err).Msg("responses_stream_retry")
					continue
				}
				if policy.Enabled && isContextLengthExceededErr(err) {
					base.Warn().Err(err).Int("overflow_attempt", overflowAttempt+1).Msg("responses_stream_context_overflow")
					break
				}
				base.Error().Err(err).Msg("responses_stream_error")
				span.RecordError(err)
				return err
			}

			if c.isSelfHosted() {
				p := c.tokenizeCount(ctx, buildPromptText(managedMsgs))
				a := c.tokenizeCount(ctx, assistantContent.String())
				llm.RecordTokenAttributes(span, p, a, p+a)
				if p > 0 || a > 0 {
					llm.RecordTokenMetricsFromContext(ctx, effectiveModel, p, a)
				}
				llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": p, "completion_tokens": a, "total_tokens": p + a})
			} else {
				llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
				if promptTokens > 0 || completionTokens > 0 {
					llm.RecordTokenMetricsFromContext(ctx, effectiveModel, promptTokens, completionTokens)
				}
				llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
			}
			base.Debug().Msg("responses_stream_ok")
			streamCompleted = true
			break
		}

		if streamCompleted {
			return nil
		}
		if policy.Enabled && isContextLengthExceededErr(lastErr) && overflowAttempt+1 < policy.OverflowRetries {
			managedMsgs = trimOldestNonSystem(managedMsgs, policy.OverflowRetryTrim)
			policy.ToolOutputMaxChars = max(1024, policy.ToolOutputMaxChars/2)
			continue
		}
		if lastErr != nil {
			span.RecordError(lastErr)
			return lastErr
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("responses_stream failed after context retries")
}

func isConnectionReset(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.ECONNRESET) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection reset by peer")
}

func (c *Client) trimResponsesMessagesToBudget(ctx context.Context, msgs []llm.Message, model string, policy responsesContextPolicy) []llm.Message {
	if !policy.Enabled || policy.MaxInputTokens <= 0 || len(msgs) == 0 {
		return msgs
	}
	tok := NewResponsesTokenizer(c, firstNonEmpty(model, c.model), nil, policy.ToolOutputMaxChars)
	count, err := tok.CountMessagesTokens(ctx, msgs)
	if err != nil || count <= policy.MaxInputTokens {
		return msgs
	}

	systems := make([]llm.Message, 0, 2)
	nonSystem := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			systems = append(systems, m)
			continue
		}
		nonSystem = append(nonSystem, m)
	}
	if len(nonSystem) == 0 {
		return msgs
	}
	if policy.MinKeepMessages > len(nonSystem) {
		policy.MinKeepMessages = len(nonSystem)
	}
	low := 0
	high := len(nonSystem) - policy.MinKeepMessages
	best := high
	for low <= high {
		mid := (low + high) / 2
		candidate := append(append(make([]llm.Message, 0, len(systems)+len(nonSystem)-mid), systems...), nonSystem[mid:]...)
		candidateCount, candidateErr := tok.CountMessagesTokens(ctx, candidate)
		if candidateErr != nil {
			break
		}
		if candidateCount <= policy.MaxInputTokens {
			best = mid
			high = mid - 1
			continue
		}
		low = mid + 1
	}
	trimmed := append(append(make([]llm.Message, 0, len(systems)+len(nonSystem)-best), systems...), nonSystem[best:]...)
	return trimmed
}

func trimOldestNonSystem(msgs []llm.Message, dropCount int) []llm.Message {
	if dropCount <= 0 || len(msgs) == 0 {
		return msgs
	}
	systems := make([]llm.Message, 0, 2)
	nonSystem := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			systems = append(systems, m)
			continue
		}
		nonSystem = append(nonSystem, m)
	}
	if len(nonSystem) <= 1 {
		return msgs
	}
	if dropCount >= len(nonSystem) {
		dropCount = len(nonSystem) - 1
	}
	trimmed := append(append(make([]llm.Message, 0, len(systems)+len(nonSystem)-dropCount), systems...), nonSystem[dropCount:]...)
	return trimmed
}
