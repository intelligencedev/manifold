package agentd

import (
	"context"

	"manifold/internal/config"
	"manifold/internal/specialists"
)

func (a *app) specialistsRegistryForUser(ctx context.Context, userID int64) (*specialists.Registry, error) {
	if !a.cfg.Auth.Enabled || userID == systemUserID {
		return a.specRegistry, nil
	}
	a.specRegMu.RLock()
	if reg, ok := a.userSpecRegs[userID]; ok {
		a.specRegMu.RUnlock()
		return reg, nil
	}
	a.specRegMu.RUnlock()

	list, err := a.specStore.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Derive a per-user base LLM config from the user's orchestrator overlay, if present.
	base := a.cfg.LLMClient
	if sp, ok, _ := a.specStore.GetByName(ctx, userID, specialists.OrchestratorName); ok {
		base, _ = specialists.ApplyLLMClientOverride(base, sp)
	}
	reg := specialists.NewRegistryFromStore(specialists.StoreRegistryRequest{
		Base:       base,
		List:       list,
		HTTPClient: a.httpClient,
		Tools:      a.baseToolRegistry,
		Workdir:    a.cfg.Workdir,
	})
	reg.SetPromptOverrides(promptInstructionOverrides(a.cfg))
	reg.SetRequestInfoEnabled(config.RequestInfoEnabled(a.cfg.RequestInfoEnabled))
	reg.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)

	a.specRegMu.Lock()
	if a.userSpecRegs == nil {
		a.userSpecRegs = map[int64]*specialists.Registry{}
	}
	a.userSpecRegs[userID] = reg
	a.specRegMu.Unlock()
	return reg, nil
}

func (a *app) invalidateSpecialistsCache(ctx context.Context, userID int64) {
	if userID == systemUserID {
		if list, err := a.specStore.List(ctx, systemUserID); err == nil {
			specialists.ReplaceFromStore(a.specRegistry, specialists.StoreRegistryRequest{
				Base:       a.cfg.LLMClient,
				Defaults:   a.cfg.Specialists,
				List:       list,
				HTTPClient: a.httpClient,
				Tools:      a.baseToolRegistry,
			})
			a.specRegistry.SetPromptOverrides(promptInstructionOverrides(a.cfg))
			a.specRegistry.SetRequestInfoEnabled(config.RequestInfoEnabled(a.cfg.RequestInfoEnabled))
			a.specRegistry.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
			a.specRegMu.Lock()
			if a.userSpecRegs == nil {
				a.userSpecRegs = map[int64]*specialists.Registry{}
			}
			a.userSpecRegs[systemUserID] = a.specRegistry
			a.specRegMu.Unlock()
			a.refreshEngineSystemPrompt()
		}
		return
	}
	a.specRegMu.Lock()
	if a.userSpecRegs != nil {
		delete(a.userSpecRegs, userID)
	}
	a.specRegMu.Unlock()
}
