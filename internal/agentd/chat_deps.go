package agentd

import (
	"context"
	"net/http"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/auth"
	persist "manifold/internal/persistence"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

// ChatDeps remains an agentd alias for compatibility while ownership lives in
// the package that builds chat engines.
type ChatDeps = chatpkg.Deps

func (a *app) chatDeps() chatpkg.Deps {
	if a == nil {
		return chatpkg.Deps{}
	}
	return chatpkg.Deps{
		Cfg:                                    a.cfg,
		HTTPClient:                             a.httpClient,
		BaseToolRegistry:                       a.baseToolRegistry,
		ToolIndex:                              a.toolIndex,
		SpecStore:                              a.specStore,
		TeamStore:                              a.teamStore,
		MCPPool:                                a.mcpPool,
		ChatStore:                              a.chatStore,
		ActivityStore:                          a.activityStore,
		LLMRequestStore:                        a.llmRequestStore,
		DurableClient:                          a.durableClient,
		DurableStore:                           a.durableStore,
		DurableRegistry:                        a.durableRegistry,
		FleetBus:                               a.fleetBus,
		WorkspaceManager:                       a.workspaceManager,
		Projects:                               a.projectsService,
		ToolRegistry:                           a.baseToolRegistry,
		CloneEngineForUser:                     adaptCloneEngineForChat(a.cloneEngineForUser),
		SpecialistsRegistryForUser:             a.specialistsRegistryForUser,
		ComposeSystemPromptForUserWithOverride: a.composeSystemPromptForUserWithOverride,
		ConfigureBeliefRunState:                a.configureBeliefRunState,
		AttachSessionEvolvingMemory:            a.attachSessionEvolvingMemory,
		ConfigureUnifiedMemoryRuntime:          adaptMemoryRuntimeForChat(a.configureUnifiedMemoryRuntime),
		ResolveTeamOrchestratorSpecialist:      a.resolveTeamOrchestratorSpecialist,
		TeamDelegator:                          a,
		EvolvingConfig:                         a.evolvingCfg,
		ReMemMaxInnerSteps:                     a.rememMaxInnerSteps,
		NewSkillReadTool: func(projectDir string) tools.Tool {
			return newSkillReadTool(projectDir)
		},
		NewSkillSearchTool: func(projectDir string) tools.Tool {
			return newSkillSearchTool(projectDir)
		},
		HarnessRunConfig:      harnessRunConfig,
		HarnessOverrideConfig: harnessOverrideConfig,
		SummaryCallTimeout:    summaryCallTimeout,
		SpecRegistry: func(owner int64) *specialists.Registry {
			if owner == systemUserID {
				return a.specRegistry
			}
			return nil
		},
	}
}

func (a *app) chatService() *chatpkg.Service {
	return chatpkg.New(a.chatDeps())
}

func (a *app) chatSessionHandlerDeps() chatpkg.SessionHandlerDeps {
	return chatpkg.SessionHandlerDeps{
		AuthEnabled: a.cfg != nil && a.cfg.Auth.Enabled,
		Store:       a.chatStore,
		ResolveAccess: func(ctx context.Context, r *http.Request) (chatpkg.SessionAccess, error) {
			u, ok := auth.CurrentUser(ctx)
			if !ok {
				return chatpkg.SessionAccess{}, chatpkg.ErrUnauthorized
			}
			userID, isAdmin, err := resolveChatAccess(ctx, a.authStore, u)
			if err != nil {
				return chatpkg.SessionAccess{}, err
			}
			return chatpkg.SessionAccess{UserID: userID, CurrentUser: u, IsAdmin: isAdmin}, nil
		},
		SetCORS: setChatCORSHeaders,
		OverlaySessions: func(ctx context.Context, userID *int64, sessions []persist.ChatSession) []persist.ChatSession {
			return a.overlayCommandPolicySessionStates(ctx, userID, sessions)
		},
		OverlaySession: func(ctx context.Context, userID *int64, session persist.ChatSession) persist.ChatSession {
			return a.overlayCommandPolicySessionState(ctx, userID, session)
		},
		EnsureTemporaryProject: a.ensureTemporaryChatProject,
		RequestOwner:           chatRequestOwner,
	}
}

func adaptCloneEngineForChat(fn func(context.Context, int64, string, string, string, ...chatMemoryRunSettings) *agent.Engine) func(context.Context, int64, string, string, string, ...chatpkg.MemoryRunSettings) *agent.Engine {
	if fn == nil {
		return nil
	}
	return func(ctx context.Context, userID int64, sessionID, projectID, objectiveID string, settings ...chatpkg.MemoryRunSettings) *agent.Engine {
		converted := make([]chatMemoryRunSettings, len(settings))
		for i, setting := range settings {
			converted[i] = chatMemoryRunSettings(setting)
		}
		return fn(ctx, userID, sessionID, projectID, objectiveID, converted...)
	}
}

func adaptMemoryRuntimeForChat(fn func(*agent.Engine, *memory.EvolvingMemory, chatMemoryRunSettings)) func(*agent.Engine, *memory.EvolvingMemory, chatpkg.MemoryRunSettings) {
	if fn == nil {
		return nil
	}
	return func(eng *agent.Engine, em *memory.EvolvingMemory, settings chatpkg.MemoryRunSettings) {
		fn(eng, em, chatMemoryRunSettings(settings))
	}
}
