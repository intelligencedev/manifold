package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"
	rs "github.com/openai/openai-go/v2/responses"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

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
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses Chat", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	policy := c.responsesContextPolicy(effectiveModel)
	managedMsgs := c.trimResponsesMessagesToBudget(ctx, msgs, effectiveModel, policy)
	resp, dur, managedMsgs, err := c.runChatResponsesWithRetries(ctx, managedMsgs, tools, effectiveModel, extra, policy, log, span)
	if err != nil {
		return llm.Message{}, err
	}

	out := messageFromResponsesOutput(resp)
	logResponsesOK(log, effectiveModel, len(tools), len(managedMsgs), dur, resp)
	c.recordResponsesUsage(ctx, span, managedMsgs, effectiveModel, out.Content, resp)
	llm.LogRedactedResponse(ctx, map[string]any{"output_text_len": len(out.Content), "tool_calls": len(out.ToolCalls)})

	return out, nil
}

func (c *Client) runChatResponsesWithRetries(
	ctx context.Context,
	managedMsgs []llm.Message,
	tools []llm.ToolSchema,
	effectiveModel string,
	extra map[string]any,
	policy responsesContextPolicy,
	log *zerolog.Logger,
	span trace.Span,
) (*rs.Response, time.Duration, []llm.Message, error) {
	for overflowAttempt := 0; overflowAttempt < policy.OverflowRetries; overflowAttempt++ {
		params := c.chatResponsesParams(managedMsgs, tools, effectiveModel, extra, policy)
		start := time.Now()
		resp, err := c.sdk.Responses.New(ctx, params)
		dur := time.Since(start)
		if err == nil {
			return resp, dur, managedMsgs, nil
		}
		if policy.Enabled && isContextLengthExceededErr(err) && overflowAttempt+1 < policy.OverflowRetries {
			managedMsgs = trimOldestNonSystem(managedMsgs, policy.OverflowRetryTrim)
			policy.ToolOutputMaxChars = max(1024, policy.ToolOutputMaxChars/2)
			log.Warn().Err(err).Str("model", string(params.Model)).Int("overflow_attempt", overflowAttempt+1).Msg("responses_context_overflow_retry")
			continue
		}
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("responses_error")
		span.RecordError(err)
		return nil, dur, managedMsgs, err
	}
	return nil, 0, managedMsgs, fmt.Errorf("responses failed after context retries")
}

func (c *Client) chatResponsesParams(managedMsgs []llm.Message, tools []llm.ToolSchema, effectiveModel string, extra map[string]any, policy responsesContextPolicy) rs.ResponseNewParams {
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
	applyResponsesExtraFields(&params, c.extra, extra, len(tools))
	return params
}

func applyResponsesExtraFields(params *rs.ResponseNewParams, base map[string]any, extra map[string]any, toolCount int) {
	merged := map[string]any{}
	if len(base) > 0 || len(extra) > 0 {
		merged = make(map[string]any, len(base)+len(extra))
		maps.Copy(merged, base)
		maps.Copy(merged, extra)
	}
	if len(merged) > 0 {
		if toolCount == 0 {
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
}

func messageFromResponsesOutput(resp *rs.Response) llm.Message {
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
	return out
}

func logResponsesOK(log *zerolog.Logger, effectiveModel string, toolCount int, messageCount int, dur time.Duration, resp *rs.Response) {
	promptTokens := int(resp.Usage.InputTokens)
	completionTokens := int(resp.Usage.OutputTokens)
	totalTokens := int(resp.Usage.TotalTokens)
	f := log.With().Str("model", effectiveModel).Int("tools", toolCount).Dur("duration", dur).Int("messages", messageCount)
	f = f.Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens)
	f = f.Int("prompt_tokens_details_cached_tokens", int(resp.Usage.InputTokensDetails.CachedTokens)).
		Int("completion_tokens_details_reasoning_tokens", int(resp.Usage.OutputTokensDetails.ReasoningTokens))
	fields := f.Logger()
	fields.Debug().Msg("responses_ok")
}

func (c *Client) recordResponsesUsage(ctx context.Context, span trace.Span, managedMsgs []llm.Message, effectiveModel string, content string, resp *rs.Response) {
	if c.isSelfHosted() {
		p := c.tokenizeCount(ctx, buildPromptText(managedMsgs))
		a := c.tokenizeCount(ctx, content)
		llm.RecordTokenAttributes(span, p, a, p+a)
		llm.RecordTokenMetricsFromContext(ctx, effectiveModel, p, a)
		return
	}
	promptTokens := int(resp.Usage.InputTokens)
	completionTokens := int(resp.Usage.OutputTokens)
	totalTokens := int(resp.Usage.TotalTokens)
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	llm.RecordTokenMetricsFromContext(ctx, effectiveModel, promptTokens, completionTokens)
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
