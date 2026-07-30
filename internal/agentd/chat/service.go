package chat

import (
	"context"
	"errors"
)

var errServiceUnavailable = errors.New("chat service unavailable")

// Service owns chat-engine construction behind an explicit dependency slice.
// The HTTP package may retain protocol-specific preparation, but it no longer
// needs to know which application fields are required to build an engine.
type Service struct {
	deps Deps
}

// New creates a chat service with the supplied composition-root dependencies.
func New(deps Deps) *Service {
	return &Service{deps: deps}
}

// BuildOrchestrator constructs an orchestrator engine.
func (s *Service) BuildOrchestrator(ctx context.Context, req BuildRequest) BuildResult {
	if s == nil {
		return BuildResult{Err: errServiceUnavailable}
	}
	return s.deps.BuildOrchestrator(ctx, req)
}

// BuildSpecialist constructs a specialist engine.
func (s *Service) BuildSpecialist(ctx context.Context, req BuildRequest) BuildResult {
	if s == nil {
		return BuildResult{Err: errServiceUnavailable}
	}
	return s.deps.BuildSpecialist(ctx, req)
}

// BuildTeam constructs a team orchestrator engine.
func (s *Service) BuildTeam(ctx context.Context, req BuildRequest) BuildResult {
	if s == nil {
		return BuildResult{Err: errServiceUnavailable}
	}
	return s.deps.BuildTeam(ctx, req)
}
