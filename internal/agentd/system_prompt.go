package agentd

import "manifold/internal/agent/prompts"

// composeSystemPrompt builds the base system prompt (including AGENTS.md, if present)
// and appends the current specialists catalog so LLM clients see available names.
func (a *app) composeSystemPrompt() string {
	base := prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
	if a.specRegistry != nil {
		base = a.specRegistry.AppendToSystemPrompt(base)
	}
	return base
}

// refreshEngineSystemPrompt recomputes and assigns the system prompt on the live engine.
func (a *app) refreshEngineSystemPrompt() {
	if a.engine == nil {
		return
	}
	a.engine.System = a.composeSystemPrompt()
}
