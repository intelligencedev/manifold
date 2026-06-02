package agent

import (
	"context"
	"fmt"

	"manifold/internal/llm"
)

const preparedContextCheckpointKey = "prepared_context"

func assistantCheckpointKey(step int) string {
	return fmt.Sprintf("assistant:%d", step)
}

func toolCheckpointKey(step int, toolCallID string) string {
	return fmt.Sprintf("tool:%d:%s", step, toolCallID)
}

func finalCheckpointKey() string {
	return "final_result"
}

func (e *Engine) loadCheckpoint(ctx context.Context, key string, target any) (bool, error) {
	if e == nil || e.Checkpointer == nil {
		return false, nil
	}
	return e.Checkpointer.Load(ctx, key, target)
}

func (e *Engine) saveCheckpoint(ctx context.Context, key string, value any) error {
	if e == nil || e.Checkpointer == nil {
		return nil
	}
	return e.Checkpointer.Save(ctx, key, value)
}

func (e *Engine) loadPreparedContext(ctx context.Context) ([]llm.Message, bool, error) {
	var msgs []llm.Message
	found, err := e.loadCheckpoint(ctx, preparedContextCheckpointKey, &msgs)
	return msgs, found, err
}

func (e *Engine) savePreparedContext(ctx context.Context, msgs []llm.Message) error {
	return e.saveCheckpoint(ctx, preparedContextCheckpointKey, msgs)
}
