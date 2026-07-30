package agent

import (
	"context"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
)

func TestRunHarnessLoopModeSelectsStreamExecution(t *testing.T) {
	t.Parallel()

	provider := &harnessScriptedProvider{streamResponses: []harnessStreamResponse{{Deltas: []string{"stream final"}}}}
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		HarnessEnabled: true,
	}

	final, err := eng.runHarnessLoopMode(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, true)
	if err != nil {
		t.Fatalf("runHarnessLoopMode: %v", err)
	}
	if final != "stream final" {
		t.Fatalf("final = %q, want %q", final, "stream final")
	}
	if len(provider.streamCalls) != 1 {
		t.Fatalf("stream calls = %d, want 1", len(provider.streamCalls))
	}
}
