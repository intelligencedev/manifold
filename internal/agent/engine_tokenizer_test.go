package agent

import (
	"context"
	"testing"

	"manifold/internal/llm"
)

type fakeTokenizer struct{ countCalls int }

func (f *fakeTokenizer) CountTokens(ctx context.Context, text string) (int, error) {
	f.countCalls++
	return len(text), nil
}

func (f *fakeTokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	f.countCalls++
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
	}
	return total, nil
}

type fakeTokenizableProvider struct{ tok *fakeTokenizer }

func (f *fakeTokenizableProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{}, nil
}

func (f *fakeTokenizableProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func (f *fakeTokenizableProvider) Tokenizer(cache *llm.TokenCache) llm.Tokenizer { //nolint:revive
	return f.tok
}

func TestAttachTokenizerSetsProviderTokenizer(t *testing.T) {
	tok := &fakeTokenizer{}
	prov := &fakeTokenizableProvider{tok: tok}
	eng := &Engine{}

	eng.AttachTokenizer(prov, nil)

	if eng.Tokenizer != tok {
		t.Fatalf("expected tokenizer to be attached")
	}
	if !eng.TokenizationFallbackToHeuristic {
		t.Fatalf("expected heuristic fallback to be enabled")
	}

	if count := eng.countTokens(context.Background(), "hi"); count != 2 {
		t.Fatalf("expected tokenizer count to be used, got %d", count)
	}
	if tok.countCalls == 0 {
		t.Fatalf("expected tokenizer to be invoked")
	}
}

func TestAttachTokenizerNoProviderNoop(t *testing.T) {
	eng := &Engine{}
	eng.AttachTokenizer(nil, nil)
	if eng.Tokenizer != nil {
		t.Fatalf("expected no tokenizer to be attached")
	}
}
