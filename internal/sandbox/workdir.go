package sandbox

import "context"

// Context key for dynamic base directory used by tools at runtime.
type baseDirCtxKey struct{}

// Context keys for session and project identifiers.
type sessionIDCtxKey struct{}
type projectIDCtxKey struct{}

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
