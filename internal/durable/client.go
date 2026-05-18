package durable

import (
	"context"
	"encoding/json"
	"time"
)

type Client struct {
	store Store
}

func NewClient(store Store) *Client {
	return &Client{store: store}
}

func (c *Client) Store() Store {
	if c == nil {
		return nil
	}
	return c.store
}

func (c *Client) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	if c == nil || c.store == nil {
		return SpawnResult{}, ErrNotFound
	}
	task, created, err := c.store.SpawnTask(ctx, req)
	if err != nil {
		return SpawnResult{}, err
	}
	return SpawnResult{TaskID: task.ID, Created: created}, nil
}

func (c *Client) FetchResult(ctx context.Context, userID int64, taskID string) (*ResultSnapshot, error) {
	if c == nil || c.store == nil {
		return nil, ErrNotFound
	}
	task, ok, err := c.store.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTaskNotFound
	}
	return &ResultSnapshot{TaskID: task.ID, State: task.Status, Result: task.Result, Failure: task.Failure, Error: task.Error}, nil
}

func (c *Client) AwaitResult(ctx context.Context, userID int64, taskID string, poll time.Duration) (*ResultSnapshot, error) {
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		snapshot, err := c.FetchResult(ctx, userID, taskID)
		if err != nil {
			return nil, err
		}
		if snapshot.IsTerminal() {
			return snapshot, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) EmitEvent(ctx context.Context, userID int64, queue, name string, payload map[string]any) (Event, error) {
	if c == nil || c.store == nil {
		return Event{}, ErrNotFound
	}
	return c.store.EmitEvent(ctx, userID, queue, name, payload)
}

func (c *Client) Cancel(ctx context.Context, userID int64, taskID string) error {
	if c == nil || c.store == nil {
		return ErrNotFound
	}
	return c.store.CancelTask(ctx, userID, taskID)
}

func (c *Client) ListEvents(ctx context.Context, userID int64, taskID string, afterSequence int64) ([]Event, TaskStatus, bool, error) {
	if c == nil || c.store == nil {
		return nil, "", false, ErrNotFound
	}
	return c.store.ListTaskEvents(ctx, userID, taskID, afterSequence)
}

func MarshalResult(value map[string]any) json.RawMessage {
	if value == nil {
		value = map[string]any{}
	}
	b, _ := json.Marshal(value)
	return b
}
