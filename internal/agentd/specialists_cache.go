package agentd

import (
	"context"
	"strings"

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
	// Derive a per-user base LLM config from user's orchestrator overlay if present
	base := a.cfg.LLMClient
	if sp, ok, _ := a.specStore.GetByName(ctx, userID, "orchestrator"); ok {
		provider := strings.TrimSpace(sp.Provider)
		if provider != "" {
			base.Provider = provider
		}
		switch base.Provider {
		case "google":
			if sp.BaseURL != "" {
				base.Google.BaseURL = sp.BaseURL
			}
			if sp.APIKey != "" {
				base.Google.APIKey = sp.APIKey
			}
			if sp.Model != "" {
				base.Google.Model = sp.Model
			}
		case "anthropic":
			if sp.BaseURL != "" {
				base.Anthropic.BaseURL = sp.BaseURL
			}
			if sp.APIKey != "" {
				base.Anthropic.APIKey = sp.APIKey
			}
			if sp.Model != "" {
				base.Anthropic.Model = sp.Model
			}
		default:
			if sp.BaseURL != "" {
				base.OpenAI.BaseURL = sp.BaseURL
			}
			if sp.APIKey != "" {
				base.OpenAI.APIKey = sp.APIKey
			}
			if sp.Model != "" {
				base.OpenAI.Model = sp.Model
			}
			if sp.ExtraHeaders != nil {
				base.OpenAI.ExtraHeaders = sp.ExtraHeaders
			}
			if sp.ExtraParams != nil {
				base.OpenAI.ExtraParams = sp.ExtraParams
			}
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
			a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
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
