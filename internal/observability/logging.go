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

// otelWriterEnabled tracks whether OTLP log export is configured.
// Set by EnableOTelLogging after InitOTel succeeds.
var otelWriterEnabled bool

// currentLogWriter stores the underlying io.Writer for the global logger.
// This allows EnableOTelLogging to wrap it with a MultiWriter.
var currentLogWriter io.Writer
var otelLogWriter io.Writer
var sideLogWriter io.Writer

// consoleWriter is the sink used for the MANIFOLD_LOG_STDOUT tee. It is a
// package var so tests can substitute an in-memory buffer for os.Stdout.
var consoleWriter io.Writer = os.Stdout

// EnvLogStdout, when truthy, tees logs to stdout in addition to the log file so
// they are visible on the console during a normal run.
const EnvLogStdout = "MANIFOLD_LOG_STDOUT"

// ConsoleLoggingRequested reports whether MANIFOLD_LOG_STDOUT requests that logs
// also stream to stdout. Truthy values: 1, true, yes, on (case-insensitive).
func ConsoleLoggingRequested() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvLogStdout))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// InitLogger initializes zerolog with sane defaults. If logPath is non-empty,
// logs are written to that file (append mode). When MANIFOLD_LOG_STDOUT is
// truthy, logs are additionally teed to stdout. If opening the file fails,
// logging falls back to stdout.
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
	// Tee to the console when requested and the primary sink is a file, so logs
	// are visible on stdout without discarding the persistent file log. This is
	// folded into the primary writer (not sideLogWriter) so a later AddLogWriter
	// call cannot clobber it.
	if w != os.Stdout && ConsoleLoggingRequested() {
		w = io.MultiWriter(w, consoleWriter)
	}
	currentLogWriter = w // Store for later use by EnableOTelLogging
	rebuildLoggerOutput()
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

// EnableOTelLogging adds an OTLP log writer to the global zerolog logger.
// Call this AFTER InitOTel succeeds to bridge zerolog -> OTLP logs.
func EnableOTelLogging(serviceName string) {
	if otelWriterEnabled {
		return
	}
	otelWriter := NewOTelWriter(serviceName)
	otelLogWriter = otelWriter
	rebuildLoggerOutput()
	otelWriterEnabled = true
}

// AddLogWriter adds a best-effort side-channel writer to the global zerolog
// output while preserving the existing logger sink.
func AddLogWriter(writer io.Writer) {
	sideLogWriter = writer
	rebuildLoggerOutput()
}

func rebuildLoggerOutput() {
	writers := make([]io.Writer, 0, 3)
	if currentLogWriter != nil {
		writers = append(writers, currentLogWriter)
	} else {
		writers = append(writers, os.Stdout)
	}
	if otelLogWriter != nil {
		writers = append(writers, otelLogWriter)
	}
	if sideLogWriter != nil {
		writers = append(writers, sideLogWriter)
	}
	log.Logger = log.Output(io.MultiWriter(writers...)).With().Timestamp().Logger()
}
