package llm

import (
	"context"
	"encoding/json"
)

type ToolCall struct {
	Name string
	Args json.RawMessage
	ID   string
	// ThoughtSignature carries provider-specific context (Gemini 3) that must be
	// echoed back on subsequent turns to keep function calling valid.
	ThoughtSignature string
}

// GeneratedImage represents an image payload returned by the model.
// Data holds the raw bytes (already decoded from base64), and MIMEType
// should be a valid image MIME like image/png or image/jpeg.
type GeneratedImage struct {
	Data     []byte
	MIMEType string
}

type Message struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
	ToolID  string
	// ToolCalls are only set on assistant messages
	ToolCalls []ToolCall
	// Images captures inline image payloads returned by the provider.
	Images []GeneratedImage
	// Compaction carries responses API compaction state when available.
	Compaction *CompactionItem
}

type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type StreamHandler interface {
	OnDelta(content string)
	OnToolCall(tc ToolCall)
	OnImage(img GeneratedImage)
}

type Provider interface {
	Chat(ctx context.Context, msgs []Message, tools []ToolSchema, model string) (Message, error)
	ChatStream(ctx context.Context, msgs []Message, tools []ToolSchema, model string, h StreamHandler) error
}
