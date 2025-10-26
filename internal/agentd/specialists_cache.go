package agentd

import (
	"context"

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
	// Derive a per-user base OpenAI config from user's orchestrator overlay if present
	base := a.cfg.OpenAI
	if sp, ok, _ := a.specStore.GetByName(ctx, userID, "orchestrator"); ok {
		if sp.BaseURL != "" {
			base.BaseURL = sp.BaseURL
		}
		if sp.APIKey != "" {
			base.APIKey = sp.APIKey
		}
		if sp.Model != "" {
			base.Model = sp.Model
		}
	}
	reg := specialists.NewRegistry(base, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)

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
			a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
			a.specRegMu.Lock()
			if a.userSpecRegs == nil {
				a.userSpecRegs = map[int64]*specialists.Registry{}
			}
			a.userSpecRegs[systemUserID] = a.specRegistry
			a.specRegMu.Unlock()
		}
		return
	}
	a.specRegMu.Lock()
	if a.userSpecRegs != nil {
		delete(a.userSpecRegs, userID)
	}
	a.specRegMu.Unlock()
}
