package durable

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type taskContextKey struct{}

type TaskContext struct {
	Task   Task
	Run    Run
	Store  Store
	Client *Client
}

func WithTaskContext(ctx context.Context, tc TaskContext) context.Context {
	return context.WithValue(ctx, taskContextKey{}, tc)
}

func FromContext(ctx context.Context) (TaskContext, bool) {
	tc, ok := ctx.Value(taskContextKey{}).(TaskContext)
	return tc, ok
}

func Step[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		return fn(ctx)
	}
	key := strings.TrimSpace(name)
	if key == "" {
		return zero, fmt.Errorf("durable step name required")
	}
	if raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key); err != nil {
		return zero, err
	} else if found {
		var out T
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &out); err != nil {
				return zero, err
			}
		}
		return out, nil
	}
	value, err := fn(ctx)
	if err != nil {
		return zero, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	saved, err := tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, raw)
	if err != nil {
		return zero, err
	}
	var out T
	if len(saved) > 0 {
		if err := json.Unmarshal(saved, &out); err != nil {
			return zero, err
		}
	}
	return out, nil
}

func SleepFor(ctx context.Context, name string, d time.Duration) error {
	return SleepUntil(ctx, name, time.Now().UTC().Add(d))
}

func SleepUntil(ctx context.Context, name string, wakeAt time.Time) error {
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		timer := time.NewTimer(time.Until(wakeAt))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	key := "sleep:" + strings.TrimSpace(name)
	if _, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key); err != nil {
		return err
	} else if found {
		return nil
	}
	now := time.Now().UTC()
	if wakeAt.After(now) {
		w := Wait{ID: waitID(tc.Task.ID, WaitKindTimer, key), TaskID: tc.Task.ID, RunID: tc.Run.ID, Kind: WaitKindTimer, WakeAt: &wakeAt, Status: "waiting"}
		wait, err := tc.Store.CreateWait(ctx, w)
		if err != nil {
			return err
		}
		if wait.Status == "fired" {
			_, err := tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, []byte(`true`))
			return err
		}
		return ErrSuspended
	}
	_, err := tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, []byte(`true`))
	return err
}

func AwaitEvent[T any](ctx context.Context, eventName string, timeout time.Duration) (T, error) {
	var zero T
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		return zero, fmt.Errorf("durable event wait requires task context")
	}
	eventName = strings.TrimSpace(eventName)
	if eventName == "" {
		return zero, fmt.Errorf("event name required")
	}
	key := "event:" + eventName
	if raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key); err != nil {
		return zero, err
	} else if found {
		var out T
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &out); err != nil {
				return zero, err
			}
		}
		return out, nil
	}
	w := Wait{ID: waitID(tc.Task.ID, WaitKindEvent, eventName), TaskID: tc.Task.ID, RunID: tc.Run.ID, Kind: WaitKindEvent, EventName: eventName, Status: "waiting"}
	if timeout > 0 {
		wakeAt := time.Now().UTC().Add(timeout)
		w.WakeAt = &wakeAt
	}
	wait, err := tc.Store.CreateWait(ctx, w)
	if err != nil {
		return zero, err
	}
	if wait.Status == "fired" {
		return zero, fmt.Errorf("event wait timed out: %s", eventName)
	}
	return zero, ErrSuspended
}

func SpawnChild(ctx context.Context, name string, req SpawnRequest) (SpawnResult, error) {
	tc, ok := FromContext(ctx)
	if !ok || tc.Client == nil || tc.Store == nil {
		return SpawnResult{}, fmt.Errorf("durable child spawn requires task context")
	}
	key := "child:" + strings.TrimSpace(name)
	if raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key); err != nil {
		return SpawnResult{}, err
	} else if found {
		var out SpawnResult
		if err := json.Unmarshal(raw, &out); err != nil {
			return SpawnResult{}, err
		}
		return out, nil
	}
	req.ParentTaskID = tc.Task.ID
	req.ParentRunID = tc.Run.ID
	if req.UserID == 0 {
		req.UserID = tc.Task.UserID
	}
	result, err := tc.Client.Spawn(ctx, req)
	if err != nil {
		return SpawnResult{}, err
	}
	raw, _ := json.Marshal(result)
	saved, err := tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, raw)
	if err != nil {
		return SpawnResult{}, err
	}
	var out SpawnResult
	if err := json.Unmarshal(saved, &out); err != nil {
		return SpawnResult{}, err
	}
	return out, nil
}

func AwaitChild(ctx context.Context, childTaskID string) (*ResultSnapshot, error) {
	tc, ok := FromContext(ctx)
	if !ok || tc.Client == nil || tc.Store == nil {
		return nil, fmt.Errorf("durable child wait requires task context")
	}
	childTaskID = strings.TrimSpace(childTaskID)
	if childTaskID == "" {
		return nil, fmt.Errorf("child task id required")
	}
	if childTaskID == tc.Task.ID {
		return nil, ErrDeadlock
	}
	key := "child-result:" + childTaskID
	if raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key); err != nil {
		return nil, err
	} else if found {
		var out ResultSnapshot
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	snapshot, err := tc.Client.FetchResult(ctx, tc.Task.UserID, childTaskID)
	if err == nil && snapshot.IsTerminal() {
		raw, _ := json.Marshal(snapshot)
		saved, saveErr := tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, raw)
		if saveErr != nil {
			return nil, saveErr
		}
		var out ResultSnapshot
		if err := json.Unmarshal(saved, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	if err != nil && err != ErrTaskNotFound {
		return nil, err
	}
	w := Wait{ID: waitID(tc.Task.ID, WaitKindChild, childTaskID), TaskID: tc.Task.ID, RunID: tc.Run.ID, Kind: WaitKindChild, ChildTaskID: childTaskID, Status: "waiting"}
	if _, err := tc.Store.CreateWait(ctx, w); err != nil {
		return nil, err
	}
	return nil, ErrSuspended
}

func Heartbeat(ctx context.Context, d time.Duration) error {
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		return nil
	}
	return tc.Store.Heartbeat(ctx, tc.Task.ID, tc.Run.ID, tc.Run.WorkerID, time.Now().UTC().Add(d))
}

func RecordEvent(ctx context.Context, name string, payload map[string]any) (Event, error) {
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		return Event{}, fmt.Errorf("durable event record requires task context")
	}
	return tc.Store.AppendTaskEvent(ctx, tc.Task.ID, name, payload)
}

func RecordEventOnce(ctx context.Context, eventKey string, name string, payload map[string]any) (Event, error) {
	tc, ok := FromContext(ctx)
	if !ok || tc.Store == nil {
		return Event{}, fmt.Errorf("durable event record requires task context")
	}
	return tc.Store.AppendTaskEventOnce(ctx, tc.Task.ID, eventKey, name, payload)
}

func waitID(taskID string, kind WaitKind, key string) string {
	sum := sha1.Sum([]byte(taskID + "\x00" + string(kind) + "\x00" + key))
	return taskID + ":" + string(kind) + ":" + hex.EncodeToString(sum[:8])
}
