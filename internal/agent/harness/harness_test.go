package harness

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/llm"
)

type scriptedProvider struct {
	responses       []llm.Message
	calls           [][]llm.Message
	streamResponses []streamResponse
	streamCalls     [][]llm.Message
}

type streamResponse struct {
	Deltas           []string
	ToolCalls        []llm.ToolCall
	Images           []llm.GeneratedImage
	ThoughtSummaries []string
	ThoughtSignature string
	Err              error
}

func (p *scriptedProvider) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	p.calls = append(p.calls, append([]llm.Message(nil), msgs...))
	if len(p.responses) == 0 {
		return llm.Message{}, errors.New("no scripted response")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func (p *scriptedProvider) ChatStream(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	p.streamCalls = append(p.streamCalls, append([]llm.Message(nil), msgs...))
	if len(p.streamResponses) == 0 {
		return errors.New("no scripted stream response")
	}
	resp := p.streamResponses[0]
	p.streamResponses = p.streamResponses[1:]
	if resp.Err != nil {
		return resp.Err
	}
	for _, summary := range resp.ThoughtSummaries {
		h.OnThoughtSummary(summary)
	}
	for _, delta := range resp.Deltas {
		h.OnDelta(delta)
	}
	for _, toolCall := range resp.ToolCalls {
		h.OnToolCall(toolCall)
	}
	for _, image := range resp.Images {
		h.OnImage(image)
	}
	if resp.ThoughtSignature != "" {
		h.OnThoughtSignature(resp.ThoughtSignature)
	}
	return nil
}

func TestSerializeMessagesStripsHarnessMetadata(t *testing.T) {
	messages := []HarnessMessage{
		{
			Message: llm.Message{Role: "user", Content: "hello"},
			Meta:    MessageMeta{Type: MessageTypeNudge, StepIndex: 3},
		},
	}

	serialized := SerializeMessages(messages)

	require.Equal(t, []llm.Message{{Role: "user", Content: "hello"}}, serialized)
}

func TestValidatorGuardedChatAllowsFinalText(t *testing.T) {
	validator := NewResponseValidator(RunConfig{Mode: ModeGuardedChat}, nil)

	result := validator.Validate(llm.Message{Role: "assistant", Content: "done"}, nil)

	require.True(t, result.Valid)
}

func TestValidatorRejectsUnknownTool(t *testing.T) {
	validator := NewResponseValidator(RunConfig{Mode: ModeGuardedChat}, []llm.ToolSchema{{Name: "known_tool"}})

	result := validator.Validate(llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			Name: "missing_tool",
			Args: json.RawMessage(`{}`),
		}},
	}, nil)

	require.False(t, result.Valid)
	require.Equal(t, ValidationReasonUnknownTool, result.Reason)
	require.Equal(t, []string{"missing_tool"}, result.UnknownTools)
	require.Contains(t, result.Nudge, "known_tool")
}

func TestRunInferenceNudgesWorkflowTextThenAcceptsToolCall(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: "I can answer directly."},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "lookup", Args: json.RawMessage(`{"query":"x"}`)}}},
	}}
	cfg := RunConfig{
		Mode:              ModeWorkflow,
		MaxRetriesPerStep: 1,
		Workflow:          WorkflowConfig{TerminalTools: []string{"agent_response"}},
	}

	result, err := RunInference(context.Background(), provider, WrapMessages([]llm.Message{{Role: "user", Content: "go"}}), []llm.ToolSchema{{Name: "lookup"}}, "model", cfg, nil)

	require.NoError(t, err)
	require.Equal(t, 2, result.Attempts)
	require.Len(t, provider.calls, 2)
	require.Len(t, provider.calls[1], 3)
	require.Equal(t, "user", provider.calls[1][2].Role)
	require.Contains(t, provider.calls[1][2].Content, "Do not answer with bare text")
	require.Len(t, result.Message.ToolCalls, 1)
}

func TestRunInferencePairsRejectedToolCallWithOutputBeforeRetry(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", ID: "call_terminal", Args: json.RawMessage(`{"text":"too soon"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "lookup", ID: "call_lookup", Args: json.RawMessage(`{"query":"x"}`)}}},
	}}
	cfg := RunConfig{
		Mode:              ModeWorkflow,
		MaxRetriesPerStep: 1,
		Workflow: WorkflowConfig{
			RequiredSteps: []string{"lookup"},
			TerminalTools: []string{"agent_response"},
		},
	}

	result, err := RunInference(context.Background(), provider, WrapMessages([]llm.Message{{Role: "user", Content: "go"}}), []llm.ToolSchema{{Name: "lookup"}, {Name: "agent_response"}}, "model", cfg, nil)

	require.NoError(t, err)
	require.Equal(t, 2, result.Attempts)
	require.Len(t, provider.calls, 2)
	require.Len(t, provider.calls[1], 4)
	require.Equal(t, "assistant", provider.calls[1][1].Role)
	require.Equal(t, "call_terminal", provider.calls[1][1].ToolCalls[0].ID)
	require.Equal(t, "tool", provider.calls[1][2].Role)
	require.Equal(t, "call_terminal", provider.calls[1][2].ToolID)
	require.Contains(t, provider.calls[1][2].Content, "Tool call rejected by harness")
	require.Equal(t, "user", provider.calls[1][3].Role)
	require.Contains(t, provider.calls[1][3].Content, "cannot be called yet")
	require.Equal(t, "lookup", result.Message.ToolCalls[0].Name)
}

func TestRunInferenceRescuesEmbeddedToolCallJSONWhenEnabled(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: `{"tool":"lookup","args":{"query":"forge"}}`},
	}}
	cfg := RunConfig{
		Mode:          ModeWorkflow,
		RescueEnabled: true,
		Workflow:      WorkflowConfig{TerminalTools: []string{"agent_response"}},
	}

	result, err := RunInference(context.Background(), provider, WrapMessages([]llm.Message{{Role: "user", Content: "go"}}), []llm.ToolSchema{{Name: "lookup"}}, "model", cfg, nil)

	require.NoError(t, err)
	require.Len(t, result.Message.ToolCalls, 1)
	require.Equal(t, "lookup", result.Message.ToolCalls[0].Name)
	require.JSONEq(t, `{"query":"forge"}`, string(result.Message.ToolCalls[0].Args))
	require.Empty(t, result.Message.Content)
}

func TestRunInferenceDoesNotRescueEmbeddedToolCallJSONWhenDisabled(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: `{"tool":"lookup","args":{"query":"forge"}}`},
		{Role: "assistant", Content: `{"tool":"lookup","args":{"query":"forge"}}`},
	}}
	cfg := RunConfig{
		Mode:              ModeWorkflow,
		RescueEnabled:     false,
		MaxRetriesPerStep: 1,
		Workflow:          WorkflowConfig{TerminalTools: []string{"agent_response"}},
	}

	_, err := RunInference(context.Background(), provider, nil, []llm.ToolSchema{{Name: "lookup"}}, "model", cfg, nil)

	require.ErrorIs(t, err, ErrValidationRetriesExhausted)
}

func TestRunInferenceReturnsRetryExhaustion(t *testing.T) {
	provider := &scriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: "text one"},
		{Role: "assistant", Content: "text two"},
	}}
	cfg := RunConfig{Mode: ModeWorkflow, MaxRetriesPerStep: 1}

	_, err := RunInference(context.Background(), provider, nil, []llm.ToolSchema{{Name: "lookup"}}, "model", cfg, nil)

	require.ErrorIs(t, err, ErrValidationRetriesExhausted)
	var exhausted RetryExhaustedError
	require.ErrorAs(t, err, &exhausted)
	require.Equal(t, 2, exhausted.Attempts)
	require.Equal(t, ValidationReasonBareTextInWorkflow, exhausted.Last.Reason)
}

func TestRunStreamInferenceNudgesThenReturnsAcceptedStream(t *testing.T) {
	provider := &scriptedProvider{streamResponses: []streamResponse{
		{ToolCalls: []llm.ToolCall{{Name: "missing_tool", Args: json.RawMessage(`{}`)}}},
		{
			Deltas:           []string{"accepted ", "text"},
			ThoughtSummaries: []string{"summary"},
			ThoughtSignature: "signature",
		},
	}}
	cfg := RunConfig{Mode: ModeGuardedChat, MaxRetriesPerStep: 1}

	result, err := RunStreamInference(context.Background(), provider, WrapMessages([]llm.Message{{Role: "user", Content: "go"}}), []llm.ToolSchema{{Name: "known_tool"}}, "model", cfg, nil)

	require.NoError(t, err)
	require.Equal(t, 2, result.Attempts)
	require.Equal(t, "accepted text", result.Message.Content)
	require.Equal(t, []string{"accepted ", "text"}, result.Deltas)
	require.Equal(t, []string{"summary"}, result.ThoughtSummaries)
	require.Equal(t, "signature", result.Message.ThoughtSignature)
	require.Len(t, provider.streamCalls, 2)
	require.Contains(t, provider.streamCalls[1][len(provider.streamCalls[1])-1].Content, "unavailable tool")
}

func TestRunStreamInferencePairsRejectedToolCallWithOutputBeforeRetry(t *testing.T) {
	provider := &scriptedProvider{streamResponses: []streamResponse{
		{ToolCalls: []llm.ToolCall{{Name: "agent_response", ID: "call_terminal", Args: json.RawMessage(`{"text":"too soon"}`)}}},
		{ToolCalls: []llm.ToolCall{{Name: "lookup", ID: "call_lookup", Args: json.RawMessage(`{"query":"x"}`)}}},
	}}
	cfg := RunConfig{
		Mode:              ModeWorkflow,
		MaxRetriesPerStep: 1,
		Workflow: WorkflowConfig{
			RequiredSteps: []string{"lookup"},
			TerminalTools: []string{"agent_response"},
		},
	}

	result, err := RunStreamInference(context.Background(), provider, WrapMessages([]llm.Message{{Role: "user", Content: "go"}}), []llm.ToolSchema{{Name: "lookup"}, {Name: "agent_response"}}, "model", cfg, nil)

	require.NoError(t, err)
	require.Equal(t, 2, result.Attempts)
	require.Len(t, provider.streamCalls, 2)
	require.Len(t, provider.streamCalls[1], 4)
	require.Equal(t, "assistant", provider.streamCalls[1][1].Role)
	require.Equal(t, "call_terminal", provider.streamCalls[1][1].ToolCalls[0].ID)
	require.Equal(t, "tool", provider.streamCalls[1][2].Role)
	require.Equal(t, "call_terminal", provider.streamCalls[1][2].ToolID)
	require.Contains(t, provider.streamCalls[1][2].Content, "Tool call rejected by harness")
	require.Equal(t, "user", provider.streamCalls[1][3].Role)
	require.Contains(t, provider.streamCalls[1][3].Content, "cannot be called yet")
	require.Equal(t, "lookup", result.Message.ToolCalls[0].Name)
}

func TestStepEnforcerBlocksTerminalUntilRequiredStepsComplete(t *testing.T) {
	tracker := NewStepTracker()
	enforcer := NewStepEnforcer(WorkflowConfig{
		RequiredSteps: []string{"search"},
		TerminalTools: []string{"agent_response"},
	})

	check := enforcer.Check([]llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}, tracker)

	require.False(t, check.OK)
	require.Equal(t, ValidationReasonRequiredStepMissing, check.Reason)
	require.Equal(t, []string{"search"}, check.MissingSteps)

	tracker.RecordSuccess(llm.ToolCall{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)})
	check = enforcer.Check([]llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}, tracker)

	require.True(t, check.OK)
}

func TestCompactMessagesDropsOldNudgesAndKeepsRecentStep(t *testing.T) {
	tracker := NewStepTracker()
	tracker.RecordSuccess(llm.ToolCall{Name: "search", ID: "call-search", Args: json.RawMessage(`{"query":"forge"}`)})
	messages := []HarnessMessage{
		{Message: llm.Message{Role: "system", Content: "system"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "user", Content: "current request"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "assistant", Content: "old text answer"}, Meta: MessageMeta{Type: MessageTypeAssistant, StepIndex: 0}},
		{Message: llm.Message{Role: "user", Content: "old retry nudge"}, Meta: MessageMeta{Type: MessageTypeNudge, StepIndex: 0}},
		{Message: llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				Name: "search",
				ID:   "call-search",
				Args: json.RawMessage(`{"query":"forge"}`),
			}},
			ThoughtSignature: "sig",
		}, Meta: MessageMeta{Type: MessageTypeAssistant, StepIndex: 0}},
		{Message: llm.Message{Role: "tool", ToolID: "call-search", Content: strings.Repeat("result ", 200)}, Meta: MessageMeta{Type: MessageTypeTool, StepIndex: 0, ToolName: "search", ToolCallID: "call-search"}},
		{Message: llm.Message{Role: "assistant", Content: "recent text"}, Meta: MessageMeta{Type: MessageTypeAssistant, StepIndex: 1}},
	}

	result := CompactMessages(messages, CompactConfig{
		Enabled:             true,
		KeepRecentSteps:     1,
		ContextWindowTokens: 100,
		PhaseThresholds:     []float64{0.01, 0.02, 0.03},
		PerMessageRunes:     128,
	}, tracker)

	require.True(t, result.Changed)
	require.Equal(t, 3, result.Phase)
	require.Equal(t, 1, result.DroppedNudges)
	require.Equal(t, 1, result.CompactedToolResults)
	require.Equal(t, 1, result.CompactedAssistantMessages)
	require.Equal(t, "system", result.Messages[0].Message.Content)
	require.Equal(t, "current request", result.Messages[1].Message.Content)
	require.Contains(t, result.Messages[2].Message.Content, "Steps completed: search(call-search)")

	var sawOldNudge bool
	var sawCompactedTool bool
	var sawRecentText bool
	var sawToolCallSkeleton bool
	for _, message := range result.Messages {
		if message.Message.Content == "old retry nudge" {
			sawOldNudge = true
		}
		if strings.Contains(message.Message.Content, "[COMPACTED tool result: search id=call-search]") {
			sawCompactedTool = true
		}
		if message.Message.Content == "recent text" {
			sawRecentText = true
		}
		if len(message.Message.ToolCalls) == 1 && message.Message.ToolCalls[0].ID == "call-search" && message.Message.ThoughtSignature == "sig" {
			sawToolCallSkeleton = true
		}
	}
	require.False(t, sawOldNudge)
	require.True(t, sawCompactedTool)
	require.True(t, sawRecentText)
	require.True(t, sawToolCallSkeleton)
}

func TestCompactMessagesKeepsRuntimeContextOnCurrentPrompt(t *testing.T) {
	tracker := NewStepTracker()
	messages := []HarnessMessage{
		{Message: llm.Message{Role: "system", Content: "system"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "user", Content: "prior request"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "user", Content: "[RUNTIME CONTEXT]\nruntime memory\n\ncurrent request"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "assistant", Content: strings.Repeat("old answer ", 200)}, Meta: MessageMeta{Type: MessageTypeAssistant, StepIndex: 0}},
		{Message: llm.Message{Role: "tool", ToolID: "call-search", Content: strings.Repeat("result ", 200)}, Meta: MessageMeta{Type: MessageTypeTool, StepIndex: 0}},
	}

	result := CompactMessages(messages, CompactConfig{
		Enabled:             true,
		KeepRecentSteps:     0,
		ContextWindowTokens: 80,
		PhaseThresholds:     []float64{0.01, 0.02, 0.03},
		PerMessageRunes:     64,
	}, tracker)

	require.True(t, result.Changed)
	require.GreaterOrEqual(t, len(result.Messages), 3)
	require.Equal(t, "system", result.Messages[0].Message.Content)
	var current string
	for _, message := range result.Messages {
		if message.Meta.Type == MessageTypePrompt && message.Message.Role == "user" && strings.Contains(message.Message.Content, "current request") {
			current = message.Message.Content
			break
		}
	}
	require.Contains(t, current, "[RUNTIME CONTEXT]")
	require.Contains(t, current, "runtime memory")
	require.Contains(t, current, "current request")
}

func TestCompactMessagesDoesNotEraseAuthoritativeStepState(t *testing.T) {
	tracker := NewStepTracker()
	tracker.RecordSuccess(llm.ToolCall{Name: "search", ID: "call-search", Args: json.RawMessage(`{"url":"https://example.com"}`)})
	messages := []HarnessMessage{
		{Message: llm.Message{Role: "user", Content: "fetch it"}, Meta: MessageMeta{Type: MessageTypePrompt}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", ID: "call-search", Args: json.RawMessage(`{"url":"https://example.com"}`)}}}, Meta: MessageMeta{Type: MessageTypeAssistant, StepIndex: 0}},
		{Message: llm.Message{Role: "tool", ToolID: "call-search", Content: strings.Repeat("large ", 200)}, Meta: MessageMeta{Type: MessageTypeTool, StepIndex: 0, ToolName: "search", ToolCallID: "call-search"}},
	}

	_ = CompactMessages(messages, CompactConfig{
		Enabled:             true,
		KeepRecentSteps:     0,
		ContextWindowTokens: 80,
		PhaseThresholds:     []float64{0.01, 0.02, 0.03},
	}, tracker)
	check := NewStepEnforcer(WorkflowConfig{
		ToolPrerequisites: map[string][]Prerequisite{
			"fetch": {{Tool: "search", MatchArg: "url"}},
		},
	}).Check([]llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com"}`)}}, tracker)

	require.True(t, check.OK)
}

func TestStepEnforcerChecksNameOnlyPrerequisite(t *testing.T) {
	enforcer := NewStepEnforcer(WorkflowConfig{
		ToolPrerequisites: map[string][]Prerequisite{
			"summarize": {{Tool: "search"}},
		},
	})

	check := enforcer.Check([]llm.ToolCall{{Name: "summarize", Args: json.RawMessage(`{}`)}}, NewStepTracker())

	require.False(t, check.OK)
	require.Equal(t, ValidationReasonPrerequisiteMissing, check.Reason)

	tracker := NewStepTracker()
	tracker.RecordSuccess(llm.ToolCall{Name: "search", Args: json.RawMessage(`{}`)})
	check = enforcer.Check([]llm.ToolCall{{Name: "summarize", Args: json.RawMessage(`{}`)}}, tracker)
	require.True(t, check.OK)
}

func TestStepEnforcerChecksArgMatchedPrerequisiteAgainstPreBatchState(t *testing.T) {
	enforcer := NewStepEnforcer(WorkflowConfig{
		ToolPrerequisites: map[string][]Prerequisite{
			"fetch": {{Tool: "search", MatchArg: "url"}},
		},
	})
	tracker := NewStepTracker()
	tracker.RecordSuccess(llm.ToolCall{Name: "search", Args: json.RawMessage(`{"url":"https://example.com/a"}`)})

	check := enforcer.Check([]llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/b"}`)}}, tracker)
	require.False(t, check.OK)

	check = enforcer.Check([]llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}, tracker)
	require.True(t, check.OK)

	emptyTracker := NewStepTracker()
	check = enforcer.Check([]llm.ToolCall{
		{Name: "search", Args: json.RawMessage(`{"url":"https://example.com/a"}`)},
		{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)},
	}, emptyTracker)
	require.False(t, check.OK, "batch prerequisites should be checked before any batch call is recorded")
}

func TestDetectToolErrorPayload(t *testing.T) {
	tests := []struct {
		name          string
		payload       []byte
		wantErr       bool
		wantMessage   string
		wantPolicyHit bool
	}{
		{name: "ok false", payload: []byte(`{"ok":false,"error":"bad args"}`), wantErr: true, wantMessage: "bad args"},
		{name: "error only", payload: []byte(`{"error":"tool not found"}`), wantErr: true, wantMessage: "tool not found"},
		{name: "policy", payload: []byte(`{"ok":false,"error":"tool call blocked by policy","policy_id":"p1"}`), wantErr: true, wantMessage: "tool call blocked by policy", wantPolicyHit: true},
		{name: "ok true", payload: []byte(`{"ok":true,"error":"warning"}`), wantErr: false},
		{name: "not json", payload: []byte(`plain text`), wantErr: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := DetectToolErrorPayload(test.payload)
			require.Equal(t, test.wantErr, ok)
			require.Equal(t, test.wantMessage, got.Message)
			require.Equal(t, test.wantPolicyHit, got.PolicyBlocked)
		})
	}
}

func TestToolErrorTrackerResetsOnSuccessAndExhausts(t *testing.T) {
	tracker := NewToolErrorTracker(2)

	count, err := tracker.RecordFailure("flaky", ToolError{Message: "first"})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, 1, tracker.Consecutive())

	tracker.RecordSuccess()
	require.Equal(t, 0, tracker.Consecutive())

	count, err = tracker.RecordFailure("flaky", ToolError{Message: "second"})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	count, err = tracker.RecordFailure("flaky", ToolError{Message: "third"})

	require.ErrorIs(t, err, ErrToolErrorsExhausted)
	require.Equal(t, 2, count)
	var exhausted ToolErrorsExhaustedError
	require.ErrorAs(t, err, &exhausted)
	require.Equal(t, "flaky", exhausted.ToolName)
	require.Equal(t, "third", exhausted.Last.Message)
}
