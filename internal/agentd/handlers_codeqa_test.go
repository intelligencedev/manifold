package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"manifold/internal/codeqa"
	codeqaservice "manifold/internal/codeqa/service"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools/cli"
)

type fakeCodeQAProvider struct{}

func (f fakeCodeQAProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	if strings.Contains(prompt, "OPTION_A = CANDIDATE") {
		return llm.Message{Role: "assistant", Content: `{"verdict":"option_a_better","confidence":0.92,"scores":{"correctness":0.4,"maintainability":0.4,"test_quality":0.2},"evidence":["foo/foo.go"]}`}, nil
	}
	return llm.Message{Role: "assistant", Content: `{"verdict":"option_b_better","confidence":0.92,"scores":{"correctness":-0.4,"maintainability":-0.4,"test_quality":-0.2},"evidence":["foo/foo.go"]}`}, nil
}

func (f fakeCodeQAProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

func TestCodeQARunsLifecycle(t *testing.T) {
	t.Parallel()

	repo := initCodeQATestRepo(t)
	setupCodeQARepo(t, repo)
	gitCodeQA(t, repo, "commit", "-am", "base")
	writeCodeQAFile(t, repo, "foo/foo.go", "package foo\n\n// Add keeps the contract stable.\nfunc Add(a, b int) int { return a + b }\n")
	gitCodeQA(t, repo, "commit", "-am", "head")

	executor := cli.NewExecutor(config.ExecConfig{MaxCommandSeconds: 60}, repo, 128*1024)
	runner := codeqa.NewCLICommandRunner(executor, []string{"go", "gofmt"})
	svc := codeqaservice.New(codeqa.Options{
		ArtifactDir:            t.TempDir(),
		DefaultMaxDiffBytes:    128 * 1024,
		DefaultMaxChangedFiles: 12,
		AcceptThreshold:        0.10,
		MinConfidence:          0.70,
		JudgeModel:             "fake",
		HighRiskGlobs:          []string{"**/auth/**"},
		Workdir:                repo,
	}, runner, fakeCodeQAProvider{}, nil)

	a := &app{
		cfg:           &config.Config{Workdir: repo},
		codeQAService: svc,
		codeQARuntime: newCodeQARuntime(),
	}

	body, _ := json.Marshal(codeqa.RunRequest{BaseRef: "HEAD~1", HeadRef: "HEAD", IncludeRepoContext: true})
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/codeqa/runs", bytes.NewReader(body))
	a.codeQARunsHandler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 create, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		RunID  string           `json:"run_id"`
		Status codeqa.RunStatus `json:"status"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.RunID == "" {
		t.Fatal("expected run id")
	}
	if created.Status != codeqa.StatusQueued {
		t.Fatalf("expected queued create status, got %s", created.Status)
	}

	var detail codeqa.RunResult
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		detailRec := httptest.NewRecorder()
		detailReq := httptest.NewRequest(http.MethodGet, "/api/codeqa/runs/"+created.RunID, nil)
		a.codeQARunDetailHandler().ServeHTTP(detailRec, detailReq)
		if detailRec.Code == http.StatusOK {
			if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
				t.Fatalf("unmarshal detail response: %v", err)
			}
			if detail.Status == codeqa.StatusCompleted || detail.Status == codeqa.StatusFailed {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if detail.RunID == "" {
		t.Fatal("run detail was never returned")
	}
	if detail.Status != codeqa.StatusCompleted {
		t.Fatalf("expected completed status, got %s (%s)", detail.Status, detail.Error)
	}
	if detail.Aggregate.Action != codeqa.ActionAccept {
		t.Fatalf("expected accept action, got %s aggregate=%+v gates=%+v judges=%+v", detail.Aggregate.Action, detail.Aggregate, detail.Gates, detail.Judges)
	}

	eventsRec := httptest.NewRecorder()
	eventsReq := httptest.NewRequest(http.MethodGet, "/api/codeqa/runs/"+created.RunID+"/events", nil)
	a.codeQARunDetailHandler().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 events, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var eventsResp struct {
		Status string            `json:"status"`
		Events []codeqa.RunEvent `json:"events"`
	}
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsResp); err != nil {
		t.Fatalf("unmarshal events response: %v", err)
	}
	if eventsResp.Status != string(codeqa.StatusCompleted) {
		t.Fatalf("expected completed events status, got %s", eventsResp.Status)
	}
	if len(eventsResp.Events) < 4 {
		t.Fatalf("expected multiple events, got %+v", eventsResp.Events)
	}

	sseRec := httptest.NewRecorder()
	sseReq := httptest.NewRequest(http.MethodGet, "/api/codeqa/runs/"+created.RunID+"/events", nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	a.codeQARunDetailHandler().ServeHTTP(sseRec, sseReq)
	if sseRec.Code != http.StatusOK {
		t.Fatalf("expected 200 sse, got %d body=%s", sseRec.Code, sseRec.Body.String())
	}
	if !strings.Contains(sseRec.Body.String(), "run_completed") {
		t.Fatalf("expected run_completed in sse body, got %s", sseRec.Body.String())
	}
}

func initCodeQATestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitCodeQA(t, repo, "init")
	gitCodeQA(t, repo, "config", "user.email", "codeqa@example.com")
	gitCodeQA(t, repo, "config", "user.name", "CodeQA")
	return repo
}

func setupCodeQARepo(t *testing.T, repo string) {
	t.Helper()
	writeCodeQAFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	writeCodeQAFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int { return a + b }\n")
	writeCodeQAFile(t, repo, "foo/foo_test.go", "package foo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	gitCodeQA(t, repo, "add", ".")
}

func writeCodeQAFile(t *testing.T, repo string, rel string, content string) {
	t.Helper()
	fullPath := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitCodeQA(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}
