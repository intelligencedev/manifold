package agent

import (
    "context"
    "fmt"

    "gptagent/internal/llm"
    "gptagent/internal/tools"
)

type Engine struct {
    LLM      llm.Provider
    Tools    tools.Registry
    MaxSteps int
    System   string
}

// Run executes the agent loop until the model produces a final answer.
func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
    msgs := BuildInitialLLMMessages(e.System, userInput, history)

    var final string
    for step := 0; step < e.MaxSteps; step++ {
        msg, err := e.LLM.Chat(ctx, msgs, e.Tools.Schemas(), e.model())
        if err != nil { return "", err }
        msgs = append(msgs, msg)
        if len(msg.ToolCalls) == 0 { final = msg.Content; break }
        for _, tc := range msg.ToolCalls {
            payload, err := e.Tools.Dispatch(ctx, tc.Name, tc.Args)
            if err != nil { payload = []byte(fmt.Sprintf(`{"error":%q}`, err.Error())) }
            msgs = append(msgs, llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID})
        }
    }
    if final == "" { final = "(no final text — increase max steps or check logs)" }
    return final, nil
}

func (e *Engine) model() string { return "" }

// Message exists for future agent-level message modeling.
// Message type removed in favor of llm.Message throughout the engine API.
