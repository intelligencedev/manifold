package agent

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/llm"
)

type LLMRequestSnapshot struct {
	ID               string
	Messages         []llm.Message
	Tools            []llm.ToolSchema
	Model            string
	Provider         string
	InputTokens      int
	MaxContextTokens int
	CreatedAt        time.Time
}

func (e *Engine) emitLLMRequest(ctx context.Context, msgs []llm.Message, schemas []llm.ToolSchema, model string) string {
	if e.OnLLMRequest == nil {
		return ""
	}
	id := uuid.NewString()
	e.OnLLMRequest(LLMRequestSnapshot{
		ID:               id,
		Messages:         cloneLLMMessages(msgs),
		Tools:            cloneToolSchemas(schemas),
		Model:            strings.TrimSpace(model),
		Provider:         providerName(e.LLM),
		InputTokens:      e.countSnapshotTokens(ctx, msgs),
		MaxContextTokens: e.contextWindowForSnapshot(model),
		CreatedAt:        time.Now().UTC(),
	})
	return id
}

func (e *Engine) countSnapshotTokens(ctx context.Context, msgs []llm.Message) int {
	if e.Tokenizer != nil {
		if n, err := e.Tokenizer.CountMessagesTokens(ctx, msgs); err == nil {
			return n
		}
	}
	if p, ok := e.LLM.(llm.TokenizableProvider); ok {
		if tok := p.Tokenizer(); tok != nil {
			if n, err := tok.CountMessagesTokens(ctx, msgs); err == nil {
				return n
			}
		}
	}
	return llm.EstimateTokensForMessages(msgs)
}

func (e *Engine) contextWindowForSnapshot(model string) int {
	if e.ContextWindowTokens > 0 {
		return e.ContextWindowTokens
	}
	if size, _ := llm.ContextSize(model); size > 0 {
		return size
	}
	if size, _ := llm.ContextSize(e.Model); size > 0 {
		return size
	}
	return 0
}

func providerName(provider llm.Provider) string {
	if provider == nil {
		return ""
	}
	t := reflect.TypeOf(provider)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() != "" {
		return t.Name()
	}
	return t.String()
}

func cloneLLMMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, msg := range msgs {
		out[i] = msg
		if len(msg.ToolCalls) > 0 {
			out[i].ToolCalls = append([]llm.ToolCall(nil), msg.ToolCalls...)
			for j := range out[i].ToolCalls {
				if msg.ToolCalls[j].Args != nil {
					out[i].ToolCalls[j].Args = append([]byte(nil), msg.ToolCalls[j].Args...)
				}
			}
		}
		if len(msg.Images) > 0 {
			out[i].Images = append([]llm.GeneratedImage(nil), msg.Images...)
		}
		if len(msg.Videos) > 0 {
			out[i].Videos = append([]llm.GeneratedVideo(nil), msg.Videos...)
		}
	}
	return out
}

func cloneToolSchemas(schemas []llm.ToolSchema) []llm.ToolSchema {
	out := make([]llm.ToolSchema, len(schemas))
	for i, schema := range schemas {
		out[i] = schema
		if schema.Parameters != nil {
			out[i].Parameters = cloneMap(schema.Parameters)
		}
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
