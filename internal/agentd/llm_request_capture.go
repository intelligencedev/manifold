package agentd

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
)

type llmRequestCaptureConfig struct {
	Store               persist.LLMRequestStore
	SessionID           string
	UserID              *int64
	RunID               string
	MessageID           string
	ParentUserMessageID string
	SpecialistID        string
	CallID              string
	ParentCallID        string
}

func attachLLMRequestCapture(eng *agent.Engine, cfg llmRequestCaptureConfig) {
	if eng == nil || cfg.Store == nil || strings.TrimSpace(cfg.SessionID) == "" {
		return
	}
	previous := eng.OnLLMRequest
	eng.OnLLMRequest = func(snapshot agent.LLMRequestSnapshot) {
		if previous != nil {
			previous(snapshot)
		}
		payload, err := buildLLMRequestPayload(snapshot)
		if err != nil {
			log.Error().Err(err).Str("session", cfg.SessionID).Msg("build_llm_request_payload")
			return
		}
		req := persist.LLMRequest{
			ID:                  snapshot.ID,
			SessionID:           cfg.SessionID,
			UserID:              cloneCollectorUserID(cfg.UserID),
			RunID:               cfg.RunID,
			MessageID:           cfg.MessageID,
			ParentUserMessageID: cfg.ParentUserMessageID,
			CallID:              cfg.CallID,
			ParentCallID:        cfg.ParentCallID,
			SpecialistID:        firstNonEmptyString(cfg.SpecialistID, eng.AgentRole),
			Provider:            snapshot.Provider,
			Model:               snapshot.Model,
			InputTokens:         snapshot.InputTokens,
			MaxContextTokens:    snapshot.MaxContextTokens,
			Payload:             payload,
			Redacted:            true,
			CreatedAt:           snapshot.CreatedAt,
		}
		if err := cfg.Store.AppendLLMRequest(context.Background(), req); err != nil {
			log.Error().Err(err).Str("session", cfg.SessionID).Str("request_id", snapshot.ID).Msg("store_llm_request")
		}
	}
}

func buildLLMRequestPayload(snapshot agent.LLMRequestSnapshot) (json.RawMessage, error) {
	payload := map[string]any{
		"model":    snapshot.Model,
		"messages": snapshotMessages(snapshot.Messages),
		"tools":    snapshot.Tools,
	}
	if snapshot.Provider != "" {
		payload["provider"] = snapshot.Provider
	}
	if snapshot.InputTokens > 0 {
		payload["input_tokens"] = snapshot.InputTokens
	}
	if snapshot.MaxContextTokens > 0 {
		payload["max_context_tokens"] = snapshot.MaxContextTokens
	}
	b, err := json.Marshal(redactValue(payload))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func snapshotMessages(messages []llm.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		m := map[string]any{
			"role":    msg.Role,
			"content": redactString(msg.Content),
		}
		if msg.ToolID != "" {
			m["tool_id"] = msg.ToolID
		}
		if len(msg.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"name": tc.Name,
					"args": redactJSONBytes(tc.Args),
				})
			}
			m["tool_calls"] = calls
		}
		out = append(out, m)
	}
	return out
}

func redactJSONBytes(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return redactString(string(raw))
	}
	return redactValue(value)
}

func redactValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSecretKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValue(item)
		}
		return out
	case string:
		return redactString(v)
	default:
		return v
	}
}

func isSecretKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	secretMarkers := []string{"api_key", "apikey", "authorization", "password", "passwd", "secret", "token", "credential", "private_key", "access_key"}
	for _, marker := range secretMarkers {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

var secretValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s,;}]+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)[^\s,;}]+`),
	regexp.MustCompile(`(?i)(password\s*[:=]\s*)[^\s,;}]+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s,;}]+`),
}

func redactString(value string) string {
	out := value
	for _, pattern := range secretValuePatterns {
		out = pattern.ReplaceAllString(out, `${1}[REDACTED]`)
	}
	return out
}
