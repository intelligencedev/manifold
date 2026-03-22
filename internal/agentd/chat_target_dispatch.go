package agentd

import (
	"context"
	"net/http"
	"strings"

	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/specialists"
	"manifold/internal/workspaces"

	"github.com/rs/zerolog/log"
)

func workflowLikeTimeout(workflowSeconds, fallbackSeconds int) int {
	if workflowSeconds > 0 {
		return workflowSeconds
	}
	return fallbackSeconds
}

type chatTargetDispatchOptions struct {
	Prompt               string
	SessionID            string
	EphemeralSession     bool
	UserID               *int64
	IncludeSummary       bool
	RunContext           context.Context
	CheckedOutWorkspace  *workspaces.Workspace
	Build                func(context.Context) chatEngineBuildResult
	NotFoundMessage      string
	InternalErrorMessage string
	Stream               chatStreamOptions
	JSON                 chatJSONOptions
}

type chatTargetDescriptor struct {
	Build                func(context.Context) chatEngineBuildResult
	NotFoundMessage      string
	InternalErrorMessage string
	IncludeSummary       bool
	RunContext           context.Context
	CheckedOutWorkspace  *workspaces.Workspace
	Stream               chatStreamOptions
	JSON                 chatJSONOptions
}

func (a *app) describeChatTarget(target chatDispatchTarget, systemPromptOverride string, owner int64) (chatTargetDescriptor, bool) {
	if target.SpecialistName != "" && !strings.EqualFold(target.SpecialistName, specialists.OrchestratorName) {
		return chatTargetDescriptor{
			Build: func(ctx context.Context) chatEngineBuildResult {
				return a.buildSpecialistChatEngine(ctx, target.SpecialistName, systemPromptOverride, owner)
			},
			NotFoundMessage:      "specialist not found",
			InternalErrorMessage: "specialist registry unavailable",
			Stream: chatStreamOptions{
				Endpoint:              "/agent/run",
				IncludeMatrixMessages: true,
				KeepAlive:             true,
				EmitThoughtSummary:    true,
				EmitSummaryEvents:     true,
				StructuredErrors:      true,
				InheritImagePrompt:    true,
			},
			JSON: chatJSONOptions{
				Endpoint:              "/agent/run",
				IncludeMatrixMessages: true,
				InheritImagePrompt:    true,
			},
		}, true
	}

	if target.TeamName != "" {
		teamTimeout := workflowLikeTimeout(a.cfg.WorkflowTimeoutSeconds, a.cfg.AgentRunTimeoutSeconds)
		return chatTargetDescriptor{
			Build: func(ctx context.Context) chatEngineBuildResult {
				return a.buildTeamChatEngine(ctx, target.TeamName, owner)
			},
			NotFoundMessage:      "team not found",
			InternalErrorMessage: "failed to load team",
			Stream: chatStreamOptions{
				Endpoint:           "/agent/run",
				KeepAlive:          true,
				EmitThoughtSummary: true,
				EmitSummaryEvents:  true,
				StructuredErrors:   true,
				InheritImagePrompt: true,
				TimeoutSeconds:     teamTimeout,
			},
			JSON: chatJSONOptions{
				Endpoint:           "/agent/run",
				InheritImagePrompt: true,
				TimeoutSeconds:     teamTimeout,
			},
		}, true
	}

	return chatTargetDescriptor{}, false
}

func newAgentStreamTracer(w http.ResponseWriter) *agentStreamTracer {
	tracer := &agentStreamTracer{}
	if fl, ok := w.(http.Flusher); ok {
		tracer = &agentStreamTracer{w: w, fl: fl}
	}
	return tracer
}

func writeChatTargetBuildError(w http.ResponseWriter, build chatEngineBuildResult, notFoundMessage, internalMessage string) {
	switch build.StatusCode {
	case http.StatusNotFound:
		http.Error(w, notFoundMessage, http.StatusNotFound)
	default:
		statusCode := build.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		http.Error(w, internalMessage, statusCode)
	}
}

func dispatchOptionsFromDescriptor(descriptor chatTargetDescriptor, prompt, sessionID string, ephemeralSession bool, userID *int64) chatTargetDispatchOptions {
	return chatTargetDispatchOptions{
		Prompt:               prompt,
		SessionID:            sessionID,
		EphemeralSession:     ephemeralSession,
		UserID:               userID,
		IncludeSummary:       descriptor.IncludeSummary,
		RunContext:           descriptor.RunContext,
		CheckedOutWorkspace:  descriptor.CheckedOutWorkspace,
		Build:                descriptor.Build,
		NotFoundMessage:      descriptor.NotFoundMessage,
		InternalErrorMessage: descriptor.InternalErrorMessage,
		Stream:               descriptor.Stream,
		JSON:                 descriptor.JSON,
	}
}

func (a *app) agentRunOrchestratorDescriptor(baseCtx context.Context, owner int64, req chatRunRequest, checkedOutWorkspace *workspaces.Workspace) chatTargetDescriptor {
	return chatTargetDescriptor{
		Build: func(ctx context.Context) chatEngineBuildResult {
			return a.buildOrchestratorChatEngine(ctx, owner, req.SessionID, "", checkedOutWorkspace)
		},
		InternalErrorMessage: "agent unavailable",
		IncludeSummary:       true,
		RunContext:           llm.WithUserID(baseCtx, owner),
		CheckedOutWorkspace:  checkedOutWorkspace,
		Stream: chatStreamOptions{
			Endpoint:           "/agent/run",
			KeepAlive:          true,
			EmitThoughtSummary: true,
			EmitSummaryEvents:  true,
			StructuredErrors:   true,
		},
		JSON: chatJSONOptions{Endpoint: "/agent/run"},
	}
}

func (a *app) promptOrchestratorDescriptor(baseCtx context.Context, owner int64, req chatRunRequest, checkedOutWorkspace *workspaces.Workspace) chatTargetDescriptor {
	return chatTargetDescriptor{
		Build: func(ctx context.Context) chatEngineBuildResult {
			return a.buildOrchestratorChatEngine(ctx, owner, req.SessionID, req.SystemPrompt, checkedOutWorkspace)
		},
		InternalErrorMessage: "agent unavailable",
		RunContext:           llm.WithUserID(baseCtx, owner),
		CheckedOutWorkspace:  checkedOutWorkspace,
		Stream: chatStreamOptions{
			Endpoint:              "/api/prompt",
			IncludeMatrixMessages: true,
			StructuredErrors:      false,
		},
		JSON: chatJSONOptions{Endpoint: "/api/prompt", IncludeMatrixMessages: true},
	}
}

func (a *app) dispatchBuiltChatTarget(w http.ResponseWriter, r *http.Request, opts chatTargetDispatchOptions) bool {
	build := opts.Build(r.Context())
	if build.Err != nil {
		writeChatTargetBuildError(w, build, opts.NotFoundMessage, opts.InternalErrorMessage)
		return true
	}

	targetSupportsCompaction := providerSupportsCompaction(build.Engine.LLM)
	history, summary, err := a.chatMemory.BuildContextForProvider(r.Context(), opts.UserID, opts.SessionID, targetSupportsCompaction)
	if err != nil {
		if err == persist.ErrForbidden {
			http.Error(w, "forbidden", http.StatusForbidden)
			return true
		}
		log.Error().Err(err).Str("session", opts.SessionID).Msg("load_chat_history")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}

	runCtx := opts.RunContext
	if runCtx == nil {
		runCtx = r.Context()
	}
	req := chatRunRequest{Prompt: opts.Prompt, SessionID: opts.SessionID, EphemeralSession: opts.EphemeralSession}

	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		prun := a.runs.create(opts.Prompt)
		streamOpts := opts.Stream
		if streamOpts.StoreModel == "" {
			streamOpts.StoreModel = build.ModelLabel
		}
		if opts.IncludeSummary {
			streamOpts.InitialSummary = summary
		}
		if streamOpts.Tracer == nil {
			streamOpts.Tracer = newAgentStreamTracer(w)
		}
		a.executeStreamChat(w, r, runCtx, build.Engine, req, history, prun.ID, opts.UserID, opts.CheckedOutWorkspace, streamOpts)
		return true
	}

	prun := a.runs.create(opts.Prompt)
	jsonOpts := opts.JSON
	if jsonOpts.StoreModel == "" {
		jsonOpts.StoreModel = build.ModelLabel
	}
	a.executeJSONChat(w, r, runCtx, build.Engine, req, history, prun.ID, opts.UserID, opts.CheckedOutWorkspace, jsonOpts)
	return true
}

func (a *app) handleChatTarget(w http.ResponseWriter, r *http.Request, target chatDispatchTarget, prompt, sessionID string, ephemeralSession bool, systemPromptOverride string, userID *int64, owner int64, fallback chatTargetDescriptor) bool {
	descriptor, ok := a.describeChatTarget(target, systemPromptOverride, owner)
	if !ok {
		if fallback.Build == nil {
			return false
		}
		descriptor = fallback
	}

	if descriptor.RunContext == nil {
		descriptor.RunContext = r.Context()
	}
	return a.dispatchBuiltChatTarget(w, r, dispatchOptionsFromDescriptor(descriptor, prompt, sessionID, ephemeralSession, userID))
}
