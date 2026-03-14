package pulse

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	pulsecore "manifold/internal/pulse"
	"manifold/internal/sandbox"
)

func TestToolUpsertListAndDeleteTask(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRoomID(context.Background(), "!room:test")

	upsertRaw, err := json.Marshal(map[string]any{
		"action":           "upsert_task",
		"title":            "Check issues",
		"prompt":           "Review open issues and summarize blockers",
		"interval_seconds": 600,
	})
	if err != nil {
		t.Fatalf("marshal upsert args: %v", err)
	}
	upsertResp, err := tool.Call(ctx, upsertRaw)
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	upsertMap := upsertResp.(map[string]any)
	if ok, _ := upsertMap["ok"].(bool); !ok {
		t.Fatalf("expected upsert response ok=true, got %#v", upsertMap)
	}
	createdTask, ok := upsertMap["task"].(persistence.PulseTask)
	if !ok {
		t.Fatalf("expected persistence.PulseTask in response, got %#v", upsertMap["task"])
	}
	if createdTask.ID == "" {
		t.Fatalf("expected task id in response")
	}

	listRaw, err := json.Marshal(map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("marshal list args: %v", err)
	}
	listResp, err := tool.Call(ctx, listRaw)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	listMap := listResp.(map[string]any)
	if count, _ := listMap["task_count"].(int); count != 1 {
		t.Fatalf("expected task_count=1, got %#v", listMap["task_count"])
	}

	deleteRaw, err := json.Marshal(map[string]any{"action": "delete_task", "task_id": createdTask.ID})
	if err != nil {
		t.Fatalf("marshal delete args: %v", err)
	}
	deleteResp, err := tool.Call(ctx, deleteRaw)
	if err != nil {
		t.Fatalf("delete task: %v", err)
	}
	deleteMap := deleteResp.(map[string]any)
	if ok, _ := deleteMap["ok"].(bool); !ok {
		t.Fatalf("expected delete response ok=true, got %#v", deleteMap)
	}
}

func TestToolEnableDisableAndSetInterval(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRoomID(context.Background(), "!room:test")

	created := createTestTask(t, tool, ctx)

	disableRaw, err := json.Marshal(map[string]any{"action": "disable_task", "task_id": created.ID})
	if err != nil {
		t.Fatalf("marshal disable args: %v", err)
	}
	disableResp, err := tool.Call(ctx, disableRaw)
	if err != nil {
		t.Fatalf("disable task: %v", err)
	}
	disableMap := disableResp.(map[string]any)
	disabledTask := disableMap["task"].(persistence.PulseTask)
	if disabledTask.Enabled {
		t.Fatalf("expected task to be disabled")
	}

	enableRaw, err := json.Marshal(map[string]any{"action": "enable_task", "task_id": created.ID})
	if err != nil {
		t.Fatalf("marshal enable args: %v", err)
	}
	enableResp, err := tool.Call(ctx, enableRaw)
	if err != nil {
		t.Fatalf("enable task: %v", err)
	}
	enableMap := enableResp.(map[string]any)
	enabledTask := enableMap["task"].(persistence.PulseTask)
	if !enabledTask.Enabled {
		t.Fatalf("expected task to be enabled")
	}

	intervalRaw, err := json.Marshal(map[string]any{"action": "set_interval", "task_id": created.ID, "interval_seconds": 1200})
	if err != nil {
		t.Fatalf("marshal set interval args: %v", err)
	}
	intervalResp, err := tool.Call(ctx, intervalRaw)
	if err != nil {
		t.Fatalf("set interval: %v", err)
	}
	intervalMap := intervalResp.(map[string]any)
	updatedTask := intervalMap["task"].(persistence.PulseTask)
	if updatedTask.IntervalSeconds != 1200 {
		t.Fatalf("expected interval 1200, got %d", updatedTask.IntervalSeconds)
	}
}

func TestToolClearClaim(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRoomID(context.Background(), "!room:test")

	room, err := store.EnsureRoom(ctx, "!room:test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}
	claimed, err := store.ClaimRoom(ctx, room.RoomID, "claim-token", room.CreatedAt.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}

	clearRaw, err := json.Marshal(map[string]any{"action": "clear_claim"})
	if err != nil {
		t.Fatalf("marshal clear args: %v", err)
	}
	clearResp, err := tool.Call(ctx, clearRaw)
	if err != nil {
		t.Fatalf("clear claim: %v", err)
	}
	clearMap := clearResp.(map[string]any)
	if ok, _ := clearMap["ok"].(bool); !ok {
		t.Fatalf("expected clear response ok=true, got %#v", clearMap)
	}
	clearedRoom := clearMap["room"].(persistence.PulseRoom)
	if clearedRoom.ActiveClaimToken != "" {
		t.Fatalf("expected room claim token to be cleared, got %q", clearedRoom.ActiveClaimToken)
	}
	if !clearedRoom.ActiveClaimUntil.IsZero() {
		t.Fatalf("expected room claim expiry to be cleared, got %v", clearedRoom.ActiveClaimUntil)
	}
}

func TestToolConfigureRoomPreservesProjectWhenOmitted(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithProjectID(sandbox.WithRoomID(context.Background(), "!room:test"), "project-123")

	setProjectRaw, err := json.Marshal(map[string]any{
		"action":     "configure_room",
		"project_id": "project-123",
	})
	if err != nil {
		t.Fatalf("marshal configure args: %v", err)
	}
	if _, err := tool.Call(ctx, setProjectRaw); err != nil {
		t.Fatalf("configure room with project: %v", err)
	}

	enableOnlyRaw, err := json.Marshal(map[string]any{
		"action":  "configure_room",
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("marshal enable args: %v", err)
	}
	resp, err := tool.Call(ctx, enableOnlyRaw)
	if err != nil {
		t.Fatalf("configure room enable only: %v", err)
	}
	respMap := resp.(map[string]any)
	room := respMap["room"].(persistence.PulseRoom)
	if room.ProjectID != "project-123" {
		t.Fatalf("expected project_id to be preserved, got %q", room.ProjectID)
	}
}

func TestToolConfigureRoomRejectsMismatchedProject(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithProjectID(sandbox.WithRoomID(context.Background(), "!room:test"), "project-123")

	raw, err := json.Marshal(map[string]any{
		"action":     "configure_room",
		"project_id": "35749",
	})
	if err != nil {
		t.Fatalf("marshal configure args: %v", err)
	}
	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("configure room mismatch: %v", err)
	}
	respMap := resp.(map[string]any)
	if ok, _ := respMap["ok"].(bool); ok {
		t.Fatalf("expected configure_room mismatch to fail, got %#v", respMap)
	}
	if got, _ := respMap["error"].(string); got != "project_id must match the current request project context" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func createTestTask(t *testing.T, tool *Tool, ctx context.Context) persistence.PulseTask {
	t.Helper()
	upsertRaw, err := json.Marshal(map[string]any{
		"action":           "upsert_task",
		"title":            "Check issues",
		"prompt":           "Review open issues and summarize blockers",
		"interval_seconds": 600,
	})
	if err != nil {
		t.Fatalf("marshal upsert args: %v", err)
	}
	upsertResp, err := tool.Call(ctx, upsertRaw)
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	upsertMap := upsertResp.(map[string]any)
	createdTask, ok := upsertMap["task"].(persistence.PulseTask)
	if !ok {
		t.Fatalf("expected persistence.PulseTask in response, got %#v", upsertMap["task"])
	}
	return createdTask
}
