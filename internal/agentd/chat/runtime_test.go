package chat

import (
	"testing"

	"manifold/internal/agent"
)

type testStreamWriter struct{}

func (testStreamWriter) Write(any)        {}
func (testStreamWriter) WriteText(string) {}

func TestAttachEngineRuntimeUsesInjectedHooks(t *testing.T) {
	engine := &agent.Engine{}
	streamConfigured := false
	captureConfigured := false
	AttachEngineRuntime(RuntimeDeps{
		ConfigureStreamCallbacks: func(*agent.Engine, StreamWriter, bool, bool) { streamConfigured = true },
		AttachCapture:            func(*agent.Engine, CaptureConfig) { captureConfigured = true },
	}, EngineAttachConfig{Engine: engine, StreamWriter: testStreamWriter{}, Capture: CaptureConfig{}})
	if !streamConfigured || !captureConfigured {
		t.Fatalf("streamConfigured=%v captureConfigured=%v", streamConfigured, captureConfigured)
	}
}
