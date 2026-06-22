package agentd

import (
	"time"

	"manifold/internal/config"
)

func summaryCallTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Summary.CallTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(cfg.Summary.CallTimeoutSeconds) * time.Second
}
