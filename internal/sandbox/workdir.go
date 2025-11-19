package sandbox

import "context"

// Context key for dynamic base directory used by tools at runtime.
type baseDirCtxKey struct{}

// WithBaseDir attaches a per-request/per-run base directory to ctx.
// Tools that operate on the filesystem should prefer this value over
// their default configured workdir.
func WithBaseDir(ctx context.Context, dir string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), baseDirCtxKey{}, dir)
	}
	return context.WithValue(ctx, baseDirCtxKey{}, dir)
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
