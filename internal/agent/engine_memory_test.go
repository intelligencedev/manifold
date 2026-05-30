package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
)

type memoryTestProvider struct {
	chatResponse   string
	streamResponse string
}

func (p *memoryTestProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: p.chatResponse}, nil
}

func (p *memoryTestProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	handler.OnDelta(p.streamResponse)
	return nil
}

type recordingMemoryStore struct {
	saveCh chan []*memory.MemoryEntry
}

func (s *recordingMemoryStore) Load(context.Context, int64, string) ([]*memory.MemoryEntry, error) {
	return nil, nil
}

func (s *recordingMemoryStore) Save(_ context.Context, _ int64, _ string, entries []*memory.MemoryEntry) error {
	snapshot := make([]*memory.MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			snapshot = append(snapshot, nil)
			continue
		}
		copyEntry := *entry
		snapshot = append(snapshot, &copyEntry)
	}
	s.saveCh <- snapshot
	return nil
}

func engineTestEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, s := range texts {
		vec := make([]float32, 4)
		for j := 0; j < len(s); j++ {
			vec[j%len(vec)] += float32(s[j]) / 255.0
		}
		out[i] = vec
	}
	return out, nil
}

func TestRunStreamStoresEvolvingMemory(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{
		chatResponse:   "summary",
		streamResponse: "streamed final",
	}
	store := &recordingMemoryStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   engineTestEmbedFn,
		LLM:       provider,
		Model:     "test-model",
		Store:     store,
		UserID:    7,
		SessionID: "session-1",
	})

	eng := &Engine{
		LLM:            provider,
		MaxSteps:       1,
		EvolvingMemory: em,
		Tools:          tools.NewRegistry(),
	}

	final, err := eng.RunStream(context.Background(), "remember this", nil)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if final != "streamed final" {
		t.Fatalf("expected streamed final response, got %q", final)
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved memory, got %d", len(saved))
		}
		if saved[0] == nil {
			t.Fatalf("expected saved memory entry")
		}
		if saved[0].Input != "remember this" {
			t.Fatalf("expected saved input to match prompt, got %q", saved[0].Input)
		}
		if saved[0].Output != "streamed final" {
			t.Fatalf("expected saved output to match final response, got %q", saved[0].Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for evolving memory save after RunStream")
	}
}

func TestRunWithReMemStoresStructuredFeedback(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{
		chatResponse:   `{"action":"ACT","content":"ready"}`,
		streamResponse: "remem final",
	}
	store := &recordingMemoryStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   engineTestEmbedFn,
		LLM:       provider,
		Store:     store,
		UserID:    7,
		SessionID: "session-remem",
	})

	eng := &Engine{
		LLM:            provider,
		MaxSteps:       1,
		EvolvingMemory: em,
		Tools:          tools.NewRegistry(),
		ReMemEnabled:   true,
		ReMemController: memory.NewReMemController(memory.ReMemConfig{
			LLM:           provider,
			Memory:        em,
			MaxInnerSteps: 1,
		}),
	}

	final, err := eng.Run(context.Background(), "remember from remem", nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if final != "remem final" {
		t.Fatalf("expected remem final response, got %q", final)
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 || saved[0] == nil {
			t.Fatalf("expected one saved memory, got %#v", saved)
		}
		if saved[0].StructuredFeedback == nil || saved[0].StructuredFeedback.Type != memory.FeedbackSuccess || !saved[0].StructuredFeedback.Correct {
			t.Fatalf("expected structured success feedback, got %#v", saved[0].StructuredFeedback)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReMem evolving memory save")
	}
}

func TestDeriveMemoryFeedbackClassifiesFailuresAndPartialRuns(t *testing.T) {
	t.Parallel()

	feedback, structured := deriveMemoryFeedback("", errors.New("boom"))
	if feedback != string(memory.FeedbackFailure) || structured == nil || structured.Type != memory.FeedbackFailure || structured.Correct {
		t.Fatalf("expected failure feedback, got %q %#v", feedback, structured)
	}

	feedback, structured = deriveMemoryFeedback("(no final text — increase max steps or check logs)", nil)
	if feedback != string(memory.FeedbackPartial) || structured == nil || structured.Type != memory.FeedbackPartial || structured.Correct {
		t.Fatalf("expected partial feedback, got %q %#v", feedback, structured)
	}
}

type learningMemoryProvider struct{}

func (p *learningMemoryProvider) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: "When solving frobnicate tasks, answer with beta."}, nil
}

func (p *learningMemoryProvider) ChatStream(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	joined := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		joined = append(joined, msg.Content)
	}
	if strings.Contains(strings.Join(joined, "\n"), "answer with beta") {
		handler.OnDelta("beta")
		return nil
	}
	handler.OnDelta("alpha")
	return nil
}

func TestEvolvingMemoryImprovesSecondSimilarRun(t *testing.T) {
	t.Parallel()

	provider := &learningMemoryProvider{}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return [][]float32{{1, 0}}, nil
		},
		LLM:       provider,
		TopK:      1,
		EnableRAG: true,
	})
	eng := &Engine{
		LLM:            provider,
		MaxSteps:       1,
		EvolvingMemory: em,
		Tools:          tools.NewRegistry(),
	}

	first, err := eng.RunStream(context.Background(), "frobnicate task", nil)
	if err != nil {
		t.Fatalf("first RunStream failed: %v", err)
	}
	if first != "alpha" {
		t.Fatalf("expected first run without memory to return alpha, got %q", first)
	}
	deadline := time.After(2 * time.Second)
	for len(em.ExportMemories()) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first run memory")
		case <-time.After(10 * time.Millisecond):
		}
	}

	second, err := eng.RunStream(context.Background(), "similar frobnicate request", nil)
	if err != nil {
		t.Fatalf("second RunStream failed: %v", err)
	}
	if second != "beta" {
		t.Fatalf("expected second run to use memory and return beta, got %q", second)
	}
}
