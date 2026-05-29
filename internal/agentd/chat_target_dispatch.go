package agentd

import (
	"context"
	"net/http"
	"strings"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
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
	ProjectID            string
	ObjectiveID          string
	EphemeralSession     bool
	MemorySettings       chatMemoryRunSettings
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

type chatTargetDescribeRequest struct {
	Target               chatDispatchTarget
	SessionID            string
	ProjectID            string
	ObjectiveID          string
	SystemPromptOverride string
	Owner                int64
	MemorySettings       chatMemoryRunSettings
}

func (a *app) describeChatTarget(req chatTargetDescribeRequest) (chatTargetDescriptor, bool) {
	if req.Target.SpecialistName != "" && !strings.EqualFold(req.Target.SpecialistName, specialists.OrchestratorName) {
		return chatTargetDescriptor{
			Build: func(ctx context.Context) chatEngineBuildResult {
				return a.buildSpecialistChatEngine(ctx, chatEngineBuildRequest{
					Name:                 req.Target.SpecialistName,
					SystemPromptOverride: req.SystemPromptOverride,
					SessionID:            req.SessionID,
					ProjectID:            req.ProjectID,
					ObjectiveID:          req.ObjectiveID,
					Owner:                req.Owner,
					MemorySettings:       req.MemorySettings,
				})
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

	if req.Target.TeamName != "" {
		teamTimeout := workflowLikeTimeout(a.cfg.WorkflowTimeoutSeconds, a.cfg.AgentRunTimeoutSeconds)
		return chatTargetDescriptor{
			Build: func(ctx context.Context) chatEngineBuildResult {
				return a.buildTeamChatEngine(ctx, chatEngineBuildRequest{
					Name:           req.Target.TeamName,
					SessionID:      req.SessionID,
					ProjectID:      req.ProjectID,
					ObjectiveID:    req.ObjectiveID,
					Owner:          req.Owner,
					MemorySettings: req.MemorySettings,
				})
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

type chatTargetDispatchRequest struct {
	Prompt           string
	SessionID        string
	ProjectID        string
	ObjectiveID      string
	EphemeralSession bool
	UserID           *int64
	MemorySettings   chatMemoryRunSettings
}

func dispatchOptionsFromDescriptor(descriptor chatTargetDescriptor, req chatTargetDispatchRequest) chatTargetDispatchOptions {
	return chatTargetDispatchOptions{
		Prompt:               req.Prompt,
		SessionID:            req.SessionID,
		ProjectID:            req.ProjectID,
		ObjectiveID:          req.ObjectiveID,
		EphemeralSession:     req.EphemeralSession,
		MemorySettings:       req.MemorySettings,
		UserID:               req.UserID,
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
			return a.buildOrchestratorChatEngine(ctx, chatEngineBuildRequest{
				SessionID:           req.SessionID,
				ProjectID:           req.ProjectID,
				ObjectiveID:         req.ObjectiveID,
				Owner:               owner,
				CheckedOutWorkspace: checkedOutWorkspace,
				MemorySettings:      chatMemorySettingsFromRunRequest(req),
			})
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
			return a.buildOrchestratorChatEngine(ctx, chatEngineBuildRequest{
				SystemPromptOverride: req.SystemPrompt,
				SessionID:            req.SessionID,
				ProjectID:            req.ProjectID,
				ObjectiveID:          req.ObjectiveID,
				Owner:                owner,
				CheckedOutWorkspace:  checkedOutWorkspace,
				MemorySettings:       chatMemorySettingsFromRunRequest(req),
			})
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
	build = sanitizeImageGenerationBuild(build)

	var history []llm.Message
	var summary *memory.SummaryResult
	if !build.ImageGeneration {
		var err error
		history, summary, err = a.chatMemory.BuildContextForProvider(r.Context(), opts.UserID, opts.SessionID, build.Engine.LLM, build.Engine.Model, memory.SummaryPolicy{
			TargetContextWindowTokens:    build.Engine.ContextWindowTokens,
			PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
		})
		if err != nil {
			if err == persist.ErrForbidden {
				http.Error(w, "forbidden", http.StatusForbidden)
				return true
			}
			log.Error().Err(err).Str("session", opts.SessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return true
		}
		build.Engine.SkipInitialSummarization = summary != nil && summary.Triggered
	}

	runCtx := opts.RunContext
	if runCtx == nil {
		runCtx = r.Context()
	}
	if strings.TrimSpace(opts.ObjectiveID) != "" {
		runCtx = sandbox.WithObjectiveID(runCtx, opts.ObjectiveID)
	}
	runCtx = applyBuildImagePrompt(runCtx, build)
	req := chatRunRequest{
		Prompt:                opts.Prompt,
		SessionID:             opts.SessionID,
		ProjectID:             opts.ProjectID,
		ObjectiveID:           opts.ObjectiveID,
		EphemeralSession:      opts.EphemeralSession,
		EvolvingMemoryEnabled: boolPtr(opts.MemorySettings.EvolvingMemoryEnabled),
		BeliefMemoryEnabled:   boolPtr(opts.MemorySettings.BeliefMemoryEnabled),
	}

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
		a.executeStreamChat(w, r, chatExecutionRequest{
			RunContext:          runCtx,
			Engine:              build.Engine,
			RunRequest:          req,
			History:             history,
			RunID:               prun.ID,
			UserID:              opts.UserID,
			CheckedOutWorkspace: opts.CheckedOutWorkspace,
		}, streamOpts)
		return true
	}

	prun := a.runs.create(opts.Prompt)
	jsonOpts := opts.JSON
	if jsonOpts.StoreModel == "" {
		jsonOpts.StoreModel = build.ModelLabel
	}
	a.executeJSONChat(w, r, chatExecutionRequest{
		RunContext:          runCtx,
		Engine:              build.Engine,
		RunRequest:          req,
		History:             history,
		RunID:               prun.ID,
		UserID:              opts.UserID,
		CheckedOutWorkspace: opts.CheckedOutWorkspace,
	}, jsonOpts)
	return true
}

type chatTargetHandleRequest struct {
	Target               chatDispatchTarget
	Prompt               string
	SessionID            string
	ProjectID            string
	ObjectiveID          string
	EphemeralSession     bool
	SystemPromptOverride string
	UserID               *int64
	Owner                int64
	Fallback             chatTargetDescriptor
	MemorySettings       chatMemoryRunSettings
}

func (a *app) handleChatTarget(w http.ResponseWriter, r *http.Request, req chatTargetHandleRequest) bool {
	descriptor, ok := a.describeChatTarget(chatTargetDescribeRequest{
		Target:               req.Target,
		SessionID:            req.SessionID,
		ProjectID:            req.ProjectID,
		ObjectiveID:          req.ObjectiveID,
		SystemPromptOverride: req.SystemPromptOverride,
		Owner:                req.Owner,
		MemorySettings:       req.MemorySettings,
	})
	if !ok {
		if req.Fallback.Build == nil {
			return false
		}
		descriptor = req.Fallback
	}

	if descriptor.RunContext == nil {
		descriptor.RunContext = r.Context()
	}
	return a.dispatchBuiltChatTarget(w, r, dispatchOptionsFromDescriptor(descriptor, chatTargetDispatchRequest{
		Prompt:           req.Prompt,
		SessionID:        req.SessionID,
		ProjectID:        req.ProjectID,
		ObjectiveID:      req.ObjectiveID,
		EphemeralSession: req.EphemeralSession,
		UserID:           req.UserID,
		MemorySettings:   req.MemorySettings,
	}))
}
