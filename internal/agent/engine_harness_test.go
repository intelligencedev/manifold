package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/agent/harness"
	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"
)

type harnessScriptedProvider struct {
	responses       []llm.Message
	calls           [][]llm.Message
	schemas         [][]llm.ToolSchema
	streamResponses []harnessStreamResponse
	streamCalls     [][]llm.Message
	streamSchemas   [][]llm.ToolSchema
}

type harnessStreamResponse struct {
	Deltas           []string
	ToolCalls        []llm.ToolCall
	Images           []llm.GeneratedImage
	ThoughtSummaries []string
	ThoughtSignature string
	Err              error
}

func (p *harnessScriptedProvider) Chat(_ context.Context, msgs []llm.Message, schemas []llm.ToolSchema, _ string) (llm.Message, error) {
	p.calls = append(p.calls, append([]llm.Message(nil), msgs...))
	p.schemas = append(p.schemas, append([]llm.ToolSchema(nil), schemas...))
	if len(p.responses) == 0 {
		return llm.Message{}, errors.New("no scripted response")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func (p *harnessScriptedProvider) ChatStream(_ context.Context, msgs []llm.Message, schemas []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	p.streamCalls = append(p.streamCalls, append([]llm.Message(nil), msgs...))
	p.streamSchemas = append(p.streamSchemas, append([]llm.ToolSchema(nil), schemas...))
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

type harnessSearchTool struct {
	called int
}

func (t *harnessSearchTool) Name() string {
	return "search"
}

func (t *harnessSearchTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "test search tool",
		"parameters":  map[string]any{"type": "object"},
	}
}

func (t *harnessSearchTool) Call(context.Context, json.RawMessage) (any, error) {
	t.called++
	return map[string]any{"ok": true, "result": "found"}, nil
}

type harnessFlakyTool struct {
	called int
}

func (t *harnessFlakyTool) Name() string {
	return "flaky"
}

func (t *harnessFlakyTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "test flaky tool",
		"parameters":  map[string]any{"type": "object"},
	}
}

func (t *harnessFlakyTool) Call(_ context.Context, raw json.RawMessage) (any, error) {
	t.called++
	var args struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(raw, &args)
	if args.Value == "bad" {
		return nil, errors.New("bad value")
	}
	return map[string]any{"ok": true, "value": args.Value}, nil
}

func TestRunHarnessDefaultsToGuardedChatWhenEnabled(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{responses: []llm.Message{{Role: "assistant", Content: "plain final"}}}
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		HarnessEnabled: true,
	}

	final, err := eng.Run(context.Background(), "hello", nil)

	require.NoError(t, err)
	require.Equal(t, "plain final", final)
	require.Len(t, provider.calls, 1)
}

func TestRunStreamHarnessEmitsLLMRequestSnapshot(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{{
		Deltas: []string{"plain final"},
	}}}
	var snapshots []LLMRequestSnapshot
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		Model:          "gpt-test",
		HarnessEnabled: true,
		HarnessConfig:  harness.RunConfig{Mode: "guarded_chat"},
		OnLLMRequest: func(snapshot LLMRequestSnapshot) {
			snapshots = append(snapshots, snapshot)
		},
	}

	final, err := eng.RunStream(context.Background(), "hello", nil)

	require.NoError(t, err)
	require.Equal(t, "plain final", final)
	require.Len(t, snapshots, 1)
	require.Equal(t, "gpt-test", snapshots[0].Model)
	require.Len(t, snapshots[0].Messages, 1)
	require.Contains(t, snapshots[0].Messages[0].Content, "[CURRENT REQUEST]")
	require.True(t, strings.HasSuffix(snapshots[0].Messages[0].Content, "hello"))
}

func TestRunHarnessDisabledIgnoresWorkflowValidation(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{responses: []llm.Message{{Role: "assistant", Content: "legacy final"}}}
	var assistants []llm.Message
	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		HarnessEnabled: false,
		HarnessConfig: harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		OnAssistant: func(message llm.Message) {
			assistants = append(assistants, message)
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.Run(context.Background(), "hello", nil)

	require.NoError(t, err)
	require.Equal(t, "legacy final", final)
	require.Len(t, provider.calls, 1)
	require.Len(t, assistants, 1)
	require.Len(t, turnMessages, 1)
	require.Equal(t, "legacy final", assistants[0].Content)
	require.Equal(t, "legacy final", turnMessages[0].Content)
}

func TestRunStreamHarnessDisabledStreamsLegacyDeltas(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{{
		Deltas: []string{"legacy ", "stream"},
	}}}
	var deltas []string
	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		HarnessEnabled: false,
		HarnessConfig: harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		OnDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.RunStream(context.Background(), "hello", nil)

	require.NoError(t, err)
	require.Equal(t, "legacy stream", final)
	require.Equal(t, []string{"legacy ", "stream"}, deltas)
	require.Len(t, provider.streamCalls, 1)
	require.Len(t, turnMessages, 1)
	require.Equal(t, "legacy stream", turnMessages[0].Content)
}

func TestRunHarnessAllowsEmptyTerminalToolResponseWithZeroMaxSteps(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":""}`)}}},
	}}
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       0,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
	}

	final, err := eng.Run(context.Background(), "finish empty", nil)

	require.NoError(t, err)
	require.Equal(t, "", final)
	require.Len(t, provider.calls, 1)
}

func TestRunHarnessWorkflowEnforcesRequiredStepBeforeTerminal(t *testing.T) {
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"too soon"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}},
	}}
	search := &harnessSearchTool{}
	registry := tools.NewRegistry()
	registry.Register(search)
	registry.Register(utility.NewAgentResponseTool())

	var started []string
	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       3,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 2,
			Workflow: harness.WorkflowConfig{
				RequiredSteps: []string{"search"},
				TerminalTools: []string{"agent_response"},
			},
		},
		OnToolStart: func(toolName string, _ []byte, _ string) {
			started = append(started, toolName)
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.Run(context.Background(), "answer through the workflow", nil)

	require.NoError(t, err)
	require.Equal(t, "done", final)
	require.Equal(t, 1, search.called)
	require.Equal(t, []string{"search", "agent_response"}, started)
	require.Len(t, provider.calls, 3)
	require.Contains(t, provider.calls[1][len(provider.calls[1])-1].Content, "cannot be called yet")

	var sawNudge bool
	for _, message := range turnMessages {
		if message.Role == "user" && message.Content != "" && containsAll(message.Content, "agent_response", "required") {
			sawNudge = true
			break
		}
	}
	require.True(t, sawNudge, "expected harness nudge in turn messages, got %#v", turnMessages)
}

func TestRunHarnessWorkflowOverlaysAgentResponseTerminalTool(t *testing.T) {
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done without explicit registry"}`)}}},
	}}
	search := &harnessSearchTool{}
	registry := tools.NewRegistry()
	registry.Register(search)

	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       2,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
	}

	final, err := eng.Run(context.Background(), "answer through the workflow", nil)

	require.NoError(t, err)
	require.Equal(t, "done without explicit registry", final)
	require.Equal(t, 1, search.called)
	require.Len(t, provider.calls, 2)
	require.True(t, providerSchemasIncludeTool(provider.schemas[0], "agent_response"), "expected overlaid agent_response schema")
}

func TestRunHarnessWorkflowStillAppliesAfterReMem(t *testing.T) {
	rememProvider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: `{"action":"ACT","content":"ready"}`},
	}}
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: "too soon"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}},
	}}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{LLM: rememProvider})
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       2,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 1,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		EvolvingMemory: em,
		ReMemEnabled:   true,
		ReMemController: memory.NewReMemController(memory.ReMemConfig{
			LLM:           rememProvider,
			Memory:        em,
			MaxInnerSteps: 1,
		}),
	}

	final, err := eng.Run(context.Background(), "answer through the workflow", nil)

	require.NoError(t, err)
	require.Equal(t, "done", final)
	require.Len(t, provider.calls, 2)
	require.Contains(t, provider.calls[1][len(provider.calls[1])-1].Content, "Do not answer with bare text")
}

func TestRunHarnessToolErrorNudgeAllowsRecovery(t *testing.T) {
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"bad"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"good"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"recovered"}`)}}},
	}}
	flaky := &harnessFlakyTool{}
	registry := tools.NewRegistry()
	registry.Register(flaky)
	registry.Register(utility.NewAgentResponseTool())

	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       3,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:          harness.ModeWorkflow,
			MaxToolErrors: 2,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.Run(context.Background(), "recover from a tool error", nil)

	require.NoError(t, err)
	require.Equal(t, "recovered", final)
	require.Equal(t, 2, flaky.called)
	require.Len(t, provider.calls, 3)
	require.Contains(t, provider.calls[1][len(provider.calls[1])-1].Content, "bad value")

	var sawToolErrorNudge bool
	for _, message := range turnMessages {
		if message.Role == "user" && containsAll(message.Content, "flaky", "bad value", "Retry") {
			sawToolErrorNudge = true
			break
		}
	}
	require.True(t, sawToolErrorNudge, "expected tool-error nudge in turn messages, got %#v", turnMessages)
}

func TestRunHarnessToolErrorBudgetExhaustion(t *testing.T) {
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"bad"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"bad"}`)}}},
	}}
	flaky := &harnessFlakyTool{}
	registry := tools.NewRegistry()
	registry.Register(flaky)

	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       3,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:          harness.ModeGuardedChat,
			MaxToolErrors: 2,
		},
	}

	final, err := eng.Run(context.Background(), "fail repeatedly", nil)

	require.Empty(t, final)
	require.ErrorIs(t, err, harness.ErrToolErrorsExhausted)
	var exhausted harness.ToolErrorsExhaustedError
	require.ErrorAs(t, err, &exhausted)
	require.Equal(t, "flaky", exhausted.ToolName)
	require.Equal(t, 2, exhausted.Count)
	require.Equal(t, 2, flaky.called)
	require.Len(t, provider.calls, 2)
	require.Contains(t, provider.calls[1][len(provider.calls[1])-1].Content, "bad value")
}

func TestRunHarnessCompactsOldStepBeforeProviderCall(t *testing.T) {
	provider := &harnessScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"good"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}},
	}}
	search := &harnessSearchTool{}
	flaky := &harnessFlakyTool{}
	registry := tools.NewRegistry()
	registry.Register(search)
	registry.Register(flaky)
	registry.Register(utility.NewAgentResponseTool())

	eng := &Engine{
		LLM:                          provider,
		Tools:                        registry,
		MaxSteps:                     3,
		ContextWindowTokens:          100,
		SummaryReserveBufferTokens:   0,
		SummaryMaxSummaryChunkTokens: 16,
		HarnessEnabled:               true,
		HarnessConfig: harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
			Compact: harness.CompactConfig{
				Enabled:         true,
				KeepRecentSteps: 1,
				PhaseThresholds: []float64{0.01, 0.02, 0.03},
			},
		},
	}

	final, err := eng.Run(context.Background(), "compact the old step", nil)

	require.NoError(t, err)
	require.Equal(t, "done", final)
	require.Len(t, provider.calls, 3)
	require.True(t, messagesContain(provider.calls[2], "Harness compacted context"), "expected compact hint in provider call: %#v", provider.calls[2])
	require.True(t, messagesContain(provider.calls[2], "[COMPACTED tool result: search"), "expected compacted old tool result in provider call: %#v", provider.calls[2])
	require.True(t, messagesContain(provider.calls[2], `"value":"good"`), "expected recent step to remain visible: %#v", provider.calls[2])
}

func TestRunStreamHarnessDefaultsToGuardedChatStreamsAcceptedFinal(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{{
		Deltas:           []string{"plain ", "final"},
		ThoughtSummaries: []string{"thinking"},
	}}}
	var deltas []string
	var summaries []string
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		HarnessEnabled: true,
		OnDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
		OnThoughtSummary: func(summary string) {
			summaries = append(summaries, summary)
		},
	}

	final, err := eng.RunStream(context.Background(), "hello", nil)

	require.NoError(t, err)
	require.Equal(t, "plain final", final)
	require.Equal(t, []string{"plain ", "final"}, deltas)
	require.Equal(t, []string{"thinking"}, summaries)
	require.Len(t, provider.streamCalls, 1)
}

func TestRunStreamHarnessWorkflowBuffersInvalidTextAndNudges(t *testing.T) {
	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{
		{Deltas: []string{"too soon"}},
		{ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}},
	}}
	search := &harnessSearchTool{}
	registry := tools.NewRegistry()
	registry.Register(search)
	registry.Register(utility.NewAgentResponseTool())

	var deltas []string
	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       2,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 1,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		OnDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.RunStream(context.Background(), "answer through the workflow", nil)

	require.NoError(t, err)
	require.Equal(t, "done", final)
	require.Empty(t, deltas, "invalid streamed text should be buffered and dropped")
	require.Equal(t, 1, search.called)
	require.Len(t, provider.streamCalls, 3)
	require.Contains(t, provider.streamCalls[1][len(provider.streamCalls[1])-1].Content, "Do not answer with bare text")

	var sawNudge bool
	for _, message := range turnMessages {
		if message.Role == "user" && containsAll(message.Content, "Do not answer with bare text", "agent_response") {
			sawNudge = true
			break
		}
	}
	require.True(t, sawNudge, "expected harness nudge in turn messages, got %#v", turnMessages)
}

func TestRunStreamHarnessToolErrorNudgeAllowsRecovery(t *testing.T) {
	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{
		{ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"bad"}`)}}},
		{ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"good"}`)}}},
		{ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"recovered"}`)}}},
	}}
	flaky := &harnessFlakyTool{}
	registry := tools.NewRegistry()
	registry.Register(flaky)
	registry.Register(utility.NewAgentResponseTool())

	var turnMessages []llm.Message
	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       3,
		HarnessEnabled: true,
		HarnessConfig: harness.RunConfig{
			Mode:          harness.ModeWorkflow,
			MaxToolErrors: 2,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		},
		OnTurnMessage: func(message llm.Message) {
			turnMessages = append(turnMessages, message)
		},
	}

	final, err := eng.RunStream(context.Background(), "recover from a tool error", nil)

	require.NoError(t, err)
	require.Equal(t, "recovered", final)
	require.Equal(t, 2, flaky.called)
	require.Len(t, provider.streamCalls, 3)
	require.Contains(t, provider.streamCalls[1][len(provider.streamCalls[1])-1].Content, "bad value")

	var sawToolErrorNudge bool
	for _, message := range turnMessages {
		if message.Role == "user" && containsAll(message.Content, "flaky", "bad value", "Retry") {
			sawToolErrorNudge = true
			break
		}
	}
	require.True(t, sawToolErrorNudge, "expected tool-error nudge in turn messages, got %#v", turnMessages)
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

func messagesContain(messages []llm.Message, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, needle) {
			return true
		}
		for _, call := range message.ToolCalls {
			if strings.Contains(call.Name, needle) || strings.Contains(string(call.Args), needle) || strings.Contains(call.ID, needle) {
				return true
			}
		}
	}
	return false
}

func providerSchemasIncludeTool(schemas []llm.ToolSchema, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}
