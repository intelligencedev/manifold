package agentd

import (
	"sync"

	"manifold/internal/agent"
	chatpkg "manifold/internal/agentd/chat"
)

type chatEngineAttachConfig = chatpkg.EngineAttachConfig

func attachChatEngineRuntime(deps ChatDeps, cfg chatEngineAttachConfig) {
	chatpkg.AttachEngineRuntime(chatpkg.RuntimeDeps{
		FleetBus:        deps.FleetBus,
		LLMRequestStore: deps.LLMRequestStore,
		ConfigureStreamCallbacks: func(eng *agent.Engine, writer chatpkg.StreamWriter, thoughtSummary, summaryEvents bool) {
			stream, ok := writer.(chatEventWriter)
			if !ok {
				return
			}
			configureCommonStreamCallbacks(eng, stream, thoughtSummary, summaryEvents)
		},
		ConfigureTracer: func(tracer agent.AgentTracer, mutex *sync.Mutex, onTrace func(agent.AgentTrace)) agent.AgentTracer {
			streamTracer, ok := tracer.(*agentStreamTracer)
			if !ok {
				return tracer
			}
			streamTracer.mu = mutex
			streamTracer.onTrace = onTrace
			return streamTracer
		},
		AttachCapture: func(eng *agent.Engine, capture chatpkg.CaptureConfig) {
			attachLLMRequestCapture(eng, capture)
		},
	}, cfg)
}
