package chat

import (
	"net/http"
	"strings"

	"manifold/internal/auth"
)

// DetailHandlerDeps is the explicit dependency slice for chat-session detail
// routing. Concrete session operations remain injected so this package never
// reaches into the agentd composition root.
type DetailHandlerDeps struct {
	Access      func(http.ResponseWriter, *http.Request) (*int64, *auth.User, bool)
	SetCORS     func(http.ResponseWriter, *http.Request, string)
	Activities  func(http.ResponseWriter, *http.Request, *int64, string)
	Messages    func(http.ResponseWriter, *http.Request, *int64, string, string)
	LLMRequests func(http.ResponseWriter, *http.Request, *int64, string, string)
	Runs        func(http.ResponseWriter, *http.Request, *int64, string)
	Title       func(http.ResponseWriter, *http.Request, *int64, string)
	Session     func(http.ResponseWriter, *http.Request, *auth.User, *int64, string)
}

// DetailHandler returns the router for a chat session and its subresources.
func DetailHandler(deps DetailHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Access == nil {
			http.Error(w, "chat handler unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, currentUser, ok := deps.Access(w, r)
		if !ok {
			return
		}
		id, subresource, subresourceID, ok := ParseDetailPath(r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if deps.SetCORS != nil {
			deps.SetCORS(w, r, subresource)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch subresource {
		case "activities":
			deps.Activities(w, r, userID, id)
		case "messages":
			deps.Messages(w, r, userID, id, subresourceID)
		case "llm-requests":
			deps.LLMRequests(w, r, userID, id, subresourceID)
		case "runs":
			deps.Runs(w, r, userID, id)
		case "title":
			deps.Title(w, r, userID, id)
		default:
			deps.Session(w, r, currentUser, userID, id)
		}
	}
}

// ParseDetailPath extracts the session id and optional subresource from the
// stable chat detail route.
func ParseDetailPath(r *http.Request) (string, string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/chat/sessions/"), "/")
	if rest == "" {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	var subresource, subresourceID string
	if len(parts) >= 2 {
		subresource = parts[1]
	}
	if len(parts) >= 3 {
		subresourceID = parts[2]
	}
	return parts[0], subresource, subresourceID, true
}
