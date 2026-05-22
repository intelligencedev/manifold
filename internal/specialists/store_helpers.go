package specialists

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/tools"
)

// OrchestratorName is the reserved specialist name for the main orchestrator.
const OrchestratorName = "orchestrator"

// ConfigsFromStore converts persisted specialists into config entries.
func ConfigsFromStore(list []persistence.Specialist) []config.SpecialistConfig {
	out := make([]config.SpecialistConfig, 0, len(list))
	for _, s := range list {
		if isOrchestrator(s.Name) {
			continue
		}
		out = append(out, config.SpecialistConfig{
			Name:                       s.Name,
			Description:                s.Description,
			Provider:                   s.Provider,
			BaseURL:                    s.BaseURL,
			APIKey:                     s.APIKey,
			Model:                      s.Model,
			SummaryContextWindowTokens: s.SummaryContextWindowTokens,
			EnableTools:                s.EnableTools,
			ImageGeneration:            s.ImageGeneration,
			AutoDiscover:               s.AutoDiscover,
			Paused:                     s.Paused,
			AllowTools:                 s.AllowTools,
			ReasoningEffort:            s.ReasoningEffort,
			System:                     s.System,
			ExtraHeaders:               s.ExtraHeaders,
			ExtraParams:                s.ExtraParams,
			Harness:                    harnessConfigFromStore(s.Harness),
		})
	}
	return out
}

// ConfigsOrDefaults converts persisted specialists when available and otherwise
// falls back to the provided default configs.
func ConfigsOrDefaults(defaults []config.SpecialistConfig, list []persistence.Specialist, err error) []config.SpecialistConfig {
	if err == nil {
		return ConfigsFromStore(list)
	}
	return cloneSpecialistConfigs(defaults)
}

// NewRegistryFromStore builds a registry from persisted specialists when the
// store query succeeds, or from the provided defaults otherwise.
func NewRegistryFromStore(base config.LLMClientConfig, defaults []config.SpecialistConfig, list []persistence.Specialist, err error, httpClient *http.Client, toolsReg tools.Registry, workdir string) *Registry {
	return NewRegistryWithWorkdir(base, ConfigsOrDefaults(defaults, list, err), httpClient, toolsReg, workdir)
}

// ReplaceFromStore refreshes an existing registry from persisted specialists
// when available, or from defaults otherwise.
func ReplaceFromStore(reg *Registry, base config.LLMClientConfig, defaults []config.SpecialistConfig, list []persistence.Specialist, err error, httpClient *http.Client, toolsReg tools.Registry) {
	if reg == nil {
		return
	}
	reg.ReplaceFromConfigs(base, ConfigsOrDefaults(defaults, list, err), httpClient, toolsReg)
}

// SeedStore persists default specialists that are missing from the store.
func SeedStore(ctx context.Context, store persistence.SpecialistsStore, userID int64, defaults []config.SpecialistConfig) error {
	if store == nil {
		return errors.New("specialists store is nil")
	}
	list, err := store.List(ctx, userID)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(list))
	for _, s := range list {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		existing[name] = struct{}{}
	}
	for _, sc := range defaults {
		name := strings.TrimSpace(sc.Name)
		if name == "" {
			continue
		}
		if _, ok := existing[name]; ok {
			continue
		}
		_, err := store.Upsert(ctx, userID, persistence.Specialist{
			Name:                       name,
			Provider:                   sc.Provider,
			Description:                sc.Description,
			BaseURL:                    sc.BaseURL,
			APIKey:                     sc.APIKey,
			Model:                      sc.Model,
			SummaryContextWindowTokens: sc.SummaryContextWindowTokens,
			EnableTools:                sc.EnableTools,
			ImageGeneration:            sc.ImageGeneration,
			AutoDiscover:               sc.AutoDiscover,
			Paused:                     sc.Paused,
			AllowTools:                 sc.AllowTools,
			ReasoningEffort:            sc.ReasoningEffort,
			System:                     sc.System,
			ExtraHeaders:               sc.ExtraHeaders,
			ExtraParams:                sc.ExtraParams,
			Harness:                    harnessConfigToStore(sc.Harness),
		})
		if err != nil {
			return err
		}
		existing[name] = struct{}{}
	}
	return nil
}

func isOrchestrator(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), OrchestratorName)
}

func harnessConfigFromStore(in *persistence.SpecialistHarness) *config.HarnessConfig {
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
	for tool, prereqs := range in.ToolPrerequisites {
		for _, prereq := range prereqs {
			out.ToolPrerequisites[tool] = append(out.ToolPrerequisites[tool], config.HarnessPrerequisite{
				Tool:     prereq.Tool,
				MatchArg: prereq.MatchArg,
			})
		}
	}
	if len(out.ToolPrerequisites) == 0 {
		out.ToolPrerequisites = nil
	}
	return out
}

func harnessConfigToStore(in *config.HarnessConfig) *persistence.SpecialistHarness {
	if in == nil {
		return nil
	}
	out := &persistence.SpecialistHarness{
		Enabled:           in.Enabled,
		Mode:              in.Mode,
		RescueEnabled:     in.RescueEnabled,
		MaxRetriesPerStep: in.MaxRetriesPerStep,
		MaxToolErrors:     in.MaxToolErrors,
		TerminalTools:     append([]string(nil), in.TerminalTools...),
		RequiredSteps:     append([]string(nil), in.RequiredSteps...),
		ToolPrerequisites: make(map[string][]persistence.SpecialistHarnessPrerequisite, len(in.ToolPrerequisites)),
		Compact: persistence.SpecialistHarnessCompact{
			Enabled:         in.Compact.Enabled,
			KeepRecentSteps: in.Compact.KeepRecentSteps,
			PhaseThresholds: append([]float64(nil), in.Compact.PhaseThresholds...),
		},
	}
	for tool, prereqs := range in.ToolPrerequisites {
		for _, prereq := range prereqs {
			out.ToolPrerequisites[tool] = append(out.ToolPrerequisites[tool], persistence.SpecialistHarnessPrerequisite{
				Tool:     prereq.Tool,
				MatchArg: prereq.MatchArg,
			})
		}
	}
	if len(out.ToolPrerequisites) == 0 {
		out.ToolPrerequisites = nil
	}
	return out
}
