package agentd

import (
	"context"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
)

// prepareChatHistoryForBuild is the shared history/summary phase for live and
// durable chat runs. Both paths use the same provider-aware context policy;
// durable execution only adds its resume-specific final-message trim.
func (a *app) prepareChatHistoryForBuild(ctx context.Context, userID *int64, sessionID string, build *chatEngineBuildResult) ([]llm.Message, *memory.SummaryResult, error) {
	if build.ImageGeneration || build.VideoGeneration {
		return nil, nil, nil
	}
	history, summary, err := a.chatMemory.BuildContextForProvider(ctx, userID, sessionID, build.Engine.LLM, build.Engine.Model, memory.SummaryPolicy{
		TargetContextWindowTokens:    build.Engine.ContextWindowTokens,
		PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
	})
	if err != nil {
		return nil, nil, err
	}
	build.Engine.SkipInitialSummarization = summary != nil && summary.Triggered
	return history, summary, nil
}
