package agentd

import (
	"net/http"

	observabilityapi "manifold/internal/agentd/observability"
)

type llmRequestListItem = observabilityapi.LLMRequestListItem
type llmRequestContextResponse = observabilityapi.LLMRequestContextResponse

func (a *app) llmRequestHandlerDeps() observabilityapi.LLMRequestDeps {
	return observabilityapi.LLMRequestDeps{
		Store: a.llmRequestStore,
		Access: func(w http.ResponseWriter, r *http.Request) (*int64, bool) {
			userID, _, ok := a.chatDetailAccess(w, r)
			return userID, ok
		},
		SetCORS: setChatCORSHeaders,
		WriteJSON: func(w http.ResponseWriter, value any) {
			writeChatJSON(w, value, "encode_llm_request")
		},
		StoreError: writeChatDetailStoreError,
	}
}

func (a *app) handleChatLLMRequests(w http.ResponseWriter, r *http.Request, userID *int64, sessionID, messageID string) {
	observabilityapi.HandleChatLLMRequests(w, r, a.llmRequestHandlerDeps(), userID, sessionID, messageID)
}

func (a *app) llmRequestContextHandler() http.HandlerFunc {
	return observabilityapi.LLMRequestContextHandler(a.llmRequestHandlerDeps())
}
