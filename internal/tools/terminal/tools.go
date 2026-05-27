package terminal

import (
	"context"
	"encoding/json"
	"fmt"
)

type startTool struct{ manager *Manager }
type readTool struct{ manager *Manager }
type writeTool struct{ manager *Manager }
type stopTool struct{ manager *Manager }
type listTool struct{ manager *Manager }

func NewStartTool(manager *Manager) *startTool { return &startTool{manager: manager} }
func NewReadTool(manager *Manager) *readTool   { return &readTool{manager: manager} }
func NewWriteTool(manager *Manager) *writeTool { return &writeTool{manager: manager} }
func NewStopTool(manager *Manager) *stopTool   { return &stopTool{manager: manager} }
func NewListTool(manager *Manager) *listTool   { return &listTool{manager: manager} }

func (t *startTool) Name() string { return "terminal_start" }
func (t *readTool) Name() string  { return "terminal_read" }
func (t *writeTool) Name() string { return "terminal_write" }
func (t *stopTool) Name() string  { return "terminal_stop" }
func (t *listTool) Name() string  { return "terminal_list" }

func (t *startTool) JSONSchema() map[string]any { return startSchema(t.manager) }
func (t *readTool) JSONSchema() map[string]any  { return readSchema(t.manager) }
func (t *writeTool) JSONSchema() map[string]any { return writeSchema() }
func (t *stopTool) JSONSchema() map[string]any  { return stopSchema() }
func (t *listTool) JSONSchema() map[string]any  { return listSchema() }

func (t *startTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args StartRequest
	if len(raw) == 0 {
		return StartResult{OK: false, Error: "command is required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.manager.Start(ctx, args)
	if err != nil {
		return StartResult{OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (t *readTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args ReadRequest
	if len(raw) == 0 {
		return ReadResult{OK: false, Error: "terminal_id is required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.manager.Read(ctx, args)
	if err != nil {
		return ReadResult{OK: false, Error: err.Error(), TerminalID: args.TerminalID}, nil
	}
	return res, nil
}

func (t *writeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args WriteRequest
	if len(raw) == 0 {
		return WriteResult{OK: false, Error: "terminal_id and input are required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.manager.Write(ctx, args)
	if err != nil {
		return WriteResult{OK: false, Error: err.Error(), TerminalID: args.TerminalID}, nil
	}
	return res, nil
}

func (t *stopTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args StopRequest
	if len(raw) == 0 {
		return StopResult{OK: false, Error: "terminal_id is required"}, nil
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.manager.Stop(ctx, args)
	if err != nil {
		return StopResult{OK: false, Error: err.Error(), TerminalID: args.TerminalID}, nil
	}
	return res, nil
}

func (t *listTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	res, err := t.manager.List(ctx)
	if err != nil {
		return ListResult{OK: false, Error: err.Error()}, nil
	}
	return res, nil
}
