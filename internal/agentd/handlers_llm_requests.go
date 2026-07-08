package agentd

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/observability"
)

type llmRequestListItem struct {
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

type llmRequestContextResponse struct {
	llmRequestListItem
	Payload json.RawMessage `json:"payload"`
}

func (a *app) handleChatLLMRequests(w http.ResponseWriter, r *http.Request, userID *int64, sessionID, messageID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.llmRequestStore == nil {
		http.Error(w, "context inspector unavailable", http.StatusServiceUnavailable)
		return
	}
	if messageID == "" {
		http.NotFound(w, r)
		return
	}
	reqs, err := a.llmRequestStore.ListLLMRequestsForMessage(r.Context(), userID, sessionID, messageID)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "list_llm_requests")
		return
	}
	out := make([]llmRequestListItem, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, llmRequestListItem{
			ID:                  req.ID,
			Provider:            req.Provider,
			Model:               req.Model,
			SpecialistID:        req.SpecialistID,
			CreatedAt:           req.CreatedAt,
			InputTokens:         req.InputTokens,
			OutputTokens:        req.OutputTokens,
			MaxContextTokens:    req.MaxContextTokens,
			Redacted:            req.Redacted,
			CallID:              req.CallID,
			ParentCallID:        req.ParentCallID,
			ParentUserMessageID: req.ParentUserMessageID,
		})
	}
	writeChatJSON(w, out, "encode_llm_requests")
}

func (a *app) llmRequestContextHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := a.chatDetailAccess(w, r)
		if !ok {
			return
		}
		setChatCORSHeaders(w, r, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/llm-requests/")
		id = strings.TrimSuffix(id, "/context")
		id = strings.Trim(id, "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		if a.llmRequestStore == nil {
			http.Error(w, "context inspector unavailable", http.StatusServiceUnavailable)
			return
		}
		req, err := a.llmRequestStore.GetLLMRequest(r.Context(), userID, id)
		if err != nil {
			writeChatDetailStoreError(w, r, err, "", "get_llm_request")
			return
		}
		payload := req.Payload
		if !json.Valid(payload) {
			log.Warn().Str("request_id", req.ID).Msg("invalid_llm_request_payload")
			payload = []byte("{}")
		}
		payload = observability.RedactJSON(payload)
		writeChatJSON(w, llmRequestContextResponse{
			llmRequestListItem: llmRequestListItem{
				ID:                  req.ID,
				Provider:            req.Provider,
				Model:               req.Model,
				SpecialistID:        req.SpecialistID,
				CreatedAt:           req.CreatedAt,
				InputTokens:         req.InputTokens,
				OutputTokens:        req.OutputTokens,
				MaxContextTokens:    req.MaxContextTokens,
				Redacted:            req.Redacted,
				CallID:              req.CallID,
				ParentCallID:        req.ParentCallID,
				ParentUserMessageID: req.ParentUserMessageID,
			},
			Payload: payload,
		}, "encode_llm_request_context")
	}
}
