package agentd

import (
	"context"
	"errors"
	"strings"
	"time"

	"manifold/internal/persistence"
	"manifold/internal/pulse"
)

func (a *app) upsertMatrixTask(ctx context.Context, roomID, taskID string, req matrixTaskUpsertRequest) (matrixTaskResponse, error) {
	roomConfig, ok := a.matrixRoomConfig(roomID)
	if !ok {
		return matrixTaskResponse{}, errors.New("matrix room not configured")
	}
	resolvedTarget := strings.TrimSpace(req.RouteTarget)
	if resolvedTarget == "" {
		resolvedTarget = strings.TrimSpace(roomConfig.DefaultTarget)
	}
	if resolvedTarget == "" {
		return matrixTaskResponse{}, errors.New("route target required")
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Prompt) == "" {
		return matrixTaskResponse{}, errors.New("title and prompt are required")
	}
	schedule, err := matrixScheduleFromRequest(req.ScheduleType, req.IntervalSeconds, req.SpecificTime, req.SpecificAt)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	room, err := a.pulseRuntime.store.EnsureRoom(ctx, roomID, resolvedTarget)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	task := persistence.PulseTask{
		ID:              strings.TrimSpace(taskID),
		RoomID:          roomID,
		RouteTarget:     resolvedTarget,
		Title:           strings.TrimSpace(req.Title),
		Prompt:          strings.TrimSpace(req.Prompt),
		ScheduleType:    schedule.ScheduleType,
		IntervalSeconds: schedule.IntervalSeconds,
		SpecificTime:    schedule.SpecificTime,
		SpecificAt:      schedule.SpecificAt,
		Enabled:         room.Enabled,
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	stored, err := a.pulseRuntime.store.UpsertTask(ctx, task)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	return a.matrixTaskResponseFor(ctx, room, stored)
}

func (a *app) patchMatrixTask(ctx context.Context, roomID, taskID string, req matrixTaskPatchRequest) (matrixTaskResponse, error) {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	applyMatrixTaskPatchFields(&task, req)
	if err := applyMatrixTaskSchedulePatch(&task, req); err != nil {
		return matrixTaskResponse{}, err
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if task.RouteTarget == "" {
		task.RouteTarget = room.RouteTarget
	}
	previousRouteTarget := room.RouteTarget
	stored, err := a.pulseRuntime.store.UpsertTask(ctx, task)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	if previousRouteTarget != "" && previousRouteTarget != stored.RouteTarget {
		if err := a.pulseRuntime.store.DeleteTask(ctx, roomID, previousRouteTarget, stored.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return matrixTaskResponse{}, err
		}
	}
	updatedRoom, err := a.pulseRuntime.store.GetRoom(ctx, roomID, stored.RouteTarget)
	if err != nil {
		updatedRoom = room
	}
	return a.matrixTaskResponseFor(ctx, updatedRoom, stored)
}

func applyMatrixTaskPatchFields(task *persistence.PulseTask, req matrixTaskPatchRequest) {
	if req.RouteTarget != nil {
		task.RouteTarget = strings.TrimSpace(*req.RouteTarget)
	}
	if req.Title != nil {
		task.Title = strings.TrimSpace(*req.Title)
	}
	if req.Prompt != nil {
		task.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.IntervalSeconds != nil {
		task.ScheduleType = pulse.ScheduleInterval
		task.IntervalSeconds = *req.IntervalSeconds
		task.SpecificTime = ""
		task.SpecificAt = time.Time{}
	}
}

func applyMatrixTaskSchedulePatch(task *persistence.PulseTask, req matrixTaskPatchRequest) error {
	if req.ScheduleType == nil && req.SpecificTime == nil && req.SpecificAt == nil {
		return nil
	}
	scheduleType, intervalSeconds, specificTime, specificAt := matrixTaskSchedulePatchValues(*task, req)
	schedule, err := matrixScheduleFromRequest(scheduleType, intervalSeconds, specificTime, specificAt)
	if err != nil {
		return err
	}
	task.ScheduleType = schedule.ScheduleType
	task.IntervalSeconds = schedule.IntervalSeconds
	task.SpecificTime = schedule.SpecificTime
	task.SpecificAt = schedule.SpecificAt
	return nil
}

func matrixTaskSchedulePatchValues(task persistence.PulseTask, req matrixTaskPatchRequest) (string, int, string, string) {
	scheduleType := task.ScheduleType
	if req.ScheduleType != nil {
		scheduleType = *req.ScheduleType
	}
	intervalSeconds := task.IntervalSeconds
	if req.IntervalSeconds != nil {
		intervalSeconds = *req.IntervalSeconds
	}
	specificTime := task.SpecificTime
	if req.SpecificTime != nil {
		specificTime = *req.SpecificTime
	}
	specificAt := ""
	if !task.SpecificAt.IsZero() {
		specificAt = task.SpecificAt.Format(time.RFC3339)
	}
	if req.SpecificAt != nil {
		specificAt = *req.SpecificAt
	}
	scheduleType, specificTime, specificAt = normalizeMatrixTaskSchedulePatch(scheduleType, specificTime, specificAt, req)
	return scheduleType, intervalSeconds, specificTime, specificAt
}

func normalizeMatrixTaskSchedulePatch(scheduleType, specificTime, specificAt string, req matrixTaskPatchRequest) (string, string, string) {
	if req.ScheduleType == nil {
		return scheduleType, specificTime, specificAt
	}
	switch strings.TrimSpace(*req.ScheduleType) {
	case pulse.ScheduleDailyTime:
		if req.SpecificAt == nil {
			specificAt = ""
		}
	case pulse.ScheduleOnceAt:
		if req.SpecificTime == nil {
			specificTime = ""
		}
	case pulse.ScheduleInterval:
		if req.SpecificTime == nil {
			specificTime = ""
		}
		if req.SpecificAt == nil {
			specificAt = ""
		}
	}
	return scheduleType, specificTime, specificAt
}

func (a *app) patchMatrixTaskEnabled(ctx context.Context, roomID, taskID string, enabled *bool) (matrixTaskResponse, error) {
	return a.patchMatrixTask(ctx, roomID, taskID, matrixTaskPatchRequest{Enabled: enabled})
}

func (a *app) forceMatrixTaskRunNow(ctx context.Context, roomID, taskID string) (matrixTaskResponse, error) {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	stored, err := a.enqueuePulseTaskRun(ctx, room, task, pulseRunReasonManual, time.Now().UTC())
	if err != nil {
		return matrixTaskResponse{}, err
	}
	return a.matrixTaskResponseFor(ctx, room, stored)
}

func (a *app) deleteMatrixTask(ctx context.Context, roomID, taskID string) error {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return err
	}
	return a.pulseRuntime.store.DeleteTask(ctx, room.RoomID, room.RouteTarget, task.ID)
}

func (a *app) matrixTaskByID(ctx context.Context, roomID, taskID string) (persistence.PulseRoom, persistence.PulseTask, error) {
	rooms, err := a.pulseRuntime.store.ListRooms(ctx, "")
	if err != nil {
		return persistence.PulseRoom{}, persistence.PulseTask{}, err
	}
	for _, room := range rooms {
		if room.RoomID != roomID {
			continue
		}
		tasks, err := a.pulseRuntime.store.ListTasks(ctx, room.RoomID, room.RouteTarget)
		if err != nil {
			return persistence.PulseRoom{}, persistence.PulseTask{}, err
		}
		for _, task := range tasks {
			if task.ID == taskID {
				return room, task, nil
			}
		}
	}
	return persistence.PulseRoom{}, persistence.PulseTask{}, persistence.ErrNotFound
}

func (a *app) matrixTaskResponseFor(ctx context.Context, room persistence.PulseRoom, task persistence.PulseTask) (matrixTaskResponse, error) {
	planner := pulse.NewService()
	plan := planner.EvaluateRoom(time.Now().UTC(), room, []persistence.PulseTask{task}, room.RouteTarget)
	if len(plan.Tasks) == 0 {
		return matrixTaskResponse{}, persistence.ErrNotFound
	}
	status := plan.Tasks[0]
	response := matrixTaskResponse{
		ID:                status.Task.ID,
		RoomID:            status.Task.RoomID,
		RouteTarget:       status.Task.RouteTarget,
		Title:             status.Task.Title,
		Prompt:            status.Task.Prompt,
		ScheduleType:      status.Task.ScheduleType,
		ScheduleLabel:     status.ScheduleLabel,
		IntervalSeconds:   status.Task.IntervalSeconds,
		IntervalHuman:     status.IntervalHuman,
		SpecificTime:      status.Task.SpecificTime,
		SpecificAt:        status.Task.SpecificAt,
		Enabled:           status.Task.Enabled,
		RoomEnabled:       room.Enabled,
		Due:               status.Due,
		LastRunAt:         status.Task.LastRunAt,
		LastRunHuman:      status.LastRunHuman,
		LastResultSummary: status.Task.LastResultSummary,
		CreatedAt:         status.Task.CreatedAt,
		UpdatedAt:         status.Task.UpdatedAt,
	}
	if !status.NextRunAt.IsZero() {
		response.NextRunAt = status.NextRunAt
		response.NextRunHuman = status.NextRunAt.Format(time.RFC3339)
	}
	a.applyMatrixTaskRunState(ctx, &response, status.Task)
	return response, nil
}

func matrixScheduleFromRequest(scheduleType string, intervalSeconds int, specificTime, specificAt string) (persistence.PulseTask, error) {
	task := persistence.PulseTask{
		ScheduleType:    strings.TrimSpace(scheduleType),
		IntervalSeconds: intervalSeconds,
		SpecificTime:    strings.TrimSpace(specificTime),
	}
	if strings.TrimSpace(specificAt) != "" {
		parsed, err := parseMatrixSpecificAt(strings.TrimSpace(specificAt))
		if err != nil {
			return task, errors.New("specificAt must be RFC3339 or YYYY-MM-DDTHH:MM")
		}
		task.SpecificAt = parsed
	}
	return pulse.NormalizeTaskSchedule(task)
}

func parseMatrixSpecificAt(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local); err == nil {
		return parsed, nil
	}
	return time.Time{}, errors.New("invalid specificAt")
}
