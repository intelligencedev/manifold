package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
)

type mapRunCheckpointer struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapRunCheckpointer() *mapRunCheckpointer {
	return &mapRunCheckpointer{data: map[string][]byte{}}
}

func (c *mapRunCheckpointer) Load(_ context.Context, key string, target any) (bool, error) {
	c.mu.Lock()
	raw, ok := c.data[key]
	c.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(raw, target)
}

func (c *mapRunCheckpointer) Save(_ context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.data[key] = raw
	c.mu.Unlock()
	return nil
}

type checkpointReplayProvider struct {
	calls int
}

func (p *checkpointReplayProvider) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	p.calls++
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != "tool" {
		return llm.Message{}, errors.New("expected replayed tool result before model call")
	}
	return llm.Message{Role: "assistant", Content: "done"}, nil
}

func (p *checkpointReplayProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return errors.New("unexpected stream call")
}

type countingTool struct {
	calls *int
}

func (t countingTool) Name() string { return "lookup" }

func (t countingTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        "lookup",
		"description": "lookup",
		"parameters":  map[string]any{"type": "object"},
	}
}

func (t countingTool) Call(context.Context, json.RawMessage) (any, error) {
	(*t.calls)++
	return map[string]any{"ok": true}, nil
}

func TestRunReplaysAssistantAndToolCheckpointsWithoutDispatchingTool(t *testing.T) {
	t.Parallel()

	cp := newMapRunCheckpointer()
	assistant := llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "call-lookup", Name: "lookup", Args: json.RawMessage(`{"query":"x"}`)}},
	}
	if err := cp.Save(context.Background(), assistantCheckpointKey(0), assistant); err != nil {
		t.Fatalf("save assistant checkpoint: %v", err)
	}
	if err := cp.Save(context.Background(), toolCheckpointKey(0, "call-lookup"), llm.Message{Role: "tool", ToolID: "call-lookup", Content: `{"ok":true}`}); err != nil {
		t.Fatalf("save tool checkpoint: %v", err)
	}

	toolCalls := 0
	reg := tools.NewRegistry()
	reg.Register(countingTool{calls: &toolCalls})
	provider := &checkpointReplayProvider{}
	eng := &Engine{
		LLM:          provider,
		Tools:        reg,
		MaxSteps:     3,
		Checkpointer: cp,
	}

	final, err := eng.Run(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if final != "done" {
		t.Fatalf("final = %q, want done", final)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if toolCalls != 0 {
		t.Fatalf("tool dispatched %d times, want 0", toolCalls)
	}
}
