package pulse

import (
	"strings"
	"testing"
	"time"

	"manifold/internal/persistence"
)

func TestEvaluateRoomMarksDueTasks(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "@manibot:matrix.test", Enabled: true}
	tasks := []persistence.PulseTask{
		{
			ID:              "task-due",
			RoomID:          room.RoomID,
			RouteTarget:     room.RouteTarget,
			Title:           "Check inbox",
			Prompt:          "Review new items",
			IntervalSeconds: 600,
			Enabled:         true,
			LastRunAt:       now.Add(-11 * time.Minute),
		},
		{
			ID:              "task-wait",
			RoomID:          room.RoomID,
			RouteTarget:     room.RouteTarget,
			Title:           "Review logs",
			Prompt:          "Check log anomalies",
			IntervalSeconds: 900,
			Enabled:         true,
			LastRunAt:       now.Add(-5 * time.Minute),
		},
	}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if got := len(plan.DueTasks); got != 1 {
		t.Fatalf("expected 1 due task, got %d", got)
	}
	if plan.DueTasks[0].ID != "task-due" {
		t.Fatalf("expected task-due to be due, got %q", plan.DueTasks[0].ID)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 task statuses, got %d", len(plan.Tasks))
	}
	if !plan.Tasks[0].Due {
		t.Fatalf("expected first status to be due")
	}
	if plan.Tasks[1].Due {
		t.Fatalf("expected second status to be waiting")
	}
}

func TestBuildPromptIncludesTaskDetails(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "@manibot:matrix.test", Enabled: true, ProjectID: "project-1"}
	tasks := []persistence.PulseTask{{
		ID:              "task-1",
		RoomID:          room.RoomID,
		RouteTarget:     room.RouteTarget,
		Title:           "Prepare summary",
		Prompt:          "Collect updates and summarize them",
		IntervalSeconds: 300,
		Enabled:         true,
	}}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	prompt := svc.BuildPrompt(now, plan, 5*time.Minute)
	checks := []string{
		"[pulse mode]",
		"Room ID: !room:test",
		"Route target: @manibot:matrix.test",
		"Project ID: project-1",
		"route_target: @manibot:matrix.test",
		"title: Prepare summary",
		"Collect updates and summarize them",
		"not automatically posted to Matrix",
		"Use matrix_room_message only when a task requires a room-facing message",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Fatalf("expected prompt to contain %q, got %q", check, prompt)
		}
	}
}

func TestEvaluateRoomFiltersOtherBotsTasks(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "@gpt_bot:matrix.test", Enabled: true}
	tasks := []persistence.PulseTask{
		{ID: "task-gpt", RoomID: room.RoomID, RouteTarget: room.RouteTarget, Title: "GPT task", Prompt: "Do GPT work", IntervalSeconds: 60, Enabled: true},
		{ID: "task-other", RoomID: room.RoomID, RouteTarget: "@manibot:matrix.test", Title: "Other task", Prompt: "Do code work", IntervalSeconds: 60, Enabled: true},
	}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 visible task for bot, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].Task.ID != "task-gpt" {
		t.Fatalf("expected GPT task, got %q", plan.Tasks[0].Task.ID)
	}
}

func TestEvaluateRoomDailyTimeDueOncePerDay(t *testing.T) {
	t.Parallel()

	svc := NewService()
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "bot", Enabled: true}
	now := time.Date(2026, 5, 11, 9, 5, 0, 0, time.Local)
	tasks := []persistence.PulseTask{{
		ID:           "daily",
		RoomID:       room.RoomID,
		RouteTarget:  room.RouteTarget,
		Title:        "Daily check",
		Prompt:       "Check things",
		ScheduleType: ScheduleDailyTime,
		SpecificTime: "09:00",
		Enabled:      true,
		LastRunAt:    time.Date(2026, 5, 10, 9, 10, 0, 0, time.Local),
	}}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if len(plan.DueTasks) != 1 || !plan.Tasks[0].Due {
		t.Fatalf("expected daily task to be due, got %#v", plan.Tasks)
	}

	tasks[0].LastRunAt = time.Date(2026, 5, 11, 9, 1, 0, 0, time.Local)
	plan = svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if len(plan.DueTasks) != 0 || plan.Tasks[0].Due {
		t.Fatalf("expected daily task to run only once per day, got %#v", plan.Tasks)
	}
	if plan.Tasks[0].ScheduleLabel != "daily at 09:00 local time" {
		t.Fatalf("unexpected schedule label %q", plan.Tasks[0].ScheduleLabel)
	}
}

func TestEvaluateRoomSpecificSchedulesAreDueImmediatelyWhenNeverRun(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.Local)
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "bot", Enabled: true}
	tasks := []persistence.PulseTask{
		{ID: "daily", RoomID: room.RoomID, RouteTarget: room.RouteTarget, Title: "Daily", Prompt: "Daily", ScheduleType: ScheduleDailyTime, SpecificTime: "17:00", Enabled: true},
		{ID: "once", RoomID: room.RoomID, RouteTarget: room.RouteTarget, Title: "Once", Prompt: "Once", ScheduleType: ScheduleOnceAt, SpecificAt: now.Add(24 * time.Hour), Enabled: true},
	}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if len(plan.DueTasks) != 2 {
		t.Fatalf("expected never-run specific schedules to be due immediately, got %#v", plan.DueTasks)
	}
}

func TestEvaluateRoomOnceAtDoesNotRepeat(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.Local)
	room := persistence.PulseRoom{RoomID: "!room:test", RouteTarget: "bot", Enabled: true}
	tasks := []persistence.PulseTask{{
		ID:           "once",
		RoomID:       room.RoomID,
		RouteTarget:  room.RouteTarget,
		Title:        "Once",
		Prompt:       "Once",
		ScheduleType: ScheduleOnceAt,
		SpecificAt:   now.Add(-time.Hour),
		Enabled:      true,
		LastRunAt:    now.Add(-30 * time.Minute),
	}}

	plan := svc.EvaluateRoom(now, room, tasks, room.RouteTarget)
	if len(plan.DueTasks) != 0 || plan.Tasks[0].Due {
		t.Fatalf("expected one-off task not to repeat, got %#v", plan.Tasks)
	}
}

func TestPulseSessionIDDeterministic(t *testing.T) {
	t.Parallel()

	left := PulseSessionID("matrix", "!room:test")
	right := PulseSessionID("matrix", "!room:test")
	other := PulseSessionID("matrix", "!other:test")
	if left != right {
		t.Fatalf("expected deterministic session id, got %q and %q", left, right)
	}
	if left == other {
		t.Fatalf("expected different room ids to produce different pulse session ids")
	}
}
