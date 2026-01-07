package agentd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// agentdSettings mirrors the frontend AgentdSettings shape.
type agentdSettings struct {
	OpenAISummaryModel         string `json:"openaiSummaryModel"`
	OpenAISummaryURL           string `json:"openaiSummaryUrl"`
	SummaryEnabled             bool   `json:"summaryEnabled"`
	SummaryReserveBufferTokens int    `json:"summaryReserveBufferTokens"`

	EmbedBaseURL    string            `json:"embedBaseUrl"`
	EmbedModel      string            `json:"embedModel"`
	EmbedAPIKey     string            `json:"embedApiKey"`
	EmbedAPIHeader  string            `json:"embedApiHeader"`
	EmbedAPIHeaders map[string]string `json:"embedApiHeaders"`
	EmbedPath       string            `json:"embedPath"`

	AgentRunTimeoutSeconds  int `json:"agentRunTimeoutSeconds"`
	StreamRunTimeoutSeconds int `json:"streamRunTimeoutSeconds"`
	WorkflowTimeoutSeconds  int `json:"workflowTimeoutSeconds"`

	BlockBinaries       string `json:"blockBinaries"`
	MaxCommandSeconds   int    `json:"maxCommandSeconds"`
	OutputTruncateBytes int    `json:"outputTruncateBytes"`

	OTELServiceName string `json:"otelServiceName"`
	ServiceVersion  string `json:"serviceVersion"`
	Environment     string `json:"environment"`
	OTLPEndpoint    string `json:"otelExporterOtlpEndpoint"`

	LogPath     string `json:"logPath"`
	LogLevel    string `json:"logLevel"`
	LogPayloads bool   `json:"logPayloads"`

	SearXNGURL    string `json:"searxngUrl"`
	WebSearXNGURL string `json:"webSearxngUrl"`

	DatabaseURL string `json:"databaseUrl"`
	DBURL       string `json:"dbUrl"`
	PostgresDSN string `json:"postgresDsn"`

	SearchBackend string `json:"searchBackend"`
	SearchDSN     string `json:"searchDsn"`
	SearchIndex   string `json:"searchIndex"`

	VectorBackend string `json:"vectorBackend"`
	VectorDSN     string `json:"vectorDsn"`
	VectorIndex   string `json:"vectorIndex"`
	VectorDims    int    `json:"vectorDimensions"`
	VectorMetric  string `json:"vectorMetric"`

	GraphBackend string `json:"graphBackend"`
	GraphDSN     string `json:"graphDsn"`
}

func (a *app) agentdConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			a.handleGetAgentdConfig(w, r)
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			a.handleUpdateAgentdConfig(w, r)
		default:
			w.Header().Set("Allow", "GET, POST, PUT, PATCH")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) handleGetAgentdConfig(w http.ResponseWriter, r *http.Request) {
	// Auth gate if enabled
	if a.cfg.Auth.Enabled {
		if _, err := a.requireUserID(r); err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	s := agentdSettings{
		OpenAISummaryModel:         a.cfg.OpenAI.SummaryModel,
		OpenAISummaryURL:           a.cfg.OpenAI.SummaryBaseURL,
		SummaryEnabled:             a.cfg.SummaryEnabled,
		SummaryReserveBufferTokens: a.cfg.SummaryReserveBufferTokens,

		EmbedBaseURL:    a.cfg.Embedding.BaseURL,
		EmbedModel:      a.cfg.Embedding.Model,
		EmbedAPIKey:     a.cfg.Embedding.APIKey,
		EmbedAPIHeader:  a.cfg.Embedding.APIHeader,
		EmbedAPIHeaders: a.cfg.Embedding.Headers,
		EmbedPath:       a.cfg.Embedding.Path,

		AgentRunTimeoutSeconds:  a.cfg.AgentRunTimeoutSeconds,
		StreamRunTimeoutSeconds: a.cfg.StreamRunTimeoutSeconds,
		WorkflowTimeoutSeconds:  a.cfg.WorkflowTimeoutSeconds,

		BlockBinaries:       strings.Join(a.cfg.Exec.BlockBinaries, ","),
		MaxCommandSeconds:   a.cfg.Exec.MaxCommandSeconds,
		OutputTruncateBytes: a.cfg.OutputTruncateByte,

		OTELServiceName: a.cfg.Obs.ServiceName,
		ServiceVersion:  a.cfg.Obs.ServiceVersion,
		Environment:     a.cfg.Obs.Environment,
		OTLPEndpoint:    a.cfg.Obs.OTLP,

		LogPath:     a.cfg.LogPath,
		LogLevel:    a.cfg.LogLevel,
		LogPayloads: a.cfg.LogPayloads,

		SearXNGURL:    a.cfg.Web.SearXNGURL,
		WebSearXNGURL: a.cfg.Web.SearXNGURL,

		DatabaseURL: a.cfg.Databases.DefaultDSN,
		DBURL:       a.cfg.Databases.DefaultDSN,
		PostgresDSN: a.cfg.Databases.DefaultDSN,

		SearchBackend: a.cfg.Databases.Search.Backend,
		SearchDSN:     a.cfg.Databases.Search.DSN,
		SearchIndex:   a.cfg.Databases.Search.Index,

		VectorBackend: a.cfg.Databases.Vector.Backend,
		VectorDSN:     a.cfg.Databases.Vector.DSN,
		VectorIndex:   a.cfg.Databases.Vector.Index,
		VectorDims:    a.cfg.Databases.Vector.Dimensions,
		VectorMetric:  a.cfg.Databases.Vector.Metric,

		GraphBackend: a.cfg.Databases.Graph.Backend,
		GraphDSN:     a.cfg.Databases.Graph.DSN,
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *app) handleUpdateAgentdConfig(w http.ResponseWriter, r *http.Request) {
	// Auth gate if enabled
	if a.cfg.Auth.Enabled {
		if _, err := a.requireUserID(r); err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	var payload agentdSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Apply to in-memory config so subsequent GET reflects changes immediately.
	if payload.OpenAISummaryModel != "" {
		a.cfg.OpenAI.SummaryModel = payload.OpenAISummaryModel
	}
	if payload.OpenAISummaryURL != "" {
		a.cfg.OpenAI.SummaryBaseURL = payload.OpenAISummaryURL
	}
	a.cfg.SummaryEnabled = payload.SummaryEnabled
	if payload.SummaryReserveBufferTokens != 0 {
		a.cfg.SummaryReserveBufferTokens = payload.SummaryReserveBufferTokens
	}

	if payload.EmbedBaseURL != "" {
		a.cfg.Embedding.BaseURL = payload.EmbedBaseURL
	}
	if payload.EmbedModel != "" {
		a.cfg.Embedding.Model = payload.EmbedModel
	}
	if payload.EmbedAPIKey != "" {
		a.cfg.Embedding.APIKey = payload.EmbedAPIKey
	}
	if payload.EmbedAPIHeader != "" {
		a.cfg.Embedding.APIHeader = payload.EmbedAPIHeader
	}
	if payload.EmbedAPIHeaders != nil {
		a.cfg.Embedding.Headers = payload.EmbedAPIHeaders
	}
	if payload.EmbedPath != "" {
		a.cfg.Embedding.Path = payload.EmbedPath
	}

	if payload.AgentRunTimeoutSeconds != 0 {
		a.cfg.AgentRunTimeoutSeconds = payload.AgentRunTimeoutSeconds
	}
	if payload.StreamRunTimeoutSeconds != 0 {
		a.cfg.StreamRunTimeoutSeconds = payload.StreamRunTimeoutSeconds
	}
	if payload.WorkflowTimeoutSeconds != 0 {
		a.cfg.WorkflowTimeoutSeconds = payload.WorkflowTimeoutSeconds
	}

	if strings.TrimSpace(payload.BlockBinaries) != "" {
		// Validate block binaries are bare names
		items := strings.Split(payload.BlockBinaries, ",")
		var cleaned []string
		for _, it := range items {
			it = strings.TrimSpace(it)
			if it == "" {
				continue
			}
			if strings.Contains(it, "/") || strings.Contains(it, "\\") {
				writeError(w, http.StatusBadRequest, fmt.Errorf("blockBinaries must be bare binary names (no paths): %q", it))
				return
			}
			cleaned = append(cleaned, it)
		}
		a.cfg.Exec.BlockBinaries = cleaned
	}
	if payload.MaxCommandSeconds != 0 {
		a.cfg.Exec.MaxCommandSeconds = payload.MaxCommandSeconds
	}
	if payload.OutputTruncateBytes != 0 {
		a.cfg.OutputTruncateByte = payload.OutputTruncateBytes
	}

	if payload.OTELServiceName != "" {
		a.cfg.Obs.ServiceName = payload.OTELServiceName
	}
	if payload.ServiceVersion != "" {
		a.cfg.Obs.ServiceVersion = payload.ServiceVersion
	}
	if payload.Environment != "" {
		a.cfg.Obs.Environment = payload.Environment
	}
	if payload.OTLPEndpoint != "" {
		a.cfg.Obs.OTLP = payload.OTLPEndpoint
	}

	if payload.LogPath != "" {
		a.cfg.LogPath = payload.LogPath
	}
	if payload.LogLevel != "" {
		a.cfg.LogLevel = payload.LogLevel
	}
	a.cfg.LogPayloads = payload.LogPayloads

	if payload.SearXNGURL != "" {
		a.cfg.Web.SearXNGURL = payload.SearXNGURL
	}
	if payload.WebSearXNGURL != "" {
		a.cfg.Web.SearXNGURL = payload.WebSearXNGURL
	}

	// Databases
	if payload.DatabaseURL != "" {
		a.cfg.Databases.DefaultDSN = payload.DatabaseURL
	}
	if payload.DBURL != "" {
		a.cfg.Databases.DefaultDSN = payload.DBURL
	}
	if payload.PostgresDSN != "" {
		a.cfg.Databases.DefaultDSN = payload.PostgresDSN
	}

	if payload.SearchBackend != "" {
		a.cfg.Databases.Search.Backend = payload.SearchBackend
	}
	if payload.SearchDSN != "" {
		a.cfg.Databases.Search.DSN = payload.SearchDSN
	}
	if payload.SearchIndex != "" {
		a.cfg.Databases.Search.Index = payload.SearchIndex
	}
	if payload.VectorBackend != "" {
		a.cfg.Databases.Vector.Backend = payload.VectorBackend
	}
	if payload.VectorDSN != "" {
		a.cfg.Databases.Vector.DSN = payload.VectorDSN
	}
	if payload.VectorIndex != "" {
		a.cfg.Databases.Vector.Index = payload.VectorIndex
	}
	if payload.VectorDims != 0 {
		a.cfg.Databases.Vector.Dimensions = payload.VectorDims
	}
	if payload.VectorMetric != "" {
		a.cfg.Databases.Vector.Metric = payload.VectorMetric
	}
	if payload.GraphBackend != "" {
		a.cfg.Databases.Graph.Backend = payload.GraphBackend
	}
	if payload.GraphDSN != "" {
		a.cfg.Databases.Graph.DSN = payload.GraphDSN
	}

	// Persist to config.yaml to survive restarts.
	if err := persistToConfigYAML(payload); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("persist config.yaml: %w", err))
		return
	}

	// Indicate that a restart is required for some changes to fully apply.
	w.Header().Set("X-Needs-Restart", "true")
	writeJSON(w, http.StatusOK, payload)
}

func persistToConfigYAML(s agentdSettings) error {
	path := findConfigYAMLPath()

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &root)
	}

	setPath := func(m map[string]any, path []string, value any) {
		if len(path) == 0 {
			return
		}
		cur := m
		for i := 0; i < len(path)-1; i++ {
			k := path[i]
			nxt, ok := cur[k]
			if ok {
				if mm, ok := nxt.(map[string]any); ok {
					cur = mm
					continue
				}
			}
			mm := map[string]any{}
			cur[k] = mm
			cur = mm
		}
		cur[path[len(path)-1]] = value
	}

	// Summary (token-based)
	setPath(root, []string{"summaryEnabled"}, s.SummaryEnabled)
	if s.SummaryReserveBufferTokens != 0 {
		setPath(root, []string{"summaryReserveBufferTokens"}, s.SummaryReserveBufferTokens)
	}

	// OpenAI summary overrides (for OpenAI-compatible providers)
	if s.OpenAISummaryModel != "" {
		setPath(root, []string{"llm_client", "openai", "summaryModel"}, s.OpenAISummaryModel)
	}
	if s.OpenAISummaryURL != "" {
		setPath(root, []string{"llm_client", "openai", "summaryBaseURL"}, s.OpenAISummaryURL)
	}

	// Embedding
	if s.EmbedBaseURL != "" {
		setPath(root, []string{"embedding", "baseURL"}, s.EmbedBaseURL)
	}
	if s.EmbedModel != "" {
		setPath(root, []string{"embedding", "model"}, s.EmbedModel)
	}
	if s.EmbedAPIKey != "" {
		setPath(root, []string{"embedding", "apiKey"}, s.EmbedAPIKey)
	}
	if s.EmbedAPIHeader != "" {
		setPath(root, []string{"embedding", "apiHeader"}, s.EmbedAPIHeader)
	}
	if s.EmbedPath != "" {
		setPath(root, []string{"embedding", "path"}, s.EmbedPath)
	}
	if s.EmbedAPIHeaders != nil && len(s.EmbedAPIHeaders) > 0 {
		setPath(root, []string{"embedding", "headers"}, s.EmbedAPIHeaders)
	}

	// Runtime timeouts
	if s.AgentRunTimeoutSeconds != 0 {
		setPath(root, []string{"agentRunTimeoutSeconds"}, s.AgentRunTimeoutSeconds)
	}
	if s.StreamRunTimeoutSeconds != 0 {
		setPath(root, []string{"streamRunTimeoutSeconds"}, s.StreamRunTimeoutSeconds)
	}
	if s.WorkflowTimeoutSeconds != 0 {
		setPath(root, []string{"workflowTimeoutSeconds"}, s.WorkflowTimeoutSeconds)
	}

	// Exec
	if strings.TrimSpace(s.BlockBinaries) != "" {
		parts := strings.Split(s.BlockBinaries, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		setPath(root, []string{"exec", "blockBinaries"}, out)
	}
	if s.MaxCommandSeconds != 0 {
		setPath(root, []string{"exec", "maxCommandSeconds"}, s.MaxCommandSeconds)
	}
	if s.OutputTruncateBytes != 0 {
		setPath(root, []string{"outputTruncateBytes"}, s.OutputTruncateBytes)
	}

	// Observability
	if s.OTELServiceName != "" {
		setPath(root, []string{"obs", "serviceName"}, s.OTELServiceName)
	}
	if s.ServiceVersion != "" {
		setPath(root, []string{"obs", "serviceVersion"}, s.ServiceVersion)
	}
	if s.Environment != "" {
		setPath(root, []string{"obs", "environment"}, s.Environment)
	}
	if s.OTLPEndpoint != "" {
		setPath(root, []string{"obs", "otlp"}, s.OTLPEndpoint)
	}

	// Logging
	setPath(root, []string{"logPayloads"}, s.LogPayloads)
	if s.LogPath != "" {
		setPath(root, []string{"logPath"}, s.LogPath)
	}
	if s.LogLevel != "" {
		setPath(root, []string{"logLevel"}, s.LogLevel)
	}

	// Web
	if s.SearXNGURL != "" {
		setPath(root, []string{"web", "searXNGURL"}, s.SearXNGURL)
	} else if s.WebSearXNGURL != "" {
		setPath(root, []string{"web", "searXNGURL"}, s.WebSearXNGURL)
	}

	// Databases
	if s.DatabaseURL != "" {
		setPath(root, []string{"databases", "defaultDSN"}, s.DatabaseURL)
	} else if s.DBURL != "" {
		setPath(root, []string{"databases", "defaultDSN"}, s.DBURL)
	} else if s.PostgresDSN != "" {
		setPath(root, []string{"databases", "defaultDSN"}, s.PostgresDSN)
	}
	if s.SearchBackend != "" {
		setPath(root, []string{"databases", "search", "backend"}, s.SearchBackend)
	}
	if s.SearchDSN != "" {
		setPath(root, []string{"databases", "search", "dsn"}, s.SearchDSN)
	}
	if s.SearchIndex != "" {
		setPath(root, []string{"databases", "search", "index"}, s.SearchIndex)
	}
	if s.VectorBackend != "" {
		setPath(root, []string{"databases", "vector", "backend"}, s.VectorBackend)
	}
	if s.VectorDSN != "" {
		setPath(root, []string{"databases", "vector", "dsn"}, s.VectorDSN)
	}
	if s.VectorIndex != "" {
		setPath(root, []string{"databases", "vector", "index"}, s.VectorIndex)
	}
	if s.VectorDims != 0 {
		setPath(root, []string{"databases", "vector", "dimensions"}, s.VectorDims)
	}
	if s.VectorMetric != "" {
		setPath(root, []string{"databases", "vector", "metric"}, s.VectorMetric)
	}
	if s.GraphBackend != "" {
		setPath(root, []string{"databases", "graph", "backend"}, s.GraphBackend)
	}
	if s.GraphDSN != "" {
		setPath(root, []string{"databases", "graph", "dsn"}, s.GraphDSN)
	}

	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

// persistToDotEnv updates (or creates) a local .env file with the provided settings.
func persistToDotEnv(s agentdSettings) error {
	path := findDotEnvPath()
	// Read existing
	values, _ := readDotEnv(path)

	// Map settings -> env keys
	set := func(k, v string) {
		if v == "" {
			return
		}
		values[k] = v
	}
	set("OPENAI_SUMMARY_MODEL", s.OpenAISummaryModel)
	set("OPENAI_SUMMARY_URL", s.OpenAISummaryURL)
	set("SUMMARY_ENABLED", fmtBool(s.SummaryEnabled))
	if s.SummaryReserveBufferTokens != 0 {
		set("SUMMARY_RESERVE_BUFFER_TOKENS", fmtInt(s.SummaryReserveBufferTokens))
	}

	set("EMBED_BASE_URL", s.EmbedBaseURL)
	set("EMBED_MODEL", s.EmbedModel)
	set("EMBED_API_KEY", s.EmbedAPIKey)
	set("EMBED_API_HEADER", s.EmbedAPIHeader)
	if s.EmbedAPIHeaders != nil && len(s.EmbedAPIHeaders) > 0 {
		if b, err := json.Marshal(s.EmbedAPIHeaders); err == nil {
			set("EMBED_API_HEADERS", string(b))
		}
	}
	set("EMBED_PATH", s.EmbedPath)

	if s.AgentRunTimeoutSeconds != 0 {
		set("AGENT_RUN_TIMEOUT_SECONDS", fmtInt(s.AgentRunTimeoutSeconds))
	}
	if s.StreamRunTimeoutSeconds != 0 {
		set("STREAM_RUN_TIMEOUT_SECONDS", fmtInt(s.StreamRunTimeoutSeconds))
	}
	if s.WorkflowTimeoutSeconds != 0 {
		set("WORKFLOW_TIMEOUT_SECONDS", fmtInt(s.WorkflowTimeoutSeconds))
	}

	if strings.TrimSpace(s.BlockBinaries) != "" {
		set("BLOCK_BINARIES", normalizeCSV(s.BlockBinaries))
	}
	if s.MaxCommandSeconds != 0 {
		set("MAX_COMMAND_SECONDS", fmtInt(s.MaxCommandSeconds))
	}
	if s.OutputTruncateBytes != 0 {
		set("OUTPUT_TRUNCATE_BYTES", fmtInt(s.OutputTruncateBytes))
	}

	set("OTEL_SERVICE_NAME", s.OTELServiceName)
	set("SERVICE_VERSION", s.ServiceVersion)
	set("ENVIRONMENT", s.Environment)
	set("OTEL_EXPORTER_OTLP_ENDPOINT", s.OTLPEndpoint)

	set("LOG_PATH", s.LogPath)
	set("LOG_LEVEL", s.LogLevel)
	set("LOG_PAYLOADS", fmtBool(s.LogPayloads))

	if s.SearXNGURL != "" {
		set("SEARXNG_URL", s.SearXNGURL)
	} else if s.WebSearXNGURL != "" {
		set("SEARXNG_URL", s.WebSearXNGURL)
	}

	// Databases
	if s.DatabaseURL != "" {
		set("DATABASE_URL", s.DatabaseURL)
	} else if s.DBURL != "" {
		set("DATABASE_URL", s.DBURL)
	} else if s.PostgresDSN != "" {
		set("DATABASE_URL", s.PostgresDSN)
	}
	set("SEARCH_BACKEND", s.SearchBackend)
	set("SEARCH_DSN", s.SearchDSN)
	set("SEARCH_INDEX", s.SearchIndex)
	set("VECTOR_BACKEND", s.VectorBackend)
	set("VECTOR_DSN", s.VectorDSN)
	set("VECTOR_INDEX", s.VectorIndex)
	if s.VectorDims != 0 {
		set("VECTOR_DIMENSIONS", fmtInt(s.VectorDims))
	}
	set("VECTOR_METRIC", s.VectorMetric)
	set("GRAPH_BACKEND", s.GraphBackend)
	set("GRAPH_DSN", s.GraphDSN)

	return writeDotEnv(path, values)
}

func fmtInt(n int) string { return fmt.Sprintf("%d", n) }
func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func normalizeCSV(s string) string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// readDotEnv parses a minimal KEY=VALUE file. Quoted values are supported.
func readDotEnv(path string) (map[string]string, error) {
	res := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return res, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	re := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$`)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.TrimSpace(line) == "" {
			continue
		}
		m := re.FindStringSubmatch(line)
		if len(m) == 3 {
			k := m[1]
			v := strings.TrimSpace(m[2])
			v = strings.Trim(v, "\"")
			res[k] = v
		}
	}
	return res, scanner.Err()
}

func writeDotEnv(path string, values map[string]string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// Write in sorted deterministic order
	// Preserve existing comments is out of scope; we emit a simple file.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	_, _ = bw.WriteString("# Managed by agentd /api/config/agentd. Manual edits may be overwritten.\n")
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	// Simple alpha sort
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		v := values[k]
		// Quote if contains spaces or special chars
		if strings.ContainsAny(v, " #\t\n\r") {
			v = "\"" + v + "\""
		}
		_, _ = bw.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}
	return bw.Flush()
}

func findDotEnvPath() string {
	// Prefer .env in current working directory; fallback to example.env name
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	// If example exists, write next to it as .env
	if _, err := os.Stat("example.env"); err == nil {
		return ".env"
	}
	return ".env"
}
