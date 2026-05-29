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
	rs "github.com/openai/openai-go/v2/responses"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// chatStreamResponses streams output via the Responses API.
func (c *Client) chatStreamResponses(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	effectiveModel := firstNonEmpty(model, c.model)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses ChatStream", effectiveModel, len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	policy := c.responsesContextPolicy(effectiveModel)
	managedMsgs := c.trimResponsesMessagesToBudget(ctx, msgs, effectiveModel, policy)

	var lastErr error
	for overflowAttempt := 0; overflowAttempt < policy.OverflowRetries; overflowAttempt++ {
		params := c.responsesStreamParams(managedMsgs, tools, effectiveModel, policy)
		completed, err := c.runResponsesStreamAttempts(ctx, params, managedMsgs, tools, effectiveModel, h, policy, overflowAttempt, log, span)
		if completed {
			return nil
		}
		lastErr = err
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

func (c *Client) responsesStreamParams(managedMsgs []llm.Message, tools []llm.ToolSchema, effectiveModel string, policy responsesContextPolicy) rs.ResponseNewParams {
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
	applyResponsesStreamExtraFields(&params, c.extra, len(tools))
	return params
}

func applyResponsesStreamExtraFields(params *rs.ResponseNewParams, extra map[string]any, toolCount int) {
	merged := map[string]any{}
	if len(extra) > 0 {
		merged = make(map[string]any, len(extra))
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

func (c *Client) runResponsesStreamAttempts(
	ctx context.Context,
	params rs.ResponseNewParams,
	managedMsgs []llm.Message,
	tools []llm.ToolSchema,
	effectiveModel string,
	h llm.StreamHandler,
	policy responsesContextPolicy,
	overflowAttempt int,
	log *zerolog.Logger,
	span trace.Span,
) (bool, error) {
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 && ctx.Err() != nil {
			return false, ctx.Err()
		}
		state, err, dur := c.runResponsesStreamOnce(ctx, params, h)
		base := state.logger(log, effectiveModel, len(tools), dur)
		if err != nil {
			lastErr = err
			if state.shouldRetryAfterReset(ctx, attempt, err) {
				base.Warn().Err(err).Msg("responses_stream_retry")
				continue
			}
			if policy.Enabled && isContextLengthExceededErr(err) {
				base.Warn().Err(err).Int("overflow_attempt", overflowAttempt+1).Msg("responses_stream_context_overflow")
				return false, err
			}
			base.Error().Err(err).Msg("responses_stream_error")
			span.RecordError(err)
			return false, err
		}
		c.recordResponsesStreamSuccess(ctx, span, managedMsgs, effectiveModel, state)
		base.Debug().Msg("responses_stream_ok")
		return true, nil
	}
	return false, lastErr
}

func (c *Client) runResponsesStreamOnce(ctx context.Context, params rs.ResponseNewParams, h llm.StreamHandler) (*responsesStreamState, error, time.Duration) {
	start := time.Now()
	stream := c.sdk.Responses.NewStreaming(ctx, params)
	state := newResponsesStreamState()
	for stream.Next() {
		state.handleEvent(stream.Current(), h)
	}
	err := stream.Err()
	_ = stream.Close()
	return state, err, time.Since(start)
}

type responsesStreamCallAcc struct {
	name string
	id   string
	args strings.Builder
	done bool
}

type responsesStreamState struct {
	acc              map[int64]*responsesStreamCallAcc
	promptTokens     int
	completionTokens int
	totalTokens      int
	assistantContent strings.Builder
	summaryIndex     int64
	summaryText      strings.Builder
	sawDelta         bool
	sawToolCall      bool
	sawSummary       bool
}

func newResponsesStreamState() *responsesStreamState {
	return &responsesStreamState{acc: make(map[int64]*responsesStreamCallAcc), summaryIndex: -1}
}

func (s *responsesStreamState) handleEvent(ev rs.ResponseStreamEventUnion, h llm.StreamHandler) {
	switch v := ev.AsAny().(type) {
	case rs.ResponseTextDeltaEvent:
		s.handleTextDelta(v, h)
	case rs.ResponseReasoningSummaryTextDeltaEvent:
		s.handleSummaryDelta(v, h)
	case rs.ResponseReasoningSummaryTextDoneEvent:
		s.handleSummaryDone(v, h)
	case rs.ResponseOutputItemAddedEvent:
		s.handleOutputItemAdded(v)
	case rs.ResponseFunctionCallArgumentsDeltaEvent:
		s.callAcc(v.OutputIndex).args.WriteString(v.Delta)
	case rs.ResponseFunctionCallArgumentsDoneEvent:
		s.finishToolCall(v.OutputIndex, v.Arguments, h)
	case rs.ResponseCustomToolCallInputDeltaEvent:
		s.callAcc(v.OutputIndex).args.WriteString(v.Delta)
	case rs.ResponseCustomToolCallInputDoneEvent:
		s.finishToolCall(v.OutputIndex, v.Input, h)
	case rs.ResponseCompletedEvent:
		s.promptTokens = int(v.Response.Usage.InputTokens)
		s.completionTokens = int(v.Response.Usage.OutputTokens)
		s.totalTokens = int(v.Response.Usage.TotalTokens)
	}
}

func (s *responsesStreamState) handleTextDelta(v rs.ResponseTextDeltaEvent, h llm.StreamHandler) {
	if v.Delta == "" {
		return
	}
	s.sawDelta = true
	h.OnDelta(v.Delta)
	s.assistantContent.WriteString(v.Delta)
}

func (s *responsesStreamState) handleSummaryDelta(v rs.ResponseReasoningSummaryTextDeltaEvent, h llm.StreamHandler) {
	if s.summaryIndex != v.SummaryIndex {
		s.summaryIndex = v.SummaryIndex
		s.summaryText.Reset()
	}
	if v.Delta != "" {
		s.sawSummary = true
		s.summaryText.WriteString(v.Delta)
		h.OnThoughtSummary(s.summaryText.String())
	}
}

func (s *responsesStreamState) handleSummaryDone(v rs.ResponseReasoningSummaryTextDoneEvent, h llm.StreamHandler) {
	if s.summaryIndex != v.SummaryIndex {
		s.summaryIndex = v.SummaryIndex
	}
	s.summaryText.Reset()
	if v.Text != "" {
		s.sawSummary = true
		s.summaryText.WriteString(v.Text)
		h.OnThoughtSummary(s.summaryText.String())
	}
}

func (s *responsesStreamState) handleOutputItemAdded(v rs.ResponseOutputItemAddedEvent) {
	switch v.Item.Type {
	case "function_call":
		fn := v.Item.AsFunctionCall()
		s.startToolCall(v.OutputIndex, fn.Name, firstNonEmpty(fn.CallID, fn.ID), fn.Arguments)
	case "mcp_call":
		ct := v.Item.AsMcpCall()
		s.startToolCall(v.OutputIndex, ct.Name, ct.ID, ct.Arguments)
	}
}

func (s *responsesStreamState) startToolCall(outputIndex int64, name string, id string, args string) {
	ca := s.callAcc(outputIndex)
	ca.name = name
	ca.id = id
	if args != "" && ca.args.Len() == 0 {
		ca.args.WriteString(args)
	}
}

func (s *responsesStreamState) callAcc(outputIndex int64) *responsesStreamCallAcc {
	ca := s.acc[outputIndex]
	if ca == nil {
		ca = &responsesStreamCallAcc{}
		s.acc[outputIndex] = ca
	}
	return ca
}

func (s *responsesStreamState) finishToolCall(outputIndex int64, args string, h llm.StreamHandler) {
	ca := s.acc[outputIndex]
	if ca == nil || ca.done {
		return
	}
	if ca.args.Len() == 0 && args != "" {
		ca.args.WriteString(args)
	}
	ca.done = true
	s.sawToolCall = true
	h.OnToolCall(llm.ToolCall{Name: ca.name, Args: json.RawMessage(ca.args.String()), ID: ca.id})
}

func (s *responsesStreamState) logger(log *zerolog.Logger, model string, toolCount int, dur time.Duration) zerolog.Logger {
	return log.With().Str("model", model).Int("tools", toolCount).Dur("duration", dur).
		Int("prompt_tokens", s.promptTokens).
		Int("completion_tokens", s.completionTokens).
		Int("total_tokens", s.totalTokens).
		Logger()
}

func (s *responsesStreamState) shouldRetryAfterReset(ctx context.Context, attempt int, err error) bool {
	return attempt == 0 && isConnectionReset(err) && !s.sawDelta && !s.sawToolCall && !s.sawSummary && ctx.Err() == nil
}

func (c *Client) recordResponsesStreamSuccess(ctx context.Context, span trace.Span, managedMsgs []llm.Message, effectiveModel string, state *responsesStreamState) {
	if c.isSelfHosted() {
		p := c.tokenizeCount(ctx, buildPromptText(managedMsgs))
		a := c.tokenizeCount(ctx, state.assistantContent.String())
		llm.RecordTokenAttributes(span, p, a, p+a)
		if p > 0 || a > 0 {
			llm.RecordTokenMetricsFromContext(ctx, effectiveModel, p, a)
		}
		llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": p, "completion_tokens": a, "total_tokens": p + a})
		return
	}
	llm.RecordTokenAttributes(span, state.promptTokens, state.completionTokens, state.totalTokens)
	if state.promptTokens > 0 || state.completionTokens > 0 {
		llm.RecordTokenMetricsFromContext(ctx, effectiveModel, state.promptTokens, state.completionTokens)
	}
	llm.LogRedactedResponse(ctx, map[string]int{
		"prompt_tokens":     state.promptTokens,
		"completion_tokens": state.completionTokens,
		"total_tokens":      state.totalTokens,
	})
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
