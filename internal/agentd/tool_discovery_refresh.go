package agentd

import (
	"github.com/rs/zerolog/log"

	"manifold/internal/specialists"
	agenttools "manifold/internal/tools/agents"
	tooldiscovery "manifold/internal/tools/discovery"
)

func (a *app) refreshToolDiscoveryIndex() {
	if a == nil || a.baseToolRegistry == nil {
		return
	}
	a.toolIndex = tooldiscovery.NewToolIndex(a.baseToolRegistry.Schemas())
	if a.cfg == nil {
		return
	}

	a.refreshOrchestratorToolRegistry()
	if a.engine != nil {
		a.engine.Tools = a.toolRegistry
		if d, ok := a.engine.Delegator.(*agenttools.Delegator); ok {
			d.SetRegistry(a.toolRegistry)
		}
	}

	a.propagateToolDiscoveryIndexToSpecialists()
	log.Info().Int("tools", len(a.baseToolRegistry.Schemas())).Msg("tool_discovery_index_refreshed")
}

func (a *app) propagateToolDiscoveryIndexToSpecialists() {
	if a == nil || a.cfg == nil {
		return
	}
	if a.specRegistry != nil {
		a.specRegistry.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
	}

	a.specRegMu.RLock()
	regs := make([]*specialists.Registry, 0, len(a.userSpecRegs))
	for _, reg := range a.userSpecRegs {
		if reg != nil {
			regs = append(regs, reg)
		}
	}
	a.specRegMu.RUnlock()

	for _, reg := range regs {
		reg.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
	}
}
