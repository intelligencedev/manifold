package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

type LLMPlanner struct {
	Client    *openai.Client
	ToolSpecs []ToolSpec
	SystemTpl string
}

func (p *LLMPlanner) Plan(ctx context.Context, goal string, mem []MemoryItem) ([]Step, error) {
	sys := fmt.Sprintf(p.SystemTpl, toJSON(p.ToolSpecs))
	user := fmt.Sprintf("Goal: %s", goal)

	stream, err := p.Client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:          "gpt-4o-mini",
		Stream:         true,
		Temperature:    0.0,
		MaxTokens:      1024,
		ResponseFormat: &openai.ChatCompletionResponseFormat{},
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
	})
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var buf []byte
	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		buf = append(buf, []byte(chunk.Choices[0].Delta.Content)...)
	}

	var out []Step
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, err
	}
	// assign deterministic IDs
	for i := range out {
		out[i].ID = uuid.NewString()
	}
	return out, nil
}

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
