package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestNormalizeAgentdSettings_PrefersCanonicalAliases(t *testing.T) {
	t.Parallel()

	settings := normalizeAgentdSettings(agentdSettings{
		SearXNGURL:    "https://legacy.example",
		WebSearXNGURL: "https://web.example",
		DatabaseURL:   "postgres://legacy",
		DBURL:         "postgres://dburl",
		PostgresDSN:   "postgres://canonical",
	})

	if settings.SearXNGURL != "https://web.example" || settings.WebSearXNGURL != "https://web.example" {
		t.Fatalf("expected web alias precedence for searxng URL, got %#v", settings)
	}
	if settings.DatabaseURL != "postgres://canonical" || settings.DBURL != "postgres://canonical" || settings.PostgresDSN != "postgres://canonical" {
		t.Fatalf("expected postgres DSN precedence, got %#v", settings)
	}
}

func TestApplyAgentdSettings_UsesNormalizedAliases(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	settings := agentdSettings{
		SearXNGURL:    "https://legacy.example",
		WebSearXNGURL: "https://web.example",
		DatabaseURL:   "postgres://legacy",
		DBURL:         "postgres://dburl",
		PostgresDSN:   "postgres://canonical",
	}

	if err := applyAgentdSettings(cfg, settings); err != nil {
		t.Fatalf("applyAgentdSettings error: %v", err)
	}

	if cfg.Web.SearXNGURL != "https://web.example" {
		t.Fatalf("expected normalized web searxng URL, got %q", cfg.Web.SearXNGURL)
	}
	if cfg.Databases.DefaultDSN != "postgres://canonical" {
		t.Fatalf("expected normalized default DSN, got %q", cfg.Databases.DefaultDSN)
	}
	if currentAgentdSettings(cfg).PostgresDSN != "postgres://canonical" {
		t.Fatalf("expected GET projection to mirror canonical DSN")
	}
}

func TestApplyAgentdSettings_RejectsPathLikeBlockBinaries(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	err := applyAgentdSettings(cfg, agentdSettings{BlockBinaries: "git,/bin/rm"})
	if err == nil {
		t.Fatal("expected validation error for path-like block binary")
	}
}

func TestApplyAgentdSettingsYAML_UsesNormalizedAliases(t *testing.T) {
	t.Parallel()

	root := map[string]any{}
	applyAgentdSettingsYAML(root, agentdSettings{
		SearXNGURL:    "https://legacy.example",
		WebSearXNGURL: "https://web.example",
		DatabaseURL:   "postgres://legacy",
		DBURL:         "postgres://dburl",
		PostgresDSN:   "postgres://canonical",
		BlockBinaries: "git, rg",
	})

	web, ok := root["web"].(map[string]any)
	if !ok || web["searXNGURL"] != "https://web.example" {
		t.Fatalf("expected normalized web URL in YAML map, got %#v", root["web"])
	}
	databases, ok := root["databases"].(map[string]any)
	if !ok || databases["defaultDSN"] != "postgres://canonical" {
		t.Fatalf("expected normalized DSN in YAML map, got %#v", root["databases"])
	}
	execCfg, ok := root["exec"].(map[string]any)
	if !ok {
		t.Fatalf("expected exec config in YAML map")
	}
	binaries, ok := execCfg["blockBinaries"].([]string)
	if !ok || len(binaries) != 2 || binaries[0] != "git" || binaries[1] != "rg" {
		t.Fatalf("expected split block binaries, got %#v", execCfg["blockBinaries"])
	}
}
