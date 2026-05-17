package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/auth"
	"manifold/internal/fleet"
)

var (
	errInputRequestNotFound  = errors.New("input request not found")
	errInputRequestForbidden = errors.New("input request forbidden")
)

type inputRequestBroker struct {
	mu      sync.Mutex
	pending map[string]*pendingInputRequest
}

type pendingInputRequestSnapshot struct {
	Request inputrequest.Request `json:"request"`
	Session string               `json:"session_id,omitempty"`
	RunID   string               `json:"run_id,omitempty"`
	UserID  *int64               `json:"-"`
}

type pendingInputRequest struct {
	request  inputrequest.Request
	session  string
	runID    string
	userID   *int64
	response chan inputrequest.Response
}

type streamInputRequester struct {
	broker  *inputRequestBroker
	stream  *chatSSEWriter
	session string
	runID   string
	userID  *int64
	bus     *fleet.Bus
}

func newInputRequestBroker() *inputRequestBroker {
	return &inputRequestBroker{pending: map[string]*pendingInputRequest{}}
}

func (a *app) activeInputRequestBroker() *inputRequestBroker {
	if a.inputRequests == nil {
		a.inputRequests = newInputRequestBroker()
	}
	return a.inputRequests
}

func newStreamInputRequester(broker *inputRequestBroker, stream *chatSSEWriter, sessionID, runID string, userID *int64, bus *fleet.Bus) inputrequest.Requester {
	if broker == nil || stream == nil {
		return nil
	}
	return &streamInputRequester{
		broker:  broker,
		stream:  stream,
		session: strings.TrimSpace(sessionID),
		runID:   strings.TrimSpace(runID),
		userID:  cloneInputRequestUserID(userID),
		bus:     bus,
	}
}

func (r *streamInputRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	if strings.TrimSpace(req.ID) == "" {
		return inputrequest.Response{}, errors.New("input request id is required")
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	pending := &pendingInputRequest{
		request:  req,
		session:  r.session,
		runID:    r.runID,
		userID:   cloneInputRequestUserID(r.userID),
		response: make(chan inputrequest.Response, 1),
	}
	if err := r.broker.register(pending); err != nil {
		return inputrequest.Response{}, err
	}
	defer r.broker.cancel(req.ID)

	r.stream.write(inputRequestEventPayload(req, r.session, r.runID))
	if r.bus != nil {
		r.bus.Publish(fleet.Event{Kind: fleet.EventInputRequest, RunID: r.runID, SessionID: r.session, CallID: req.CallID, ParentCallID: req.ParentCallID, ToolID: req.ToolID, Agent: req.Agent, Depth: req.Depth, UserID: derefInputUserID(r.userID), Data: map[string]any{"request_id": req.ID, "question": req.Question, "reason": req.Reason}})
	}

	select {
	case resp := <-pending.response:
		return resp, nil
	case <-ctx.Done():
		r.stream.write(map[string]any{
			"type":       "input_request_cancelled",
			"request_id": req.ID,
			"error":      ctx.Err().Error(),
		})
		return inputrequest.Response{}, ctx.Err()
	}
}

func (b *inputRequestBroker) register(pending *pendingInputRequest) error {
	if b == nil || pending == nil {
		return errors.New("input request broker unavailable")
	}
	id := strings.TrimSpace(pending.request.ID)
	if id == "" {
		return errors.New("input request id is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == nil {
		b.pending = map[string]*pendingInputRequest{}
	}
	if _, exists := b.pending[id]; exists {
		return errors.New("duplicate input request id")
	}
	b.pending[id] = pending
	return nil
}

func (b *inputRequestBroker) answer(userID *int64, response inputrequest.Response) (inputrequest.Request, string, string, error) {
	if b == nil {
		return inputrequest.Request{}, "", "", errInputRequestNotFound
	}
	id := strings.TrimSpace(response.RequestID)
	if id == "" {
		return inputrequest.Request{}, "", "", errInputRequestNotFound
	}
	b.mu.Lock()
	pending := b.pending[id]
	if pending == nil {
		b.mu.Unlock()
		return inputrequest.Request{}, "", "", errInputRequestNotFound
	}
	if !sameInputRequestUser(pending.userID, userID) {
		b.mu.Unlock()
		return inputrequest.Request{}, "", "", errInputRequestForbidden
	}
	delete(b.pending, id)
	b.mu.Unlock()

	if response.RespondedAt.IsZero() {
		response.RespondedAt = time.Now().UTC()
	}
	select {
	case pending.response <- response:
	default:
	}
	return pending.request, pending.session, pending.runID, nil
}

func (b *inputRequestBroker) cancel(requestID string) {
	if b == nil {
		return
	}
	id := strings.TrimSpace(requestID)
	if id == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, id)
}

func (b *inputRequestBroker) list(userID *int64) []pendingInputRequestSnapshot {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]pendingInputRequestSnapshot, 0, len(b.pending))
	for _, pending := range b.pending {
		if !sameInputRequestUser(pending.userID, userID) {
			continue
		}
		out = append(out, pendingInputRequestSnapshot{Request: pending.request, Session: pending.session, RunID: pending.runID, UserID: cloneInputRequestUserID(pending.userID)})
	}
	return out
}

func inputRequestEventPayload(req inputrequest.Request, sessionID, runID string) map[string]any {
	return map[string]any{
		"type":            "input_request",
		"request_id":      req.ID,
		"question":        req.Question,
		"reason":          req.Reason,
		"choices":         req.Choices,
		"allow_free_text": req.AllowFreeText,
		"multiple":        req.Multiple,
		"agent":           req.Agent,
		"model":           req.Model,
		"call_id":         req.CallID,
		"parent_call_id":  req.ParentCallID,
		"tool_id":         req.ToolID,
		"depth":           req.Depth,
		"session_id":      sessionID,
		"run_id":          runID,
		"created_at":      req.CreatedAt,
	}
}

func (a *app) chatInputRequestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		requestID, ok := parseInputRequestAnswerPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		var userID *int64
		if a.cfg != nil && a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID = id
		}

		defer r.Body.Close()
		var body struct {
			Answer    string   `json:"answer"`
			ChoiceIDs []string `json:"choice_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		choiceIDs := make([]string, 0, len(body.ChoiceIDs))
		for _, id := range body.ChoiceIDs {
			if id = strings.TrimSpace(id); id != "" {
				choiceIDs = append(choiceIDs, id)
			}
		}
		req, sessionID, runID, err := a.activeInputRequestBroker().answer(userID, inputrequest.Response{
			RequestID: requestID,
			Answer:    strings.TrimSpace(body.Answer),
			ChoiceIDs: choiceIDs,
		})
		if err != nil {
			switch {
			case errors.Is(err, errInputRequestForbidden):
				http.Error(w, "forbidden", http.StatusForbidden)
			case errors.Is(err, errInputRequestNotFound):
				http.NotFound(w, r)
			default:
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		if a.fleetBus != nil {
			a.fleetBus.Publish(fleet.Event{Kind: fleet.EventInputAnswered, RunID: runID, SessionID: sessionID, UserID: derefInputUserID(userID), Data: map[string]any{"request_id": req.ID, "answer": strings.TrimSpace(body.Answer), "choice_ids": choiceIDs}})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"request_id": req.ID,
		})
	}
}

func derefInputUserID(userID *int64) int64 {
	if userID == nil {
		return systemUserID
	}
	return *userID
}

func parseInputRequestAnswerPath(path string) (string, bool) {
	rest := strings.TrimPrefix(path, "/api/chat/input-requests/")
	if rest == path {
		return "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "answer" {
		return "", false
	}
	id := strings.TrimSpace(parts[0])
	return id, id != ""
}

func sameInputRequestUser(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func cloneInputRequestUserID(userID *int64) *int64 {
	if userID == nil {
		return nil
	}
	v := *userID
	return &v
}

func inputRequestContext(ctx context.Context, requester inputrequest.Requester, meta inputrequest.RunMetadata) context.Context {
	ctx = inputrequest.WithRequester(ctx, requester)
	return inputrequest.WithRunMetadata(ctx, meta)
}
