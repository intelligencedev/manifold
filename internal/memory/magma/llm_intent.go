package magma

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/llm"
)

type llmIntentPayload struct {
	Intents []string `json:"intents"`
}

func (q QueryEngine) classifyIntentWithLLM(ctx context.Context, query string) (IntentCategory, bool) {
	if q.Service == nil || q.Service.cfg.LLM == nil {
		return 0, false
	}
	msg, err := q.Service.cfg.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: magmaIntentSystemPrompt},
		{Role: "user", Content: strings.TrimSpace(query)},
	}, nil, q.Service.cfg.Model)
	if err != nil {
		return 0, false
	}
	var payload llmIntentPayload
	if err := json.Unmarshal([]byte(extractJSONObject(msg.Content)), &payload); err != nil {
		return 0, false
	}
	var intent IntentCategory
	for _, value := range payload.Intents {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "temporal":
			intent |= IntentTemporal
		case "entity":
			intent |= IntentEntity
		case "semantic":
			intent |= IntentSemantic
		case "causal":
			intent |= IntentCausal
		}
	}
	return intent, intent != 0
}

const magmaIntentSystemPrompt = `Classify the memory retrieval query into one or more MAGMA intents. Return strict JSON only:
{"intents":["temporal"|"entity"|"semantic"|"causal"]}
Use temporal for before/after/when questions, entity for people/things/relationships, causal for why/cause/effect questions, and semantic for broad topical recall.`
