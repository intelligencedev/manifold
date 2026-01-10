package llm

import (
	"context"
	"testing"
	"time"
)

// fake handler implementing StreamHandler for testing streaming callbacks
type fakeHandler struct {
	deltas []string
	calls  []ToolCall
	images []GeneratedImage
}

func (f *fakeHandler) OnDelta(content string) { f.deltas = append(f.deltas, content) }
func (f *fakeHandler) OnToolCall(tc ToolCall) { f.calls = append(f.calls, tc) }
func (f *fakeHandler) OnImage(img GeneratedImage) {
	f.images = append(f.images, img)
}
func (f *fakeHandler) OnThoughtSummary(string) {}

// fake provider implementing Provider interface
type fakeProvider struct {
	resp Message
	err  error
	// for stream
	streamDeltas []string
}

func (f *fakeProvider) Chat(ctx context.Context, msgs []Message, tools []ToolSchema, model string) (Message, error) {
	// simple echo behavior: return last user message as assistant reply
	if f.err != nil {
		return Message{}, f.err
	}
	if len(msgs) == 0 {
		return f.resp, nil
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return Message{Role: "assistant", Content: msgs[i].Content}, nil
		}
	}
	return f.resp, nil
}

func (f *fakeProvider) ChatStream(ctx context.Context, msgs []Message, tools []ToolSchema, model string, h StreamHandler) error {
	if f.err != nil {
		return f.err
	}
	for _, d := range f.streamDeltas {
		h.OnDelta(d)
		// simulate slight delay
		time.Sleep(1 * time.Millisecond)
	}
	// simulate a tool call
	h.OnToolCall(ToolCall{Name: "fn", Args: nil, ID: "1"})
	return nil
}

func TestFakeProviderChat(t *testing.T) {
	p := &fakeProvider{resp: Message{Role: "assistant", Content: "ok"}}
	msg, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", msg.Role)
	}
	if msg.Content != "hello" {
		t.Fatalf("expected echo content 'hello', got %q", msg.Content)
	}
}

func TestFakeProviderStream(t *testing.T) {
	p := &fakeProvider{streamDeltas: []string{"a", "b", "c"}}
	h := &fakeHandler{}
	if err := p.ChatStream(context.Background(), nil, nil, "", h); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(h.deltas) != 3 {
		t.Fatalf("expected 3 deltas got %d", len(h.deltas))
	}
	if len(h.calls) != 1 {
		t.Fatalf("expected 1 tool call got %d", len(h.calls))
	}
}
