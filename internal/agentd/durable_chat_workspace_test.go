package agentd

import (
	"context"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

func TestPrepareDurableChatRunAttachesCheckedOutWorkspaceContext(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	toolReg := tools.NewRegistry()
	provider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ok"}}
	chatStore := newPromptHandlerChatStore()
	app := &app{
		cfg: &config.Config{
			Workdir:     t.TempDir(),
			EnableTools: true,
			MaxSteps:    2,
		},
		llm:              provider,
		baseToolRegistry: toolReg,
		specRegistry:     specialists.NewRegistry(config.LLMClientConfig{}, nil, nil, toolReg),
		engine: &agent.Engine{
			LLM:      provider,
			Tools:    toolReg,
			MaxSteps: 2,
			System:   "system",
			Model:    "test-model",
		},
		chatStore:  chatStore,
		chatMemory: memory.NewManager(chatStore, provider, memory.Config{}),
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			return workspaces.Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, BaseDir: projectDir}, nil
		}},
	}

	prepared, err := app.prepareDurableChatRun(context.Background(), durable.Task{ID: "run-1", UserID: 42}, durableChatTaskParams{
		Request: chatRunRequest{
			Prompt:    "list files",
			SessionID: "session-1",
			ProjectID: "project-1",
		},
		Endpoint: "/agent/run",
		Owner:    42,
	})
	if err != nil {
		t.Fatalf("prepareDurableChatRun: %v", err)
	}

	if got, ok := sandbox.BaseDirFromContext(prepared.exec.RunContext); !ok || got != projectDir {
		t.Fatalf("base dir = %q ok=%v, want checked-out project workspace %q", got, ok, projectDir)
	}
	if got, ok := sandbox.ProjectIDFromContext(prepared.exec.RunContext); !ok || got != "project-1" {
		t.Fatalf("project id = %q ok=%v, want project-1", got, ok)
	}
}
