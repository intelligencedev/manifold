package agentd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/config"
)

func TestInputRequestBrokerAnswerDeliversResponse(t *testing.T) {
	t.Parallel()

	broker := newInputRequestBroker()
	userID := int64(42)
	pending := &pendingInputRequest{
		request: inputrequest.Request{
			ID:       "req-1",
			Question: "Pick a target",
		},
		userID:   &userID,
		response: make(chan inputrequest.Response, 1),
	}
	if err := broker.register(pending); err != nil {
		t.Fatalf("register: %v", err)
	}

	otherUserID := int64(7)
	if _, _, _, err := broker.answer(&otherUserID, inputrequest.Response{RequestID: "req-1"}); !errors.Is(err, errInputRequestForbidden) {
		t.Fatalf("expected forbidden for different user, got %v", err)
	}

	req, _, _, err := broker.answer(&userID, inputrequest.Response{
		RequestID: "req-1",
		Answer:    "staging",
		ChoiceIDs: []string{"staging"},
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if req.ID != "req-1" {
		t.Fatalf("unexpected request returned: %+v", req)
	}

	select {
	case resp := <-pending.response:
		if resp.Answer != "staging" {
			t.Fatalf("unexpected answer: %+v", resp)
		}
		if resp.RespondedAt.IsZero() {
			t.Fatal("expected responded_at to be set")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broker response")
	}

	if _, _, _, err := broker.answer(&userID, inputrequest.Response{RequestID: "req-1"}); !errors.Is(err, errInputRequestNotFound) {
		t.Fatalf("expected not found after answer, got %v", err)
	}
}

func TestParseInputRequestAnswerPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		id   string
		ok   bool
	}{
		{name: "answer path", path: "/api/chat/input-requests/req-1/answer", id: "req-1", ok: true},
		{name: "trailing slash", path: "/api/chat/input-requests/req-1/answer/", id: "req-1", ok: true},
		{name: "missing id", path: "/api/chat/input-requests//answer", ok: false},
		{name: "wrong suffix", path: "/api/chat/input-requests/req-1", ok: false},
		{name: "wrong prefix", path: "/chat/input-requests/req-1/answer", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, ok := parseInputRequestAnswerPath(tt.path)
			if ok != tt.ok || id != tt.id {
				t.Fatalf("parseInputRequestAnswerPath(%q) = %q, %v; want %q, %v", tt.path, id, ok, tt.id, tt.ok)
			}
		})
	}
}

func TestChatInputRequestHandlerAnswersPendingRequest(t *testing.T) {
	t.Parallel()

	broker := newInputRequestBroker()
	a := &app{
		cfg:           &config.Config{},
		inputRequests: broker,
	}
	pending := &pendingInputRequest{
		request: inputrequest.Request{
			ID:       "req-handler",
			Question: "Which option?",
		},
		response: make(chan inputrequest.Response, 1),
	}
	if err := broker.register(pending); err != nil {
		t.Fatalf("register: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"answer":     "Use staging",
		"choice_ids": []string{" staging ", ""},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/input-requests/req-handler/answer", bytes.NewReader(body))

	a.chatInputRequestHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	select {
	case resp := <-pending.response:
		if resp.Answer != "Use staging" {
			t.Fatalf("unexpected answer: %+v", resp)
		}
		if len(resp.ChoiceIDs) != 1 || resp.ChoiceIDs[0] != "staging" {
			t.Fatalf("unexpected choice ids: %+v", resp.ChoiceIDs)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler response")
	}
}
