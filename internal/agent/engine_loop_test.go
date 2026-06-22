package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
)

type maxStepsToolProvider struct{}

func (p *maxStepsToolProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			Name: "lookup",
			Args: json.RawMessage(`{"query":"again"}`),
			ID:   "call_lookup",
		}},
	}, nil
}

func (p *maxStepsToolProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	h.OnToolCall(llm.ToolCall{
		Name: "lookup",
		Args: json.RawMessage(`{"query":"again"}`),
		ID:   "call_lookup",
	})
	return nil
}

type unboundedStepsProvider struct {
	calls int
}

func (p *unboundedStepsProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	p.calls++
	if p.calls <= 9 {
		return llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				Name: "lookup",
				Args: json.RawMessage(`{"query":"again"}`),
				ID:   "call_lookup",
			}},
		}, nil
	}
	return llm.Message{Role: "assistant", Content: "done"}, nil
}

func (p *unboundedStepsProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	p.calls++
	if p.calls <= 9 {
		h.OnToolCall(llm.ToolCall{
			Name: "lookup",
			Args: json.RawMessage(`{"query":"again"}`),
			ID:   "call_lookup",
		})
		return nil
	}
	h.OnDelta("done")
	return nil
}

func TestRunReturnsMaxStepsErrorInsteadOfFallbackText(t *testing.T) {
	t.Parallel()

	eng := &Engine{
		LLM:      &maxStepsToolProvider{},
		Tools:    tools.NewRegistry(),
		MaxSteps: 1,
	}

	final, err := eng.Run(context.Background(), "keep using tools", nil)

	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got final=%q err=%v", final, err)
	}
	if strings.Contains(final, "no final text") {
		t.Fatalf("fallback text leaked as final response: %q", final)
	}
}

func TestRunStreamReturnsMaxStepsErrorInsteadOfFallbackText(t *testing.T) {
	t.Parallel()

	eng := &Engine{
		LLM:      &maxStepsToolProvider{},
		Tools:    tools.NewRegistry(),
		MaxSteps: 1,
	}

	final, err := eng.RunStream(context.Background(), "keep using tools", nil)

	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got final=%q err=%v", final, err)
	}
	if strings.Contains(final, "no final text") {
		t.Fatalf("fallback text leaked as final response: %q", final)
	}
}

func TestRunZeroMaxStepsIsUnbounded(t *testing.T) {
	t.Parallel()

	provider := &unboundedStepsProvider{}
	eng := &Engine{
		LLM:      provider,
		Tools:    tools.NewRegistry(),
		MaxSteps: 0,
	}

	final, err := eng.Run(context.Background(), "keep using tools", nil)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if final != "done" {
		t.Fatalf("expected final response, got %q", final)
	}
	if provider.calls != 10 {
		t.Fatalf("expected 10 calls, got %d", provider.calls)
	}
}

func TestRunStreamZeroMaxStepsIsUnbounded(t *testing.T) {
	t.Parallel()

	provider := &unboundedStepsProvider{}
	eng := &Engine{
		LLM:      provider,
		Tools:    tools.NewRegistry(),
		MaxSteps: 0,
	}

	final, err := eng.RunStream(context.Background(), "keep using tools", nil)

	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if final != "done" {
		t.Fatalf("expected final response, got %q", final)
	}
	if provider.calls != 10 {
		t.Fatalf("expected 10 calls, got %d", provider.calls)
	}
}
