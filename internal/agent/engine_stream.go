package agent

import "manifold/internal/llm"

type streamHandler struct {
	onDelta            func(string)
	onThoughtSummary   func(string)
	onThoughtSignature func(string)
	onToolCall         func(llm.ToolCall)
	onImage            func(llm.GeneratedImage)
	onVideo            func(llm.GeneratedVideo)
}

func (h *streamHandler) OnDelta(content string) {
	if h.onDelta != nil {
		h.onDelta(content)
	}
}

func (h *streamHandler) OnToolCall(tc llm.ToolCall) {
	if h.onToolCall != nil {
		h.onToolCall(tc)
	}
}

func (h *streamHandler) OnImage(img llm.GeneratedImage) {
	if h.onImage != nil {
		h.onImage(img)
	}
}

func (h *streamHandler) OnVideo(video llm.GeneratedVideo) {
	if h.onVideo != nil {
		h.onVideo(video)
	}
}

func (h *streamHandler) OnThoughtSummary(summary string) {
	if h.onThoughtSummary != nil {
		h.onThoughtSummary(summary)
	}
}

func (h *streamHandler) OnThoughtSignature(sig string) {
	if h.onThoughtSignature != nil {
		h.onThoughtSignature(sig)
	}
}

func (e *Engine) model() string { return e.Model }

// runLoop contains the core non-streaming agent step loop shared by Run.
