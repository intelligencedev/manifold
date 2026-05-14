package databases

import (
	"context"
	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
	"strings"
	"time"
)

func (s *pgPulseStore) getTask(ctx context.Context, roomID, routeTarget, taskID string) (persistence.PulseTask, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, room_id, route_target, title, prompt, schedule_type, interval_seconds, specific_time, specific_at, enabled, last_run_at, last_result_summary, created_at, updated_at
FROM pulse_tasks
WHERE room_id = $1 AND route_target = $2 AND id = $3
`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget), strings.TrimSpace(taskID))
	if err != nil {
		return persistence.PulseTask{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	return scanPulseTask(rows)
}

func scanPulseRoom(rows interface{ Scan(...any) error }) (persistence.PulseRoom, error) {
	var room persistence.PulseRoom
	var projectID, claimToken, summary, pulseErr *string
	var claimUntil, attemptAt, completedAt *time.Time
	if err := rows.Scan(
		&room.RoomID,
		&room.RouteTarget,
		&projectID,
		&room.Enabled,
		&room.Revision,
		&claimToken,
		&claimUntil,
		&attemptAt,
		&completedAt,
		&summary,
		&pulseErr,
		&room.CreatedAt,
		&room.UpdatedAt,
	); err != nil {
		return persistence.PulseRoom{}, err
	}
	if projectID != nil {
		room.ProjectID = *projectID
	}
	if claimToken != nil {
		room.ActiveClaimToken = *claimToken
	}
	if claimUntil != nil {
		room.ActiveClaimUntil = claimUntil.UTC()
	}
	if attemptAt != nil {
		room.LastPulseAttemptAt = attemptAt.UTC()
	}
	if completedAt != nil {
		room.LastPulseCompletedAt = completedAt.UTC()
	}
	if summary != nil {
		room.LastPulseSummary = *summary
	}
	if pulseErr != nil {
		room.LastPulseError = *pulseErr
	}
	return room, nil
}

func scanPulseTask(rows interface{ Scan(...any) error }) (persistence.PulseTask, error) {
	var task persistence.PulseTask
	var specificAt *time.Time
	var lastRunAt *time.Time
	var lastSummary *string
	if err := rows.Scan(
		&task.ID,
		&task.RoomID,
		&task.RouteTarget,
		&task.Title,
		&task.Prompt,
		&task.ScheduleType,
		&task.IntervalSeconds,
		&task.SpecificTime,
		&specificAt,
		&task.Enabled,
		&lastRunAt,
		&lastSummary,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return persistence.PulseTask{}, err
	}
	if specificAt != nil {
		task.SpecificAt = specificAt.UTC()
	}
	if lastRunAt != nil {
		task.LastRunAt = lastRunAt.UTC()
	}
	if lastSummary != nil {
		task.LastResultSummary = *lastSummary
	}
	normalized, err := pulsecore.NormalizeTaskSchedule(task)
	if err != nil {
		return task, nil
	}
	normalized.ID = task.ID
	normalized.RoomID = task.RoomID
	normalized.RouteTarget = task.RouteTarget
	normalized.Title = task.Title
	normalized.Prompt = task.Prompt
	normalized.Enabled = task.Enabled
	normalized.LastRunAt = task.LastRunAt
	normalized.LastResultSummary = task.LastResultSummary
	normalized.CreatedAt = task.CreatedAt
	normalized.UpdatedAt = task.UpdatedAt
	return normalized, nil
}

func clonePulseRoom(room persistence.PulseRoom) persistence.PulseRoom {
	return room
}

func clonePulseTask(task persistence.PulseTask) persistence.PulseTask {
	return task
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func emptyToNil(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
