package provider

import (
	"context"

	"intelligence.dev/internal/llm"
)

// LLMAdapter wraps the shared llm.Provider and adapts it to the playground provider interface.
type LLMAdapter struct {
	provider llm.Provider
	model    string
}

// NewLLMAdapter builds a new adapter with the given provider and default model.
func NewLLMAdapter(provider llm.Provider, model string) *LLMAdapter {
	return &LLMAdapter{provider: provider, model: model}
}

// Name returns the underlying provider type identifier.
func (a *LLMAdapter) Name() string {
	return "llm"
}

// Complete renders the prompt as a single user message and invokes the llm provider.
func (a *LLMAdapter) Complete(ctx context.Context, req Request) (Response, error) {
	msgs := []llm.Message{{Role: "user", Content: req.Prompt}}
	model := req.Model
	if model == "" {
		model = a.model
	}
	msg, err := a.provider.Chat(ctx, msgs, nil, model)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Output:       msg.Content,
		Tokens:       len(msg.Content) / 4,
		Latency:      0,
		ProviderName: a.Name(),
	}, nil
}
