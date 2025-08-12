package llm

import (
	"context"
	"encoding/json"
)

type ToolCall struct {
	Name string
	Args json.RawMessage
	ID   string
}

type Message struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
	ToolID  string
	// ToolCalls are only set on assistant messages
	ToolCalls []ToolCall
}

type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type StreamHandler interface {
	OnDelta(content string)
	OnToolCall(tc ToolCall)
}

type Provider interface {
	Chat(ctx context.Context, msgs []Message, tools []ToolSchema, model string) (Message, error)
	ChatStream(ctx context.Context, msgs []Message, tools []ToolSchema, model string, h StreamHandler) error
}
