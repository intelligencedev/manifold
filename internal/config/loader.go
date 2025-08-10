package config

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/joho/godotenv"
)

// Load reads configuration from environment variables (optionally .env).
func Load() (Config, error) {
    _ = godotenv.Load()

    cfg := Config{}
    cfg.OpenAI.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
    cfg.OpenAI.Model = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_MODEL")), "gpt-4o-mini")
    cfg.Workdir = strings.TrimSpace(os.Getenv("WORKDIR"))
    cfg.Exec.MaxCommandSeconds = intFromEnv("MAX_COMMAND_SECONDS", 30)
    cfg.OutputTruncateByte = intFromEnv("OUTPUT_TRUNCATE_BYTES", 64*1024)

    cfg.Obs.ServiceName = firstNonEmpty(os.Getenv("OTEL_SERVICE_NAME"), "singularityio")
    cfg.Obs.ServiceVersion = strings.TrimSpace(os.Getenv("SERVICE_VERSION"))
    cfg.Obs.Environment = firstNonEmpty(os.Getenv("ENVIRONMENT"), "dev")
    cfg.Obs.OTLP = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

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

