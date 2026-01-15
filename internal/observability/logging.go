package observability

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// InitLogger initializes zerolog with sane defaults. If logPath is non-empty,
// logs are also written to that file (append mode). If opening the file fails,
// logs fall back to stdout, and an error is printed to stderr.
func InitLogger(logPath string, level string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	var w io.Writer = os.Stdout
	if logPath != "" {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			// When a log file is configured, write only to the file to avoid
			// interfering with interactive UIs (e.g., TUI) that use stdout.
			w = f
		} else {
			// best-effort; continue with stdout
			_, _ = fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", logPath, err)
		}
	}
	log.Logger = log.Output(w).With().Timestamp().Logger()
	// Parse level
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "warning" {
		level = "warn"
	}
	lvl := zerolog.InfoLevel
	if level != "" {
		if l, err := zerolog.ParseLevel(level); err == nil {
			lvl = l
		}
	}
	zerolog.SetGlobalLevel(lvl)
	// Redirect the standard library logger so ALL logs are captured.
	stdlog.SetFlags(0)
	stdlog.SetOutput(log.Logger)
}
