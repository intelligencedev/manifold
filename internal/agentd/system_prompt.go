package agentd

import (
	"context"

	"manifold/internal/agent/prompts"
	"manifold/internal/config"
)

func promptInstructionOverrides(cfg *config.Config) prompts.InstructionOverrides {
	if cfg == nil {
		return prompts.InstructionOverrides{}
	}
	return prompts.InstructionOverrides{
		BaseSystem:                 cfg.PromptOverrides.BaseSystem,
		MemoryInstructions:         cfg.PromptOverrides.MemoryInstructions,
		ToolDiscoveryInstructions:  cfg.PromptOverrides.ToolDiscoveryInstructions,
		SkillDiscoveryInstructions: cfg.PromptOverrides.SkillDiscoveryInstructions,
	}
}

// composeSystemPrompt builds the stable base system prompt (including AGENTS.md,
// if present). Dynamic specialist catalogs are inserted after the static prompt
// boundary so provider system-prompt caches remain effective.
func (a *app) composeSystemPrompt() string {
	return prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt, promptInstructionOverrides(a.cfg))
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
	return prompts.DefaultSystemPrompt(a.cfg.Workdir, systemPrompt, promptInstructionOverrides(a.cfg))
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
