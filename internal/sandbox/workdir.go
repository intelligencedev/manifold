package sandbox

import (
	"context"
	"strings"
	"sync"
)

// Context key for dynamic base directory used by tools at runtime.
type baseDirCtxKey struct{}

// Context keys for session and project identifiers.
type sessionIDCtxKey struct{}
type projectIDCtxKey struct{}
type roomIDCtxKey struct{}
type matrixOutboxCtxKey struct{}

// Context key for forwarding auth cookies to internal service calls.
type authCookieCtxKey struct{}

// MatrixMessage describes a queued outbound Matrix room message.
type MatrixMessage struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

// MatrixOutbox stores Matrix room messages queued during one request.
type MatrixOutbox struct {
	mu       sync.Mutex
	messages []MatrixMessage
}

// NewMatrixOutbox constructs an empty Matrix outbox.
func NewMatrixOutbox() *MatrixOutbox {
	return &MatrixOutbox{messages: make([]MatrixMessage, 0, 4)}
}

// Add queues a Matrix message when room ID and text are both non-empty.
func (o *MatrixOutbox) Add(roomID, text string) {
	if o == nil {
		return
	}
	roomID = strings.TrimSpace(roomID)
	text = strings.TrimSpace(text)
	if roomID == "" || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.messages = append(o.messages, MatrixMessage{RoomID: roomID, Text: text})
}

// Messages returns a snapshot of queued Matrix messages.
func (o *MatrixOutbox) Messages() []MatrixMessage {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]MatrixMessage, len(o.messages))
	copy(out, o.messages)
	return out
}

// WithBaseDir attaches a per-request/per-run base directory to ctx.
// Tools that operate on the filesystem should prefer this value over
// their default configured workdir.
func WithBaseDir(ctx context.Context, dir string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), baseDirCtxKey{}, dir)
	}
	return context.WithValue(ctx, baseDirCtxKey{}, dir)
}

// WithSessionID attaches a chat session identifier to ctx.
// Tools like ask_agent can use this to inherit the current session.
func WithSessionID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), sessionIDCtxKey{}, id)
	}
	return context.WithValue(ctx, sessionIDCtxKey{}, id)
}

// WithProjectID attaches a project identifier to ctx.
// Tools like ask_agent can use this to inherit the current project scope.
func WithProjectID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), projectIDCtxKey{}, id)
	}
	return context.WithValue(ctx, projectIDCtxKey{}, id)
}

// WithRoomID attaches a Matrix room identifier to ctx.
func WithRoomID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), roomIDCtxKey{}, id)
	}
	return context.WithValue(ctx, roomIDCtxKey{}, id)
}

// WithMatrixOutbox attaches a Matrix outbox to ctx.
func WithMatrixOutbox(ctx context.Context, outbox *MatrixOutbox) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), matrixOutboxCtxKey{}, outbox)
	}
	return context.WithValue(ctx, matrixOutboxCtxKey{}, outbox)
}

// SessionIDFromContext returns the session ID previously set with
// WithSessionID. The boolean is false if no value is present.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v := ctx.Value(sessionIDCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// ProjectIDFromContext returns the project ID previously set with
// WithProjectID. The boolean is false if no value is present.
func ProjectIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v := ctx.Value(projectIDCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// RoomIDFromContext returns the room ID previously set with WithRoomID.
func RoomIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v := ctx.Value(roomIDCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// MatrixOutboxFromContext returns the Matrix outbox previously set with WithMatrixOutbox.
func MatrixOutboxFromContext(ctx context.Context) (*MatrixOutbox, bool) {
	if ctx == nil {
		return nil, false
	}
	if v := ctx.Value(matrixOutboxCtxKey{}); v != nil {
		if outbox, ok := v.(*MatrixOutbox); ok && outbox != nil {
			return outbox, true
		}
	}
	return nil, false
}

// BaseDirFromContext returns the base directory previously set with
// WithBaseDir. The boolean is false if no value is present.
func BaseDirFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v := ctx.Value(baseDirCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// ResolveBaseDir returns the base directory from context when available,
// otherwise returns defaultDir.
func ResolveBaseDir(ctx context.Context, defaultDir string) string {
	if v, ok := BaseDirFromContext(ctx); ok {
		return v
	}
	return defaultDir
}

// WithAuthCookie attaches an auth cookie (name=value) to ctx for forwarding
// to internal service calls like delegate_to_team.
func WithAuthCookie(ctx context.Context, cookie string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), authCookieCtxKey{}, cookie)
	}
	return context.WithValue(ctx, authCookieCtxKey{}, cookie)
}

// AuthCookieFromContext returns the auth cookie previously set with
// WithAuthCookie. The boolean is false if no value is present.
func AuthCookieFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if v := ctx.Value(authCookieCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}
