package agentd

import (
	"context"
)

func (a *app) registerDurableHandlers() {
	if a == nil || a.durableRegistry == nil {
		return
	}
	a.durableRegistry.Register(warppDurableQueue, warppDurableRunTaskName, a.runDurableWarppTask)
	a.durableRegistry.Register(durableChatQueue, durableChatRunTaskName, a.runDurableChatTask)
	a.durableRegistry.Register(durablePulseQueue, durablePulseRunTaskName, a.runDurablePulseTask)
}

func (a *app) runDurableWarppTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	return a.warppService().RunDurableTask(ctx, params)
}
