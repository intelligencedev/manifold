package agentd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/durable"
	"manifold/internal/persistence"
	"manifold/internal/pulse"
)

const (
	durablePulseQueue       = "pulse"
	durablePulseRunTaskName = "pulse.run"

	pulseRunReasonScheduled = "scheduled"
	pulseRunReasonManual    = "manual"
)

type pulseRunEnvelope struct {
	roomID      string
	routeTarget string
	taskID      string
	reason      string
	now         time.Time
}

type durablePulseRunParams struct {
	roomID      string
	routeTarget string
	taskID      string
	projectID   string
	reason      string
}

func (a *app) durablePulseAvailable() bool {
	if a == nil || a.durableClient == nil || a.durableStore == nil || a.durableRegistry == nil {
		return false
	}
	_, ok := a.durableRegistry.Get(durablePulseQueue, durablePulseRunTaskName)
	return ok
}

func (a *app) enqueuePulseTaskRun(ctx context.Context, room persistence.PulseRoom, task persistence.PulseTask, reason string, now time.Time) (persistence.PulseTask, error) {
	if a == nil || a.pulseRuntime == nil || a.pulseRuntime.store == nil {
		return persistence.PulseTask{}, errors.New("pulse runtime unavailable")
	}
	if !a.durablePulseAvailable() {
		return persistence.PulseTask{}, errors.New("durable pulse backend unavailable")
	}
	run, err := newPulseRunEnvelope(room, task, reason, now)
	if err != nil {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}

	claimed, err := a.claimPulseRun(ctx, run)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	if !claimed {
		latest, latestErr := a.pulseTaskInRoute(ctx, run.roomID, run.routeTarget, run.taskID)
		if latestErr != nil {
			return persistence.PulseTask{}, latestErr
		}
		return latest, nil
	}
	defer func() {
		_ = a.pulseRuntime.store.ClearRoomClaim(context.WithoutCancel(ctx), run.roomID, run.routeTarget)
	}()

	latestRoom, err := a.pulseRuntime.store.GetRoom(ctx, run.roomID, run.routeTarget)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	latest, err := a.pulseTaskInRoute(ctx, run.roomID, run.routeTarget, run.taskID)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	if !latestRoom.Enabled || !latest.Enabled {
		return latest, errors.New("pulse task is disabled")
	}
	if a.pulseRunStillActive(ctx, latest.ActiveDurableTaskID) {
		return latest, nil
	}

	spawn, err := a.spawnPulseRunTask(ctx, run, latestRoom, latest)
	if err != nil {
		return latest, err
	}
	updated, err := a.pulseRuntime.store.MarkTaskRunQueued(ctx, run.roomID, run.routeTarget, run.taskID, spawn.TaskID)
	if err != nil {
		_ = a.durableClient.Cancel(context.WithoutCancel(ctx), systemUserID, spawn.TaskID)
		return latest, err
	}
	return updated, nil
}

func newPulseRunEnvelope(room persistence.PulseRoom, task persistence.PulseTask, reason string, now time.Time) (pulseRunEnvelope, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = pulseRunReasonScheduled
	}
	run := pulseRunEnvelope{
		roomID:      strings.TrimSpace(room.RoomID),
		routeTarget: strings.TrimSpace(task.RouteTarget),
		taskID:      strings.TrimSpace(task.ID),
		reason:      reason,
		now:         now,
	}
	if run.routeTarget == "" {
		run.routeTarget = strings.TrimSpace(room.RouteTarget)
	}
	if run.roomID == "" || run.taskID == "" {
		return pulseRunEnvelope{}, persistence.ErrNotFound
	}
	return run, nil
}

func (a *app) claimPulseRun(ctx context.Context, run pulseRunEnvelope) (bool, error) {
	lease := defaultMatrixPulseLease
	if a.pulseRuntime.lease > 0 {
		lease = a.pulseRuntime.lease
	}
	return a.pulseRuntime.store.ClaimRoom(ctx, run.roomID, run.routeTarget, uuid.NewString(), run.now.Add(lease))
}

func (a *app) spawnPulseRunTask(ctx context.Context, run pulseRunEnvelope, room persistence.PulseRoom, task persistence.PulseTask) (durable.SpawnResult, error) {
	return a.durableClient.Spawn(ctx, durable.SpawnRequest{
		Queue:  durablePulseQueue,
		Name:   durablePulseRunTaskName,
		UserID: systemUserID,
		Params: map[string]any{
			"room_id":      run.roomID,
			"route_target": run.routeTarget,
			"task_id":      run.taskID,
			"project_id":   strings.TrimSpace(room.ProjectID),
			"reason":       run.reason,
			"requested_at": run.now.Format(time.RFC3339Nano),
		},
		IdempotencyKey: pulseRunIdempotencyKey(run.reason, run.roomID, run.routeTarget, task, run.now),
		RetryPolicy: durable.RetryPolicy{
			MaxAttempts:      3,
			Backoff:          "exponential",
			BaseDelaySeconds: 1,
			MaxDelaySeconds:  30,
		},
	})
}

func (a *app) pulseRunStillActive(ctx context.Context, taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || a == nil || a.durableStore == nil {
		return false
	}
	task, found, err := a.durableStore.GetTask(ctx, systemUserID, taskID)
	if err != nil || !found {
		return false
	}
	return task.Status == durable.TaskStatusQueued || task.Status == durable.TaskStatusRunning || task.Status == durable.TaskStatusWaiting
}

func (a *app) applyMatrixTaskRunState(ctx context.Context, response *matrixTaskResponse, task persistence.PulseTask) {
	if a == nil || a.durableStore == nil || response == nil {
		return
	}
	if activeRunID := strings.TrimSpace(task.ActiveDurableTaskID); activeRunID != "" {
		response.ActiveRunID = activeRunID
		if durableTask, found, err := a.durableStore.GetTask(ctx, systemUserID, activeRunID); err == nil && found {
			response.ActiveRunStatus = string(durableTask.Status)
			queuedAt := durableTask.CreatedAt
			response.ActiveRunQueuedAt = &queuedAt
			if durableTask.Status == durable.TaskStatusCompleted || durableTask.Status == durable.TaskStatusFailed || durableTask.Status == durable.TaskStatusCancelled {
				response.LastRunID = activeRunID
				response.LastRunStatus = string(durableTask.Status)
			}
		}
	}
	if response.LastRunID != "" {
		return
	}
	if lastRunID := strings.TrimSpace(task.LastDurableTaskID); lastRunID != "" {
		response.LastRunID = lastRunID
		if durableTask, found, err := a.durableStore.GetTask(ctx, systemUserID, lastRunID); err == nil && found {
			response.LastRunStatus = string(durableTask.Status)
		}
	}
}

func pulseRunIdempotencyKey(reason, roomID, routeTarget string, task persistence.PulseTask, now time.Time) string {
	reason = strings.TrimSpace(reason)
	if reason == pulseRunReasonManual {
		return "pulse:manual:" + uuid.NewString()
	}
	bucket := now.UTC().Truncate(defaultMatrixPulsePollInterval).Unix()
	return fmt.Sprintf("pulse:%s:%s:%s:%s:%d", reason, roomID, routeTarget, task.ID, bucket)
}

func (a *app) pulseTaskInRoute(ctx context.Context, roomID, routeTarget, taskID string) (persistence.PulseTask, error) {
	if a == nil || a.pulseRuntime == nil || a.pulseRuntime.store == nil {
		return persistence.PulseTask{}, errors.New("pulse runtime unavailable")
	}
	tasks, err := a.pulseRuntime.store.ListTasks(ctx, roomID, routeTarget)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	taskID = strings.TrimSpace(taskID)
	for _, task := range tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return persistence.PulseTask{}, persistence.ErrNotFound
}

func (a *app) runDurablePulseTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("durable task context unavailable")
	}
	run, err := parseDurablePulseRunParams(params)
	if err != nil {
		return nil, err
	}
	room, err := a.pulseRuntime.store.GetRoom(ctx, run.roomID, run.routeTarget)
	if err != nil {
		return nil, err
	}
	task, err := a.pulseTaskInRoute(ctx, run.roomID, run.routeTarget, run.taskID)
	if err != nil {
		return nil, err
	}
	if run.projectID == "" {
		run.projectID = strings.TrimSpace(room.ProjectID)
	}
	now := time.Now().UTC()
	prompt := buildSingleTaskPulsePrompt(now, room, task, run.routeTarget)

	recordDurablePulseStarted(ctx, run, task)
	result, runErr := a.handlePulseRoom(ctx, run.roomID, run.routeTarget, run.projectID, prompt)
	recordDurablePulseFinished(ctx, run, result, runErr)
	if err := a.completeDurablePulseRun(context.WithoutCancel(ctx), tc.Task.ID, run, result, runErr); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("%w; complete pulse: %v", runErr, err)
		}
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	return map[string]any{
		"ok":           true,
		"room_id":      run.roomID,
		"route_target": run.routeTarget,
		"task_id":      run.taskID,
		"summary":      result,
	}, nil
}

func parseDurablePulseRunParams(params map[string]any) (durablePulseRunParams, error) {
	run := durablePulseRunParams{}
	run.roomID, _ = params["room_id"].(string)
	run.routeTarget, _ = params["route_target"].(string)
	run.taskID, _ = params["task_id"].(string)
	run.projectID, _ = params["project_id"].(string)
	run.reason, _ = params["reason"].(string)
	run.roomID = strings.TrimSpace(run.roomID)
	run.routeTarget = strings.TrimSpace(run.routeTarget)
	run.taskID = strings.TrimSpace(run.taskID)
	run.projectID = strings.TrimSpace(run.projectID)
	run.reason = strings.TrimSpace(run.reason)
	if run.roomID == "" || run.taskID == "" {
		return durablePulseRunParams{}, fmt.Errorf("room_id and task_id are required")
	}
	return run, nil
}

func buildSingleTaskPulsePrompt(now time.Time, room persistence.PulseRoom, task persistence.PulseTask, routeTarget string) string {
	service := pulse.NewService()
	plan := service.EvaluateRoom(now, room, []persistence.PulseTask{task}, routeTarget)
	for i := range plan.Tasks {
		if plan.Tasks[i].Task.ID != task.ID {
			continue
		}
		plan.Tasks[i].Due = true
		plan.Tasks[i].RemainingHuman = "now"
		plan.Tasks[i].NextRunAt = now
	}
	plan.DueTasks = []persistence.PulseTask{task}
	return service.BuildPrompt(now, plan, defaultMatrixPulsePollInterval)
}

func recordDurablePulseStarted(ctx context.Context, run durablePulseRunParams, task persistence.PulseTask) {
	_, _ = durable.RecordEvent(ctx, "pulse.started", map[string]any{
		"room_id":      run.roomID,
		"route_target": run.routeTarget,
		"task_id":      run.taskID,
		"title":        task.Title,
		"reason":       run.reason,
	})
}

func recordDurablePulseFinished(ctx context.Context, run durablePulseRunParams, result string, runErr error) {
	if runErr != nil {
		_, _ = durable.RecordEvent(ctx, "pulse.failed", map[string]any{
			"room_id":      run.roomID,
			"route_target": run.routeTarget,
			"task_id":      run.taskID,
			"error":        runErr.Error(),
		})
		return
	}
	_, _ = durable.RecordEvent(ctx, "pulse.completed", map[string]any{
		"room_id":      run.roomID,
		"route_target": run.routeTarget,
		"task_id":      run.taskID,
		"summary":      result,
	})
}

func (a *app) completeDurablePulseRun(ctx context.Context, durableTaskID string, run durablePulseRunParams, result string, runErr error) error {
	completion := persistence.RoomPulseCompletion{
		RoomID:        run.roomID,
		RouteTarget:   run.routeTarget,
		DurableTaskID: durableTaskID,
		CompletedAt:   time.Now().UTC(),
		Summary:       result,
		DueTaskIDs:    []string{run.taskID},
	}
	if runErr != nil {
		completion.Error = runErr.Error()
	}
	return a.pulseRuntime.store.CompleteRoomPulse(ctx, completion)
}
