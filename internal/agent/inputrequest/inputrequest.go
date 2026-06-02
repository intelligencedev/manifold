package inputrequest

import (
	"context"
	"time"
)

// Choice describes one selectable answer option for an information request.
type Choice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// RunMetadata identifies the agent context that emitted an information request.
type RunMetadata struct {
	Agent        string
	Model        string
	CallID       string
	ParentCallID string
	ToolID       string
	Depth        int
}

// Request is emitted by an agent tool when the agent needs information before it
// can continue.
type Request struct {
	ID            string    `json:"id"`
	Question      string    `json:"question"`
	Reason        string    `json:"reason,omitempty"`
	Choices       []Choice  `json:"choices,omitempty"`
	AllowFreeText bool      `json:"allow_free_text"`
	Multiple      bool      `json:"multiple"`
	Agent         string    `json:"agent,omitempty"`
	Model         string    `json:"model,omitempty"`
	CallID        string    `json:"call_id,omitempty"`
	ParentCallID  string    `json:"parent_call_id,omitempty"`
	ToolID        string    `json:"tool_id,omitempty"`
	Depth         int       `json:"depth,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Response is returned to the requesting tool once the caller/user answers.
type Response struct {
	RequestID   string    `json:"request_id"`
	Answer      string    `json:"answer,omitempty"`
	ChoiceIDs   []string  `json:"choice_ids,omitempty"`
	RespondedAt time.Time `json:"responded_at"`
}

// Requester handles blocking information requests from tools.
type Requester interface {
	RequestInfo(ctx context.Context, req Request) (Response, error)
}

type requesterContextKey struct{}
type metadataContextKey struct{}

// WithRequester attaches an information-request responder to a run context.
func WithRequester(ctx context.Context, requester Requester) context.Context {
	if ctx == nil || requester == nil {
		return ctx
	}
	return context.WithValue(ctx, requesterContextKey{}, requester)
}

// RequesterFromContext returns the active information-request responder.
func RequesterFromContext(ctx context.Context) Requester {
	if ctx == nil {
		return nil
	}
	requester, _ := ctx.Value(requesterContextKey{}).(Requester)
	return requester
}

// WithRunMetadata attaches the agent identity that should be shown with
// requests emitted from the context.
func WithRunMetadata(ctx context.Context, meta RunMetadata) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, metadataContextKey{}, meta)
}

// RunMetadataFromContext returns metadata for the currently running agent.
func RunMetadataFromContext(ctx context.Context) RunMetadata {
	if ctx == nil {
		return RunMetadata{}
	}
	meta, _ := ctx.Value(metadataContextKey{}).(RunMetadata)
	return meta
}
