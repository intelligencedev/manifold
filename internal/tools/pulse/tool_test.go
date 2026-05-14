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
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@manibot:matrix.test")

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
	if createdTask.RouteTarget != "@manibot:matrix.test" {
		t.Fatalf("expected task route target to default from context, got %q", createdTask.RouteTarget)
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
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@manibot:matrix.test")

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

func TestToolUpsertTaskWithDailyTimeSchedule(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "bot")

	raw, err := json.Marshal(map[string]any{
		"action":        "upsert_task",
		"title":         "Morning check",
		"prompt":        "Check status",
		"schedule_type": "daily_time",
		"specific_time": "09:30",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("upsert daily task: %v", err)
	}
	task := resp.(map[string]any)["task"].(persistence.PulseTask)
	if task.ScheduleType != pulsecore.ScheduleDailyTime || task.SpecificTime != "09:30" || task.IntervalSeconds != 0 {
		t.Fatalf("unexpected daily task schedule: %#v", task)
	}
}

func TestToolSetScheduleToOnceAt(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "bot")
	created := createTestTask(t, tool, ctx)

	raw, err := json.Marshal(map[string]any{
		"action":        "set_schedule",
		"task_id":       created.ID,
		"schedule_type": "once_at",
		"specific_at":   "2026-05-12T09:00",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	task := resp.(map[string]any)["task"].(persistence.PulseTask)
	if task.ScheduleType != pulsecore.ScheduleOnceAt || task.SpecificAt.IsZero() || task.IntervalSeconds != 0 {
		t.Fatalf("unexpected once_at schedule: %#v", task)
	}
}

func TestToolRejectsConflictingScheduleFields(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "bot")

	raw, err := json.Marshal(map[string]any{
		"action":        "upsert_task",
		"title":         "Bad schedule",
		"prompt":        "Nope",
		"schedule_type": "daily_time",
		"specific_time": "09:30",
		"specific_at":   "2026-05-12T09:00",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("upsert conflicting schedule: %v", err)
	}
	respMap := resp.(map[string]any)
	if ok, _ := respMap["ok"].(bool); ok {
		t.Fatalf("expected conflicting schedule to fail, got %#v", respMap)
	}
}

func TestToolClearClaim(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@manibot:matrix.test")

	room, err := store.EnsureRoom(ctx, "!room:test", "@manibot:matrix.test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.RouteTarget, "claim-token", room.CreatedAt.Add(5*time.Minute))
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
	ctx := sandbox.WithProjectID(sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@manibot:matrix.test"), "project-123")

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
	ctx := sandbox.WithProjectID(sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@manibot:matrix.test"), "project-123")

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

func TestToolCanAssignTaskToAnotherBot(t *testing.T) {
	t.Parallel()

	store := databases.NewPulseStore(nil)
	tool := &Tool{store: store, service: pulsecore.NewService()}
	ctx := sandbox.WithRouteTarget(sandbox.WithRoomID(context.Background(), "!room:test"), "@gpt_bot:matrix.test")

	raw, err := json.Marshal(map[string]any{
		"action":           "upsert_task",
		"route_target":     "@manibot:matrix.test",
		"title":            "Review code",
		"prompt":           "Check the latest patch",
		"interval_seconds": 300,
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("upsert delegated task: %v", err)
	}
	respMap := resp.(map[string]any)
	task := respMap["task"].(persistence.PulseTask)
	if task.RouteTarget != "@manibot:matrix.test" {
		t.Fatalf("expected delegated route target, got %q", task.RouteTarget)
	}

	listRaw, err := json.Marshal(map[string]any{"action": "list", "route_target": "@manibot:matrix.test"})
	if err != nil {
		t.Fatalf("marshal list args: %v", err)
	}
	listResp, err := tool.Call(ctx, listRaw)
	if err != nil {
		t.Fatalf("list delegated tasks: %v", err)
	}
	listMap := listResp.(map[string]any)
	if count, _ := listMap["task_count"].(int); count != 1 {
		t.Fatalf("expected task_count=1 for delegated bot, got %#v", listMap["task_count"])
	}
}
