package pulse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
	"manifold/internal/validation"
)

const toolName = "pulse_tasks"

type toolArgs struct {
	Action          string  `json:"action"`
	RouteTarget     string  `json:"route_target"`
	TaskID          string  `json:"task_id"`
	Title           string  `json:"title"`
	Prompt          string  `json:"prompt"`
	IntervalSeconds int     `json:"interval_seconds"`
	Enabled         *bool   `json:"enabled"`
	ProjectID       *string `json:"project_id"`
}

type Tool struct {
	store   persistence.PulseStore
	service *pulsecore.Service
}

func New(store persistence.PulseStore) tools.Tool {
	return &Tool{store: store, service: pulsecore.NewService()}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        toolName,
		"description": "Manage recurring Matrix pulse tasks for the current room. Requires a room-scoped request context.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "One of: list, configure_room, upsert_task, delete_task, enable_task, disable_task, set_interval, clear_claim.",
				},
				"route_target": map[string]any{
					"type":        "string",
					"description": "Optional route target for this room scope. Defaults to the current Matrix route target for this request. Use this to inspect or assign tasks for another route target in the same room.",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Task identifier for updates or deletion. Omit to create a new task.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Short task title.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The instruction text that should run when the task is due.",
				},
				"interval_seconds": map[string]any{
					"type":        "integer",
					"description": "How often the task should execute, in seconds.",
				},
				"enabled": map[string]any{
					"type":        "boolean",
					"description": "Enable or disable a room or task.",
				},
				"project_id": map[string]any{
					"type":        "string",
					"description": "Optional room project ID. When provided, it must match the current request's active project context.",
				},
			},
			"required": []string{"action"},
		},
	}
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t.store == nil {
		return map[string]any{"ok": false, "error": "pulse store unavailable"}, nil
	}
	roomID, ok := sandbox.RoomIDFromContext(ctx)
	if !ok {
		return map[string]any{"ok": false, "error": "pulse_tasks requires a room-scoped request"}, nil
	}
	currentRouteTarget, _ := sandbox.RouteTargetFromContext(ctx)
	var args toolArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("invalid arguments: %v", err)}, nil
		}
	}
	targetRouteTarget := strings.TrimSpace(args.RouteTarget)
	if targetRouteTarget == "" {
		targetRouteTarget = strings.TrimSpace(currentRouteTarget)
	}
	if targetRouteTarget == "" {
		return map[string]any{"ok": false, "error": "pulse_tasks requires route_target in the request context or arguments"}, nil
	}
	switch strings.TrimSpace(strings.ToLower(args.Action)) {
	case "list":
		return t.handleList(ctx, roomID, targetRouteTarget)
	case "configure_room":
		return t.handleConfigureRoom(ctx, roomID, targetRouteTarget, args.ProjectID, args.Enabled)
	case "clear_claim":
		return t.handleClearClaim(ctx, roomID, targetRouteTarget)
	case "upsert_task":
		return t.handleUpsertTask(ctx, roomID, targetRouteTarget, args)
	case "delete_task":
		return t.handleDeleteTask(ctx, roomID, targetRouteTarget, args.TaskID)
	case "enable_task":
		return t.handleSetTaskEnabled(ctx, roomID, targetRouteTarget, args.TaskID, true)
	case "disable_task":
		return t.handleSetTaskEnabled(ctx, roomID, targetRouteTarget, args.TaskID, false)
	case "set_interval":
		return t.handleSetTaskInterval(ctx, roomID, targetRouteTarget, args.TaskID, args.IntervalSeconds)
	default:
		return map[string]any{"ok": false, "error": "unsupported action"}, nil
	}
}

func (t *Tool) handleClearClaim(ctx context.Context, roomID, routeTarget string) (any, error) {
	if err := t.store.ClearRoomClaim(ctx, roomID, routeTarget); err != nil {
		return nil, err
	}
	room, err := t.store.GetRoom(ctx, roomID, routeTarget)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "room": room, "route_target": routeTarget}, nil
}

func (t *Tool) handleList(ctx context.Context, roomID, routeTarget string) (any, error) {
	room, err := t.store.EnsureRoom(ctx, roomID, routeTarget)
	if err != nil {
		return nil, err
	}
	tasks, err := t.store.ListTasks(ctx, roomID, routeTarget)
	if err != nil {
		return nil, err
	}
	plan := t.service.EvaluateRoom(time.Now().UTC(), room, tasks, routeTarget)
	return map[string]any{
		"ok":           true,
		"room":         room,
		"task_count":   len(tasks),
		"plan":         plan,
		"route_target": routeTarget,
	}, nil
}

func (t *Tool) handleConfigureRoom(ctx context.Context, roomID, routeTarget string, projectID *string, enabled *bool) (any, error) {
	room, err := t.store.EnsureRoom(ctx, roomID, routeTarget)
	if err != nil {
		return nil, err
	}
	if projectID != nil {
		cleanProjectID := strings.TrimSpace(*projectID)
		if cleanProjectID != "" {
			validatedProjectID, err := validation.ProjectID(cleanProjectID)
			if err != nil {
				return map[string]any{"ok": false, "error": "invalid project_id"}, nil
			}
			ctxProjectID, ok := sandbox.ProjectIDFromContext(ctx)
			if !ok || strings.TrimSpace(ctxProjectID) == "" {
				return map[string]any{"ok": false, "error": "project_id changes require an active project-scoped request"}, nil
			}
			if validatedProjectID != strings.TrimSpace(ctxProjectID) {
				return map[string]any{"ok": false, "error": "project_id must match the current request project context"}, nil
			}
			room.ProjectID = validatedProjectID
		} else {
			room.ProjectID = ""
		}
	}
	if enabled != nil {
		room.Enabled = *enabled
	}
	room.RouteTarget = routeTarget
	updated, err := t.store.UpsertRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "room": updated, "route_target": routeTarget}, nil
}

func (t *Tool) handleUpsertTask(ctx context.Context, roomID, routeTarget string, args toolArgs) (any, error) {
	if strings.TrimSpace(args.Title) == "" {
		return map[string]any{"ok": false, "error": "title is required"}, nil
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return map[string]any{"ok": false, "error": "prompt is required"}, nil
	}
	if args.IntervalSeconds <= 0 {
		return map[string]any{"ok": false, "error": "interval_seconds must be positive"}, nil
	}
	room, err := t.store.EnsureRoom(ctx, roomID, routeTarget)
	if err != nil {
		return nil, err
	}
	task := persistence.PulseTask{
		ID:              strings.TrimSpace(args.TaskID),
		RoomID:          roomID,
		RouteTarget:     routeTarget,
		Title:           strings.TrimSpace(args.Title),
		Prompt:          strings.TrimSpace(args.Prompt),
		IntervalSeconds: args.IntervalSeconds,
		Enabled:         true,
	}
	if args.Enabled != nil {
		task.Enabled = *args.Enabled
	}
	if task.ID != "" {
		existingTasks, err := t.store.ListTasks(ctx, roomID, routeTarget)
		if err != nil {
			return nil, err
		}
		for _, existing := range existingTasks {
			if existing.ID == task.ID {
				task.LastRunAt = existing.LastRunAt
				task.LastResultSummary = existing.LastResultSummary
				break
			}
		}
	}
	created, err := t.store.UpsertTask(ctx, task)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "room": room, "task": created, "route_target": routeTarget}, nil
}

func (t *Tool) handleDeleteTask(ctx context.Context, roomID, routeTarget, taskID string) (any, error) {
	if strings.TrimSpace(taskID) == "" {
		return map[string]any{"ok": false, "error": "task_id is required"}, nil
	}
	if err := t.store.DeleteTask(ctx, roomID, routeTarget, taskID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "deleted": strings.TrimSpace(taskID), "route_target": routeTarget}, nil
}

func (t *Tool) handleSetTaskEnabled(ctx context.Context, roomID, routeTarget, taskID string, enabled bool) (any, error) {
	updated, err := t.updateExistingTask(ctx, roomID, routeTarget, taskID, func(task persistence.PulseTask) persistence.PulseTask {
		task.Enabled = enabled
		return task
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "task": updated}, nil
}

func (t *Tool) handleSetTaskInterval(ctx context.Context, roomID, routeTarget, taskID string, intervalSeconds int) (any, error) {
	if intervalSeconds <= 0 {
		return map[string]any{"ok": false, "error": "interval_seconds must be positive"}, nil
	}
	updated, err := t.updateExistingTask(ctx, roomID, routeTarget, taskID, func(task persistence.PulseTask) persistence.PulseTask {
		task.IntervalSeconds = intervalSeconds
		return task
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "task": updated}, nil
}

func (t *Tool) updateExistingTask(ctx context.Context, roomID, routeTarget, taskID string, mutate func(persistence.PulseTask) persistence.PulseTask) (persistence.PulseTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return persistence.PulseTask{}, fmt.Errorf("task_id is required")
	}
	tasks, err := t.store.ListTasks(ctx, roomID, routeTarget)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			updated := mutate(task)
			return t.store.UpsertTask(ctx, updated)
		}
	}
	return persistence.PulseTask{}, persistence.ErrNotFound
}
