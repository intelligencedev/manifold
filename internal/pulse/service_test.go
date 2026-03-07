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
	room := persistence.PulseRoom{RoomID: "!room:test", Enabled: true}
	tasks := []persistence.PulseTask{
		{
			ID:              "task-due",
			RoomID:          room.RoomID,
			Title:           "Check inbox",
			Prompt:          "Review new items",
			IntervalSeconds: 600,
			Enabled:         true,
			LastRunAt:       now.Add(-11 * time.Minute),
		},
		{
			ID:              "task-wait",
			RoomID:          room.RoomID,
			Title:           "Review logs",
			Prompt:          "Check log anomalies",
			IntervalSeconds: 900,
			Enabled:         true,
			LastRunAt:       now.Add(-5 * time.Minute),
		},
	}

	plan := svc.EvaluateRoom(now, room, tasks)
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
	room := persistence.PulseRoom{RoomID: "!room:test", Enabled: true, ProjectID: "project-1"}
	tasks := []persistence.PulseTask{{
		ID:              "task-1",
		RoomID:          room.RoomID,
		Title:           "Prepare summary",
		Prompt:          "Collect updates and summarize them",
		IntervalSeconds: 300,
		Enabled:         true,
	}}

	plan := svc.EvaluateRoom(now, room, tasks)
	prompt := svc.BuildPrompt(now, plan, 5*time.Minute)
	checks := []string{
		"[pulse mode]",
		"Room ID: !room:test",
		"Project ID: project-1",
		"title: Prepare summary",
		"Collect updates and summarize them",
		"not posted to Matrix automatically",
		"matrix_room_message",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Fatalf("expected prompt to contain %q, got %q", check, prompt)
		}
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
