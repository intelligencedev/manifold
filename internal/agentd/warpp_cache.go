package agentd

import (
	"context"
	"encoding/json"

	"manifold/internal/warpp"
)

func (a *app) warppRegistryForUser(ctx context.Context, userID int64) (*warpp.Registry, error) {
	if !a.cfg.Auth.Enabled || userID == systemUserID {
		a.warppMu.RLock()
		reg := a.warppRegistries[systemUserID]
		a.warppMu.RUnlock()
		return reg, nil
	}

	a.warppMu.RLock()
	if reg, ok := a.warppRegistries[userID]; ok {
		a.warppMu.RUnlock()
		return reg, nil
	}
	a.warppMu.RUnlock()

	list, err := a.warppStore.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	reg := &warpp.Registry{}
	for _, pw := range list {
		b, err := json.Marshal(pw)
		if err != nil {
			continue
		}
		var wf warpp.Workflow
		if err := json.Unmarshal(b, &wf); err != nil {
			continue
		}
		reg.Upsert(wf, "")
	}

	a.warppMu.Lock()
	if a.warppRegistries == nil {
		a.warppRegistries = map[int64]*warpp.Registry{}
	}
	a.warppRegistries[userID] = reg
	a.warppMu.Unlock()
	return reg, nil
}

func (a *app) warppRunnerForUser(ctx context.Context, userID int64) (*warpp.Runner, error) {
	reg, err := a.warppRegistryForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &warpp.Runner{Workflows: reg, Tools: a.warppRunner.Tools}, nil
}

func (a *app) invalidateWarppCache(ctx context.Context, userID int64) {
	if userID == systemUserID {
		if list, err := a.warppStore.ListWorkflows(ctx, systemUserID); err == nil {
			reg := &warpp.Registry{}
			for _, pw := range list {
				b, err := json.Marshal(pw)
				if err != nil {
					continue
				}
				var wf warpp.Workflow
				if err := json.Unmarshal(b, &wf); err != nil {
					continue
				}
				reg.Upsert(wf, "")
			}
			a.warppMu.Lock()
			if a.warppRegistries == nil {
				a.warppRegistries = map[int64]*warpp.Registry{}
			}
			a.warppRegistries[systemUserID] = reg
			if a.warppRunner != nil {
				a.warppRunner.Workflows = reg
			}
			a.warppMu.Unlock()
		}
		return
	}

	a.warppMu.Lock()
	if a.warppRegistries != nil {
		delete(a.warppRegistries, userID)
	}
	a.warppMu.Unlock()
}
