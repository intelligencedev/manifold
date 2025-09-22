package auth

import (
	"context"
	"time"
)

// User represents an authenticated user account.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Picture   string    `json:"picture"`
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Session represents a login session bound to a user.
type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// contextKey prevents collisions for context values.
type contextKey string

const userContextKey contextKey = "sio.user"

// WithUser returns a new context with the given user attached.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// CurrentUser extracts the user from context if present.
func CurrentUser(ctx context.Context) (*User, bool) {
	v := ctx.Value(userContextKey)
	if v == nil {
		return nil, false
	}
	u, ok := v.(*User)
	return u, ok && u != nil
}
