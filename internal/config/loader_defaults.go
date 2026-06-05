package config

import (
	"reflect"
	"strings"
)

func applyDefaults(cfg *Config) {
	applyLLMDefaults(cfg)
	applyUnifiedMemoryAliases(cfg)
	applyObservabilityDefaults(cfg)
	applyRuntimeDefaults(cfg)
	applyCodeQADefaults(cfg)
	applyBeliefDefaults(cfg)
	applyCodeQAFallbackDefaults(cfg)
	applyTimeoutAndTokenDefaults(cfg)
	applyEmbeddingAndRerankingDefaults(cfg)
	applyMCPAndDatabaseDefaults(cfg)
	applyAuthAndTransitDefaults(cfg)
	syncUnifiedMemoryConfig(cfg)
}

func applyUnifiedMemoryAliases(cfg *Config) {
	if cfg == nil {
		return
	}
	applyUnifiedMemoryRetrievalDefaults(cfg)
	if !cfg.MemoryConfigured {
		return
	}
	if !reflect.ValueOf(cfg.Memory.Evolving).IsZero() {
		cfg.EvolvingMemory = cfg.Memory.Evolving
	}
	if !reflect.ValueOf(cfg.Memory.Belief).IsZero() {
		cfg.BeliefMemory = cfg.Memory.Belief
	}
	if !reflect.ValueOf(cfg.Memory.Magma).IsZero() {
		cfg.Magma = cfg.Memory.Magma
	}
	if hasConfigLLMClientOverride(cfg.Memory.LLMClients.Evolving) {
		cfg.EvolvingMemory.LLMClient = cfg.Memory.LLMClients.Evolving
	}
	if hasConfigLLMClientOverride(cfg.Memory.LLMClients.BeliefDistillation) {
		cfg.BeliefMemory.LLMClient = cfg.Memory.LLMClients.BeliefDistillation
	}
	if hasConfigLLMClientOverride(cfg.Memory.LLMClients.MagmaConsolidation) {
		cfg.Magma.Consolidation.LLMClient = cfg.Memory.LLMClients.MagmaConsolidation
	}
	cfg.EvolvingMemory.Enabled = cfg.Memory.Enabled
	cfg.BeliefMemory.Enabled = cfg.Memory.Enabled
	cfg.Magma.Enabled = cfg.Memory.Enabled
}

func syncUnifiedMemoryConfig(cfg *Config) {
	if cfg == nil {
		return
	}
	applyUnifiedMemoryRetrievalDefaults(cfg)
	if cfg.MemoryConfigured {
		cfg.EvolvingMemory.Enabled = cfg.Memory.Enabled
		cfg.BeliefMemory.Enabled = cfg.Memory.Enabled
		cfg.Magma.Enabled = cfg.Memory.Enabled
	} else {
		cfg.Memory.Enabled = cfg.EvolvingMemory.Enabled && cfg.BeliefMemory.Enabled && cfg.Magma.Enabled
	}
	cfg.Memory.Evolving = cfg.EvolvingMemory
	cfg.Memory.Belief = cfg.BeliefMemory
	cfg.Memory.Magma = cfg.Magma
	cfg.Memory.LLMClients.Evolving = cfg.EvolvingMemory.LLMClient
	cfg.Memory.LLMClients.BeliefDistillation = cfg.BeliefMemory.LLMClient
	if hasConfigLLMClientOverride(cfg.Memory.LLMClients.MagmaConsolidation) {
		cfg.Magma.Consolidation.LLMClient = cfg.Memory.LLMClients.MagmaConsolidation
	} else if hasConfigLLMClientOverride(cfg.Magma.Consolidation.LLMClient) {
		cfg.Memory.LLMClients.MagmaConsolidation = cfg.Magma.Consolidation.LLMClient
	}
	cfg.Memory.Magma.Consolidation.LLMClient = cfg.Magma.Consolidation.LLMClient
}

func applyUnifiedMemoryRetrievalDefaults(cfg *Config) {
	if cfg.Memory.Retrieval.MaxTokensPerPrompt <= 0 {
		cfg.Memory.Retrieval.MaxTokensPerPrompt = 2200
	}
	if cfg.Memory.Retrieval.TimeoutMs <= 0 {
		cfg.Memory.Retrieval.TimeoutMs = 700
	}
	cfg.Memory.Retrieval.IncludeRecent = true
}

func applyLLMDefaults(cfg *Config) {
	if cfg.LLMClient.Provider == "" {
		cfg.LLMClient.Provider = "openai"
	}
	if cfg.LLMClient.OpenAI.BaseURL == "" {
		cfg.LLMClient.OpenAI.BaseURL = OpenAIAPIV1BaseURL
	}
	if cfg.LLMClient.OpenAI.Model == "" {
		cfg.LLMClient.OpenAI.Model = "gpt-4o-mini"
	}
	if cfg.LLMClient.OpenAI.API == "" {
		cfg.LLMClient.OpenAI.API = "completions"
	}
	applySummaryDefaults(cfg)
}

func applyObservabilityDefaults(cfg *Config) {
	if cfg.Obs.ServiceName == "" {
		cfg.Obs.ServiceName = "manifold"
	}
	if cfg.Obs.Environment == "" {
		cfg.Obs.Environment = "dev"
	}
	if cfg.Obs.Local.MetricsWindowMinutes <= 0 {
		cfg.Obs.Local.MetricsWindowMinutes = 60
	}
	if cfg.Obs.Local.MetricsBucketSeconds <= 0 {
		cfg.Obs.Local.MetricsBucketSeconds = 30
	}
	if cfg.Obs.Local.MaxLogs <= 0 {
		cfg.Obs.Local.MaxLogs = 5000
	}
	if cfg.Obs.Local.MaxTraces <= 0 {
		cfg.Obs.Local.MaxTraces = 1000
	}
	if cfg.Obs.Local.MaxSpansPerTrace <= 0 {
		cfg.Obs.Local.MaxSpansPerTrace = 256
	}
	if cfg.Obs.ClickHouse.MetricsTable == "" {
		cfg.Obs.ClickHouse.MetricsTable = "metrics"
	}
	if cfg.Obs.ClickHouse.TracesTable == "" {
		cfg.Obs.ClickHouse.TracesTable = "traces"
	}
	if cfg.Obs.ClickHouse.LogsTable == "" {
		cfg.Obs.ClickHouse.LogsTable = "logs"
	}
	if cfg.Obs.ClickHouse.TimestampColumn == "" {
		cfg.Obs.ClickHouse.TimestampColumn = "TimeUnix"
	}
	if cfg.Obs.ClickHouse.ValueColumn == "" {
		cfg.Obs.ClickHouse.ValueColumn = "Value"
	}
	if cfg.Obs.ClickHouse.ModelAttributeKey == "" {
		cfg.Obs.ClickHouse.ModelAttributeKey = "llm.model"
	}
	if cfg.Obs.ClickHouse.PromptMetricName == "" {
		cfg.Obs.ClickHouse.PromptMetricName = "llm.prompt_tokens"
	}
	if cfg.Obs.ClickHouse.CompletionMetricName == "" {
		cfg.Obs.ClickHouse.CompletionMetricName = "llm.completion_tokens"
	}
	if cfg.Obs.ClickHouse.LookbackHours <= 0 {
		cfg.Obs.ClickHouse.LookbackHours = 24
	}
	if cfg.Obs.ClickHouse.TimeoutSeconds <= 0 {
		cfg.Obs.ClickHouse.TimeoutSeconds = 5
	}
}

func applyRuntimeDefaults(cfg *Config) {
	if cfg.Web.SearXNGURL == "" {
		cfg.Web.SearXNGURL = "http://localhost:8080"
	}
	ApplyExecDefaults(&cfg.Exec)
	if cfg.OutputTruncateByte <= 0 {
		cfg.OutputTruncateByte = 64 * 1024
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 8
	}
	applyHarnessDefaults(cfg)
	if cfg.MaxDiscoveredTools <= 0 {
		cfg.MaxDiscoveredTools = 20
	}
}

func ApplyExecDefaults(execCfg *ExecConfig) {
	if execCfg == nil {
		return
	}
	if execCfg.MaxCommandSeconds <= 0 {
		execCfg.MaxCommandSeconds = 30
	}
	if execCfg.MaxTerminalSessions <= 0 {
		execCfg.MaxTerminalSessions = 8
	}
	if execCfg.MaxTerminalRuntimeSeconds <= 0 {
		execCfg.MaxTerminalRuntimeSeconds = execCfg.MaxCommandSeconds
	}
	if execCfg.TerminalIdleTTLSeconds <= 0 {
		execCfg.TerminalIdleTTLSeconds = 1800
	}
	if execCfg.TerminalOutputBufferBytes <= 0 {
		execCfg.TerminalOutputBufferBytes = 256 * 1024
	}
	if execCfg.Sandbox.Enabled == nil {
		execCfg.Sandbox.Enabled = boolPtr(true)
	}
	if execCfg.Sandbox.FailIfUnavailable == nil {
		execCfg.Sandbox.FailIfUnavailable = boolPtr(true)
	}
	if execCfg.Sandbox.Network.Enabled == nil {
		execCfg.Sandbox.Network.Enabled = boolPtr(false)
	}
	if len(execCfg.CommandRules) == 0 {
		execCfg.CommandRules = DefaultExecCommandRules()
	}
}

func DefaultExecCommandRules() []ExecCommandRule {
	commands := []string{"go", "gofmt", "git", "rg", "grep", "ls", "cat", "pwd", "echo", "head", "tail", "wc", "find", "sleep"}
	rules := make([]ExecCommandRule, 0, len(commands))
	for _, command := range commands {
		rules = append(rules, ExecCommandRule{
			ID:            "default:" + command,
			Decision:      "allow",
			Pattern:       []string{command},
			Contexts:      []string{"cli", "terminal"},
			Justification: "default safe command allowlist; execution is still sandboxed and path constrained",
		})
	}
	return rules
}

func boolPtr(v bool) *bool {
	return &v
}

func applyCodeQADefaults(cfg *Config) {
	if cfg.CodeQA.ArtifactDir == "" {
		cfg.CodeQA.ArtifactDir = "codeqa-artifacts"
	}
	if cfg.CodeQA.MaxConcurrentRuns <= 0 {
		cfg.CodeQA.MaxConcurrentRuns = 1
	}
	if cfg.CodeQA.MaxGateParallelism <= 0 {
		cfg.CodeQA.MaxGateParallelism = 2
	}
	if cfg.CodeQA.MaxJudgeParallelism <= 0 {
		cfg.CodeQA.MaxJudgeParallelism = 2
	}
	if cfg.CodeQA.DefaultMaxDiffBytes <= 0 {
		cfg.CodeQA.DefaultMaxDiffBytes = 128 * 1024
	}
	if cfg.CodeQA.DefaultMaxChangedFiles <= 0 {
		cfg.CodeQA.DefaultMaxChangedFiles = 12
	}
	if cfg.CodeQA.AcceptThreshold == 0 {
		cfg.CodeQA.AcceptThreshold = 0.10
	}
	if cfg.CodeQA.MinConfidence == 0 {
		cfg.CodeQA.MinConfidence = 0.70
	}
}

func applyBeliefDefaults(cfg *Config) {
	if cfg.BeliefMemory.MaxBeliefsPerPrompt <= 0 {
		cfg.BeliefMemory.MaxBeliefsPerPrompt = 5
	}
	if cfg.BeliefMemory.MaxEvidencePerBelief <= 0 {
		cfg.BeliefMemory.MaxEvidencePerBelief = 3
	}
	if cfg.BeliefMemory.DefaultConfidence == 0 {
		cfg.BeliefMemory.DefaultConfidence = 0.50
	}
	if cfg.BeliefMemory.PromotionThreshold == 0 {
		cfg.BeliefMemory.PromotionThreshold = 0.80
	}
	applyBeliefMemoryDefaults(cfg)
	if cfg.BeliefMemory.MaxRAGEvidencePerPrompt <= 0 {
		cfg.BeliefMemory.MaxRAGEvidencePerPrompt = 3
	}
	if cfg.BeliefMemory.RAGRetrievalK <= 0 {
		cfg.BeliefMemory.RAGRetrievalK = 8
	}
	if cfg.BeliefMemory.RAGMinScore < 0 {
		cfg.BeliefMemory.RAGMinScore = 0
	}
}

func applyCodeQAFallbackDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.CodeQA.JudgeModel) == "" {
		cfg.CodeQA.JudgeModel = cfg.LLMClient.OpenAI.Model
	}
	if strings.TrimSpace(cfg.CodeQA.ProposerModel) == "" {
		cfg.CodeQA.ProposerModel = cfg.CodeQA.JudgeModel
	}
	if len(cfg.CodeQA.AllowedCommands) == 0 {
		cfg.CodeQA.AllowedCommands = []string{"go", "gofmt", "ruff", "pytest", "python", "prettier", "eslint", "tsc", "npm", "npx", "stylelint", "html-validate", "cargo", "rustfmt"}
	}
	if len(cfg.CodeQA.HighRiskGlobs) == 0 {
		cfg.CodeQA.HighRiskGlobs = []string{"**/auth/**", "**/migrations/**", "**/*crypto*", "**/deploy/**", "**/.github/**"}
	}
	if len(cfg.CodeQA.ForbiddenGlobs) == 0 {
		cfg.CodeQA.ForbiddenGlobs = []string{"**/*.pem", "**/*.key", "**/.env*", "**/node_modules/**", "**/dist/**"}
	}
}

func applyTimeoutAndTokenDefaults(cfg *Config) {
	if cfg.AgentRunTimeoutSeconds < 0 {
		cfg.AgentRunTimeoutSeconds = 0
	}
	if cfg.StreamRunTimeoutSeconds < 0 {
		cfg.StreamRunTimeoutSeconds = 0
	}
	if cfg.WorkflowTimeoutSeconds < 0 {
		cfg.WorkflowTimeoutSeconds = 0
	}
	if cfg.Tokenization.CacheSize <= 0 {
		cfg.Tokenization.CacheSize = 1000
	}
	if cfg.Tokenization.CacheTTLSeconds <= 0 {
		cfg.Tokenization.CacheTTLSeconds = 3600
	}
}

func applyEmbeddingAndRerankingDefaults(cfg *Config) {
	if cfg.Embedding.BaseURL == "" {
		cfg.Embedding.BaseURL = OpenAIAPIBaseURL
	}
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "text-embedding-3-small"
	}
	if cfg.Embedding.APIHeader == "" {
		cfg.Embedding.APIHeader = "Authorization"
	}
	if cfg.Embedding.Path == "" {
		cfg.Embedding.Path = "/v1/embeddings"
	}
	if cfg.Embedding.Timeout <= 0 {
		cfg.Embedding.Timeout = 30
	}
	applyMagmaDefaults(cfg)
	if cfg.Embedding.Instructions.Mode == "" {
		cfg.Embedding.Instructions.Mode = "auto"
	} else {
		cfg.Embedding.Instructions.Mode = strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Mode))
	}
	if cfg.Embedding.Instructions.Format == "" {
		cfg.Embedding.Instructions.Format = "qwen"
	} else {
		cfg.Embedding.Instructions.Format = strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Format))
	}
	if cfg.Reranking.APIHeader == "" {
		cfg.Reranking.APIHeader = "Authorization"
	}
	if cfg.Reranking.Path == "" {
		cfg.Reranking.Path = "/v1/rerank"
	}
	if cfg.Reranking.Timeout <= 0 {
		cfg.Reranking.Timeout = 30
	}
}

func applyMCPAndDatabaseDefaults(cfg *Config) {
	for i := range cfg.MCP.Servers {
		if cfg.MCP.Servers[i].HTTP.TimeoutSeconds <= 0 {
			cfg.MCP.Servers[i].HTTP.TimeoutSeconds = 30
		}
	}
	if cfg.Databases.Search.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Search.Backend = "auto"
		} else {
			cfg.Databases.Search.Backend = "memory"
		}
	}
	if cfg.Databases.Vector.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Vector.Backend = "auto"
		} else {
			cfg.Databases.Vector.Backend = "memory"
		}
	}
	if cfg.Databases.Graph.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Graph.Backend = "auto"
		} else {
			cfg.Databases.Graph.Backend = "memory"
		}
	}
	if cfg.Databases.Chat.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Chat.Backend = "auto"
		} else {
			cfg.Databases.Chat.Backend = "memory"
		}
	}
}

func applyAuthAndTransitDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.Auth.Provider) == "" {
		cfg.Auth.Provider = "oidc"
	}
	if cfg.Transit.DefaultSearchLimit <= 0 {
		cfg.Transit.DefaultSearchLimit = 10
	}
	if cfg.Transit.DefaultListLimit <= 0 {
		cfg.Transit.DefaultListLimit = 100
	}
	if cfg.Transit.MaxBatchSize <= 0 {
		cfg.Transit.MaxBatchSize = 100
	}
}

func applyMagmaDefaults(cfg *Config) {
	if cfg.Magma.Consolidation.Model == "" {
		cfg.Magma.Consolidation.Model = "gpt-4o-mini"
	}
	if cfg.Magma.Consolidation.BatchSize <= 0 {
		cfg.Magma.Consolidation.BatchSize = 10
	}
	if cfg.Magma.Consolidation.MaxQueueSize <= 0 {
		cfg.Magma.Consolidation.MaxQueueSize = 1000
	}
	if cfg.Magma.Consolidation.WorkerCount <= 0 {
		cfg.Magma.Consolidation.WorkerCount = 2
	}
	cfg.Magma.Consolidation.Prompts.ConsolidationExtraction = strings.TrimSpace(cfg.Magma.Consolidation.Prompts.ConsolidationExtraction)
	cfg.Magma.Consolidation.Prompts.IntentClassification = strings.TrimSpace(cfg.Magma.Consolidation.Prompts.IntentClassification)
	if cfg.Magma.Graphs.Semantic.TopK <= 0 {
		cfg.Magma.Graphs.Semantic.TopK = 20
	}
	if cfg.Magma.Graphs.Semantic.SimilarityThreshold <= 0 {
		cfg.Magma.Graphs.Semantic.SimilarityThreshold = 0.7
	}
	if cfg.Magma.Graphs.Temporal.DateResolution == "" {
		cfg.Magma.Graphs.Temporal.DateResolution = "auto"
	}
	if cfg.Magma.Graphs.Causal.LLMThreshold <= 0 {
		cfg.Magma.Graphs.Causal.LLMThreshold = 0.8
	}
	if cfg.Magma.Retrieval.DefaultHops <= 0 {
		cfg.Magma.Retrieval.DefaultHops = 2
	}
	if cfg.Magma.Retrieval.DefaultMaxNodes <= 0 {
		cfg.Magma.Retrieval.DefaultMaxNodes = 10
	}
	if cfg.Magma.Retrieval.IntentClassification == "" {
		cfg.Magma.Retrieval.IntentClassification = "hybrid"
	} else {
		cfg.Magma.Retrieval.IntentClassification = strings.ToLower(strings.TrimSpace(cfg.Magma.Retrieval.IntentClassification))
	}
	if cfg.Magma.Retrieval.ContextFormat == "" {
		cfg.Magma.Retrieval.ContextFormat = "structured"
	}
	if cfg.Magma.Lifecycle.PruneIntervalMinutes < 0 {
		cfg.Magma.Lifecycle.PruneIntervalMinutes = 0
	}
	if cfg.Magma.Lifecycle.MaxEdgesPerSourceRel < 0 {
		cfg.Magma.Lifecycle.MaxEdgesPerSourceRel = 0
	}
	if cfg.Magma.Lifecycle.MinSemanticWeight < 0 {
		cfg.Magma.Lifecycle.MinSemanticWeight = 0
	}
	if cfg.Magma.Lifecycle.LowConfidenceThreshold <= 0 {
		cfg.Magma.Lifecycle.LowConfidenceThreshold = 0.6
	}
}
