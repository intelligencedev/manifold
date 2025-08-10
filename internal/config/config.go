package config

// Config is the top-level runtime configuration for the agent.
type Config struct {
	Workdir            string
	OutputTruncateByte int
	Exec               ExecConfig
	OpenAI             OpenAIConfig
	Obs                ObsConfig
	Web                WebConfig
}

type ExecConfig struct {
	BlockBinaries     []string
	MaxCommandSeconds int
}

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

type ObsConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLP           string
}

type WebConfig struct {
	SearXNGURL string
}
