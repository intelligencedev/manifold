package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if v := firstNonEmpty("", "foo", "bar"); v != "foo" {
		t.Fatalf("expected 'foo', got %q", v)
	}
	if v := firstNonEmpty(); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestParseInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		n, err := parseInt("42")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 42 {
			t.Fatalf("expected 42, got %d", n)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, err := parseInt("notanint"); err == nil {
			t.Fatalf("expected error for invalid int")
		}
	})
}

func TestLoad_FromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("AUTH_CLIENT_SECRET", "test-auth-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/manifold?sslmode=disable")
	t.Setenv("CLICKHOUSE_DSN", "tcp://clickhouse:9000?database=otel")
	t.Setenv("EMBED_API_KEY", "embed-secret")

	configText := `workdir: .
systemPrompt: Test prompt
logPath: manifold.log
logLevel: debug
logPayloads: true
outputTruncateBytes: 12345
maxSteps: 42
maxToolParallelism: 3
enableTools: true
allowTools:
  - run_cli
  - web_fetch
autoDiscover: true
maxDiscoveredTools: 7
summaryEnabled: true
summaryContextWindowTokens: 32000
summaryReserveBufferTokens: 25000
summaryMinKeepLastMessages: 4
summaryMaxKeepLastMessages: 12
summaryMaxSummaryChunkTokens: 4096
agentRunTimeoutSeconds: 60
streamRunTimeoutSeconds: 75
workflowTimeoutSeconds: 90
exec:
  blockBinaries: [rm, sudo]
  maxCommandSeconds: 900
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
    baseURL: https://api.openai.com/v1
    model: gpt-5-mini
    summaryBaseURL: https://api.openai.com/v1
    summaryModel: gpt-5-nano
    api: responses
    extraHeaders:
      X-Test: yes
    extraParams:
      reasoning_effort: medium
obs:
  serviceName: manifold
  serviceVersion: 1.2.3
  environment: local
  otlp: http://localhost:4318
  clickhouse:
    dsn: "${CLICKHOUSE_DSN}"
    database: otel
    metricsTable: metrics_sum
    tracesTable: traces
    logsTable: logs
    timestampColumn: TimeUnix
    valueColumn: Value
    modelAttributeKey: llm.model
    promptMetricName: llm.prompt_tokens
    completionMetricName: llm.completion_tokens
    lookbackHours: 24
    timeoutSeconds: 5
web:
  searXNGURL: http://localhost:8080
auth:
  enabled: true
  provider: oauth2
  clientID: manifold-client
  clientSecret: "${AUTH_CLIENT_SECRET}"
  redirectURL: http://localhost:32180/auth/callback
  allowedDomains: [example.com]
  cookieName: manifold_session
  cookieSecure: false
  cookieDomain: ""
  stateTTLSeconds: 600
  sessionTTLHours: 72
  oauth2:
    authURL: https://github.com/login/oauth/authorize
    tokenURL: https://github.com/login/oauth/access_token
    userInfoURL: https://api.github.com/user
    logoutRedirectParam: redirect_uri
    scopes: [read:user, user:email]
    providerName: github
    defaultRoles: [user]
    emailField: email
    nameField: name
    pictureField: avatar_url
    subjectField: id
databases:
  defaultDSN: "${DATABASE_URL}"
  chat:
    backend: postgres
    dsn: "${DATABASE_URL}"
  search:
    backend: postgres
    dsn: "${DATABASE_URL}"
    index: docs
  vector:
    backend: postgres
    dsn: "${DATABASE_URL}"
    index: vectors
    dimensions: 1536
    metric: cosine
  graph:
    backend: postgres
    dsn: "${DATABASE_URL}"
embedding:
  baseURL: https://api.openai.com
  model: text-embedding-3-small
  apiKey: "${EMBED_API_KEY}"
  apiHeader: Authorization
  headers:
    X-Embed-Trace: abc123
  path: /v1/embeddings
  timeoutSeconds: 30
evolvingMemory:
  enabled: true
  provider: openai
  llmClient:
    provider: local
    openai:
      baseURL: http://localhost:11434/v1
      model: qwen-memory
      api: completions
  maxSize: 1000
  topK: 4
  windowSize: 20
  enableRAG: true
  reMemEnabled: false
  maxInnerSteps: 5
  model: gpt-4o-mini
  enableSmartPrune: true
  pruneThreshold: 0.95
  relevanceDecay: 0.99
  minRelevance: 0.1
transit:
  enabled: true
  defaultSearchLimit: 10
  defaultListLimit: 100
  maxBatchSize: 100
  enableVectorSearch: true
tts:
  baseURL: https://api.openai.com/v1
  model: gpt-4o-mini-tts
  voice: alloy
stt:
  baseURL: https://api.openai.com
  model: gpt-4o-mini-transcribe
projects: {}
tokenization:
  enabled: true
  cacheSize: 300
  cacheTTLSeconds: 120
  fallbackToHeuristic: false
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Workdir != tmpDir {
		t.Fatalf("expected absolute workdir %q, got %q", tmpDir, cfg.Workdir)
	}
	if cfg.SystemPrompt != "Test prompt" {
		t.Fatalf("unexpected system prompt: %q", cfg.SystemPrompt)
	}
	if cfg.LLMClient.OpenAI.APIKey != "test-openai-key" {
		t.Fatalf("expected expanded openai api key, got %q", cfg.LLMClient.OpenAI.APIKey)
	}
	if cfg.LLMClient.OpenAI.API != "responses" {
		t.Fatalf("unexpected openai api: %q", cfg.LLMClient.OpenAI.API)
	}
	if cfg.Auth.ClientSecret != "test-auth-secret" {
		t.Fatalf("expected expanded auth client secret, got %q", cfg.Auth.ClientSecret)
	}
	if cfg.Databases.Search.Index != "docs" || cfg.Databases.Vector.Index != "vectors" {
		t.Fatalf("unexpected database config: %+v %+v", cfg.Databases.Search, cfg.Databases.Vector)
	}
	if cfg.Embedding.Headers["X-Embed-Trace"] != "abc123" {
		t.Fatalf("unexpected embedding headers: %+v", cfg.Embedding.Headers)
	}
	if cfg.EvolvingMemory.LLMClient.Provider != "local" {
		t.Fatalf("unexpected evolving memory provider: %q", cfg.EvolvingMemory.LLMClient.Provider)
	}
	if !cfg.EnableTools || len(cfg.ToolAllowList) != 2 {
		t.Fatalf("unexpected tool config: enabled=%v allow=%v", cfg.EnableTools, cfg.ToolAllowList)
	}
	if !cfg.AutoDiscover || cfg.MaxDiscoveredTools != 7 {
		t.Fatalf("unexpected discovery config: enabled=%v max=%d", cfg.AutoDiscover, cfg.MaxDiscoveredTools)
	}
	if cfg.Tokenization.FallbackToHeuristic {
		t.Fatalf("expected fallbackToHeuristic false")
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "expanded-key")

	configText := `workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMClient.OpenAI.APIKey != "expanded-key" {
		t.Fatalf("expected expanded key, got %q", cfg.LLMClient.OpenAI.APIKey)
	}
}

func TestLoad_SpecialistsFromSeparateFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "dummy")
	t.Setenv("SPECIALIST_API_KEY", "specialist-secret")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(`workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "specialists.yaml"), []byte(`specialists:
  - name: coder
    description: Code specialist
    provider: openai
    baseURL: https://example.com/v1
    apiKey: "${SPECIALIST_API_KEY}"
    model: gpt-5-mini
    enableTools: true
    autoDiscover: true
routes:
  - name: coder
    contains: [code, refactor]
`), 0o644); err != nil {
		t.Fatalf("write specialists: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Specialists) != 1 || cfg.Specialists[0].APIKey != "specialist-secret" {
		t.Fatalf("unexpected specialists: %+v", cfg.Specialists)
	}
	if cfg.Specialists[0].AutoDiscover == nil || !*cfg.Specialists[0].AutoDiscover {
		t.Fatalf("expected specialist autoDiscover to be true: %+v", cfg.Specialists[0])
	}
	if len(cfg.SpecialistRoutes) != 1 || cfg.SpecialistRoutes[0].Name != "coder" {
		t.Fatalf("unexpected routes: %+v", cfg.SpecialistRoutes)
	}
}

func TestLoad_MCPFromSeparateFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "dummy")
	t.Setenv("ACME_MCP_TOKEN", "mcp-secret")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(`workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "mcp.yaml"), []byte(`servers:
  - name: acme
    url: https://mcp.acme.com/mcp
    bearerToken: "${ACME_MCP_TOKEN}"
    headers:
      X-Client: Manifold
    http:
      timeoutSeconds: 30
`), 0o644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.MCP.Servers) != 1 || cfg.MCP.Servers[0].BearerToken != "mcp-secret" {
		t.Fatalf("unexpected mcp config: %+v", cfg.MCP.Servers)
	}
}

func TestLoad_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "dummy")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(`workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.LLMClient.OpenAI.Model != "gpt-4o-mini" {
		t.Fatalf("expected default model, got %q", cfg.LLMClient.OpenAI.Model)
	}
	if cfg.Exec.MaxCommandSeconds != 30 {
		t.Fatalf("expected default command timeout, got %d", cfg.Exec.MaxCommandSeconds)
	}
	if cfg.OutputTruncateByte != 64*1024 {
		t.Fatalf("expected default output truncate bytes, got %d", cfg.OutputTruncateByte)
	}
	if cfg.Databases.Search.Backend != "memory" {
		t.Fatalf("expected default search backend memory, got %q", cfg.Databases.Search.Backend)
	}
	if !cfg.Tokenization.FallbackToHeuristic {
		t.Fatalf("expected default tokenization fallback true")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(`workdir: .
llm_client:
  provider: openai
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatalf("expected missing api key to fail")
	}
}

func TestLoad_SeparateFileOverridesMainConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "dummy")

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(`workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
specialists:
  - name: inline
    model: inline-model
mcp:
  servers:
    - name: inline
      command: inline-command
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "specialists.yaml"), []byte(`specialists:
  - name: external
    model: external-model
`), 0o644); err != nil {
		t.Fatalf("write specialists: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "mcp.yaml"), []byte(`servers:
  - name: external
    command: external-command
`), 0o644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Specialists) != 1 || cfg.Specialists[0].Name != "external" {
		t.Fatalf("expected external specialists to override inline config, got %+v", cfg.Specialists)
	}
	if len(cfg.MCP.Servers) != 1 || cfg.MCP.Servers[0].Name != "external" {
		t.Fatalf("expected external mcp config to override inline config, got %+v", cfg.MCP.Servers)
	}
}
