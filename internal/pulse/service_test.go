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
	room := persistence.PulseRoom{RoomID: "!room:test", BotID: "@manibot:matrix.test", Enabled: true}
	tasks := []persistence.PulseTask{
		{
			ID:              "task-due",
			RoomID:          room.RoomID,
			BotID:           room.BotID,
			Title:           "Check inbox",
			Prompt:          "Review new items",
			IntervalSeconds: 600,
			Enabled:         true,
			LastRunAt:       now.Add(-11 * time.Minute),
		},
		{
			ID:              "task-wait",
			RoomID:          room.RoomID,
			BotID:           room.BotID,
			Title:           "Review logs",
			Prompt:          "Check log anomalies",
			IntervalSeconds: 900,
			Enabled:         true,
			LastRunAt:       now.Add(-5 * time.Minute),
		},
	}

	plan := svc.EvaluateRoom(now, room, tasks, room.BotID)
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
	room := persistence.PulseRoom{RoomID: "!room:test", BotID: "@manibot:matrix.test", Enabled: true, ProjectID: "project-1"}
	tasks := []persistence.PulseTask{{
		ID:              "task-1",
		RoomID:          room.RoomID,
		BotID:           room.BotID,
		Title:           "Prepare summary",
		Prompt:          "Collect updates and summarize them",
		IntervalSeconds: 300,
		Enabled:         true,
	}}

	plan := svc.EvaluateRoom(now, room, tasks, room.BotID)
	prompt := svc.BuildPrompt(now, plan, 5*time.Minute)
	checks := []string{
		"[pulse mode]",
		"Room ID: !room:test",
		"Bot ID: @manibot:matrix.test",
		"Project ID: project-1",
		"bot_id: @manibot:matrix.test",
		"title: Prepare summary",
		"Collect updates and summarize them",
		"posted directly to the Matrix room",
		"response will be sent to the room",
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
	room := persistence.PulseRoom{RoomID: "!room:test", BotID: "@gpt_bot:matrix.test", Enabled: true}
	tasks := []persistence.PulseTask{
		{ID: "task-gpt", RoomID: room.RoomID, BotID: room.BotID, Title: "GPT task", Prompt: "Do GPT work", IntervalSeconds: 60, Enabled: true},
		{ID: "task-other", RoomID: room.RoomID, BotID: "@manibot:matrix.test", Title: "Other task", Prompt: "Do code work", IntervalSeconds: 60, Enabled: true},
	}

	plan := svc.EvaluateRoom(now, room, tasks, room.BotID)
	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 visible task for bot, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].Task.ID != "task-gpt" {
		t.Fatalf("expected GPT task, got %q", plan.Tasks[0].Task.ID)
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
