package observability

import (
    "os"
    "time"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

// InitLogger initializes zerolog with sane defaults.
func InitLogger() {
    zerolog.TimeFieldFormat = time.RFC3339Nano
    log.Logger = log.Output(os.Stdout).With().Timestamp().Logger()
    zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

