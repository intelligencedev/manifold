package specialists

import (
	"context"
	"errors"
	"strings"

	"manifold/internal/config"
	"manifold/internal/persistence"
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
			Paused:                     s.Paused,
			AllowTools:                 s.AllowTools,
			ReasoningEffort:            s.ReasoningEffort,
			System:                     s.System,
			ExtraHeaders:               s.ExtraHeaders,
			ExtraParams:                s.ExtraParams,
		})
	}
	return out
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
			Paused:                     sc.Paused,
			AllowTools:                 sc.AllowTools,
			ReasoningEffort:            sc.ReasoningEffort,
			System:                     sc.System,
			ExtraHeaders:               sc.ExtraHeaders,
			ExtraParams:                sc.ExtraParams,
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
