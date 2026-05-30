package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/worker"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

func TestPlaygroundSpecialistRunnerUsesToolsAndIsolatedProjectContext(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		requests []map[string]any
	)
	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode specialist request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		mu.Lock()
		requests = append(requests, payload)
		call := len(requests)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_context","arguments":"{\"q\":\"base\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"final answer","tool_calls":[]}}]}`))
	}))
	defer specialistServer.Close()

	workdir := t.TempDir()
	projectDir := filepath.Join(workdir, "users", "0", "projects", "proj-1")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	capture := &playgroundCaptureTool{}
	baseTools := tools.NewRegistry()
	baseTools.Register(capture)

	cfg := &config.Config{
		Workdir:                workdir,
		EnableTools:            true,
		MaxSteps:               3,
		AgentRunTimeoutSeconds: 5,
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:  "test",
				BaseURL: specialistServer.URL,
				Model:   "special-model",
			},
		},
		OpenAI: config.OpenAIConfig{
			APIKey:  "test",
			BaseURL: specialistServer.URL,
			Model:   "special-model",
		},
	}
	specs := []config.SpecialistConfig{{
		Name:        "runner",
		Provider:    "openai",
		BaseURL:     specialistServer.URL,
		APIKey:      "test",
		Model:       "special-model",
		EnableTools: true,
		System:      "Use tools when useful.",
	}}
	specStore := databases.NewSpecialistsStore(nil)
	_, err := specStore.Upsert(context.Background(), systemUserID, persistence.Specialist{
		Name:        "runner",
		Provider:    "openai",
		BaseURL:     specialistServer.URL,
		APIKey:      "test",
		Model:       "special-model",
		EnableTools: true,
	})
	require.NoError(t, err)

	specRegistry := specialists.NewRegistryWithWorkdir(cfg.LLMClient, specs, specialistServer.Client(), baseTools, workdir)
	app := &app{
		cfg:              cfg,
		httpClient:       specialistServer.Client(),
		baseToolRegistry: baseTools,
		specRegistry:     specRegistry,
		userSpecRegs:     map[int64]*specialists.Registry{systemUserID: specRegistry},
		specStore:        specStore,
		workspaceManager: workspaces.NewManager(cfg),
	}
	runner := newPlaygroundSpecialistRunner(app, nil)

	resp, err := runner.RunTask(context.Background(), worker.Task{
		RunID:     "run-1",
		ShardID:   "shard-1",
		ProjectID: "proj-1",
		OwnerID:   systemUserID,
		Variant: experiment.Variant{
			ID: "variant-1",
		},
		Row: dataset.Row{
			ID:     "row-1",
			Inputs: map[string]any{"name": "Ada"},
		},
		Execution: &experiment.ExecutionConfig{
			SpecialistName: "runner",
		},
	}, "Look up the project context")
	require.NoError(t, err)
	require.Equal(t, "final answer", resp.Output)
	require.Equal(t, "specialist:runner", resp.ProviderName)
	require.Equal(t, "special-model", resp.Model)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 2)
	require.Contains(t, toolNamesFromRequest(requests[0]), "lookup_context")
	require.Equal(t, []string{"playground:run-1:row-1:variant-1"}, capture.sessions)
	require.Equal(t, []string{projectDir}, capture.baseDirs)
}

type playgroundCaptureTool struct {
	sessions []string
	baseDirs []string
}

func (t *playgroundCaptureTool) Name() string { return "lookup_context" }

func (t *playgroundCaptureTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "capture playground context",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
		},
	}
}

func (t *playgroundCaptureTool) Call(ctx context.Context, _ json.RawMessage) (any, error) {
	sessionID, _ := sandbox.SessionIDFromContext(ctx)
	baseDir, _ := sandbox.BaseDirFromContext(ctx)
	t.sessions = append(t.sessions, sessionID)
	t.baseDirs = append(t.baseDirs, baseDir)
	return map[string]any{"session": sessionID, "baseDir": baseDir}, nil
}

func toolNamesFromRequest(payload map[string]any) []string {
	rawTools, _ := payload["tools"].([]any)
	names := make([]string, 0, len(rawTools))
	for _, rawTool := range rawTools {
		tool, _ := rawTool.(map[string]any)
		if fn, _ := tool["function"].(map[string]any); fn != nil {
			if name, _ := fn["name"].(string); strings.TrimSpace(name) != "" {
				names = append(names, name)
			}
			continue
		}
		if name, _ := tool["name"].(string); strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return names
}
