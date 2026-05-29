package pulse

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"manifold/internal/persistence"

	"github.com/google/uuid"
)

const (
	ScheduleInterval  = "interval"
	ScheduleDailyTime = "daily_time"
	ScheduleOnceAt    = "once_at"
)

var ErrInvalidSchedule = errors.New("invalid pulse task schedule")

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
	ScheduleType   string                `json:"scheduleType"`
	ScheduleLabel  string                `json:"scheduleLabel"`
	NextRunAt      time.Time             `json:"nextRunAt"`
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

func NormalizeTaskSchedule(task persistence.PulseTask) (persistence.PulseTask, error) {
	task.ScheduleType = strings.TrimSpace(task.ScheduleType)
	task.SpecificTime = strings.TrimSpace(task.SpecificTime)
	if task.ScheduleType == "" {
		switch {
		case task.SpecificTime != "":
			task.ScheduleType = ScheduleDailyTime
		case !task.SpecificAt.IsZero():
			task.ScheduleType = ScheduleOnceAt
		default:
			task.ScheduleType = ScheduleInterval
		}
	}
	switch task.ScheduleType {
	case ScheduleInterval:
		if task.SpecificTime != "" || !task.SpecificAt.IsZero() {
			return task, fmt.Errorf("%w: interval tasks cannot set specific_time or specific_at", ErrInvalidSchedule)
		}
		if task.IntervalSeconds <= 0 {
			task.IntervalSeconds = 300
		}
	case ScheduleDailyTime:
		if task.SpecificTime == "" {
			return task, fmt.Errorf("%w: daily_time tasks require specific_time in HH:MM format", ErrInvalidSchedule)
		}
		if _, _, err := parseSpecificTime(task.SpecificTime); err != nil {
			return task, fmt.Errorf("%w: specific_time must use HH:MM format", ErrInvalidSchedule)
		}
		if !task.SpecificAt.IsZero() {
			return task, fmt.Errorf("%w: daily_time tasks cannot set specific_at", ErrInvalidSchedule)
		}
		// Non-interval schedules keep interval_seconds at 0 in persisted JSON/API responses.
		task.IntervalSeconds = 0
	case ScheduleOnceAt:
		if task.SpecificAt.IsZero() {
			return task, fmt.Errorf("%w: once_at tasks require specific_at", ErrInvalidSchedule)
		}
		if task.SpecificTime != "" {
			return task, fmt.Errorf("%w: once_at tasks cannot set specific_time", ErrInvalidSchedule)
		}
		task.IntervalSeconds = 0
	default:
		return task, fmt.Errorf("%w: schedule_type must be interval, daily_time, or once_at", ErrInvalidSchedule)
	}
	return task, nil
}

// EvaluateRoom determines which tasks are due at the provided time.
func (s *Service) EvaluateRoom(now time.Time, room persistence.PulseRoom, tasks []persistence.PulseTask, routeTarget string) Plan {
	now = now.UTC()
	routeTarget = strings.TrimSpace(routeTarget)
	statuses := make([]TaskStatus, 0, len(tasks))
	dueTasks := make([]persistence.PulseTask, 0, len(tasks))
	for _, task := range tasks {
		if routeTarget != "" && strings.TrimSpace(task.RouteTarget) != "" && strings.TrimSpace(task.RouteTarget) != routeTarget {
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
	b.WriteString("Use the matrix_room_message tool only when a due task explicitly requires notifying the room.\n")
	b.WriteString("Your final response is an internal pulse run log and is not automatically posted to Matrix.\n")
	b.WriteString("Keep that internal log concise and focused on what the run did.\n\n")
	b.WriteString(fmt.Sprintf("Current time (UTC): %s\n", now.UTC().Format(time.RFC3339)))
	if pollInterval > 0 {
		b.WriteString(fmt.Sprintf("Pulse poll interval: %s\n", pollInterval.Round(time.Second)))
	}
	b.WriteString(fmt.Sprintf("Room ID: %s\n", plan.Room.RoomID))
	if strings.TrimSpace(plan.Room.RouteTarget) != "" {
		b.WriteString(fmt.Sprintf("Route target: %s\n", plan.Room.RouteTarget))
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
		if strings.TrimSpace(status.Task.RouteTarget) != "" {
			b.WriteString(fmt.Sprintf("  route_target: %s\n", status.Task.RouteTarget))
		}
		b.WriteString(fmt.Sprintf("  schedule: %s\n", status.ScheduleLabel))
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
		for line := range strings.SplitSeq(strings.TrimSpace(status.Task.Prompt), "\n") {
			b.WriteString("    ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	if len(plan.DueTasks) == 0 {
		b.WriteString("No tasks are due in this poll. Review the schedule, optionally tidy the task list, and return a very short log note.\n")
	} else {
		b.WriteString("Execute the tasks marked due now. Use matrix_room_message only when a task requires a room-facing message. You may update the task list if priorities, wording, or intervals should change based on the run.\n")
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
	task, err := NormalizeTaskSchedule(task)
	if err != nil {
		task.ScheduleType = ScheduleInterval
		if task.IntervalSeconds <= 0 {
			task.IntervalSeconds = 300
		}
	}
	status := TaskStatus{Task: task, ScheduleType: task.ScheduleType}
	switch task.ScheduleType {
	case ScheduleDailyTime:
		return buildDailyTimeStatus(now, roomEnabled, task, status)
	case ScheduleOnceAt:
		return buildOnceAtStatus(now, roomEnabled, task, status)
	default:
		return buildIntervalStatus(now, roomEnabled, task, status)
	}
}

func buildIntervalStatus(now time.Time, roomEnabled bool, task persistence.PulseTask, status TaskStatus) TaskStatus {
	interval := time.Duration(task.IntervalSeconds) * time.Second
	status.IntervalHuman = interval.Round(time.Second).String()
	status.ScheduleLabel = "every " + status.IntervalHuman
	if task.LastRunAt.IsZero() {
		status.Due = roomEnabled && task.Enabled
		status.LastRunHuman = "never"
		status.RemainingHuman = "now"
		if roomEnabled && task.Enabled {
			status.NextRunAt = now.UTC()
		}
		return status
	}
	status.LastRunKnown = true
	status.Elapsed = max(now.Sub(task.LastRunAt.UTC()), 0)
	status.LastRunHuman = humanDuration(status.Elapsed) + " ago"
	if !roomEnabled || !task.Enabled {
		status.Remaining = interval
		status.RemainingHuman = interval.Round(time.Second).String()
		return status
	}
	if status.Elapsed >= interval {
		status.Due = true
		status.RemainingHuman = "now"
		status.NextRunAt = now.UTC()
		return status
	}
	status.Remaining = interval - status.Elapsed
	status.RemainingHuman = humanDuration(status.Remaining)
	status.NextRunAt = task.LastRunAt.UTC().Add(interval)
	return status
}

func buildDailyTimeStatus(now time.Time, roomEnabled bool, task persistence.PulseTask, status TaskStatus) TaskStatus {
	hour, minute, err := parseSpecificTime(task.SpecificTime)
	if err != nil {
		hour = 9
		minute = 0
	}
	localNow := now.Local()
	scheduledToday := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, time.Local)
	next := scheduledToday
	if localNow.After(scheduledToday) || localNow.Equal(scheduledToday) {
		next = scheduledToday.AddDate(0, 0, 1)
	}
	status.ScheduleLabel = fmt.Sprintf("daily at %s local time", task.SpecificTime)
	if task.LastRunAt.IsZero() {
		status.Due = roomEnabled && task.Enabled
		status.LastRunHuman = "never"
		status.RemainingHuman = "now"
		if roomEnabled && task.Enabled {
			status.NextRunAt = now.UTC()
		}
		return status
	}
	status.LastRunKnown = true
	status.Elapsed = max(now.Sub(task.LastRunAt.UTC()), 0)
	status.LastRunHuman = humanDuration(status.Elapsed) + " ago"
	if !roomEnabled || !task.Enabled {
		status.Remaining = next.Sub(localNow)
		status.RemainingHuman = humanDuration(status.Remaining)
		return status
	}
	if (localNow.After(scheduledToday) || localNow.Equal(scheduledToday)) && task.LastRunAt.Before(scheduledToday) {
		status.Due = true
		status.RemainingHuman = "now"
		status.NextRunAt = now.UTC()
		return status
	}
	status.Remaining = next.Sub(localNow)
	status.RemainingHuman = humanDuration(status.Remaining)
	status.NextRunAt = next.UTC()
	return status
}

func buildOnceAtStatus(now time.Time, roomEnabled bool, task persistence.PulseTask, status TaskStatus) TaskStatus {
	status.ScheduleLabel = "once at " + task.SpecificAt.Local().Format(time.RFC3339)
	if task.LastRunAt.IsZero() {
		status.Due = roomEnabled && task.Enabled
		status.LastRunHuman = "never"
		status.RemainingHuman = "now"
		if roomEnabled && task.Enabled {
			status.NextRunAt = now.UTC()
		}
		return status
	}
	status.LastRunKnown = true
	status.Elapsed = max(now.Sub(task.LastRunAt.UTC()), 0)
	status.LastRunHuman = humanDuration(status.Elapsed) + " ago"
	status.RemainingHuman = "done"
	return status
}

func parseSpecificTime(value string) (int, int, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, 0, err
	}
	return parsed.Hour(), parsed.Minute(), nil
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
