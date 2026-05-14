package memory

import (
	"context"
	"strings"
	"sync"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm"
)

type recordingLLMProvider struct {
	mu        sync.Mutex
	responses []string
	messages  [][]llm.Message
	tools     [][]llm.ToolSchema
}

func (p *recordingLLMProvider) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, _ string) (llm.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.messages = append(p.messages, append([]llm.Message(nil), messages...))
	p.tools = append(p.tools, append([]llm.ToolSchema(nil), tools...))
	response := ""
	if len(p.responses) > 0 {
		response = p.responses[0]
		p.responses = p.responses[1:]
	}
	return llm.Message{Role: "assistant", Content: response}, nil
}

func (p *recordingLLMProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func (p *recordingLLMProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}

func (p *recordingLLMProvider) toolCounts() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]int, 0, len(p.tools))
	for _, tools := range p.tools {
		out = append(out, len(tools))
	}
	return out
}

func (p *recordingLLMProvider) lastUserMessage() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		return ""
	}
	last := p.messages[len(p.messages)-1]
	for i := len(last) - 1; i >= 0; i-- {
		if last[i].Role == "user" {
			return last[i].Content
		}
	}
	return ""
}

func (p *recordingLLMProvider) lastSystemMessage() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		return ""
	}
	last := p.messages[len(p.messages)-1]
	for i := len(last) - 1; i >= 0; i-- {
		if last[i].Role == "system" {
			return last[i].Content
		}
	}
	return ""
}

func newTestReMemMemory(t *testing.T) *EvolvingMemory {
	t.Helper()
	return NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{},
		EmbedFn:         testEmbedFn,
		LLM:             &mockLLMProvider{response: "lesson"},
		Model:           "test-model",
	})
}

func TestParseReMemResponseStripsJSONFence(t *testing.T) {
	t.Parallel()

	resp, err := parseReMemResponse("```json\n{\"action\":\"ACT\",\"content\":\"done\"}\n```")
	if err != nil {
		t.Fatalf("parseReMemResponse failed: %v", err)
	}
	if resp.Action != ActionAct || resp.Content != "done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestParseReMemResponseRepairsProseWrappedJSON(t *testing.T) {
	t.Parallel()

	resp, err := parseReMemResponse("Here is the JSON:\n{\"action\":\"THINK\",\"content\":\"inspect state\"}\nThanks")
	if err != nil {
		t.Fatalf("parseReMemResponse failed: %v", err)
	}
	if resp.Action != ActionThink || resp.Content != "inspect state" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestReMemExecuteForcesFinalActAfterMaxSteps(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{
		`{"action":"THINK","content":"need more context"}`,
		`{"action":"ACT","content":"final answer"}`,
	}}
	rc := NewReMemController(ReMemConfig{
		Memory:        newTestReMemMemory(t),
		LLM:           provider,
		Model:         "test-model",
		MaxInnerSteps: 1,
	})

	final, trace, err := rc.Execute(context.Background(), "answer the task", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if final != "final answer" {
		t.Fatalf("expected forced final answer, got %q", final)
	}
	if len(trace) != 1 || trace[0] != "need more context" {
		t.Fatalf("expected one THINK trace, got %#v", trace)
	}
	if provider.callCount() != 2 {
		t.Fatalf("expected initial THINK plus forced ACT calls, got %d", provider.callCount())
	}
	if !strings.Contains(provider.lastUserMessage(), "hand off to the main agent now") {
		t.Fatalf("expected forced ACT prompt, got %q", provider.lastUserMessage())
	}
}

func TestReMemExecuteDoesNotForwardTools(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{
		`{"action":"THINK","content":"inspect memory only"}`,
		`{"action":"ACT","content":"handoff"}`,
	}}
	rc := NewReMemController(ReMemConfig{
		Memory:        newTestReMemMemory(t),
		LLM:           provider,
		Model:         "test-model",
		MaxInnerSteps: 1,
	})

	_, _, err := rc.Execute(context.Background(), "answer", []llm.ToolSchema{{
		Name:        "run_cli",
		Description: "Execute shell commands",
		Parameters:  map[string]any{"type": "object"},
	}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	for i, count := range provider.toolCounts() {
		if count != 0 {
			t.Fatalf("call %d forwarded %d tools", i, count)
		}
	}
}

func TestReMemSystemPromptMatchesMemoryPreparationContract(t *testing.T) {
	t.Parallel()

	prompt := reMemSystemPrompt()
	for _, want := range []string{
		"before the main agent answers the user",
		"do not include hidden chain-of-thought",
		"This does not answer the user directly",
		"only for IDs that appear in the retrieved memories",
		"Include a short reason for every memory edit",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected ReMem prompt to contain %q, got %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "private reasoning") || strings.Contains(prompt, "provide the final answer") {
		t.Fatalf("ReMem prompt still uses stale answer/private-reasoning wording: %s", prompt)
	}
}

func TestReMemExecuteParsesFencedJSON(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{"```json\n{\"action\":\"ACT\",\"content\":\"fenced final\"}\n```"}}
	rc := NewReMemController(ReMemConfig{
		Memory:        newTestReMemMemory(t),
		LLM:           provider,
		Model:         "test-model",
		MaxInnerSteps: 1,
	})

	final, _, err := rc.Execute(context.Background(), "answer", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if final != "fenced final" {
		t.Fatalf("expected fenced final, got %q", final)
	}
}

func TestStoreExperienceSkipsStrategyCardForTrivialTrace(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{"strategy card should not be used"}}
	memory := newTestReMemMemory(t)
	rc := NewReMemController(ReMemConfig{
		Memory:                   memory,
		LLM:                      provider,
		Model:                    "test-model",
		MinTraceStepsForStrategy: 3,
	})

	if err := rc.StoreExperienceEnhanced(context.Background(), "task", "output", "success", nil, []string{"one step"}); err != nil {
		t.Fatalf("StoreExperienceEnhanced failed: %v", err)
	}
	if provider.callCount() != 0 {
		t.Fatalf("expected strategy-card LLM call to be skipped, got %d calls", provider.callCount())
	}
	entries := memory.ExportMemories()
	if len(entries) != 1 {
		t.Fatalf("expected one stored memory, got %d", len(entries))
	}
	if entries[0].StrategyCard != "" {
		t.Fatalf("expected no strategy card for trivial trace, got %q", entries[0].StrategyCard)
	}
}

func TestStoreExperienceGeneratesStrategyCardForLongTrace(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{"When confronted with long traces, preserve the reusable pattern."}}
	memory := newTestReMemMemory(t)
	rc := NewReMemController(ReMemConfig{
		Memory:                   memory,
		LLM:                      provider,
		Model:                    "test-model",
		MinTraceStepsForStrategy: 3,
	})

	trace := []string{"inspect", "modify", "verify"}
	if err := rc.StoreExperienceEnhanced(context.Background(), "task", "output", "success", nil, trace); err != nil {
		t.Fatalf("StoreExperienceEnhanced failed: %v", err)
	}
	if provider.callCount() != 1 {
		t.Fatalf("expected one strategy-card LLM call, got %d", provider.callCount())
	}
	entries := memory.ExportMemories()
	if len(entries) != 1 {
		t.Fatalf("expected one stored memory, got %d", len(entries))
	}
	if entries[0].StrategyCard == "" {
		t.Fatal("expected strategy card for long trace")
	}

	systemPrompt := provider.lastSystemMessage()
	for _, want := range []string{
		"For a successful outcome",
		"For a failed or partial outcome",
		"Do not include secrets",
		"Return only the strategy card",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected strategy-card prompt to contain %q, got %q", want, systemPrompt)
		}
	}
}
