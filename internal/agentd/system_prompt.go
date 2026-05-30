package agentd

import (
	"context"

	"manifold/internal/agent/prompts"
)

// composeSystemPrompt builds the stable base system prompt (including AGENTS.md,
// if present). Dynamic specialist catalogs are inserted after the static prompt
// boundary so provider system-prompt caches remain effective.
func (a *app) composeSystemPrompt() string {
	base := prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
	if a.cfg.AutoDiscover {
		base = prompts.EnsureToolDiscoveryInstructions(base)
	}
	return base
}

// composeSystemPromptForUser builds the stable base system prompt (including AGENTS.md)
// for the provided user.
//
// IMPORTANT: specialists are scoped per user. The user-scoped catalog is
// exposed via composeUserPromptContextForUser instead of this system prompt.
func (a *app) composeSystemPromptForUser(ctx context.Context, userID int64) string {
	return a.composeSystemPromptForUserWithOverride(ctx, userID, a.cfg.SystemPrompt)
}

func (a *app) composeSystemPromptForUserWithOverride(ctx context.Context, userID int64, systemPrompt string) string {
	base := prompts.DefaultSystemPrompt(a.cfg.Workdir, systemPrompt)
	if a.cfg.AutoDiscover {
		base = prompts.EnsureToolDiscoveryInstructions(base)
	}
	return base
}

func (a *app) composeUserPromptContext() string {
	if a.specRegistry == nil {
		return ""
	}
	return a.specRegistry.UserPromptContext()
}

func (a *app) composeUserPromptContextForUser(ctx context.Context, userID int64) string {
	reg, err := a.specialistsRegistryForUser(ctx, userID)
	if err != nil || reg == nil {
		return ""
	}
	return reg.UserPromptContext()
}

// refreshEngineSystemPrompt recomputes and assigns the system prompt on the live engine.
func (a *app) refreshEngineSystemPrompt() {
	if a.engine == nil {
		return
	}
	a.engine.System = a.composeSystemPrompt()
	a.engine.UserPromptContext = a.composeUserPromptContext()
}
