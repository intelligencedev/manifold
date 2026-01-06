package agentd

import (
	"context"

	"manifold/internal/agent/prompts"
)

// composeSystemPrompt builds the base system prompt (including AGENTS.md, if present)
// and appends the current specialists catalog so LLM clients see available names.
func (a *app) composeSystemPrompt() string {
	base := prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
	if a.specRegistry != nil {
		base = a.specRegistry.AppendToSystemPrompt(base)
	}
	return base
}

// composeSystemPromptForUser builds the base system prompt (including AGENTS.md)
// and appends the specialists catalog for the provided user.
//
// IMPORTANT: specialists are scoped per user. Non-system users must not receive
// the system (user=0) specialists catalog.
func (a *app) composeSystemPromptForUser(ctx context.Context, userID int64) string {
	base := prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
	reg, err := a.specialistsRegistryForUser(ctx, userID)
	if err != nil || reg == nil {
		return base
	}
	return reg.AppendToSystemPrompt(base)
}

// refreshEngineSystemPrompt recomputes and assigns the system prompt on the live engine.
func (a *app) refreshEngineSystemPrompt() {
	if a.engine == nil {
		return
	}
	a.engine.System = a.composeSystemPrompt()
}
