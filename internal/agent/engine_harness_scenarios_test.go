package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/agent/harness"
	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"
)

type forgeScenarioTool struct {
	name    string
	results []forgeScenarioToolResult
	calls   []json.RawMessage
}

type forgeScenarioToolResult struct {
	payload any
	err     error
}

type forgeScenarioMetrics struct {
	toolStarts    []string
	toolResults   []string
	turnMessages  []llm.Message
	assistants    []llm.Message
	providerCalls int
	providerMsgs  [][]llm.Message
}

func (t *forgeScenarioTool) Name() string {
	return t.name
}

func (t *forgeScenarioTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "deterministic forge scenario tool " + t.name,
		"parameters": map[string]any{
			"type": "object",
		},
	}
}

func (t *forgeScenarioTool) Call(_ context.Context, raw json.RawMessage) (any, error) {
	t.calls = append(t.calls, append(json.RawMessage(nil), raw...))
	if len(t.results) == 0 {
		return map[string]any{"ok": true, "tool": t.name, "args": json.RawMessage(raw)}, nil
	}
	result := t.results[0]
	t.results = t.results[1:]
	if result.err != nil {
		return nil, result.err
	}
	if result.payload != nil {
		return result.payload, nil
	}
	return map[string]any{"ok": true, "tool": t.name}, nil
}

func TestForgeHarnessScenarios(t *testing.T) {
	t.Run("basic two-step workflow", func(t *testing.T) {
		search := &forgeScenarioTool{name: "search"}
		registry := forgeScenarioRegistry(search)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"done"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		})

		require.Equal(t, "done", final)
		require.Equal(t, []string{"search", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 2, metrics.providerCalls)
		require.Len(t, search.calls, 1)
		require.Zero(t, metrics.countNudgesContaining("failed"))
	})

	t.Run("sequential three-step workflow", func(t *testing.T) {
		search := &forgeScenarioTool{name: "search"}
		fetch := &forgeScenarioTool{name: "fetch"}
		summarize := &forgeScenarioTool{name: "summarize"}
		registry := forgeScenarioRegistry(search, fetch, summarize)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "summarize", Args: json.RawMessage(`{"topic":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"three-step complete"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				RequiredSteps: []string{"search", "fetch", "summarize"},
				TerminalTools: []string{"agent_response"},
				ToolPrerequisites: map[string][]harness.Prerequisite{
					"fetch":     {{Tool: "search", MatchArg: "url"}},
					"summarize": {{Tool: "fetch"}},
				},
			},
		})

		require.Equal(t, "three-step complete", final)
		require.Equal(t, []string{"search", "fetch", "summarize", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 4, metrics.providerCalls)
		require.Len(t, search.calls, 1)
		require.Len(t, fetch.calls, 1)
		require.Len(t, summarize.calls, 1)
	})

	t.Run("required-step premature terminal recovery", func(t *testing.T) {
		search := &forgeScenarioTool{name: "search"}
		registry := forgeScenarioRegistry(search)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"too soon"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"recovered"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 1,
			Workflow: harness.WorkflowConfig{
				RequiredSteps: []string{"search"},
				TerminalTools: []string{"agent_response"},
			},
		})

		require.Equal(t, "recovered", final)
		require.Equal(t, []string{"search", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 1, metrics.countNudgesContaining("cannot be called yet"))
		require.Equal(t, 3, metrics.providerCalls)
	})

	t.Run("prerequisite violation recovery", func(t *testing.T) {
		search := &forgeScenarioTool{name: "search"}
		fetch := &forgeScenarioTool{name: "fetch"}
		registry := forgeScenarioRegistry(search, fetch)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"prerequisite recovered"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 1,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
				ToolPrerequisites: map[string][]harness.Prerequisite{
					"fetch": {{Tool: "search", MatchArg: "url"}},
				},
			},
		})

		require.Equal(t, "prerequisite recovered", final)
		require.Equal(t, []string{"search", "fetch", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 1, metrics.countNudgesContaining("First call"))
		require.Equal(t, 4, metrics.providerCalls)
	})

	t.Run("unknown tool recovery", func(t *testing.T) {
		search := &forgeScenarioTool{name: "search"}
		registry := forgeScenarioRegistry(search)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "web_lookup", Args: json.RawMessage(`{"query":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"query":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"unknown recovered"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode:              harness.ModeWorkflow,
			MaxRetriesPerStep: 1,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		})

		require.Equal(t, "unknown recovered", final)
		require.Equal(t, []string{"search", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 1, metrics.countNudgesContaining("unavailable tool"))
		require.False(t, metrics.hasToolStart("web_lookup"))
	})

	t.Run("tool error recovery", func(t *testing.T) {
		flaky := &forgeScenarioTool{
			name: "flaky",
			results: []forgeScenarioToolResult{
				{err: errors.New("bad value")},
				{payload: map[string]any{"ok": true, "value": "good"}},
			},
		}
		registry := forgeScenarioRegistry(flaky)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"bad"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "flaky", Args: json.RawMessage(`{"value":"good"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"tool error recovered"}`)}}},
		}}

		final, metrics := runForgeScenario(t, provider, registry, harness.RunConfig{
			Mode:          harness.ModeWorkflow,
			MaxToolErrors: 2,
			Workflow: harness.WorkflowConfig{
				TerminalTools: []string{"agent_response"},
			},
		})

		require.Equal(t, "tool error recovered", final)
		require.Equal(t, []string{"flaky", "flaky", "agent_response"}, metrics.toolStarts)
		require.Equal(t, 1, metrics.countNudgesContaining("failed: bad value"))
		require.Equal(t, 1, metrics.countToolResultsContaining(`"error":"bad value"`))
		require.Len(t, flaky.calls, 2)
	})

	t.Run("compaction stress", func(t *testing.T) {
		search := &forgeScenarioTool{
			name: "search",
			results: []forgeScenarioToolResult{
				{payload: map[string]any{"ok": true, "blob": strings.Repeat("search-result ", 200)}},
			},
		}
		fetch := &forgeScenarioTool{
			name: "fetch",
			results: []forgeScenarioToolResult{
				{payload: map[string]any{"ok": true, "blob": strings.Repeat("fetch-result ", 200)}},
			},
		}
		summarize := &forgeScenarioTool{name: "summarize"}
		registry := forgeScenarioRegistry(search, fetch, summarize)
		provider := &harnessScriptedProvider{responses: []llm.Message{
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "search", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "fetch", Args: json.RawMessage(`{"url":"https://example.com/a"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "summarize", Args: json.RawMessage(`{"topic":"forge"}`)}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "agent_response", Args: json.RawMessage(`{"text":"compacted complete"}`)}}},
		}}

		final, metrics := runForgeScenarioWithEngine(t, provider, registry, harness.RunConfig{
			Mode: harness.ModeWorkflow,
			Workflow: harness.WorkflowConfig{
				RequiredSteps: []string{"search", "fetch", "summarize"},
				TerminalTools: []string{"agent_response"},
			},
			Compact: harness.CompactConfig{
				Enabled:         true,
				KeepRecentSteps: 1,
				PhaseThresholds: []float64{0.01, 0.02, 0.03},
			},
		}, func(eng *Engine) {
			eng.ContextWindowTokens = 120
			eng.SummaryMaxSummaryChunkTokens = 16
		})

		require.Equal(t, "compacted complete", final)
		require.True(t, metrics.providerCallContains("Harness compacted context"))
		require.True(t, metrics.providerCallContains("[COMPACTED tool result"))
		require.Equal(t, []string{"search", "fetch", "summarize", "agent_response"}, metrics.toolStarts)
	})
}

func forgeScenarioRegistry(scenarioTools ...*forgeScenarioTool) tools.Registry {
	registry := tools.NewRegistry()
	for _, tool := range scenarioTools {
		registry.Register(tool)
	}
	registry.Register(utility.NewAgentResponseTool())
	return registry
}

func runForgeScenario(t *testing.T, provider *harnessScriptedProvider, registry tools.Registry, cfg harness.RunConfig) (string, forgeScenarioMetrics) {
	t.Helper()
	return runForgeScenarioWithEngine(t, provider, registry, cfg, nil)
}

func runForgeScenarioWithEngine(t *testing.T, provider *harnessScriptedProvider, registry tools.Registry, cfg harness.RunConfig, configure func(*Engine)) (string, forgeScenarioMetrics) {
	t.Helper()
	metrics := forgeScenarioMetrics{}
	eng := &Engine{
		LLM:            provider,
		Tools:          registry,
		MaxSteps:       8,
		HarnessEnabled: true,
		HarnessConfig:  cfg,
		OnToolStart: func(toolName string, _ []byte, _ string) {
			metrics.toolStarts = append(metrics.toolStarts, toolName)
		},
		OnTool: func(_ string, _ []byte, result []byte, _ string) {
			metrics.toolResults = append(metrics.toolResults, string(result))
		},
		OnAssistant: func(message llm.Message) {
			metrics.assistants = append(metrics.assistants, message)
		},
		OnTurnMessage: func(message llm.Message) {
			metrics.turnMessages = append(metrics.turnMessages, message)
		},
	}
	if configure != nil {
		configure(eng)
	}

	final, err := eng.Run(context.Background(), "run forge scenario", nil)

	require.NoError(t, err)
	metrics.providerCalls = len(provider.calls)
	metrics.providerMsgs = provider.calls
	return final, metrics
}

func (m forgeScenarioMetrics) countNudgesContaining(substr string) int {
	count := 0
	for _, message := range m.turnMessages {
		if message.Role == "user" && strings.Contains(message.Content, substr) {
			count++
		}
	}
	return count
}

func (m forgeScenarioMetrics) countToolResultsContaining(substr string) int {
	count := 0
	for _, result := range m.toolResults {
		if strings.Contains(result, substr) {
			count++
		}
	}
	return count
}

func (m forgeScenarioMetrics) hasToolStart(name string) bool {
	for _, toolName := range m.toolStarts {
		if toolName == name {
			return true
		}
	}
	return false
}

func (m forgeScenarioMetrics) providerCallContains(substr string) bool {
	for _, call := range m.providerMsgs {
		if messagesContain(call, substr) {
			return true
		}
	}
	return false
}
