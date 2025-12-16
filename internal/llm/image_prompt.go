package llm

import "context"

// ImagePromptOptions describes optional overrides for image generation.
type ImagePromptOptions struct {
	// Size is an optional size hint understood by providers (e.g., "1K" for Gemini).
	Size string
}

type imagePromptCtxKey struct{}

// WithImagePrompt annotates ctx to request image generation support from providers.
// Presence of this context value signals that callers expect image outputs.
func WithImagePrompt(ctx context.Context, opts ImagePromptOptions) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, imagePromptCtxKey{}, opts)
}

// ImagePromptFromContext returns the requested image generation options when present.
func ImagePromptFromContext(ctx context.Context) (ImagePromptOptions, bool) {
	if ctx == nil {
		return ImagePromptOptions{}, false
	}
	if v := ctx.Value(imagePromptCtxKey{}); v != nil {
		if opts, ok := v.(ImagePromptOptions); ok {
			return opts, true
		}
	}
	return ImagePromptOptions{}, false
}
