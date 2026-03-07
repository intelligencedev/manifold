package matrixroom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/sandbox"
	"manifold/internal/tools"
)

const toolName = "matrix_room_message"

type toolArgs struct {
	Text string `json:"text"`
}

type toolResponse struct {
	OK     bool   `json:"ok"`
	Queued bool   `json:"queued"`
	RoomID string `json:"room_id,omitempty"`
	Text   string `json:"text,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Tool struct{}

func New() tools.Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        toolName,
		"description": "Queue a plain-text message for the current Matrix room. Use this only when the current task explicitly requires notifying the room.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Plain-text message to send to the current Matrix room.",
				},
			},
			"required": []string{"text"},
		},
	}
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args toolArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return toolResponse{OK: false, Queued: false, Error: fmt.Sprintf("invalid arguments: %v", err)}, nil
		}
	}
	roomID, ok := sandbox.RoomIDFromContext(ctx)
	if !ok {
		return toolResponse{OK: false, Queued: false, Error: "matrix_room_message requires a room-scoped request"}, nil
	}
	outbox, ok := sandbox.MatrixOutboxFromContext(ctx)
	if !ok {
		return toolResponse{OK: false, Queued: false, Error: "matrix outbox unavailable"}, nil
	}
	text := strings.TrimSpace(args.Text)
	if text == "" {
		return toolResponse{OK: false, Queued: false, Error: "text is required"}, nil
	}
	outbox.Add(roomID, text)
	return toolResponse{OK: true, Queued: true, RoomID: roomID, Text: text}, nil
}
