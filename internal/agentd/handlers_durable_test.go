package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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

func TestDurableTaskEventsHandlerPaginatesEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := durable.NewMemoryStore()
	client := durable.NewClient(store)
	spawn, err := client.Spawn(ctx, durable.SpawnRequest{Queue: "ops", Name: "deploy"})
	if err != nil {
		t.Fatalf("spawn deploy: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := store.AppendTaskEvent(ctx, spawn.TaskID, "event."+strconv.Itoa(i), map[string]any{"index": i}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}
	app := &app{
		cfg:           &config.Config{},
		durableStore:  store,
		durableClient: client,
	}

	latest := requestDurableEventPage(t, app, "/api/durable/tasks/"+spawn.TaskID+"/events?limit=2")
	if got := eventSequences(latest.Events); !slices.Equal(got, []int64{4, 5}) {
		t.Fatalf("latest sequences = %+v, want [4 5]", got)
	}
	if !latest.HasMoreBefore || latest.HasMoreAfter || latest.FirstSequence != 4 || latest.LastSequence != 5 || latest.Limit != 2 {
		t.Fatalf("latest page metadata = %+v", latest)
	}

	older := requestDurableEventPage(t, app, "/api/durable/tasks/"+spawn.TaskID+"/events?before=4&limit=2")
	if got := eventSequences(older.Events); !slices.Equal(got, []int64{2, 3}) {
		t.Fatalf("older sequences = %+v, want [2 3]", got)
	}
	if !older.HasMoreBefore || !older.HasMoreAfter {
		t.Fatalf("older page metadata = %+v", older)
	}

	newer := requestDurableEventPage(t, app, "/api/durable/tasks/"+spawn.TaskID+"/events?after=3&limit=2")
	if got := eventSequences(newer.Events); !slices.Equal(got, []int64{4, 5}) {
		t.Fatalf("newer sequences = %+v, want [4 5]", got)
	}
	if !newer.HasMoreBefore || newer.HasMoreAfter {
		t.Fatalf("newer page metadata = %+v", newer)
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

func requestDurableEventPage(t *testing.T, app *app, path string) durableEventPageResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	newRouter(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload durableEventPageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

type durableEventPageResponse struct {
	TaskID        string             `json:"task_id"`
	Status        durable.TaskStatus `json:"status"`
	Events        []durable.Event    `json:"events"`
	Limit         int                `json:"limit"`
	FirstSequence int64              `json:"first_sequence"`
	LastSequence  int64              `json:"last_sequence"`
	HasMoreBefore bool               `json:"has_more_before"`
	HasMoreAfter  bool               `json:"has_more_after"`
}

func eventSequences(events []durable.Event) []int64 {
	seqs := make([]int64, 0, len(events))
	for _, event := range events {
		seqs = append(seqs, event.Sequence)
	}
	return seqs
}
