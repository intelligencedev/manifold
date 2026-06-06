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
		SearXNGURL:                          "https://legacy.example",
		WebSearXNGURL:                       "https://web.example",
		DatabaseURL:                         "postgres://legacy",
		DBURL:                               "postgres://dburl",
		PostgresDSN:                         "postgres://canonical",
		SummaryContextWindowTokens:          32000,
		SummaryPlainTextContextWindowTokens: 8192,
		SummaryReserveBufferTokens:          25000,
		SummaryMinKeepLastMessages:          4,
		SummaryMaxKeepLastMessages:          12,
		SummaryMaxSummaryChunkTokens:        4096,
		SummaryCallTimeoutSeconds:           120,
		EmbedInstructionMode:                "enabled",
		EmbedInstructionFormat:              "qwen",
		EmbedRAGQueryInstruction:            "Retrieve relevant passages.",
		PromptBaseSystem:                    "Base system",
		PromptMemoryInstructions:            "[memory] custom [/memory]",
		PromptToolDiscoveryInstructions:     "[tool_discovery] custom [/tool_discovery]",
		PromptSkillDiscoveryInstructions:    "[skill_discovery] custom [/skill_discovery]",
		RerankEnabled:                       true,
		RerankBaseURL:                       "http://localhost:8203",
		RerankModel:                         "qwen3-reranker-0.6b",
		RerankInstruction:                   "Classify whether the document matches the query topic",
		CommandRules: []config.ExecCommandRule{{
			ID:       "allow-go-test",
			Decision: "allow",
			Pattern:  []string{"go", "test"},
			Contexts: []string{"cli"},
		}},
		SandboxEnabled:               boolPtr(false),
		SandboxFailIfUnavailable:     boolPtr(true),
		SandboxNetworkEnabled:        boolPtr(false),
		SandboxNetworkAllowedDomains: []string{"example.com"},
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
	if cfg.Summary.PlainTextContextWindowTokens != 8192 {
		t.Fatalf("expected plain text summary context tokens, got %d", cfg.Summary.PlainTextContextWindowTokens)
	}
	if cfg.Summary.ContextWindowTokens != 32000 || cfg.SummaryContextWindowTokens != 32000 {
		t.Fatalf("expected summary context tokens to sync, nested=%d top=%d", cfg.Summary.ContextWindowTokens, cfg.SummaryContextWindowTokens)
	}
	if cfg.Summary.MaxSummaryChunkTokens != 4096 || cfg.SummaryMaxSummaryChunkTokens != 4096 {
		t.Fatalf("expected summary chunk tokens to sync, nested=%d top=%d", cfg.Summary.MaxSummaryChunkTokens, cfg.SummaryMaxSummaryChunkTokens)
	}
	if got := currentAgentdSettings(cfg); got.SummaryTokenBudget != 7000 {
		t.Fatalf("expected projected summary token budget 7000, got %d", got.SummaryTokenBudget)
	}
	if cfg.Embedding.Instructions.Mode != "enabled" || cfg.Embedding.Instructions.RAGQuery != "Retrieve relevant passages." {
		t.Fatalf("expected embedding instruction settings, got %+v", cfg.Embedding.Instructions)
	}
	if cfg.PromptOverrides.BaseSystem != "Base system" ||
		cfg.PromptOverrides.MemoryInstructions != "[memory] custom [/memory]" ||
		cfg.PromptOverrides.ToolDiscoveryInstructions != "[tool_discovery] custom [/tool_discovery]" ||
		cfg.PromptOverrides.SkillDiscoveryInstructions != "[skill_discovery] custom [/skill_discovery]" {
		t.Fatalf("expected prompt overrides, got %+v", cfg.PromptOverrides)
	}
	if !cfg.Reranking.Enabled || cfg.Reranking.BaseURL != "http://localhost:8203" || cfg.Reranking.Model != "qwen3-reranker-0.6b" {
		t.Fatalf("expected reranking settings, got %+v", cfg.Reranking)
	}
	if cfg.Reranking.Instruction != "Classify whether the document matches the query topic" {
		t.Fatalf("expected reranking instruction, got %q", cfg.Reranking.Instruction)
	}
	if currentAgentdSettings(cfg).RerankInstruction != "Classify whether the document matches the query topic" {
		t.Fatalf("expected GET projection to include reranking instruction")
	}
	if len(cfg.Exec.CommandRules) != 1 || cfg.Exec.CommandRules[0].ID != "allow-go-test" {
		t.Fatalf("expected command rules to apply, got %+v", cfg.Exec.CommandRules)
	}
	if cfg.Exec.Sandbox.Enabled == nil || *cfg.Exec.Sandbox.Enabled {
		t.Fatalf("expected sandbox enabled=false, got %+v", cfg.Exec.Sandbox.Enabled)
	}
	if cfg.Exec.Sandbox.Network.Enabled == nil || *cfg.Exec.Sandbox.Network.Enabled || len(cfg.Exec.Sandbox.Network.AllowedDomains) != 1 {
		t.Fatalf("expected sandbox network settings, got %+v", cfg.Exec.Sandbox.Network)
	}
	if got := currentAgentdSettings(cfg); len(got.CommandRules) != 1 || got.CommandRules[0].ID != "allow-go-test" || got.SandboxEnabled == nil || *got.SandboxEnabled {
		t.Fatalf("expected GET projection to include exec security settings, got %+v", got)
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
		SearXNGURL:                          "https://legacy.example",
		WebSearXNGURL:                       "https://web.example",
		DatabaseURL:                         "postgres://legacy",
		DBURL:                               "postgres://dburl",
		PostgresDSN:                         "postgres://canonical",
		SummaryContextWindowTokens:          32000,
		SummaryPlainTextContextWindowTokens: 8192,
		SummaryReserveBufferTokens:          25000,
		SummaryMinKeepLastMessages:          4,
		SummaryMaxKeepLastMessages:          12,
		SummaryMaxSummaryChunkTokens:        4096,
		SummaryCallTimeoutSeconds:           120,
		BlockBinaries:                       "git, rg",
		EmbedInstructionMode:                "enabled",
		EmbedInstructionFormat:              "qwen",
		EmbedEvolvingMemoryQueryInstruction: "Retrieve relevant memories.",
		PromptBaseSystem:                    "Base system",
		PromptMemoryInstructions:            "[memory] custom [/memory]",
		PromptToolDiscoveryInstructions:     "[tool_discovery] custom [/tool_discovery]",
		PromptSkillDiscoveryInstructions:    "[skill_discovery] custom [/skill_discovery]",
		RerankEnabled:                       true,
		RerankBaseURL:                       "http://localhost:8203",
		RerankModel:                         "qwen3-reranker-0.6b",
		RerankInstruction:                   "Classify whether the document matches the query topic",
		CommandRules: []config.ExecCommandRule{{
			ID:       "allow-go-test",
			Decision: "allow",
			Pattern:  []string{"go", "test"},
			Contexts: []string{"cli"},
		}},
		SandboxEnabled:               boolPtr(false),
		SandboxFailIfUnavailable:     boolPtr(true),
		SandboxNetworkEnabled:        boolPtr(false),
		SandboxNetworkAllowedDomains: []string{"example.com"},
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
	summaryCfg, ok := root["summary"].(map[string]any)
	if !ok || summaryCfg["contextWindowTokens"] != 32000 || summaryCfg["plainTextContextWindowTokens"] != 8192 || summaryCfg["reserveBufferTokens"] != 25000 {
		t.Fatalf("expected summary budget settings in YAML map, got %#v", root["summary"])
	}
	if root["summaryContextWindowTokens"] != 32000 || root["summaryReserveBufferTokens"] != 25000 {
		t.Fatalf("expected top-level summary aliases in YAML map, got %#v", root)
	}
	binaries, ok := execCfg["blockBinaries"].([]string)
	if !ok || len(binaries) != 2 || binaries[0] != "git" || binaries[1] != "rg" {
		t.Fatalf("expected split block binaries, got %#v", execCfg["blockBinaries"])
	}
	if _, ok := execCfg["commandRules"]; ok {
		t.Fatalf("command rules should not be written to YAML map, got %#v", execCfg["commandRules"])
	}
	sandboxCfg, ok := execCfg["sandbox"].(map[string]any)
	if !ok || sandboxCfg["enabled"] != false || sandboxCfg["failIfUnavailable"] != true {
		t.Fatalf("expected sandbox config in YAML map, got %#v", execCfg["sandbox"])
	}
	networkCfg, ok := sandboxCfg["network"].(map[string]any)
	if !ok || networkCfg["enabled"] != false {
		t.Fatalf("expected sandbox network config in YAML map, got %#v", sandboxCfg["network"])
	}
	embeddingCfg, ok := root["embedding"].(map[string]any)
	if !ok {
		t.Fatalf("expected embedding config in YAML map")
	}
	instructions, ok := embeddingCfg["instructions"].(map[string]any)
	if !ok || instructions["mode"] != "enabled" || instructions["evolvingMemoryQuery"] != "Retrieve relevant memories." {
		t.Fatalf("expected embedding instructions in YAML map, got %#v", embeddingCfg["instructions"])
	}
	promptOverrides, ok := root["promptOverrides"].(map[string]any)
	if !ok || promptOverrides["baseSystem"] != "Base system" || promptOverrides["memoryInstructions"] != "[memory] custom [/memory]" {
		t.Fatalf("expected prompt overrides in YAML map, got %#v", root["promptOverrides"])
	}
	rerankingCfg, ok := root["reranking"].(map[string]any)
	if !ok || rerankingCfg["enabled"] != true || rerankingCfg["baseURL"] != "http://localhost:8203" || rerankingCfg["model"] != "qwen3-reranker-0.6b" {
		t.Fatalf("expected reranking config in YAML map, got %#v", root["reranking"])
	}
	if rerankingCfg["instruction"] != "Classify whether the document matches the query topic" {
		t.Fatalf("expected reranking instruction in YAML map, got %#v", root["reranking"])
	}
}
