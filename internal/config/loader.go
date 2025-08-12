package config

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/joho/godotenv"
    yaml "gopkg.in/yaml.v3"
)

// Load reads configuration from environment variables (optionally .env).
func Load() (Config, error) {
    _ = godotenv.Load()

	cfg := Config{}
	cfg.OpenAI.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	cfg.OpenAI.Model = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_MODEL")), "gpt-4o-mini")
	// Allow overriding API base via env (useful for proxies/self-hosted gateways)
	cfg.OpenAI.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_API_BASE_URL")))
	cfg.Workdir = strings.TrimSpace(os.Getenv("WORKDIR"))
	cfg.Exec.MaxCommandSeconds = intFromEnv("MAX_COMMAND_SECONDS", 30)
	cfg.OutputTruncateByte = intFromEnv("OUTPUT_TRUNCATE_BYTES", 64*1024)

	cfg.Obs.ServiceName = firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), "singularityio")
	cfg.Obs.ServiceVersion = strings.TrimSpace(os.Getenv("SERVICE_VERSION"))
	cfg.Obs.Environment = firstNonEmpty(os.Getenv("ENVIRONMENT"), "dev")
	cfg.Obs.OTLP = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

    cfg.Web.SearXNGURL = firstNonEmpty(os.Getenv("SEARXNG_URL"), "http://localhost:8080")

    // Optionally load specialist agents from YAML.
    if err := loadSpecialists(&cfg); err != nil {
        return Config{}, err
    }

	if cfg.OpenAI.APIKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required (set in .env or environment)")
	}
	if cfg.Workdir == "" {
		return Config{}, errors.New("WORKDIR is required (set in .env or environment)")
	}

	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve WORKDIR: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return Config{}, fmt.Errorf("stat WORKDIR: %w", err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("WORKDIR must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD

	// Parse blocklist
	blockStr := strings.TrimSpace(os.Getenv("BLOCK_BINARIES"))
	if blockStr != "" {
		parts := strings.Split(blockStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.Contains(p, "/") || strings.Contains(p, "\\") {
				return Config{}, fmt.Errorf("BLOCK_BINARIES must contain bare binary names only (no paths): %q", p)
			}
			cfg.Exec.BlockBinaries = append(cfg.Exec.BlockBinaries, p)
		}
	}
    return cfg, nil
}

// loadSpecialists populates cfg.Specialists by reading a YAML file if present.
// The file path can be specified with SPECIALISTS_CONFIG. If not set, the
// loader will look for configs/specialists.yaml or configs/specialists.yml.
func loadSpecialists(cfg *Config) error {
    // Allow disabling via env
    if strings.EqualFold(strings.TrimSpace(os.Getenv("SPECIALISTS_DISABLED")), "true") {
        return nil
    }
    var paths []string
    if p := strings.TrimSpace(os.Getenv("SPECIALISTS_CONFIG")); p != "" {
        paths = append(paths, p)
    }
    paths = append(paths, "configs/specialists.yaml", "configs/specialists.yml")
    var data []byte
    var chosen string
    for _, p := range paths {
        b, err := os.ReadFile(p)
        if err == nil {
            data = b
            chosen = p
            break
        }
        if os.IsNotExist(err) {
            continue
        }
        // Unexpected read error
        return fmt.Errorf("read %s: %w", p, err)
    }
    if len(data) == 0 {
        return nil // optional
    }
    // Two accepted shapes:
    //   specialists: [ {name: ..., ...}, ... ]
    // or directly a list: [ {name: ..., ...} ]
    type wrap struct {
        Specialists []SpecialistConfig `yaml:"specialists"`
    }
    var w wrap
    // Expand ${VAR} with environment variables before parsing.
    data = []byte(os.ExpandEnv(string(data)))
    if err := yaml.Unmarshal(data, &w); err == nil && len(w.Specialists) > 0 {
        cfg.Specialists = w.Specialists
        return nil
    }
    // Fallback: try list at root
    var list []SpecialistConfig
    if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
        cfg.Specialists = list
        return nil
    }
    return fmt.Errorf("%s: could not parse specialists configuration", chosen)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func intFromEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := parseInt(v); err == nil {
			return n
		}
	}
	return def
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
