package testhelpers

import (
	"context"
	"testing"

	"intelligence.dev/internal/llm"
)

type collectHandler struct {
	Deltas []string
}

func (c *collectHandler) OnDelta(s string)           { c.Deltas = append(c.Deltas, s) }
func (c *collectHandler) OnToolCall(tc llm.ToolCall) {}

func TestFakeProvider_Chat(t *testing.T) {
	fp := &FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ok"}}
	m, err := fp.Chat(context.Background(), nil, nil, "model")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Content != "ok" {
		t.Fatalf("unexpected content: %q", m.Content)
	}
}

func TestFakeProvider_ChatStream(t *testing.T) {
	fp := &FakeProvider{StreamDeltas: []string{"a", "b", "c"}}
	h := &collectHandler{}
	if err := fp.ChatStream(context.Background(), nil, nil, "m", h); err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if len(h.Deltas) != 3 {
		t.Fatalf("expected 3 deltas, got %d", len(h.Deltas))
	}
}
