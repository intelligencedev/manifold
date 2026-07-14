package observability

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	coreobs "manifold/internal/observability"
	persist "manifold/internal/persistence"
)

type LLMRequestListItem struct {
	ID                  string    `json:"id"`
	Provider            string    `json:"provider,omitempty"`
	Model               string    `json:"model"`
	SpecialistID        string    `json:"specialistId,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	InputTokens         int       `json:"inputTokens,omitempty"`
	OutputTokens        int       `json:"outputTokens,omitempty"`
	MaxContextTokens    int       `json:"maxContextTokens,omitempty"`
	Redacted            bool      `json:"redacted"`
	CallID              string    `json:"callId,omitempty"`
	ParentCallID        string    `json:"parentCallId,omitempty"`
	ParentUserMessageID string    `json:"parentUserMessageId,omitempty"`
}

type LLMRequestContextResponse struct {
	LLMRequestListItem
	Payload json.RawMessage `json:"payload"`
}

type LLMRequestDeps struct {
	Store      persist.LLMRequestStore
	Access     func(http.ResponseWriter, *http.Request) (*int64, bool)
	SetCORS    func(http.ResponseWriter, *http.Request, string)
	WriteJSON  func(http.ResponseWriter, any)
	StoreError func(http.ResponseWriter, *http.Request, error, string, string)
}

func item(req persist.LLMRequest) LLMRequestListItem {
	return LLMRequestListItem{ID: req.ID, Provider: req.Provider, Model: req.Model, SpecialistID: req.SpecialistID, CreatedAt: req.CreatedAt, InputTokens: req.InputTokens, OutputTokens: req.OutputTokens, MaxContextTokens: req.MaxContextTokens, Redacted: req.Redacted, CallID: req.CallID, ParentCallID: req.ParentCallID, ParentUserMessageID: req.ParentUserMessageID}
}

// HandleChatLLMRequests serves stored LLM calls for one chat message.
func HandleChatLLMRequests(w http.ResponseWriter, r *http.Request, deps LLMRequestDeps, userID *int64, sessionID, messageID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if deps.Store == nil {
		http.Error(w, "context inspector unavailable", http.StatusServiceUnavailable)
		return
	}
	if messageID == "" {
		http.NotFound(w, r)
		return
	}
	requests, err := deps.Store.ListLLMRequestsForMessage(r.Context(), userID, sessionID, messageID)
	if err != nil {
		deps.StoreError(w, r, err, sessionID, "list_llm_requests")
		return
	}
	out := make([]LLMRequestListItem, 0, len(requests))
	for _, request := range requests {
		out = append(out, item(request))
	}
	deps.WriteJSON(w, out)
}

// LLMRequestContextHandler serves the redacted context payload for one call.
func LLMRequestContextHandler(deps LLMRequestDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Access == nil || deps.Store == nil || deps.WriteJSON == nil || deps.StoreError == nil {
			http.Error(w, "context inspector unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, ok := deps.Access(w, r)
		if !ok {
			return
		}
		if deps.SetCORS != nil {
			deps.SetCORS(w, r, "GET, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/llm-requests/"), "/context"), "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		request, err := deps.Store.GetLLMRequest(r.Context(), userID, id)
		if err != nil {
			deps.StoreError(w, r, err, "", "get_llm_request")
			return
		}
		payload := request.Payload
		if !json.Valid(payload) {
			payload = []byte("{}")
		}
		deps.WriteJSON(w, LLMRequestContextResponse{LLMRequestListItem: item(request), Payload: coreobs.RedactJSON(payload)})
	}
}
