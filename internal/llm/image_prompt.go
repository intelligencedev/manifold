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

// VideoPromptOptions describes optional overrides for video generation.
type VideoPromptOptions struct{}

type videoPromptCtxKey struct{}

// WithVideoPrompt annotates ctx to request video generation support from providers.
func WithVideoPrompt(ctx context.Context, opts VideoPromptOptions) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, videoPromptCtxKey{}, opts)
}

// VideoPromptFromContext returns the requested video generation options when present.
func VideoPromptFromContext(ctx context.Context) (VideoPromptOptions, bool) {
	if ctx == nil {
		return VideoPromptOptions{}, false
	}
	if v := ctx.Value(videoPromptCtxKey{}); v != nil {
		if opts, ok := v.(VideoPromptOptions); ok {
			return opts, true
		}
	}
	return VideoPromptOptions{}, false
}
