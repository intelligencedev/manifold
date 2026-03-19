package pulse

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"manifold/internal/persistence"

	"github.com/google/uuid"
)

// TaskStatus reports whether a task is due on the current poll.
type TaskStatus struct {
	Task           persistence.PulseTask `json:"task"`
	Due            bool                  `json:"due"`
	Elapsed        time.Duration         `json:"elapsed"`
	Remaining      time.Duration         `json:"remaining"`
	LastRunKnown   bool                  `json:"lastRunKnown"`
	LastRunHuman   string                `json:"lastRunHuman"`
	RemainingHuman string                `json:"remainingHuman"`
	IntervalHuman  string                `json:"intervalHuman"`
}

// Plan describes the state of one pulse poll for a room.
type Plan struct {
	Room     persistence.PulseRoom   `json:"room"`
	Tasks    []TaskStatus            `json:"tasks"`
	DueTasks []persistence.PulseTask `json:"dueTasks"`
}

// Service centralizes pulse scheduling and prompt-building rules.
type Service struct{}

// NewService constructs a pulse scheduling service.
func NewService() *Service {
	return &Service{}
}

// EvaluateRoom determines which tasks are due at the provided time.
func (s *Service) EvaluateRoom(now time.Time, room persistence.PulseRoom, tasks []persistence.PulseTask, botID string) Plan {
	now = now.UTC()
	botID = strings.TrimSpace(botID)
	statuses := make([]TaskStatus, 0, len(tasks))
	dueTasks := make([]persistence.PulseTask, 0, len(tasks))
	for _, task := range tasks {
		if botID != "" && strings.TrimSpace(task.BotID) != "" && strings.TrimSpace(task.BotID) != botID {
			continue
		}
		status := buildTaskStatus(now, room.Enabled, task)
		statuses = append(statuses, status)
		if status.Due {
			dueTasks = append(dueTasks, task)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Due != statuses[j].Due {
			return statuses[i].Due
		}
		if statuses[i].Task.CreatedAt.Equal(statuses[j].Task.CreatedAt) {
			return statuses[i].Task.ID < statuses[j].Task.ID
		}
		return statuses[i].Task.CreatedAt.Before(statuses[j].Task.CreatedAt)
	})
	return Plan{Room: room, Tasks: statuses, DueTasks: dueTasks}
}

// BuildPrompt renders the structured pulse prompt sent to the orchestrator.
func (s *Service) BuildPrompt(now time.Time, plan Plan, pollInterval time.Duration) string {
	var b strings.Builder
	b.WriteString("[pulse mode]\n")
	b.WriteString("You are running an automated pulse for a Matrix room. Work only on the tasks listed below.\n")
	b.WriteString("Use the pulse_tasks tool when you need to add, modify, disable, enable, or delete tasks.\n")
	b.WriteString("Your final response will be posted directly to the Matrix room, so write it for the room audience.\n")
	b.WriteString("Keep the response concise and relevant to the completed tasks.\n\n")
	b.WriteString(fmt.Sprintf("Current time (UTC): %s\n", now.UTC().Format(time.RFC3339)))
	if pollInterval > 0 {
		b.WriteString(fmt.Sprintf("Pulse poll interval: %s\n", pollInterval.Round(time.Second)))
	}
	b.WriteString(fmt.Sprintf("Room ID: %s\n", plan.Room.RoomID))
	if strings.TrimSpace(plan.Room.BotID) != "" {
		b.WriteString(fmt.Sprintf("Bot ID: %s\n", plan.Room.BotID))
	}
	if strings.TrimSpace(plan.Room.ProjectID) != "" {
		b.WriteString(fmt.Sprintf("Project ID: %s\n", plan.Room.ProjectID))
	}
	b.WriteString(fmt.Sprintf("Room enabled: %t\n", plan.Room.Enabled))
	b.WriteString("\n")
	if len(plan.Tasks) == 0 {
		b.WriteString("There are currently no pulse tasks configured for this room.\n")
		b.WriteString("If you think recurring work should be scheduled, you may create tasks with the pulse_tasks tool.\n")
		return b.String()
	}
	b.WriteString("Task list:\n")
	for _, status := range plan.Tasks {
		state := "waiting"
		if status.Due {
			state = "due now"
		}
		if !status.Task.Enabled {
			state = "disabled"
		}
		b.WriteString(fmt.Sprintf("- id: %s\n", status.Task.ID))
		b.WriteString(fmt.Sprintf("  title: %s\n", strings.TrimSpace(status.Task.Title)))
		if strings.TrimSpace(status.Task.BotID) != "" {
			b.WriteString(fmt.Sprintf("  bot_id: %s\n", status.Task.BotID))
		}
		b.WriteString(fmt.Sprintf("  interval: %s\n", status.IntervalHuman))
		b.WriteString(fmt.Sprintf("  state: %s\n", state))
		if status.LastRunKnown {
			b.WriteString(fmt.Sprintf("  last_run: %s\n", status.LastRunHuman))
		} else {
			b.WriteString("  last_run: never\n")
		}
		if status.Task.Enabled && !status.Due {
			b.WriteString(fmt.Sprintf("  next_due_in: %s\n", status.RemainingHuman))
		}
		b.WriteString("  prompt: |\n")
		for _, line := range strings.Split(strings.TrimSpace(status.Task.Prompt), "\n") {
			b.WriteString("    ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	if len(plan.DueTasks) == 0 {
		b.WriteString("No tasks are due in this poll. Review the schedule, optionally tidy the task list, and return a very short log note.\n")
	} else {
		b.WriteString("Execute the tasks marked due now. Your response will be sent to the room. You may update the task list if priorities, wording, or intervals should change based on the run.\n")
	}
	return b.String()
}

// PulseSessionID returns a deterministic session identifier for pulse runs.
func PulseSessionID(prefix, roomID string) string {
	cleanPrefix := strings.TrimSpace(prefix)
	if cleanPrefix == "" {
		cleanPrefix = "matrix"
	}
	seed := cleanPrefix + ":pulse:" + strings.TrimSpace(roomID)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func buildTaskStatus(now time.Time, roomEnabled bool, task persistence.PulseTask) TaskStatus {
	status := TaskStatus{Task: task}
	if task.IntervalSeconds <= 0 {
		task.IntervalSeconds = 300
		status.Task.IntervalSeconds = task.IntervalSeconds
	}
	interval := time.Duration(task.IntervalSeconds) * time.Second
	status.IntervalHuman = interval.Round(time.Second).String()
	if task.LastRunAt.IsZero() {
		status.Due = roomEnabled && task.Enabled
		status.LastRunHuman = "never"
		status.RemainingHuman = "now"
		return status
	}
	status.LastRunKnown = true
	status.Elapsed = now.Sub(task.LastRunAt.UTC())
	if status.Elapsed < 0 {
		status.Elapsed = 0
	}
	status.LastRunHuman = humanDuration(status.Elapsed) + " ago"
	if !roomEnabled || !task.Enabled {
		status.Remaining = interval
		status.RemainingHuman = interval.Round(time.Second).String()
		return status
	}
	if status.Elapsed >= interval {
		status.Due = true
		status.RemainingHuman = "now"
		return status
	}
	status.Remaining = interval - status.Elapsed
	status.RemainingHuman = humanDuration(status.Remaining)
	return status
}

func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		minutes := int(d / time.Minute)
		seconds := int((d % time.Minute) / time.Second)
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
