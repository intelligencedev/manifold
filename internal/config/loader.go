package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	yaml "gopkg.in/yaml.v3"
)

// Load reads configuration from YAML files, with optional ${VAR} expansion from
// the current environment or a loaded .env file.
func Load() (Config, error) {
	_ = godotenv.Overload()

	cfg := Config{}
	cfg.Tokenization.FallbackToHeuristic = true

	configPath, err := findRequiredFile("config.yaml", "config.yml")
	if err != nil {
		return Config{}, err
	}
	if err := loadMainConfig(configPath, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadExternalConfigs(&cfg); err != nil {
		return Config{}, err
	}

	mergeOpenAIConfig(&cfg.LLMClient.OpenAI, cfg.OpenAI)
	applyDefaults(&cfg)
	applyDerivedConfig(&cfg)
	if err := validateConfig(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadMainConfig(path string, cfg *Config) error {
	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("%s: could not parse configuration: %w", path, err)
	}

	var aliases struct {
		OutputTruncateByte int `yaml:"outputTruncateByte"`
	}
	if err := yaml.Unmarshal(data, &aliases); err != nil {
		return fmt.Errorf("%s: could not parse configuration aliases: %w", path, err)
	}
	if cfg.OutputTruncateByte == 0 && aliases.OutputTruncateByte > 0 {
		cfg.OutputTruncateByte = aliases.OutputTruncateByte
	}

	return nil
}

func loadExternalConfigs(cfg *Config) error {
	if err := loadSpecialistsFile(cfg); err != nil {
		return err
	}
	if err := loadMCPFile(cfg); err != nil {
		return err
	}
	return nil
}

func loadSpecialistsFile(cfg *Config) error {
	path, found, err := findOptionalConfigFile(os.Getenv("SPECIALISTS_CONFIG"), "specialists.yaml", "specialists.yml")
	if err != nil || !found {
		return err
	}

	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}

	type specialistFile struct {
		Specialists []SpecialistConfig `yaml:"specialists"`
		Routes      []SpecialistRoute  `yaml:"routes"`
	}

	var wrapped specialistFile
	if err := yaml.Unmarshal(data, &wrapped); err == nil && (len(wrapped.Specialists) > 0 || len(wrapped.Routes) > 0) {
		if len(wrapped.Specialists) > 0 {
			cfg.Specialists = wrapped.Specialists
		}
		if len(wrapped.Routes) > 0 {
			cfg.SpecialistRoutes = wrapped.Routes
		}
		return nil
	}

	var list []SpecialistConfig
	if err := yaml.Unmarshal(data, &list); err == nil {
		cfg.Specialists = list
		return nil
	}

	return fmt.Errorf("%s: could not parse specialists configuration", path)
}

func loadMCPFile(cfg *Config) error {
	path, found, err := findOptionalConfigFile(os.Getenv("MCP_CONFIG"), "mcp.yaml", "mcp.yml")
	if err != nil || !found {
		return err
	}

	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}

	var wrapped struct {
		Servers []MCPServerConfig `yaml:"servers"`
		MCP     MCPConfig         `yaml:"mcp"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return fmt.Errorf("%s: could not parse MCP configuration: %w", path, err)
	}

	if len(wrapped.Servers) > 0 {
		cfg.MCP.Servers = wrapped.Servers
		return nil
	}
	if len(wrapped.MCP.Servers) > 0 {
		cfg.MCP = wrapped.MCP
	}
	return nil
}

func readExpandedYAML(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return []byte(os.ExpandEnv(string(data))), nil
}

func findRequiredFile(paths ...string) (string, error) {
	path, found, err := findFirstFile(paths...)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no configuration file found; expected one of: %s", strings.Join(paths, ", "))
	}
	return path, nil
}

func findOptionalConfigFile(override string, defaults ...string) (string, bool, error) {
	candidates := defaults
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		candidates = append([]string{trimmed}, defaults...)
	}
	return findFirstFile(candidates...)
}

func findFirstFile(paths ...string) (string, bool, error) {
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		info, err := os.Stat(trimmed)
		if err == nil {
			if info.IsDir() {
				return "", false, fmt.Errorf("configuration path must be a file: %s", trimmed)
			}
			return trimmed, true, nil
		}
		if os.IsNotExist(err) {
			continue
		}
		return "", false, fmt.Errorf("stat %s: %w", trimmed, err)
	}
	return "", false, nil
}

func applyDerivedConfig(cfg *Config) {
	cfg.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider))
	cfg.Summary.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.Summary.LLMClient.Provider))
	cfg.Harness.Mode = strings.ToLower(strings.TrimSpace(cfg.Harness.Mode))
	cfg.EvolvingMemory.Provider = strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.Provider))
	cfg.EvolvingMemory.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.LLMClient.Provider))
	cfg.BeliefMemory.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.BeliefMemory.LLMClient.Provider))
	cfg.BeliefMemory.Distillation.Mode = strings.ToLower(strings.TrimSpace(cfg.BeliefMemory.Distillation.Mode))

	if cfg.LLMClient.Provider == "local" {
		cfg.LLMClient.OpenAI.API = "completions"
	}
	if cfg.Summary.LLMClient.Provider == "local" {
		cfg.Summary.LLMClient.OpenAI.API = "completions"
	}
	cfg.OpenAI = cfg.LLMClient.OpenAI

	for i := range cfg.Specialists {
		if strings.TrimSpace(cfg.Specialists[i].Provider) == "" {
			cfg.Specialists[i].Provider = cfg.LLMClient.Provider
		}
		if cfg.Specialists[i].Harness != nil {
			cfg.Specialists[i].Harness.Mode = strings.ToLower(strings.TrimSpace(cfg.Specialists[i].Harness.Mode))
		}
	}
}

func validateConfig(cfg *Config) error {
	if err := validateConfigProviders(cfg); err != nil {
		return err
	}
	if err := validateConfigHarnesses(cfg); err != nil {
		return err
	}
	if err := validateConfigProviderCredentials(cfg); err != nil {
		return err
	}
	if err := validateConfigWorkdir(cfg); err != nil {
		return err
	}
	if err := validateConfigExec(cfg); err != nil {
		return err
	}
	if err := validateConfigMemory(cfg); err != nil {
		return err
	}
	if err := validateConfigEmbedding(cfg); err != nil {
		return err
	}
	if err := validateConfigReranking(cfg); err != nil {
		return err
	}
	return nil
}

func validateConfigProviders(cfg *Config) error {
	if err := validateProvider("llm_client.provider", cfg.LLMClient.Provider); err != nil {
		return err
	}
	if cfg.EvolvingMemory.Provider != "" {
		if err := validateProvider("evolvingMemory.provider", cfg.EvolvingMemory.Provider); err != nil {
			return err
		}
	}
	if cfg.EvolvingMemory.LLMClient.Provider != "" {
		if err := validateProvider("evolvingMemory.llmClient.provider", cfg.EvolvingMemory.LLMClient.Provider); err != nil {
			return err
		}
	}
	if cfg.Summary.Enabled {
		if err := validateProvider("summary.llm_client.provider", cfg.Summary.LLMClient.Provider); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigHarnesses(cfg *Config) error {
	if err := validateHarnessConfig("harness", cfg.Harness); err != nil {
		return err
	}
	for i, specialist := range cfg.Specialists {
		if specialist.Harness == nil {
			continue
		}
		name := strings.TrimSpace(specialist.Name)
		if name == "" {
			name = fmt.Sprintf("%d", i)
		}
		if err := validateHarnessConfig("specialists."+name+".harness", *specialist.Harness); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigProviderCredentials(cfg *Config) error {
	switch cfg.LLMClient.Provider {
	case "openai":
		if strings.TrimSpace(cfg.LLMClient.OpenAI.APIKey) == "" {
			return errors.New("llm_client.openai.apiKey is required")
		}
	case "anthropic":
		if strings.TrimSpace(cfg.LLMClient.Anthropic.APIKey) == "" {
			return errors.New("llm_client.anthropic.apiKey is required")
		}
	case "google":
		if strings.TrimSpace(cfg.LLMClient.Google.APIKey) == "" {
			return errors.New("llm_client.google.apiKey is required")
		}
	}
	return nil
}

func validateConfigWorkdir(cfg *Config) error {
	if strings.TrimSpace(cfg.Workdir) == "" {
		return errors.New("workdir is required")
	}
	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return fmt.Errorf("stat workdir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workdir must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD
	return nil
}

func validateConfigExec(cfg *Config) error {
	for _, binary := range cfg.Exec.BlockBinaries {
		if strings.Contains(binary, "/") || strings.Contains(binary, "\\") {
			return fmt.Errorf("exec.blockBinaries must contain bare binary names only (no paths): %q", binary)
		}
	}
	return nil
}

func validateConfigMemory(cfg *Config) error {
	if cfg.BeliefMemory.DefaultConfidence < 0 || cfg.BeliefMemory.DefaultConfidence > 1 {
		return fmt.Errorf("beliefMemory.defaultConfidence must be between 0 and 1 (got %g)", cfg.BeliefMemory.DefaultConfidence)
	}
	if cfg.BeliefMemory.PromotionThreshold < 0 || cfg.BeliefMemory.PromotionThreshold > 1 {
		return fmt.Errorf("beliefMemory.promotionThreshold must be between 0 and 1 (got %g)", cfg.BeliefMemory.PromotionThreshold)
	}
	if err := validateBeliefMemoryConfig(cfg.BeliefMemory); err != nil {
		return err
	}
	return nil
}

func validateConfigEmbedding(cfg *Config) error {
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Mode)) {
	case "", "auto", "enabled", "disabled":
	default:
		return fmt.Errorf("embedding.instructions.mode must be one of auto, enabled, or disabled (got %q)", cfg.Embedding.Instructions.Mode)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Format)) {
	case "", "qwen":
	default:
		return fmt.Errorf("embedding.instructions.format must be qwen (got %q)", cfg.Embedding.Instructions.Format)
	}
	return nil
}

func validateConfigReranking(cfg *Config) error {
	if cfg.Reranking.Enabled && strings.TrimSpace(cfg.Reranking.BaseURL) == "" {
		return fmt.Errorf("reranking.baseURL is required when reranking.enabled is true")
	}
	if cfg.Reranking.Timeout < 0 {
		return fmt.Errorf("reranking.timeoutSeconds must be non-negative")
	}
	return nil
}

func validateHarnessConfig(path string, cfg HarnessConfig) error {
	switch cfg.Mode {
	case "", "legacy", "guarded_chat", "workflow":
		return nil
	default:
		return fmt.Errorf("%s.mode must be one of legacy, guarded_chat, or workflow (got %q)", path, cfg.Mode)
	}
}

func validateProvider(path, provider string) error {
	switch provider {
	case "openai", "anthropic", "google", "local":
		return nil
	default:
		return fmt.Errorf("%s must be one of openai, anthropic, google, or local (got %q)", path, provider)
	}
}

func validateBeliefMemoryConfig(cfg BeliefMemoryConfig) error {
	switch cfg.Distillation.Mode {
	case "", "simple", "llm":
	default:
		return fmt.Errorf("beliefMemory.distillation.mode must be simple or llm (got %q)", cfg.Distillation.Mode)
	}
	if cfg.LLMClient.Provider != "" {
		if err := validateProvider("beliefMemory.llmClient.provider", cfg.LLMClient.Provider); err != nil {
			return err
		}
	}
	if cfg.Distillation.MaxCandidatesPerEpisode <= 0 {
		return fmt.Errorf("beliefMemory.distillation.maxCandidatesPerEpisode must be greater than 0")
	}
	if cfg.Distillation.MinCandidateConfidence < 0 || cfg.Distillation.MinCandidateConfidence > 1 {
		return fmt.Errorf("beliefMemory.distillation.minCandidateConfidence must be between 0 and 1 (got %g)", cfg.Distillation.MinCandidateConfidence)
	}
	if cfg.Distillation.AutoApplyMinConfidence < 0 || cfg.Distillation.AutoApplyMinConfidence > 1 {
		return fmt.Errorf("beliefMemory.distillation.autoApplyMinConfidence must be between 0 and 1 (got %g)", cfg.Distillation.AutoApplyMinConfidence)
	}
	if cfg.Distillation.AutoApplyMinConfidence < cfg.Distillation.MinCandidateConfidence {
		return fmt.Errorf("beliefMemory.distillation.autoApplyMinConfidence must be >= minCandidateConfidence")
	}
	if cfg.Retrieval.MinConfidence < 0 || cfg.Retrieval.MinConfidence > 1 {
		return fmt.Errorf("beliefMemory.retrieval.minConfidence must be between 0 and 1 (got %g)", cfg.Retrieval.MinConfidence)
	}
	if cfg.Retrieval.MaxTokensPerPrompt <= 0 {
		return fmt.Errorf("beliefMemory.retrieval.maxTokensPerPrompt must be greater than 0")
	}
	if cfg.Lifecycle.MinEvidenceForPromotion <= 0 {
		return fmt.Errorf("beliefMemory.lifecycle.minEvidenceForPromotion must be greater than 0")
	}
	if cfg.Lifecycle.MaxEvidenceAgainstPromotion < 0 {
		return fmt.Errorf("beliefMemory.lifecycle.maxEvidenceAgainstPromotion must be >= 0")
	}
	if cfg.Lifecycle.StaleAfterDays <= 0 {
		return fmt.Errorf("beliefMemory.lifecycle.staleAfterDays must be greater than 0")
	}
	if cfg.Lifecycle.StaleConfidenceDecay < 0 || cfg.Lifecycle.StaleConfidenceDecay > 1 {
		return fmt.Errorf("beliefMemory.lifecycle.staleConfidenceDecay must be between 0 and 1 (got %g)", cfg.Lifecycle.StaleConfidenceDecay)
	}
	if cfg.Enforcement.SoftPolicyThreshold < 0 || cfg.Enforcement.SoftPolicyThreshold > 1 {
		return fmt.Errorf("beliefMemory.enforcement.softPolicyThreshold must be between 0 and 1 (got %g)", cfg.Enforcement.SoftPolicyThreshold)
	}
	if cfg.Enforcement.HardConstraintThreshold < 0 || cfg.Enforcement.HardConstraintThreshold > 1 {
		return fmt.Errorf("beliefMemory.enforcement.hardConstraintThreshold must be between 0 and 1 (got %g)", cfg.Enforcement.HardConstraintThreshold)
	}
	if cfg.Enforcement.HardConstraintThreshold < cfg.Enforcement.SoftPolicyThreshold {
		return fmt.Errorf("beliefMemory.enforcement.hardConstraintThreshold must be >= softPolicyThreshold")
	}
	if cfg.Enforcement.HardConstraintMinEvidenceFor <= 0 {
		return fmt.Errorf("beliefMemory.enforcement.hardConstraintMinEvidenceFor must be greater than 0")
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseCommaSeparatedList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
