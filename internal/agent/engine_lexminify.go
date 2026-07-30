package agent

import (
	"context"

	"manifold/internal/llm"
	"manifold/internal/llm/lexminify"
	"manifold/internal/observability"
)

// lexMinifyForProvider returns a provider-visible copy of msgs with lexical
// minification applied when Engine.LexMinifyLevel > 0. The permanent conversation
// history used for tool dispatch, checkpoints, and turn persistence is untouched.
func (e *Engine) lexMinifyForProvider(ctx context.Context, msgs []llm.Message) []llm.Message {
	if e == nil || e.LexMinifyLevel <= lexminify.Off || len(msgs) == 0 {
		return msgs
	}
	opts := lexminify.Options{
		Level: e.LexMinifyLevel,
		Zones: lexminify.Zone(e.LexMinifyZones),
	}
	if e.LexMinifyCurrentMax > 0 {
		opts.CurrentRequestMaxLevel = e.LexMinifyCurrentMax
	}
	res := lexminify.MinifyMessages(msgs, opts)
	if res.Changed {
		observability.LoggerWithTrace(ctx).Info().
			Int("level", res.Level).
			Int("zones", int(res.Zones)).
			Int("messages_touched", res.MessagesTouched).
			Int("runes_before", res.RunesBefore).
			Int("runes_after", res.RunesAfter).
			Msg("lexminify_applied")
	}
	return res.Messages
}
