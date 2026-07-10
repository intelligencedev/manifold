package agentd

import (
	"context"
	"slices"
	"testing"

	"manifold/internal/config"
)

func TestOrchestratorSeedDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLMClient.Provider = "openai"
	a := &app{cfg: cfg, specStore: nil}

	// specStore is nil; orchestratorSpecialist tolerates a missing store (no overlay).
	sp := a.orchestratorSpecialist(context.Background(), systemUserID)

	if !sp.EnableTools {
		t.Errorf("expected EnableTools=true for a freshly-seeded orchestrator")
	}
	if sp.AutoDiscover == nil || !*sp.AutoDiscover {
		t.Errorf("expected AutoDiscover=true, got %v", sp.AutoDiscover)
	}
	if sp.RequestInfoEnabled == nil || !*sp.RequestInfoEnabled {
		t.Errorf("expected RequestInfoEnabled=true, got %v", sp.RequestInfoEnabled)
	}
	for _, tool := range []string{"run_cli", "web_fetch", "transit_search", "transit_discover"} {
		if !slices.Contains(sp.AllowTools, tool) {
			t.Errorf("expected default allow-list to include %q; got %v", tool, sp.AllowTools)
		}
	}
}

func TestOrchestratorSeedRespectsConfiguredAllowList(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLMClient.Provider = "openai"
	cfg.ToolAllowList = []string{"only_this_tool"}
	a := &app{cfg: cfg}

	sp := a.orchestratorSpecialist(context.Background(), systemUserID)
	if !slices.Equal(sp.AllowTools, []string{"only_this_tool"}) {
		t.Errorf("expected configured allow-list to be preserved, got %v", sp.AllowTools)
	}
}
