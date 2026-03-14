package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/specialists"
)

func TestDispatchBuiltChatTargetMapsNotFoundBuildError(t *testing.T) {
	t.Parallel()

	a := &app{}
	req := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	rec := httptest.NewRecorder()

	handled := a.dispatchBuiltChatTarget(rec, req, chatTargetDispatchOptions{
		Prompt:    "hello",
		SessionID: "default",
		Build: func(context.Context) chatEngineBuildResult {
			return chatEngineBuildResult{StatusCode: http.StatusNotFound, Err: context.Canceled}
		},
		NotFoundMessage:      "specialist not found",
		InternalErrorMessage: "specialist registry unavailable",
	})

	if !handled {
		t.Fatal("expected request to be handled")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "specialist not found\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestDispatchBuiltChatTargetMapsInternalBuildError(t *testing.T) {
	t.Parallel()

	a := &app{}
	req := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	rec := httptest.NewRecorder()

	handled := a.dispatchBuiltChatTarget(rec, req, chatTargetDispatchOptions{
		Prompt:    "hello",
		SessionID: "default",
		Build: func(context.Context) chatEngineBuildResult {
			return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: context.Canceled}
		},
		NotFoundMessage:      "team not found",
		InternalErrorMessage: "failed to load team",
	})

	if !handled {
		t.Fatal("expected request to be handled")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "failed to load team\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestDescribeChatTargetPrefersSpecialistOverTeam(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor, ok := a.describeChatTarget(chatDispatchTarget{SpecialistName: "weather", TeamName: "ops"}, "override", 7)
	if !ok {
		t.Fatal("expected target descriptor")
	}
	if descriptor.NotFoundMessage != "specialist not found" {
		t.Fatalf("unexpected not found message: %q", descriptor.NotFoundMessage)
	}
	if !descriptor.Stream.IncludeMatrixMessages {
		t.Fatal("expected specialist stream to include matrix messages")
	}
	if !descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected specialist JSON to include matrix messages")
	}
	if descriptor.Build == nil {
		t.Fatal("expected build function")
	}
}

func TestDescribeChatTargetSkipsOrchestratorSpecialistForTeam(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor, ok := a.describeChatTarget(chatDispatchTarget{SpecialistName: specialists.OrchestratorName, TeamName: "ops"}, "", 7)
	if !ok {
		t.Fatal("expected team target descriptor")
	}
	if descriptor.NotFoundMessage != "team not found" {
		t.Fatalf("unexpected not found message: %q", descriptor.NotFoundMessage)
	}
	if descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected team JSON matrix messages to remain disabled")
	}
}

func TestAgentRunOrchestratorDescriptorIncludesInitialSummary(t *testing.T) {
	t.Parallel()

	a := &app{}
	summary := &agentmemory.SummaryResult{Triggered: true, EstimatedTokens: 42}
	descriptor := a.agentRunOrchestratorDescriptor(context.Background(), 7, chatRunRequest{Prompt: "hello", SessionID: "sess-1"}, nil, summary)
	if descriptor.InternalErrorMessage != "agent unavailable" {
		t.Fatalf("unexpected internal error message: %q", descriptor.InternalErrorMessage)
	}
	if descriptor.Stream.InitialSummary != summary {
		t.Fatal("expected initial summary to be preserved")
	}
	if descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected agent run orchestrator JSON to omit matrix messages")
	}
}

func TestPromptOrchestratorDescriptorIncludesMatrixMessages(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor := a.promptOrchestratorDescriptor(context.Background(), 7, chatRunRequest{Prompt: "hello", SessionID: "sess-1", SystemPrompt: "override"}, nil)
	if !descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected prompt orchestrator JSON to include matrix messages")
	}
	if descriptor.Stream.StructuredErrors {
		t.Fatal("expected prompt orchestrator stream errors to remain unstructured")
	}
	if descriptor.RunContext == nil {
		t.Fatal("expected prompt orchestrator run context")
	}
	_ = llm.WithUserID(context.Background(), 1)
}
