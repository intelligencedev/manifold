package agentd

import (
	"context"
	"net/http"
	"strings"

	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/specialists"
	"manifold/internal/workspaces"
)

type chatTargetDispatchOptions struct {
	Prompt               string
	SessionID            string
	History              []llm.Message
	UserID               *int64
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
			},
			JSON: chatJSONOptions{
				Endpoint:           "/agent/run",
				InheritImagePrompt: true,
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

func dispatchOptionsFromDescriptor(descriptor chatTargetDescriptor, prompt, sessionID string, history []llm.Message, userID *int64) chatTargetDispatchOptions {
	return chatTargetDispatchOptions{
		Prompt:               prompt,
		SessionID:            sessionID,
		History:              history,
		UserID:               userID,
		RunContext:           descriptor.RunContext,
		CheckedOutWorkspace:  descriptor.CheckedOutWorkspace,
		Build:                descriptor.Build,
		NotFoundMessage:      descriptor.NotFoundMessage,
		InternalErrorMessage: descriptor.InternalErrorMessage,
		Stream:               descriptor.Stream,
		JSON:                 descriptor.JSON,
	}
}

func (a *app) agentRunOrchestratorDescriptor(baseCtx context.Context, owner int64, req chatRunRequest, checkedOutWorkspace *workspaces.Workspace, initialSummary *agentmemory.SummaryResult) chatTargetDescriptor {
	return chatTargetDescriptor{
		Build: func(ctx context.Context) chatEngineBuildResult {
			return a.buildOrchestratorChatEngine(ctx, owner, req.SessionID, "", checkedOutWorkspace)
		},
		InternalErrorMessage: "agent unavailable",
		RunContext:           llm.WithUserID(baseCtx, owner),
		CheckedOutWorkspace:  checkedOutWorkspace,
		Stream: chatStreamOptions{
			Endpoint:           "/agent/run",
			KeepAlive:          true,
			EmitThoughtSummary: true,
			EmitSummaryEvents:  true,
			StructuredErrors:   true,
			InitialSummary:     initialSummary,
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

	runCtx := opts.RunContext
	if runCtx == nil {
		runCtx = r.Context()
	}
	req := chatRunRequest{Prompt: opts.Prompt, SessionID: opts.SessionID}

	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		prun := a.runs.create(opts.Prompt)
		streamOpts := opts.Stream
		if streamOpts.StoreModel == "" {
			streamOpts.StoreModel = build.ModelLabel
		}
		if streamOpts.Tracer == nil {
			streamOpts.Tracer = newAgentStreamTracer(w)
		}
		a.executeStreamChat(w, r, runCtx, build.Engine, req, opts.History, prun.ID, opts.UserID, opts.CheckedOutWorkspace, streamOpts)
		return true
	}

	prun := a.runs.create(opts.Prompt)
	jsonOpts := opts.JSON
	if jsonOpts.StoreModel == "" {
		jsonOpts.StoreModel = build.ModelLabel
	}
	a.executeJSONChat(w, r, runCtx, build.Engine, req, opts.History, prun.ID, opts.UserID, opts.CheckedOutWorkspace, jsonOpts)
	return true
}

func (a *app) handleChatTarget(w http.ResponseWriter, r *http.Request, target chatDispatchTarget, prompt, sessionID, systemPromptOverride string, history []llm.Message, userID *int64, owner int64, fallback chatTargetDescriptor) bool {
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
	return a.dispatchBuiltChatTarget(w, r, dispatchOptionsFromDescriptor(descriptor, prompt, sessionID, history, userID))
}
