package llmtool

import (
	"context"
	"encoding/json"
	"fmt"

	"singularityio/internal/llm"
)

// Transform is a generic LLM tool that transforms input text according to an
// instruction. It uses the project's llm.Provider and returns a simple output.
type ProviderFactory func(baseURL string) llm.Provider

type Transform struct {
	Provider       llm.Provider
	DefaultModel   string
	NewWithBaseURL ProviderFactory // optional: build a new provider for a given base URL
}

func NewTransform(p llm.Provider, defaultModel string, f ProviderFactory) *Transform {
	return &Transform{Provider: p, DefaultModel: defaultModel, NewWithBaseURL: f}
}

func (t *Transform) Name() string { return "llm_transform" }

func (t *Transform) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Use the LLM to transform input text according to an instruction (summarize, synthesize, rewrite, extract, etc.)",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"instruction": map[string]any{"type": "string", "description": "High-level instruction for the model (e.g., 'Write an executive summary')"},
				"input":       map[string]any{"type": "string", "description": "Input text or context to transform"},
				"system":      map[string]any{"type": "string", "description": "Optional system prompt"},
				"model":       map[string]any{"type": "string", "description": "Optional model override"},
				"base_url":    map[string]any{"type": "string", "description": "Optional API base URL override (e.g., for a different gateway)"},
			},
			"required": []string{"instruction"},
		},
	}
}

func (t *Transform) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Instruction string `json:"instruction"`
		Input       string `json:"input"`
		System      string `json:"system"`
		Model       string `json:"model"`
		BaseURL     string `json:"base_url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	// Build simple message list
	msgs := make([]llm.Message, 0, 2)
	sys := args.System
	if sys == "" {
		sys = "You are a helpful expert writer. Follow the user's instruction precisely and keep the output clean and concise unless asked for more detail."
	}
	msgs = append(msgs, llm.Message{Role: "system", Content: sys})

	user := fmt.Sprintf("Instruction:\n%s\n\n---\n\nINPUT:\n%s\n", args.Instruction, args.Input)
	msgs = append(msgs, llm.Message{Role: "user", Content: user})

	// Choose provider: same as agent by default; override if base_url provided and factory available
	p := t.Provider
	if args.BaseURL != "" && t.NewWithBaseURL != nil {
		if np := t.NewWithBaseURL(args.BaseURL); np != nil {
			p = np
		}
	}

	model := args.Model
	if model == "" {
		model = t.DefaultModel
	}
	outMsg, err := p.Chat(ctx, msgs, nil, model)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "output": outMsg.Content}, nil
}
