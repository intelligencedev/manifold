package inputrequest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentinput "manifold/internal/agent/inputrequest"
)

type fakeRequester struct {
	request  agentinput.Request
	response agentinput.Response
}

func (f *fakeRequester) RequestInfo(ctx context.Context, req agentinput.Request) (agentinput.Response, error) {
	f.request = req
	resp := f.response
	if resp.RequestID == "" {
		resp.RequestID = req.ID
	}
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = time.Now().UTC()
	}
	return resp, nil
}

func TestToolCallRequestsInformation(t *testing.T) {
	t.Parallel()

	requester := &fakeRequester{
		response: agentinput.Response{
			Answer:    "Use the staging database.",
			ChoiceIDs: []string{"staging"},
		},
	}
	ctx := agentinput.WithRequester(context.Background(), requester)
	ctx = agentinput.WithRunMetadata(ctx, agentinput.RunMetadata{
		Agent:        "architect",
		Model:        "gpt-test",
		CallID:       "call-1",
		ParentCallID: "parent-1",
		Depth:        3,
	})

	respAny, err := New().Call(ctx, json.RawMessage(`{
		"question": "Which database should I use?",
		"reason": "The migration target is ambiguous.",
		"choices": [
			{"id": "staging", "label": "Staging", "description": "Use the staging DB"},
			"Production"
		],
		"allow_free_text": true
	}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}

	if requester.request.ID == "" {
		t.Fatal("expected request id to be generated")
	}
	if requester.request.Question != "Which database should I use?" {
		t.Fatalf("unexpected question: %q", requester.request.Question)
	}
	if requester.request.Reason != "The migration target is ambiguous." {
		t.Fatalf("unexpected reason: %q", requester.request.Reason)
	}
	if !requester.request.AllowFreeText {
		t.Fatal("expected free text to be allowed")
	}
	if requester.request.Agent != "architect" || requester.request.Model != "gpt-test" {
		t.Fatalf("unexpected metadata: %+v", requester.request)
	}
	if requester.request.CallID != "call-1" || requester.request.ParentCallID != "parent-1" || requester.request.Depth != 3 {
		t.Fatalf("unexpected call metadata: %+v", requester.request)
	}
	if len(requester.request.Choices) != 2 {
		t.Fatalf("expected 2 choices, got %d", len(requester.request.Choices))
	}
	if requester.request.Choices[0].ID != "staging" || requester.request.Choices[0].Label != "Staging" {
		t.Fatalf("unexpected first choice: %+v", requester.request.Choices[0])
	}
	if requester.request.Choices[1].ID != "choice_2" || requester.request.Choices[1].Label != "Production" {
		t.Fatalf("unexpected second choice: %+v", requester.request.Choices[1])
	}

	resp, ok := respAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type: %T", respAny)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if resp["request_id"] != requester.request.ID {
		t.Fatalf("expected request id %q, got %q", requester.request.ID, resp["request_id"])
	}
	if resp["answer"] != "Use the staging database." {
		t.Fatalf("unexpected answer: %+v", resp)
	}
}

func TestToolCallRequiresRequester(t *testing.T) {
	t.Parallel()

	respAny, err := New().Call(context.Background(), json.RawMessage(`{"question":"Continue?"}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	resp, ok := respAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type: %T", respAny)
	}
	if resp["ok"] != false {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
}

func TestToolCallDefaultsToFreeTextWithoutChoices(t *testing.T) {
	t.Parallel()

	requester := &fakeRequester{}
	ctx := agentinput.WithRequester(context.Background(), requester)

	_, err := New().Call(ctx, json.RawMessage(`{"question":"What path should I inspect?"}`))
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if !requester.request.AllowFreeText {
		t.Fatal("expected free text by default when choices are omitted")
	}
}
