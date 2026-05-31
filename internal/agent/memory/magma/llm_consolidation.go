package magma

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"manifold/internal/llm"
)

type llmExtraction struct {
	TemporalAttrs TemporalAttrs
	Entities      []EntityMention
	CausalEdges   []Edge
}

type llmExtractionPayload struct {
	Temporal *TemporalAttrs `json:"temporal,omitempty"`
	Entities []struct {
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
		Role string `json:"role,omitempty"`
	} `json:"entities,omitempty"`
	Causal []struct {
		Cause      string  `json:"cause"`
		Effect     string  `json:"effect"`
		Confidence float64 `json:"confidence,omitempty"`
	} `json:"causal,omitempty"`
}

func (s *Service) extractWithLLM(ctx context.Context, event EventNode) (llmExtraction, bool) {
	if s == nil || s.cfg.LLM == nil {
		return llmExtraction{}, false
	}
	msg, err := s.cfg.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: firstNonEmpty(s.cfg.Prompts.ConsolidationExtraction, magmaConsolidationSystemPrompt)},
		{Role: "user", Content: magmaConsolidationUserPrompt(event)},
	}, nil, s.cfg.Model)
	if err != nil {
		return llmExtraction{}, false
	}
	var payload llmExtractionPayload
	if err := json.Unmarshal([]byte(extractJSONObject(msg.Content)), &payload); err != nil {
		return llmExtraction{}, false
	}
	extracted := llmExtraction{}
	if payload.Temporal != nil {
		extracted.TemporalAttrs = *payload.Temporal
	}
	for _, entity := range payload.Entities {
		name := strings.TrimSpace(entity.Name)
		if name == "" {
			continue
		}
		entityType := strings.TrimSpace(entity.Type)
		if entityType == "" {
			entityType = "Entity"
		}
		extracted.Entities = append(extracted.Entities, EntityMention{
			ID:   entityID(event.Tenant, name),
			Type: entityType,
			Role: strings.TrimSpace(entity.Role),
			Name: name,
		})
	}
	for _, causal := range payload.Causal {
		cause := strings.TrimSpace(causal.Cause)
		effect := strings.TrimSpace(causal.Effect)
		if cause == "" || effect == "" {
			continue
		}
		confidence := causal.Confidence
		if confidence == 0 {
			confidence = 0.5
		}
		if s.cfg.CausalThreshold > 0 && confidence < s.cfg.CausalThreshold {
			continue
		}
		extracted.CausalEdges = append(extracted.CausalEdges, Edge{
			Source:    event.ID,
			GraphType: GraphCausal,
			Rel:       "CAUSES",
			Target:    event.ID,
			Props: map[string]any{
				"confidence":  confidence,
				"extractor":   "llm",
				"cause_text":  cause,
				"effect_text": effect,
			},
		})
	}
	return extracted, extracted.TemporalAttrs != (TemporalAttrs{}) || len(extracted.Entities) > 0 || len(extracted.CausalEdges) > 0
}

const magmaConsolidationSystemPrompt = `Extract MAGMA memory structure from the event. Return strict JSON only with optional keys:
{"temporal":{"date":"YYYY-MM-DD","relative_to":"","offset":""},"entities":[{"name":"","type":"","role":""}],"causal":[{"cause":"","effect":"","confidence":0.0}]}
Only include grounded facts from the event text.`

func magmaConsolidationUserPrompt(event EventNode) string {
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return "Tenant: " + event.Tenant + "\nCreated at: " + createdAt.Format(time.RFC3339) + "\nEvent text: " + event.Text
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return raw
	}
	return raw[start : end+1]
}
