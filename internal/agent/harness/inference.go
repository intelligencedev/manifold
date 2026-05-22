package harness

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/llm"
)

// InferenceResult contains the first validated assistant response and the
// provider-visible history accumulated during validation retries.
type InferenceResult struct {
	Message          llm.Message
	Attempts         int
	History          []HarnessMessage
	Validation       ValidationResult
	Deltas           []string
	ThoughtSummaries []string
}

// RunInference performs one guarded model step with validation nudges.
func RunInference(ctx context.Context, provider llm.Provider, history []HarnessMessage, schemas []llm.ToolSchema, model string, cfg RunConfig, tracker *StepTracker) (InferenceResult, error) {
	if provider == nil {
		return InferenceResult{}, ErrNilProvider
	}
	cfg = normalizeRunConfig(cfg)
	validator := NewResponseValidator(cfg, schemas)

	for attempt := 0; attempt <= cfg.MaxRetriesPerStep; attempt++ {
		msg, err := provider.Chat(ctx, SerializeMessages(history), schemas, model)
		if err != nil {
			return InferenceResult{Attempts: attempt + 1, History: history}, err
		}
		msg, _ = RescueEmbeddedToolCalls(msg, cfg.RescueEnabled)
		msg.ToolCalls = llm.NormalizeToolCalls(msg.ToolCalls)
		msg = ensureRetryableToolCallIDs(msg, len(history), attempt)

		history = append(history, HarnessMessage{
			Message: msg,
			Meta: MessageMeta{
				Type:      MessageTypeAssistant,
				StepIndex: attempt,
			},
		})
		validation := validator.Validate(msg, tracker)
		if validation.Valid {
			return InferenceResult{
				Message:    msg,
				Attempts:   attempt + 1,
				History:    history,
				Validation: validation,
			}, nil
		}

		if attempt == cfg.MaxRetriesPerStep {
			return InferenceResult{
				Message:    msg,
				Attempts:   attempt + 1,
				History:    history,
				Validation: validation,
			}, RetryExhaustedError{Attempts: attempt + 1, Last: validation}
		}

		history = appendRejectedToolOutputs(history, msg, validation, attempt)
		history = append(history, HarnessMessage{
			Message: llm.Message{
				Role:    "user",
				Content: validation.Nudge,
			},
			Meta: MessageMeta{
				Type:      MessageTypeNudge,
				StepIndex: attempt,
			},
		})
	}

	return InferenceResult{}, RetryExhaustedError{Attempts: cfg.MaxRetriesPerStep + 1}
}

// RunStreamInference performs one guarded streaming model step with validation nudges.
// Stream chunks are captured and returned only for the accepted response.
func RunStreamInference(ctx context.Context, provider llm.Provider, history []HarnessMessage, schemas []llm.ToolSchema, model string, cfg RunConfig, tracker *StepTracker) (InferenceResult, error) {
	if provider == nil {
		return InferenceResult{}, ErrNilProvider
	}
	cfg = normalizeRunConfig(cfg)
	validator := NewResponseValidator(cfg, schemas)

	for attempt := 0; attempt <= cfg.MaxRetriesPerStep; attempt++ {
		capture := &streamCaptureHandler{}
		if err := provider.ChatStream(ctx, SerializeMessages(history), schemas, model, capture); err != nil {
			return InferenceResult{Attempts: attempt + 1, History: history}, err
		}
		msg := capture.message()
		msg, _ = RescueEmbeddedToolCalls(msg, cfg.RescueEnabled)
		msg.ToolCalls = llm.NormalizeToolCalls(msg.ToolCalls)
		msg = ensureRetryableToolCallIDs(msg, len(history), attempt)

		history = append(history, HarnessMessage{
			Message: msg,
			Meta: MessageMeta{
				Type:      MessageTypeAssistant,
				StepIndex: attempt,
			},
		})
		validation := validator.Validate(msg, tracker)
		if validation.Valid {
			return InferenceResult{
				Message:          msg,
				Attempts:         attempt + 1,
				History:          history,
				Validation:       validation,
				Deltas:           append([]string(nil), capture.deltas...),
				ThoughtSummaries: append([]string(nil), capture.thoughtSummaries...),
			}, nil
		}

		if attempt == cfg.MaxRetriesPerStep {
			return InferenceResult{
				Message:    msg,
				Attempts:   attempt + 1,
				History:    history,
				Validation: validation,
			}, RetryExhaustedError{Attempts: attempt + 1, Last: validation}
		}

		history = appendRejectedToolOutputs(history, msg, validation, attempt)
		history = append(history, HarnessMessage{
			Message: llm.Message{
				Role:    "user",
				Content: validation.Nudge,
			},
			Meta: MessageMeta{
				Type:      MessageTypeNudge,
				StepIndex: attempt,
			},
		})
	}

	return InferenceResult{}, RetryExhaustedError{Attempts: cfg.MaxRetriesPerStep + 1}
}

func ensureRetryableToolCallIDs(msg llm.Message, historyLen, attempt int) llm.Message {
	for i := range msg.ToolCalls {
		if strings.TrimSpace(msg.ToolCalls[i].ID) != "" {
			continue
		}
		msg.ToolCalls[i].ID = fmt.Sprintf("call_harness_retry_%d_%d_%d", historyLen, attempt, i)
	}
	return msg
}

func appendRejectedToolOutputs(history []HarnessMessage, msg llm.Message, validation ValidationResult, attempt int) []HarnessMessage {
	if len(msg.ToolCalls) == 0 {
		return history
	}
	content := fmt.Sprintf("Tool call rejected by harness before execution: %s", validation.Nudge)
	for _, call := range msg.ToolCalls {
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			continue
		}
		history = append(history, HarnessMessage{
			Message: llm.Message{
				Role:    "tool",
				ToolID:  callID,
				Content: content,
			},
			Meta: MessageMeta{
				Type:       MessageTypeTool,
				StepIndex:  attempt,
				ToolName:   call.Name,
				ToolCallID: callID,
			},
		})
	}
	return history
}

type streamCaptureHandler struct {
	content          string
	deltas           []string
	toolCalls        []llm.ToolCall
	images           []llm.GeneratedImage
	thoughtSummaries []string
	thoughtSignature string
}

func (h *streamCaptureHandler) OnDelta(content string) {
	h.content += content
	h.deltas = append(h.deltas, content)
}

func (h *streamCaptureHandler) OnToolCall(tc llm.ToolCall) {
	h.toolCalls = append(h.toolCalls, tc)
}

func (h *streamCaptureHandler) OnImage(img llm.GeneratedImage) {
	h.images = append(h.images, img)
}

func (h *streamCaptureHandler) OnThoughtSummary(summary string) {
	h.thoughtSummaries = append(h.thoughtSummaries, summary)
}

func (h *streamCaptureHandler) OnThoughtSignature(sig string) {
	h.thoughtSignature = sig
}

func (h *streamCaptureHandler) message() llm.Message {
	return llm.Message{
		Role:             "assistant",
		Content:          h.content,
		ToolCalls:        append([]llm.ToolCall(nil), h.toolCalls...),
		Images:           append([]llm.GeneratedImage(nil), h.images...),
		ThoughtSignature: h.thoughtSignature,
	}
}
