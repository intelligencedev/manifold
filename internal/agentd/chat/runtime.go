package chat

import (
	"sync"

	"manifold/internal/agent"
	"manifold/internal/fleet"
	persist "manifold/internal/persistence"
)

// CaptureConfig identifies the chat turn associated with captured LLM calls.
type CaptureConfig struct {
	Store               persist.LLMRequestStore
	SessionID           string
	UserID              *int64
	RunID               string
	MessageID           string
	ParentUserMessageID string
	SpecialistID        string
	CallID              string
	ParentCallID        string
}

// EngineAttachConfig describes runtime hooks shared by live and durable chat.
// StreamWriter is intentionally small: serialization remains owned by the
// HTTP package through the concrete implementation.
type StreamWriter interface {
	Write(any)
	WriteText(string)
}

type EngineAttachConfig struct {
	Engine             *agent.Engine
	StreamWriter       StreamWriter
	EmitThoughtSummary bool
	EmitSummaryEvents  bool
	Fleet              FleetCallbackRequest
	Tracer             agent.AgentTracer
	TracerMutex        *sync.Mutex
	Checkpointer       agent.RunCheckpointer
	UserID             int64
	SetUserID          bool
	Capture            CaptureConfig
	Activity           func(agent.AgentTrace)
}

// RuntimeDeps contains the application callbacks needed for the few runtime
// hooks that are protocol- or persistence-specific.
type RuntimeDeps struct {
	FleetBus                 *fleet.Bus
	LLMRequestStore          persist.LLMRequestStore
	ConfigureStreamCallbacks func(*agent.Engine, StreamWriter, bool, bool)
	ConfigureTracer          func(agent.AgentTracer, *sync.Mutex, func(agent.AgentTrace)) agent.AgentTracer
	AttachCapture            func(*agent.Engine, CaptureConfig)
}

// AttachEngineRuntime attaches tracing, stream callbacks, fleet events,
// checkpoints, user identity, and LLM request capture to one engine.
func AttachEngineRuntime(deps RuntimeDeps, cfg EngineAttachConfig) {
	if cfg.Engine == nil {
		return
	}
	if cfg.Tracer != nil {
		if deps.ConfigureTracer != nil {
			cfg.Tracer = deps.ConfigureTracer(cfg.Tracer, cfg.TracerMutex, cfg.Activity)
		}
		cfg.Engine.AgentTracer = cfg.Tracer
	}
	if cfg.StreamWriter != nil && deps.ConfigureStreamCallbacks != nil {
		deps.ConfigureStreamCallbacks(cfg.Engine, cfg.StreamWriter, cfg.EmitThoughtSummary, cfg.EmitSummaryEvents)
	}
	AttachFleetCallbacks(deps.FleetBus, cfg.Engine, cfg.Fleet)
	if cfg.Checkpointer != nil {
		cfg.Engine.Checkpointer = cfg.Checkpointer
	}
	if cfg.SetUserID || cfg.UserID != 0 {
		cfg.Engine.UserID = cfg.UserID
	}
	if cfg.Capture.Store == nil {
		cfg.Capture.Store = deps.LLMRequestStore
	}
	if deps.AttachCapture != nil {
		deps.AttachCapture(cfg.Engine, cfg.Capture)
	}
}
