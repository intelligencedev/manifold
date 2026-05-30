package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/durable"
)

func TestDurableTasksHandlerListsTasks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	if _, err := client.Spawn(ctx, durable.SpawnRequest{Queue: "ops", Name: "deploy"}); err != nil {
		t.Fatalf("spawn deploy: %v", err)
	}
	if _, err := client.Spawn(ctx, durable.SpawnRequest{Queue: "mail", Name: "digest"}); err != nil {
		t.Fatalf("spawn digest: %v", err)
	}
	app := &app{
		cfg:           &config.Config{},
		durableStore:  store,
		durableClient: client,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/durable/tasks?queue=ops&status=queued&limit=10", nil)
	rec := httptest.NewRecorder()
	newRouter(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Tasks []durable.Task `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Tasks) != 1 {
		t.Fatalf("tasks count = %d, want 1: %+v", len(payload.Tasks), payload.Tasks)
	}
	if payload.Tasks[0].Queue != "ops" || payload.Tasks[0].Name != "deploy" {
		t.Fatalf("task = %+v, want ops deploy", payload.Tasks[0])
	}
}

func TestDurableTaskRetryHandlerRequeuesFailedTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	spawn, err := client.Spawn(ctx, durable.SpawnRequest{Name: "work"})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	task, run, ok, err := store.ClaimNext(ctx, []string{durable.DefaultQueue}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim task ok=%v err=%v", ok, err)
	}
	if err := store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"failed"}`), "failed", time.Time{}); err != nil {
		t.Fatalf("fail task: %v", err)
	}
	app := &app{
		cfg:           &config.Config{},
		durableStore:  store,
		durableClient: client,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/durable/tasks/"+spawn.TaskID+"/retry", strings.NewReader(`{"reset_checkpoints":false}`))
	rec := httptest.NewRecorder()
	newRouter(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Task durable.Task `json:"task"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Task.Status != durable.TaskStatusQueued || payload.Task.Attempt != 0 || payload.Task.Error != "" {
		t.Fatalf("task after retry = %+v, want queued with cleared failure", payload.Task)
	}
}
