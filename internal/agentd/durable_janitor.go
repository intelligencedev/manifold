package agentd

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultDurableTaskRetention       = 7 * 24 * time.Hour
	defaultDurableTaskJanitorInterval = time.Hour
)

func (a *app) startDurableTaskJanitor(ctx context.Context, interval time.Duration, retention time.Duration) {
	if a == nil || a.durableClient == nil || interval <= 0 || retention <= 0 {
		return
	}
	prune := func() {
		removed, err := a.durableClient.PruneTerminalTasks(ctx, time.Now().UTC().Add(-retention))
		if err != nil {
			log.Warn().Err(err).Msg("durable_task_prune_failed")
			return
		}
		if removed > 0 {
			log.Debug().Int64("removed", removed).Dur("retention", retention).Msg("durable_tasks_pruned")
		}
	}
	go func() {
		prune()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	}()
}
