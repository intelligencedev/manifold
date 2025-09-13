package testhelpers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"

	"intelligence.dev/internal/llm"
)

// FakeProvider is a simple LLM provider for tests. It can be configured
// with a fixed response or a streaming sequence.
type FakeProvider struct {
	Resp llm.Message
	Err  error

	// For streaming tests
	StreamDeltas []string
}

func (f *FakeProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if f.Err != nil {
		return llm.Message{}, f.Err
	}
	return f.Resp, nil
}

func (f *FakeProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	if f.Err != nil {
		return f.Err
	}
	for _, d := range f.StreamDeltas {
		h.OnDelta(d)
	}
	return nil
}

// NewTestServer returns an httptest.Server for the given handler func.
func NewTestServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}

// WaitGroupDoneOnce returns a function that will call wg.Done() only once; useful for
// tests that need to ensure a WaitGroup is decremented a single time from multiple places.
func WaitGroupDoneOnce(wg *sync.WaitGroup) func() {
	once := sync.Once{}
	return func() { once.Do(wg.Done) }
}
