package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"manifold/internal/agent/harness"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"
)

const proxyTerminalToolName = "agent_response"

type openAIChatCompletionRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Tools    []openAITool        `json:"tools,omitempty"`
	Stream   bool                `json:"stream,omitempty"`
}

type openAIChatMessage struct {
	Role       string                `json:"role"`
	Content    any                   `json:"content,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIProxyToolCall `json:"tool_calls,omitempty"`
}

type openAITool struct {
	Type     string               `json:"type"`
	Function openAIToolDefinition `json:"function"`
}

type openAIToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIProxyToolCall struct {
	ID       string                      `json:"id"`
	Type     string                      `json:"type"`
	Function openAIProxyToolCallFunction `json:"function"`
}

type openAIProxyToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatCompletionResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []openAICompletionChoice `json:"choices"`
}

type openAIChatCompletionChunk struct {
	ID      string                        `json:"id"`
	Object  string                        `json:"object"`
	Created int64                         `json:"created"`
	Model   string                        `json:"model"`
	Choices []openAICompletionChunkChoice `json:"choices"`
}

type openAICompletionChunkChoice struct {
	Index        int                        `json:"index"`
	Delta        openAICompletionChunkDelta `json:"delta"`
	FinishReason *string                    `json:"finish_reason"`
}

type openAICompletionChunkDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   string                `json:"content,omitempty"`
	ToolCalls []openAIProxyToolCall `json:"tool_calls,omitempty"`
}

type openAICompletionChoice struct {
	Index        int                           `json:"index"`
	Message      openAICompletionChoiceMessage `json:"message"`
	FinishReason string                        `json:"finish_reason"`
}

type openAICompletionChoiceMessage struct {
	Role      string                `json:"role"`
	Content   string                `json:"content,omitempty"`
	ToolCalls []openAIProxyToolCall `json:"tool_calls,omitempty"`
}

func (a *app) openAIChatCompletionsProxyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a == nil || a.llm == nil {
			http.Error(w, "llm provider unavailable", http.StatusServiceUnavailable)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		defer r.Body.Close()
		var req openAIChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		model := strings.TrimSpace(req.Model)
		if model == "" {
			model = a.proxyDefaultModel()
		}
		messages := openAIProxyMessages(req.Messages)
		schemas := openAIProxyToolSchemas(req.Tools)
		schemas = append(schemas, proxyTerminalToolSchema())
		runCfg := openAIProxyHarnessConfig(a.cfg)

		if req.Stream {
			result, err := harness.RunStreamInference(r.Context(), a.llm, harness.WrapMessages(messages), schemas, model, runCfg, harness.NewStepTracker())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			openAIProxyStreamResponse(w, model, result.Message)
			return
		}

		result, err := harness.RunInference(r.Context(), a.llm, harness.WrapMessages(messages), schemas, model, runCfg, harness.NewStepTracker())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		resp := openAIProxyCompletionResponse(model, result.Message)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (a *app) proxyDefaultModel() string {
	if a != nil && a.engine != nil && strings.TrimSpace(a.engine.Model) != "" {
		return strings.TrimSpace(a.engine.Model)
	}
	if a != nil && a.cfg != nil && strings.TrimSpace(a.cfg.OpenAI.Model) != "" {
		return strings.TrimSpace(a.cfg.OpenAI.Model)
	}
	return "model"
}

func openAIProxyHarnessConfig(cfg *config.Config) harness.RunConfig {
	source := config.HarnessConfig{
		Mode:              "workflow",
		RescueEnabled:     true,
		MaxRetriesPerStep: 3,
		MaxToolErrors:     2,
		TerminalTools:     []string{proxyTerminalToolName},
	}
	if cfg != nil {
		if cfg.Harness.MaxRetriesPerStep > 0 {
			source.MaxRetriesPerStep = cfg.Harness.MaxRetriesPerStep
		}
		if cfg.Harness.MaxToolErrors > 0 {
			source.MaxToolErrors = cfg.Harness.MaxToolErrors
		}
		if cfg.Harness.RescueEnabled {
			source.RescueEnabled = true
		}
	}
	return harnessRunConfig(source)
}

func openAIProxyMessages(messages []openAIChatMessage) []llm.Message {
	out := make([]llm.Message, 0, len(messages)+1)
	out = append(out, llm.Message{
		Role:    "system",
		Content: "When a final answer is ready, call the hidden agent_response tool with the final text. Use the caller's tools only when the caller needs to execute a real tool.",
	})
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			continue
		}
		out = append(out, llm.Message{
			Role:      role,
			Content:   openAIProxyContentString(message.Content),
			ToolID:    strings.TrimSpace(message.ToolCallID),
			ToolCalls: openAIProxyLLMToolCalls(message.ToolCalls),
		})
	}
	return out
}

func openAIProxyContentString(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func openAIProxyLLMToolCalls(calls []openAIProxyToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		args := json.RawMessage(strings.TrimSpace(call.Function.Arguments))
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		out = append(out, llm.ToolCall{ID: strings.TrimSpace(call.ID), Name: name, Args: args})
	}
	return out
}

func openAIProxyToolSchemas(in []openAITool) []llm.ToolSchema {
	out := make([]llm.ToolSchema, 0, len(in)+1)
	for _, tool := range in {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" || name == proxyTerminalToolName {
			continue
		}
		out = append(out, llm.ToolSchema{
			Name:        name,
			Description: strings.TrimSpace(tool.Function.Description),
			Parameters:  tool.Function.Parameters,
		})
	}
	return out
}

func proxyTerminalToolSchema() llm.ToolSchema {
	reg := tools.NewRegistry()
	reg.Register(utility.NewAgentResponseTool())
	schemas := reg.Schemas()
	if len(schemas) == 0 {
		return llm.ToolSchema{Name: proxyTerminalToolName, Parameters: map[string]any{"type": "object"}}
	}
	return schemas[0]
}

func openAIProxyCompletionResponse(model string, message llm.Message) openAIChatCompletionResponse {
	choice := openAICompletionChoice{
		Index: 0,
		Message: openAICompletionChoiceMessage{
			Role: "assistant",
		},
		FinishReason: "stop",
	}
	if final, ok := proxyTerminalToolText(message); ok {
		choice.Message.Content = final
	} else if len(message.ToolCalls) > 0 {
		choice.Message.ToolCalls = openAIProxyToolCalls(message.ToolCalls)
		choice.FinishReason = "tool_calls"
	} else {
		choice.Message.Content = message.Content
	}
	return openAIChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-manifold-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []openAICompletionChoice{choice},
	}
}

func openAIProxyStreamResponse(w http.ResponseWriter, model string, message llm.Message) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := fmt.Sprintf("chatcmpl-manifold-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	finishReason := "stop"
	delta := openAICompletionChunkDelta{Role: "assistant"}
	if final, ok := proxyTerminalToolText(message); ok {
		delta.Content = final
	} else if len(message.ToolCalls) > 0 {
		delta.ToolCalls = openAIProxyToolCalls(message.ToolCalls)
		finishReason = "tool_calls"
	} else {
		delta.Content = message.Content
	}
	openAIProxyWriteStreamChunk(w, flusher, openAIChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAICompletionChunkChoice{{Index: 0, Delta: delta}},
	})
	openAIProxyWriteStreamChunk(w, flusher, openAIChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAICompletionChunkChoice{{Index: 0, Delta: openAICompletionChunkDelta{}, FinishReason: &finishReason}},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func openAIProxyWriteStreamChunk(w http.ResponseWriter, flusher http.Flusher, chunk openAIChatCompletionChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher != nil {
		flusher.Flush()
	}
}

func proxyTerminalToolText(message llm.Message) (string, bool) {
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Name != proxyTerminalToolName {
		return "", false
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(message.ToolCalls[0].Args, &payload); err != nil {
		return "", false
	}
	return payload.Text, true
}

func openAIProxyToolCalls(calls []llm.ToolCall) []openAIProxyToolCall {
	out := make([]openAIProxyToolCall, 0, len(calls))
	for i, call := range calls {
		if call.Name == proxyTerminalToolName {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		out = append(out, openAIProxyToolCall{
			ID:   id,
			Type: "function",
			Function: openAIProxyToolCallFunction{
				Name:      call.Name,
				Arguments: string(call.Args),
			},
		})
	}
	return out
}
