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
	reg := specialists.NewRegistry(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)

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
