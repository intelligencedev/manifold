package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

type delegatorMemoryProvider struct {
	chatResponse   string
	streamResponse string
}

func (p *delegatorMemoryProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: p.chatResponse}, nil
}

func (p *delegatorMemoryProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	handler.OnDelta(p.streamResponse)
	return nil
}

type delegatorRecordingStore struct {
	saveCh chan []*memory.MemoryEntry
}

func (s *delegatorRecordingStore) Load(context.Context, int64, string) ([]*memory.MemoryEntry, error) {
	return nil, nil
}

func (s *delegatorRecordingStore) Save(_ context.Context, _ int64, _ string, entries []*memory.MemoryEntry) error {
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

func delegatorTestEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
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

func TestDelegatorRunUsesSharedEvolvingMemory(t *testing.T) {
	t.Parallel()
	const prompt = "remember this important detail"

	provider := &delegatorMemoryProvider{chatResponse: "summary", streamResponse: "delegated final"}
	store := &delegatorRecordingStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   delegatorTestEmbedFn,
		LLM:       provider,
		Model:     "test-model",
		Store:     store,
		UserID:    7,
		SessionID: "sess-1",
	})

	d := NewDelegator(tools.NewRegistry(), nil, nil, 1)
	d.SetEvolvingMemory(em)
	ctx := tools.WithProvider(context.Background(), provider)

	out, err := d.Run(ctx, agent.DelegateRequest{
		Prompt:    prompt,
		UserID:    7,
		SessionID: "sess-1",
	}, nil)
	if err != nil {
		t.Fatalf("delegator run failed: %v", err)
	}
	if out != "delegated final" {
		t.Fatalf("expected delegated final output, got %q", out)
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved memory, got %d", len(saved))
		}
		if saved[0] == nil {
			t.Fatal("expected saved memory entry")
		}
		if saved[0].Input != prompt {
			t.Fatalf("expected saved input %q, got %q", prompt, saved[0].Input)
		}
		if saved[0].Output != "delegated final" {
			t.Fatalf("expected saved output delegated final, got %q", saved[0].Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delegated evolving memory save")
	}
}

func TestDelegatorRunImageGenerationSpecialistSendsOnlyPrompt(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/images/generations" {
			http.Error(w, "unexpected chat request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"cG5nYnl0ZXM="}]}`))
	}))
	defer server.Close()

	llmConfig := config.LLMClientConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKey:  "test",
			BaseURL: server.URL,
			Model:   "fallback-model",
		},
	}
	registry := specialists.NewRegistry(llmConfig, []config.SpecialistConfig{{
		Name:            "image-maker",
		Provider:        "openai",
		BaseURL:         server.URL,
		APIKey:          "test",
		Model:           "gpt-image-2",
		System:          "Never send this system prompt to image generation.",
		EnableTools:     true,
		ImageGeneration: true,
		ExtraParams:     map[string]any{"size": "2048x2048"},
	}}, server.Client(), tools.NewRegistry())

	d := NewDelegator(tools.NewRegistry(), registry, nil, 8)
	out, err := d.Run(context.Background(), agent.DelegateRequest{
		AgentName: "image-maker",
		Prompt:    "draw a mountain",
		History: []llm.Message{
			{Role: "user", Content: "previous prompt that must be ignored"},
			{Role: "assistant", Content: "previous answer that must be ignored"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("delegator run failed: %v", err)
	}
	if out != "Generated image" {
		t.Fatalf("expected generated image result, got %q", out)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("expected image generation endpoint, got %q", gotPath)
	}
	if prompt, _ := gotBody["prompt"].(string); prompt != "draw a mountain" {
		t.Fatalf("expected delegated prompt only, got %#v", gotBody["prompt"])
	}
	if size, _ := gotBody["size"].(string); size != "2048x2048" {
		t.Fatalf("expected configured delegated image size, got %#v", gotBody["size"])
	}
}

type delegatorTraceRecorder struct {
	events []agent.AgentTrace
}

func (r *delegatorTraceRecorder) Trace(ev agent.AgentTrace) {
	r.events = append(r.events, ev)
}

func TestDelegatorTracerUsesFriendlyToolTitles(t *testing.T) {
	d := NewDelegator(tools.NewRegistry(), nil, nil, 1)
	eng := &agent.Engine{}
	recorder := &delegatorTraceRecorder{}
	d.attachTracer(eng, recorder, agent.DelegateRequest{AgentName: "developer", CallID: "call-1"}, "test-model")

	if eng.OnToolStartWithTitle == nil {
		t.Fatal("expected title-aware tool start tracer")
	}
	eng.OnToolStartWithTitle("run_cli", "", []byte(`{"command":"go"}`), "tool-1")

	if len(recorder.events) != 1 {
		t.Fatalf("expected one trace event, got %d", len(recorder.events))
	}
	ev := recorder.events[0]
	if ev.Title != "Running a workspace command safely" {
		t.Fatalf("trace title = %q, want %q", ev.Title, "Running a workspace command safely")
	}
	if ev.ToolName != "run_cli" {
		t.Fatalf("trace tool name = %q, want %q", ev.ToolName, "run_cli")
	}
	if ev.ToolTitle != "Running a workspace command safely" {
		t.Fatalf("trace tool title = %q, want %q", ev.ToolTitle, "Running a workspace command safely")
	}
}
