package agentd

import (
	"testing"

	"manifold/internal/agent"
	"manifold/internal/config"
)

func TestRefreshLexMinifyRuntimeUpdatesActiveEngine(t *testing.T) {
	t.Parallel()

	a := &app{
		cfg: &config.Config{
			LexMinify: config.LexMinifyConfig{
				Enabled:                true,
				Level:                  4,
				Zones:                  7,
				CurrentRequestMaxLevel: 2,
			},
		},
		engine: &agent.Engine{},
	}

	a.refreshLexMinifyRuntime()

	if got, want := a.engine.LexMinifyLevel, 4; got != want {
		t.Fatalf("engine lexminify level = %d, want %d", got, want)
	}
	if got, want := a.engine.LexMinifyZones, 7; got != want {
		t.Fatalf("engine lexminify zones = %d, want %d", got, want)
	}
	if got, want := a.engine.LexMinifyCurrentMax, 2; got != want {
		t.Fatalf("engine lexminify current max = %d, want %d", got, want)
	}
}
