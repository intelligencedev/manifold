package agentd

import (
	"fmt"
	"os"
	"strings"

	"manifold/internal/config"

	yaml "gopkg.in/yaml.v3"
)

func currentAgentdSettings(cfg *config.Config) agentdSettings {
	return agentdSettings{
		OpenAISummaryModel:         cfg.OpenAI.SummaryModel,
		OpenAISummaryURL:           cfg.OpenAI.SummaryBaseURL,
		SummaryEnabled:             cfg.SummaryEnabled,
		SummaryReserveBufferTokens: cfg.SummaryReserveBufferTokens,

		EmbedBaseURL:    cfg.Embedding.BaseURL,
		EmbedModel:      cfg.Embedding.Model,
		EmbedAPIKey:     cfg.Embedding.APIKey,
		EmbedAPIHeader:  cfg.Embedding.APIHeader,
		EmbedAPIHeaders: cfg.Embedding.Headers,
		EmbedPath:       cfg.Embedding.Path,

		AgentRunTimeoutSeconds:  cfg.AgentRunTimeoutSeconds,
		StreamRunTimeoutSeconds: cfg.StreamRunTimeoutSeconds,
		WorkflowTimeoutSeconds:  cfg.WorkflowTimeoutSeconds,

		BlockBinaries:       strings.Join(cfg.Exec.BlockBinaries, ","),
		MaxCommandSeconds:   cfg.Exec.MaxCommandSeconds,
		OutputTruncateBytes: cfg.OutputTruncateByte,

		OTELServiceName: cfg.Obs.ServiceName,
		ServiceVersion:  cfg.Obs.ServiceVersion,
		Environment:     cfg.Obs.Environment,
		OTLPEndpoint:    cfg.Obs.OTLP,

		LogPath:     cfg.LogPath,
		LogLevel:    cfg.LogLevel,
		LogPayloads: cfg.LogPayloads,

		SearXNGURL:    cfg.Web.SearXNGURL,
		WebSearXNGURL: cfg.Web.SearXNGURL,

		DatabaseURL: cfg.Databases.DefaultDSN,
		DBURL:       cfg.Databases.DefaultDSN,
		PostgresDSN: cfg.Databases.DefaultDSN,

		SearchBackend: cfg.Databases.Search.Backend,
		SearchDSN:     cfg.Databases.Search.DSN,
		SearchIndex:   cfg.Databases.Search.Index,

		VectorBackend: cfg.Databases.Vector.Backend,
		VectorDSN:     cfg.Databases.Vector.DSN,
		VectorIndex:   cfg.Databases.Vector.Index,
		VectorDims:    cfg.Databases.Vector.Dimensions,
		VectorMetric:  cfg.Databases.Vector.Metric,

		GraphBackend: cfg.Databases.Graph.Backend,
		GraphDSN:     cfg.Databases.Graph.DSN,
	}
}

func normalizeAgentdSettings(settings agentdSettings) agentdSettings {
	webURL := firstNonEmptyTrimmed(settings.WebSearXNGURL, settings.SearXNGURL)
	if webURL != "" {
		settings.WebSearXNGURL = webURL
		settings.SearXNGURL = webURL
	}

	defaultDSN := firstNonEmptyTrimmed(settings.PostgresDSN, settings.DBURL, settings.DatabaseURL)
	if defaultDSN != "" {
		settings.PostgresDSN = defaultDSN
		settings.DBURL = defaultDSN
		settings.DatabaseURL = defaultDSN
	}

	settings.BlockBinaries = strings.TrimSpace(settings.BlockBinaries)

	return settings
}

func applyAgentdSettings(cfg *config.Config, settings agentdSettings) error {
	settings = normalizeAgentdSettings(settings)

	if settings.OpenAISummaryModel != "" {
		cfg.OpenAI.SummaryModel = settings.OpenAISummaryModel
	}
	if settings.OpenAISummaryURL != "" {
		cfg.OpenAI.SummaryBaseURL = settings.OpenAISummaryURL
	}
	cfg.SummaryEnabled = settings.SummaryEnabled
	if settings.SummaryReserveBufferTokens != 0 {
		cfg.SummaryReserveBufferTokens = settings.SummaryReserveBufferTokens
	}

	if settings.EmbedBaseURL != "" {
		cfg.Embedding.BaseURL = settings.EmbedBaseURL
	}
	if settings.EmbedModel != "" {
		cfg.Embedding.Model = settings.EmbedModel
	}
	if settings.EmbedAPIKey != "" {
		cfg.Embedding.APIKey = settings.EmbedAPIKey
	}
	if settings.EmbedAPIHeader != "" {
		cfg.Embedding.APIHeader = settings.EmbedAPIHeader
	}
	if settings.EmbedAPIHeaders != nil {
		cfg.Embedding.Headers = settings.EmbedAPIHeaders
	}
	if settings.EmbedPath != "" {
		cfg.Embedding.Path = settings.EmbedPath
	}

	if settings.AgentRunTimeoutSeconds != 0 {
		cfg.AgentRunTimeoutSeconds = settings.AgentRunTimeoutSeconds
	}
	if settings.StreamRunTimeoutSeconds != 0 {
		cfg.StreamRunTimeoutSeconds = settings.StreamRunTimeoutSeconds
	}
	if settings.WorkflowTimeoutSeconds != 0 {
		cfg.WorkflowTimeoutSeconds = settings.WorkflowTimeoutSeconds
	}

	if settings.BlockBinaries != "" {
		binaries, err := parseBlockBinaries(settings.BlockBinaries)
		if err != nil {
			return err
		}
		cfg.Exec.BlockBinaries = binaries
	}
	if settings.MaxCommandSeconds != 0 {
		cfg.Exec.MaxCommandSeconds = settings.MaxCommandSeconds
	}
	if settings.OutputTruncateBytes != 0 {
		cfg.OutputTruncateByte = settings.OutputTruncateBytes
	}

	if settings.OTELServiceName != "" {
		cfg.Obs.ServiceName = settings.OTELServiceName
	}
	if settings.ServiceVersion != "" {
		cfg.Obs.ServiceVersion = settings.ServiceVersion
	}
	if settings.Environment != "" {
		cfg.Obs.Environment = settings.Environment
	}
	if settings.OTLPEndpoint != "" {
		cfg.Obs.OTLP = settings.OTLPEndpoint
	}

	if settings.LogPath != "" {
		cfg.LogPath = settings.LogPath
	}
	if settings.LogLevel != "" {
		cfg.LogLevel = settings.LogLevel
	}
	cfg.LogPayloads = settings.LogPayloads

	if settings.WebSearXNGURL != "" {
		cfg.Web.SearXNGURL = settings.WebSearXNGURL
	}

	if settings.PostgresDSN != "" {
		cfg.Databases.DefaultDSN = settings.PostgresDSN
	}
	if settings.SearchBackend != "" {
		cfg.Databases.Search.Backend = settings.SearchBackend
	}
	if settings.SearchDSN != "" {
		cfg.Databases.Search.DSN = settings.SearchDSN
	}
	if settings.SearchIndex != "" {
		cfg.Databases.Search.Index = settings.SearchIndex
	}
	if settings.VectorBackend != "" {
		cfg.Databases.Vector.Backend = settings.VectorBackend
	}
	if settings.VectorDSN != "" {
		cfg.Databases.Vector.DSN = settings.VectorDSN
	}
	if settings.VectorIndex != "" {
		cfg.Databases.Vector.Index = settings.VectorIndex
	}
	if settings.VectorDims != 0 {
		cfg.Databases.Vector.Dimensions = settings.VectorDims
	}
	if settings.VectorMetric != "" {
		cfg.Databases.Vector.Metric = settings.VectorMetric
	}
	if settings.GraphBackend != "" {
		cfg.Databases.Graph.Backend = settings.GraphBackend
	}
	if settings.GraphDSN != "" {
		cfg.Databases.Graph.DSN = settings.GraphDSN
	}

	return nil
}

func persistToConfigYAML(settings agentdSettings) error {
	settings = normalizeAgentdSettings(settings)
	path := findConfigYAMLPath()

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &root)
	}

	applyAgentdSettingsYAML(root, settings)

	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func applyAgentdSettingsYAML(root map[string]any, settings agentdSettings) {
	settings = normalizeAgentdSettings(settings)

	setNestedMapValue(root, []string{"summaryEnabled"}, settings.SummaryEnabled)
	if settings.SummaryReserveBufferTokens != 0 {
		setNestedMapValue(root, []string{"summaryReserveBufferTokens"}, settings.SummaryReserveBufferTokens)
	}

	if settings.OpenAISummaryModel != "" {
		setNestedMapValue(root, []string{"llm_client", "openai", "summaryModel"}, settings.OpenAISummaryModel)
	}
	if settings.OpenAISummaryURL != "" {
		setNestedMapValue(root, []string{"llm_client", "openai", "summaryBaseURL"}, settings.OpenAISummaryURL)
	}

	if settings.EmbedBaseURL != "" {
		setNestedMapValue(root, []string{"embedding", "baseURL"}, settings.EmbedBaseURL)
	}
	if settings.EmbedModel != "" {
		setNestedMapValue(root, []string{"embedding", "model"}, settings.EmbedModel)
	}
	if settings.EmbedAPIKey != "" {
		setNestedMapValue(root, []string{"embedding", "apiKey"}, settings.EmbedAPIKey)
	}
	if settings.EmbedAPIHeader != "" {
		setNestedMapValue(root, []string{"embedding", "apiHeader"}, settings.EmbedAPIHeader)
	}
	if settings.EmbedPath != "" {
		setNestedMapValue(root, []string{"embedding", "path"}, settings.EmbedPath)
	}
	if len(settings.EmbedAPIHeaders) > 0 {
		setNestedMapValue(root, []string{"embedding", "headers"}, settings.EmbedAPIHeaders)
	}

	if settings.AgentRunTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"agentRunTimeoutSeconds"}, settings.AgentRunTimeoutSeconds)
	}
	if settings.StreamRunTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"streamRunTimeoutSeconds"}, settings.StreamRunTimeoutSeconds)
	}
	if settings.WorkflowTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"workflowTimeoutSeconds"}, settings.WorkflowTimeoutSeconds)
	}

	if settings.BlockBinaries != "" {
		parts, err := parseBlockBinaries(settings.BlockBinaries)
		if err == nil {
			setNestedMapValue(root, []string{"exec", "blockBinaries"}, parts)
		}
	}
	if settings.MaxCommandSeconds != 0 {
		setNestedMapValue(root, []string{"exec", "maxCommandSeconds"}, settings.MaxCommandSeconds)
	}
	if settings.OutputTruncateBytes != 0 {
		setNestedMapValue(root, []string{"outputTruncateBytes"}, settings.OutputTruncateBytes)
	}

	if settings.OTELServiceName != "" {
		setNestedMapValue(root, []string{"obs", "serviceName"}, settings.OTELServiceName)
	}
	if settings.ServiceVersion != "" {
		setNestedMapValue(root, []string{"obs", "serviceVersion"}, settings.ServiceVersion)
	}
	if settings.Environment != "" {
		setNestedMapValue(root, []string{"obs", "environment"}, settings.Environment)
	}
	if settings.OTLPEndpoint != "" {
		setNestedMapValue(root, []string{"obs", "otlp"}, settings.OTLPEndpoint)
	}

	setNestedMapValue(root, []string{"logPayloads"}, settings.LogPayloads)
	if settings.LogPath != "" {
		setNestedMapValue(root, []string{"logPath"}, settings.LogPath)
	}
	if settings.LogLevel != "" {
		setNestedMapValue(root, []string{"logLevel"}, settings.LogLevel)
	}

	if settings.WebSearXNGURL != "" {
		setNestedMapValue(root, []string{"web", "searXNGURL"}, settings.WebSearXNGURL)
	}

	if settings.PostgresDSN != "" {
		setNestedMapValue(root, []string{"databases", "defaultDSN"}, settings.PostgresDSN)
	}
	if settings.SearchBackend != "" {
		setNestedMapValue(root, []string{"databases", "search", "backend"}, settings.SearchBackend)
	}
	if settings.SearchDSN != "" {
		setNestedMapValue(root, []string{"databases", "search", "dsn"}, settings.SearchDSN)
	}
	if settings.SearchIndex != "" {
		setNestedMapValue(root, []string{"databases", "search", "index"}, settings.SearchIndex)
	}
	if settings.VectorBackend != "" {
		setNestedMapValue(root, []string{"databases", "vector", "backend"}, settings.VectorBackend)
	}
	if settings.VectorDSN != "" {
		setNestedMapValue(root, []string{"databases", "vector", "dsn"}, settings.VectorDSN)
	}
	if settings.VectorIndex != "" {
		setNestedMapValue(root, []string{"databases", "vector", "index"}, settings.VectorIndex)
	}
	if settings.VectorDims != 0 {
		setNestedMapValue(root, []string{"databases", "vector", "dimensions"}, settings.VectorDims)
	}
	if settings.VectorMetric != "" {
		setNestedMapValue(root, []string{"databases", "vector", "metric"}, settings.VectorMetric)
	}
	if settings.GraphBackend != "" {
		setNestedMapValue(root, []string{"databases", "graph", "backend"}, settings.GraphBackend)
	}
	if settings.GraphDSN != "" {
		setNestedMapValue(root, []string{"databases", "graph", "dsn"}, settings.GraphDSN)
	}
}

func parseBlockBinaries(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") || strings.Contains(part, "\\") {
			return nil, fmt.Errorf("blockBinaries must be bare binary names (no paths): %q", part)
		}
		out = append(out, part)
	}
	return out, nil
}

func setNestedMapValue(root map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := root
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func findConfigYAMLPath() string {
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat("config.yml"); err == nil {
		return "config.yml"
	}
	return "config.yaml"
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
