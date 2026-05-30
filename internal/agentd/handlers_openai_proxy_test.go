package agentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm"
)

func TestOpenAIChatCompletionsProxyStripsTerminalTool(t *testing.T) {
	t.Parallel()

	provider := &agentRunScriptedProvider{responses: []llm.Message{
		{Role: "assistant", Content: "plain text first"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-final", Name: "agent_response", Args: json.RawMessage(`{"text":"proxy complete"}`)}}},
	}}
	a := &app{
		cfg: &config.Config{
			OpenAI:  config.OpenAIConfig{Model: "fallback-model"},
			Harness: config.HarnessConfig{MaxRetriesPerStep: 3},
		},
		llm: provider,
	}

	body := bytes.NewBufferString(`{"model":"proxy-model","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.openAIChatCompletionsProxyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp openAIChatCompletionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "proxy-model" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "proxy complete" {
		t.Fatalf("unexpected proxy response: %+v", resp)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 0 || resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected terminal tool to be stripped, got %+v", resp.Choices[0])
	}
}

func TestOpenAIChatCompletionsProxyReturnsRealToolCalls(t *testing.T) {
	t.Parallel()

	provider := &agentRunScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-lookup", Name: "lookup", Args: json.RawMessage(`{"query":"forge"}`)}}},
	}}
	a := &app{
		cfg: &config.Config{OpenAI: config.OpenAIConfig{Model: "fallback-model"}},
		llm: provider,
	}

	body := bytes.NewBufferString(`{
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"type":"function","function":{"name":"lookup","description":"Lookup facts","parameters":{"type":"object"}}}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.openAIChatCompletionsProxyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp openAIChatCompletionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Model != "fallback-model" {
		t.Fatalf("unexpected fallback model: %q", resp.Model)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool-call response, got %+v", resp)
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments != `{"query":"forge"}` {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
}

func TestOpenAIChatCompletionsProxyStreamsTerminalText(t *testing.T) {
	t.Parallel()

	provider := &agentRunScriptedProvider{streamResponses: []agentRunStreamResponse{
		{Deltas: []string{"plain text first"}},
		{ToolCalls: []llm.ToolCall{{ID: "call-final", Name: "agent_response", Args: json.RawMessage(`{"text":"streamed proxy"}`)}}},
	}}
	a := &app{
		cfg: &config.Config{OpenAI: config.OpenAIConfig{Model: "fallback-model"}},
		llm: provider,
	}

	body := bytes.NewBufferString(`{"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.openAIChatCompletionsProxyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	bodyText := rr.Body.String()
	for _, want := range []string{`"object":"chat.completion.chunk"`, `"content":"streamed proxy"`, `"finish_reason":"stop"`, `data: [DONE]`} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("expected stream body to contain %s, got %s", want, bodyText)
		}
	}
	if strings.Contains(bodyText, "plain text first") {
		t.Fatalf("invalid pre-retry stream delta leaked: %s", bodyText)
	}
}
