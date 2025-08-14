package llmtool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"singularityio/internal/llm"
)

type fakeProvider struct {
	resp   llm.Message
	err    error
	called bool
}

func (f *fakeProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	f.called = true
	return f.resp, f.err
}
func (f *fakeProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return errors.New("not implemented")
}

func TestTransform_Call_Success(t *testing.T) {
	fp := &fakeProvider{resp: llm.Message{Content: "ok output"}}
	tr := NewTransform(fp, "mymodel", nil)
	in := map[string]string{"instruction": "summarize", "input": "hello world"}
	raw, _ := json.Marshal(in)
	ctx := context.Background()
	res, err := tr.Call(ctx, raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	if m["ok"] != true {
		t.Fatalf("expected ok true, got %v", m["ok"])
	}
	if m["output"] != "ok output" {
		t.Fatalf("unexpected output: %v", m["output"])
	}
	if !fp.called {
		t.Fatalf("provider not called")
	}
}

func TestTransform_Call_ErrorFromProvider(t *testing.T) {
	fp := &fakeProvider{err: errors.New("boom")}
	tr := NewTransform(fp, "mymodel", nil)
	in := map[string]string{"instruction": "summarize"}
	raw, _ := json.Marshal(in)
	ctx := context.Background()
	res, err := tr.Call(ctx, raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	if m["ok"] != false {
		t.Fatalf("expected ok false, got %v", m["ok"])
	}
	if _, ok := m["error"]; !ok {
		t.Fatalf("expected error field")
	}
}

func TestTransform_Call_UsesNewWithBaseURL(t *testing.T) {
	created := false
	factory := func(baseURL string) llm.Provider {
		created = true
		return &fakeProvider{resp: llm.Message{Content: "from new"}}
	}
	orig := &fakeProvider{resp: llm.Message{Content: "orig"}}
	tr := NewTransform(orig, "mymodel", factory)
	in := map[string]string{"instruction": "do", "base_url": "https://api.x"}
	raw, _ := json.Marshal(in)
	ctx := context.Background()
	res, err := tr.Call(ctx, raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m := res.(map[string]any)
	if m["output"] != "from new" {
		t.Fatalf("expected from new, got %v", m["output"])
	}
	if !created {
		t.Fatalf("factory not used")
	}
}
