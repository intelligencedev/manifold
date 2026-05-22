package agentd

import (
	"manifold/internal/agent/harness"
	"manifold/internal/config"
	persist "manifold/internal/persistence"
)

func harnessRunConfig(cfg config.HarnessConfig) harness.RunConfig {
	prereqs := make(map[string][]harness.Prerequisite, len(cfg.ToolPrerequisites))
	for tool, entries := range cfg.ToolPrerequisites {
		for _, entry := range entries {
			prereqs[tool] = append(prereqs[tool], harness.Prerequisite{
				Tool:     entry.Tool,
				MatchArg: entry.MatchArg,
			})
		}
	}
	if len(prereqs) == 0 {
		prereqs = nil
	}
	return harness.RunConfig{
		Mode:              harness.Mode(cfg.Mode),
		RescueEnabled:     cfg.RescueEnabled,
		MaxRetriesPerStep: cfg.MaxRetriesPerStep,
		MaxToolErrors:     cfg.MaxToolErrors,
		Workflow: harness.WorkflowConfig{
			RequiredSteps:     append([]string(nil), cfg.RequiredSteps...),
			TerminalTools:     append([]string(nil), cfg.TerminalTools...),
			ToolPrerequisites: prereqs,
		},
		Compact: harness.CompactConfig{
			Enabled:         cfg.Compact.Enabled,
			KeepRecentSteps: cfg.Compact.KeepRecentSteps,
			PhaseThresholds: append([]float64(nil), cfg.Compact.PhaseThresholds...),
		},
	}
}

func harnessOverrideConfig(base config.HarnessConfig, override *config.HarnessConfig) config.HarnessConfig {
	if override == nil {
		return base
	}
	out := *override
	out.TerminalTools = append([]string(nil), override.TerminalTools...)
	out.RequiredSteps = append([]string(nil), override.RequiredSteps...)
	if len(override.ToolPrerequisites) > 0 {
		out.ToolPrerequisites = make(map[string][]config.HarnessPrerequisite, len(override.ToolPrerequisites))
		for tool, entries := range override.ToolPrerequisites {
			out.ToolPrerequisites[tool] = append([]config.HarnessPrerequisite(nil), entries...)
		}
	}
	out.Compact.PhaseThresholds = append([]float64(nil), override.Compact.PhaseThresholds...)
	applyHarnessOverrideDefaults(&out)
	return out
}

func applyHarnessOverrideDefaults(cfg *config.HarnessConfig) {
	if cfg.Mode == "" {
		cfg.Mode = "guarded_chat"
	}
	if cfg.MaxRetriesPerStep <= 0 {
		cfg.MaxRetriesPerStep = 3
	}
	if cfg.MaxToolErrors <= 0 {
		cfg.MaxToolErrors = 2
	}
	if len(cfg.TerminalTools) == 0 {
		cfg.TerminalTools = []string{"agent_response"}
	}
	if cfg.Compact.KeepRecentSteps <= 0 {
		cfg.Compact.KeepRecentSteps = 4
	}
	if len(cfg.Compact.PhaseThresholds) == 0 {
		cfg.Compact.PhaseThresholds = []float64{0.60, 0.75, 0.90}
	}
}

func harnessConfigFromPersist(in *persist.SpecialistHarness) *config.HarnessConfig {
	if in == nil {
		return nil
	}
	out := &config.HarnessConfig{
		Enabled:           in.Enabled,
		Mode:              in.Mode,
		RescueEnabled:     in.RescueEnabled,
		MaxRetriesPerStep: in.MaxRetriesPerStep,
		MaxToolErrors:     in.MaxToolErrors,
		TerminalTools:     append([]string(nil), in.TerminalTools...),
		RequiredSteps:     append([]string(nil), in.RequiredSteps...),
		ToolPrerequisites: make(map[string][]config.HarnessPrerequisite, len(in.ToolPrerequisites)),
		Compact: config.HarnessCompactConfig{
			Enabled:         in.Compact.Enabled,
			KeepRecentSteps: in.Compact.KeepRecentSteps,
			PhaseThresholds: append([]float64(nil), in.Compact.PhaseThresholds...),
		},
	}
	for tool, entries := range in.ToolPrerequisites {
		for _, entry := range entries {
			out.ToolPrerequisites[tool] = append(out.ToolPrerequisites[tool], config.HarnessPrerequisite{
				Tool:     entry.Tool,
				MatchArg: entry.MatchArg,
			})
		}
	}
	if len(out.ToolPrerequisites) == 0 {
		out.ToolPrerequisites = nil
	}
	return out
}
