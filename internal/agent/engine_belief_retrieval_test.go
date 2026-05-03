package agent

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/agent/belief"
	"manifold/internal/llm"
	"manifold/internal/tools"
)

type captureMessageProvider struct {
	messages []llm.Message
}

func (p *captureMessageProvider) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	p.messages = append([]llm.Message(nil), msgs...)
	return llm.Message{Role: "assistant", Content: "done"}, nil
}

func (p *captureMessageProvider) ChatStream(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	p.messages = append([]llm.Message(nil), msgs...)
	handler.OnDelta("done")
	return nil
}

type staticBeliefRetriever struct {
	request belief.RetrievalRequest
	results []belief.SearchResult
}

func (r *staticBeliefRetriever) Retrieve(_ context.Context, request belief.RetrievalRequest) ([]belief.SearchResult, error) {
	r.request = request
	return r.results, nil
}

func TestRunInjectsBoundedBeliefMemory(t *testing.T) {
	t.Parallel()

	provider := &captureMessageProvider{}
	retriever := &staticBeliefRetriever{results: []belief.SearchResult{
		{Belief: belief.Belief{Statement: "Transit stores active coordination state.", Confidence: 0.82, EvidenceFor: 4}, Scope: belief.Scope{Kind: belief.ScopeKindObjective, Path: "project-1/objective-1"}},
		{Belief: belief.Belief{Statement: "Overflow belief should not enter the prompt.", Confidence: 0.8, EvidenceFor: 3}, Scope: belief.Scope{Kind: belief.ScopeKindProject, Path: "project-1"}},
	}}
	eng := &Engine{
		LLM:                       provider,
		Tools:                     tools.NewRegistry(),
		MaxSteps:                  1,
		System:                    "base system",
		UserID:                    7,
		ProjectID:                 "project-1",
		ObjectiveID:               "objective-1",
		SessionID:                 "session-1",
		AgentRole:                 "orchestrator",
		BeliefRetriever:           retriever,
		BeliefMaxBeliefsPerPrompt: 1,
		BeliefPromptTokenBudget:   200,
	}

	if _, err := eng.Run(context.Background(), "Transit", nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if retriever.request.TenantID != 7 || retriever.request.ProjectID != "project-1" || retriever.request.ObjectiveID != "objective-1" {
		t.Fatalf("unexpected retrieval request: %+v", retriever.request)
	}
	if len(provider.messages) == 0 || provider.messages[0].Role != "system" {
		t.Fatalf("expected captured system message, got %#v", provider.messages)
	}
	system := provider.messages[0].Content
	if !strings.Contains(system, "## Shared Belief Memory") || !strings.Contains(system, "Transit stores active coordination state") {
		t.Fatalf("expected belief memory section in system prompt, got %q", system)
	}
	if strings.Contains(system, "Overflow belief") {
		t.Fatalf("expected overflow belief to be omitted, got %q", system)
	}
}

func TestRunInjectsBeliefMemoryForSystemUser(t *testing.T) {
	t.Parallel()

	provider := &captureMessageProvider{}
	retriever := &staticBeliefRetriever{results: []belief.SearchResult{
		{Belief: belief.Belief{Statement: "System-user memory is available.", Confidence: 0.9, EvidenceFor: 2}, Scope: belief.Scope{Kind: belief.ScopeKindObjective, Path: "project-1/objective-1"}},
	}}
	eng := &Engine{
		LLM:                       provider,
		Tools:                     tools.NewRegistry(),
		MaxSteps:                  1,
		System:                    "base system",
		UserID:                    0,
		ProjectID:                 "project-1",
		ObjectiveID:               "objective-1",
		SessionID:                 "session-1",
		BeliefRetriever:           retriever,
		BeliefMaxBeliefsPerPrompt: 3,
		BeliefPromptTokenBudget:   200,
	}

	if _, err := eng.Run(context.Background(), "Transit", nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if retriever.request.TenantID != 0 || retriever.request.UserID != 0 {
		t.Fatalf("expected system-user retrieval request, got %+v", retriever.request)
	}
	if len(provider.messages) == 0 || !strings.Contains(provider.messages[0].Content, "System-user memory is available") {
		t.Fatalf("expected belief memory to be injected for system user, got %#v", provider.messages)
	}
}
