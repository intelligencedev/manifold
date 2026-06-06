package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	"manifold/internal/agent/memory"
	"manifold/internal/commandexec"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	terminaltool "manifold/internal/tools/terminal"
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

	runCtx := app.durableChatRunContext("run-1", prepared, nil)
	if inputrequest.RequesterFromContext(runCtx) == nil {
		t.Fatal("expected durable chat run context to include an input requester")
	}
	if commandexec.ApprovalControllerFromContext(runCtx) == nil {
		t.Fatal("expected durable chat run context to include command approval controller")
	}
}

func TestResumeOrAttachDurableChatRunReturnsWaitingTaskWithoutRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	spawn, err := client.Spawn(ctx, durable.SpawnRequest{
		Queue:  durableChatQueue,
		Name:   durableChatRunTaskName,
		UserID: 42,
	})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	task, run, ok, err := store.ClaimNext(ctx, []string{durableChatQueue}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim task ok=%v err=%v", ok, err)
	}
	if task.ID != spawn.TaskID {
		t.Fatalf("claimed task ID = %q, want %q", task.ID, spawn.TaskID)
	}
	if err := store.MarkTaskWaiting(ctx, task.ID, run.ID); err != nil {
		t.Fatalf("mark waiting: %v", err)
	}

	app := &app{durableStore: store, durableClient: client}
	resumed, retried, err := app.resumeOrAttachDurableChatRun(ctx, 42, spawn.TaskID)
	if err != nil {
		t.Fatalf("resume or attach: %v", err)
	}
	if retried {
		t.Fatal("expected waiting task to attach without retry")
	}
	if resumed.Status != durable.TaskStatusWaiting {
		t.Fatalf("resumed task = %+v, want original waiting task", resumed)
	}
}

func TestResumeOrAttachDurableChatRunRetriesFailedTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	spawn, err := client.Spawn(ctx, durable.SpawnRequest{
		Queue:  durableChatQueue,
		Name:   durableChatRunTaskName,
		UserID: 42,
	})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	task, run, ok, err := store.ClaimNext(ctx, []string{durableChatQueue}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim task ok=%v err=%v", ok, err)
	}
	if err := store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"blocked"}`), "blocked", time.Time{}); err != nil {
		t.Fatalf("fail task: %v", err)
	}

	app := &app{durableStore: store, durableClient: client}
	resumed, retried, err := app.resumeOrAttachDurableChatRun(ctx, 42, spawn.TaskID)
	if err != nil {
		t.Fatalf("resume or attach: %v", err)
	}
	if !retried {
		t.Fatal("expected failed task to retry")
	}
	if resumed.Status != durable.TaskStatusQueued || resumed.Error != "" {
		t.Fatalf("resumed task = %+v, want retried queued task", resumed)
	}
}

func TestDurableChatRunCommandApprovalAnswerResumesRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		args json.RawMessage
	}{
		{name: "run_cli", tool: "run_cli", args: json.RawMessage(`{"command":"date"}`)},
		{name: "terminal_start", tool: "terminal_start", args: json.RawMessage(`{"command":"date"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dir := t.TempDir()
			execCfg := config.ExecConfig{
				Sandbox: config.ExecSandboxConfig{
					Enabled:           commandPolicyTestBool(false),
					FailIfUnavailable: commandPolicyTestBool(true),
					Network:           config.ExecSandboxNetworkConfig{Enabled: commandPolicyTestBool(false)},
				},
			}
			executor := cli.NewExecutor(execCfg, dir, 0)
			terminalManager := terminaltool.NewManager(execCfg, dir)
			t.Cleanup(func() { _ = terminalManager.Close() })
			baseTools := tools.NewRegistry()
			baseTools.Register(cli.NewTool(executor))
			baseTools.Register(terminaltool.NewStartTool(terminalManager))
			provider := &agentRunScriptedProvider{streamResponses: []agentRunStreamResponse{
				{ToolCalls: []llm.ToolCall{{ID: "call-date", Name: tt.tool, Args: tt.args}}},
				{Deltas: []string{"done"}},
			}}
			chatStore := newPromptHandlerChatStore()
			store := durable.NewMemoryStore()
			client := durable.NewClient(store)
			registry := durable.NewRegistry()
			app := &app{
				cfg: &config.Config{
					Workdir:     dir,
					EnableTools: true,
					MaxSteps:    2,
					Exec:        execCfg,
					OpenAI:      config.OpenAIConfig{APIKey: "test", Model: "orchestrator-model"},
					LLMClient: config.LLMClientConfig{
						Provider: "openai",
						OpenAI:   config.OpenAIConfig{APIKey: "test", Model: "orchestrator-model"},
					},
				},
				llm:              provider,
				baseToolRegistry: baseTools,
				toolRegistry:     baseTools,
				specRegistry:     specialists.NewRegistry(config.LLMClientConfig{}, nil, nil, baseTools),
				engine: &agent.Engine{
					LLM:      provider,
					Tools:    baseTools,
					MaxSteps: 2,
					System:   "system",
					Model:    "orchestrator-model",
				},
				chatStore:       chatStore,
				chatMemory:      memory.NewManager(chatStore, provider, memory.Config{}),
				runs:            newRunStore(),
				durableStore:    store,
				durableClient:   client,
				durableRegistry: registry,
				cliExecutor:     executor,
				terminalManager: terminalManager,
			}
			app.registerDurableHandlers()
			worker := durable.NewWorker(store, client, registry, durable.WorkerOptions{WorkerID: "test", Lease: time.Minute, PollInterval: 10 * time.Millisecond})

			body := strings.NewReader(`{"prompt":"what time is it?","session_id":"durable-approval"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/chat/runs", body)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			app.chatRunsHandler().ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("start status = %d body=%s", rec.Code, rec.Body.String())
			}
			var start struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &start); err != nil {
				t.Fatalf("decode start: %v", err)
			}
			if start.RunID == "" {
				t.Fatalf("missing run_id in start response: %s", rec.Body.String())
			}

			if err := worker.RunOnce(ctx); err != nil {
				t.Fatalf("first worker run: %v", err)
			}
			events, status, found, err := client.ListEvents(ctx, systemUserID, start.RunID, 0)
			if err != nil || !found {
				t.Fatalf("list events found=%v err=%v", found, err)
			}
			if status != durable.TaskStatusWaiting {
				t.Fatalf("status = %s, want waiting", status)
			}
			var requestID string
			for _, event := range events {
				payload := durableChatStreamPayload(start.RunID, event)
				if payload["type"] == "input_request" {
					requestID, _ = payload["request_id"].(string)
					break
				}
			}
			if requestID == "" {
				t.Fatalf("input request not found in events: %+v", events)
			}

			answer := strings.NewReader(`{"choice_ids":["approve_once"],"answer":"Use this once."}`)
			answerReq := httptest.NewRequest(http.MethodPost, "/api/chat/runs/"+start.RunID+"/input/"+requestID+"/answer", answer)
			answerReq.Header.Set("Content-Type", "application/json")
			answerRec := httptest.NewRecorder()
			app.chatRunDetailHandler().ServeHTTP(answerRec, answerReq)
			if answerRec.Code != http.StatusOK {
				t.Fatalf("answer status = %d body=%s", answerRec.Code, answerRec.Body.String())
			}
			resumed, retried, err := app.resumeOrAttachDurableChatRun(ctx, systemUserID, start.RunID)
			if err != nil {
				t.Fatalf("resume or attach: %v", err)
			}
			if retried || resumed.Status != durable.TaskStatusQueued {
				t.Fatalf("resume result = %+v retried=%v, want queued attach", resumed, retried)
			}
			if err := worker.RunOnce(ctx); err != nil {
				t.Fatalf("second worker run: %v", err)
			}
			events, status, found, err = client.ListEvents(ctx, systemUserID, start.RunID, 0)
			if err != nil || !found {
				t.Fatalf("list final events found=%v err=%v", found, err)
			}
			if status != durable.TaskStatusCompleted {
				t.Fatalf("status = %s, want completed", status)
			}
			var final string
			for _, event := range events {
				payload := durableChatStreamPayload(start.RunID, event)
				if payload["type"] == "final" {
					final, _ = payload["data"].(string)
				}
			}
			if final != "done" {
				t.Fatalf("final = %q, want done; events=%+v", final, events)
			}
		})
	}
}
