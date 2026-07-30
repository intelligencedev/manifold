package agentd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
)

type llmRequestCaptureConfig = chatpkg.CaptureConfig

func attachLLMRequestCapture(eng *agent.Engine, cfg llmRequestCaptureConfig) {
	if eng == nil || cfg.Store == nil || strings.TrimSpace(cfg.SessionID) == "" {
		return
	}
	inner := unwrapLLMRequestCapturingProvider(eng.LLM)
	if inner == nil {
		return
	}
	recorder := &llmRequestCaptureRecorder{
		cfg:                 cfg,
		provider:            inner,
		providerName:        llmProviderName(inner),
		specialistID:        firstNonEmptyString(cfg.SpecialistID, eng.AgentRole),
		defaultModel:        eng.Model,
		contextWindowTokens: eng.ContextWindowTokens,
		tokenizer:           eng.Tokenizer,
		captured:            map[string]string{},
	}
	eng.LLM = &llmRequestCapturingProvider{inner: inner, recorder: recorder}

	// Replace any prior agentd capture hook. Engines can be reused across turns;
	// chaining old capture closures would store new provider calls under old message IDs.
	eng.OnLLMRequest = func(snapshot agent.LLMRequestSnapshot) {
		recorder.storeSnapshot(context.Background(), snapshot)
	}
}

type llmRequestCapturingProvider struct {
	inner    llm.Provider
	recorder *llmRequestCaptureRecorder
}

func (p *llmRequestCapturingProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if p.recorder != nil {
		p.recorder.captureProviderCall(ctx, msgs, tools, model)
	}
	return p.inner.Chat(ctx, msgs, tools, model)
}

func (p *llmRequestCapturingProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	if p.recorder != nil {
		p.recorder.captureProviderCall(ctx, msgs, tools, model)
	}
	return p.inner.ChatStream(ctx, msgs, tools, model, h)
}

func (p *llmRequestCapturingProvider) Tokenizer() llm.Tokenizer {
	if p.recorder != nil && p.recorder.tokenizer != nil {
		return p.recorder.tokenizer
	}
	if tokenizable, ok := p.inner.(llm.TokenizableProvider); ok {
		return tokenizable.Tokenizer()
	}
	return nil
}

func (p *llmRequestCapturingProvider) SupportsCompaction() bool {
	return llm.ProviderSupportsCompaction(p.inner)
}

func (p *llmRequestCapturingProvider) Compact(ctx context.Context, msgs []llm.Message, model string, previous *llm.CompactionItem) (*llm.CompactionItem, error) {
	compactor, ok := llm.ProviderCompactor(p.inner)
	if !ok {
		return nil, errors.New("llm request capture: inner provider does not support compaction")
	}
	return compactor.Compact(ctx, msgs, model, previous)
}

func unwrapLLMRequestCapturingProvider(provider llm.Provider) llm.Provider {
	for {
		wrapped, ok := provider.(*llmRequestCapturingProvider)
		if !ok {
			return provider
		}
		provider = wrapped.inner
	}
}

type llmRequestCaptureRecorder struct {
	mu                  sync.Mutex
	cfg                 llmRequestCaptureConfig
	provider            llm.Provider
	providerName        string
	specialistID        string
	defaultModel        string
	contextWindowTokens int
	tokenizer           llm.Tokenizer
	captured            map[string]string
}

func (r *llmRequestCaptureRecorder) captureProviderCall(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) {
	model = strings.TrimSpace(firstNonEmptyString(model, r.defaultModel))
	r.storeSnapshot(ctx, agent.LLMRequestSnapshot{
		ID:               uuid.NewString(),
		Messages:         cloneLLMMessagesForCapture(msgs),
		Tools:            cloneToolSchemasForCapture(tools),
		Model:            model,
		Provider:         r.providerName,
		InputTokens:      r.countMessagesTokens(ctx, msgs),
		MaxContextTokens: r.contextWindowForModel(model),
		CreatedAt:        time.Now().UTC(),
	})
}

func (r *llmRequestCaptureRecorder) storeSnapshot(ctx context.Context, snapshot agent.LLMRequestSnapshot) {
	if snapshot.ID == "" {
		snapshot.ID = uuid.NewString()
	}
	snapshot.Model = strings.TrimSpace(firstNonEmptyString(snapshot.Model, r.defaultModel))
	if snapshot.Provider == "" || snapshot.Provider == "llmRequestCapturingProvider" {
		snapshot.Provider = r.providerName
	}
	if snapshot.InputTokens <= 0 {
		snapshot.InputTokens = r.countMessagesTokens(ctx, snapshot.Messages)
	}
	if snapshot.MaxContextTokens <= 0 {
		snapshot.MaxContextTokens = r.contextWindowForModel(snapshot.Model)
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}

	fingerprint := llmRequestFingerprint(snapshot.Messages, snapshot.Tools, snapshot.Model)
	if !r.reserveFingerprint(fingerprint, snapshot.ID) {
		return
	}

	payload, err := buildLLMRequestPayload(snapshot)
	if err != nil {
		r.releaseFingerprint(fingerprint, snapshot.ID)
		log.Error().Err(err).Str("session", r.cfg.SessionID).Msg("build_llm_request_payload")
		return
	}
	req := persist.LLMRequest{
		ID:                  snapshot.ID,
		SessionID:           r.cfg.SessionID,
		UserID:              cloneCollectorUserID(r.cfg.UserID),
		RunID:               r.cfg.RunID,
		MessageID:           r.cfg.MessageID,
		ParentUserMessageID: r.cfg.ParentUserMessageID,
		CallID:              r.cfg.CallID,
		ParentCallID:        r.cfg.ParentCallID,
		SpecialistID:        r.specialistID,
		Provider:            snapshot.Provider,
		Model:               snapshot.Model,
		InputTokens:         snapshot.InputTokens,
		MaxContextTokens:    snapshot.MaxContextTokens,
		Payload:             payload,
		Redacted:            true,
		CreatedAt:           snapshot.CreatedAt,
	}
	if err := r.cfg.Store.AppendLLMRequest(context.Background(), req); err != nil {
		r.releaseFingerprint(fingerprint, snapshot.ID)
		log.Error().Err(err).Str("session", r.cfg.SessionID).Str("request_id", snapshot.ID).Msg("store_llm_request")
	}
}

func (r *llmRequestCaptureRecorder) reserveFingerprint(fingerprint string, requestID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.captured == nil {
		r.captured = map[string]string{}
	}
	if _, ok := r.captured[fingerprint]; ok {
		return false
	}
	r.captured[fingerprint] = requestID
	return true
}

func (r *llmRequestCaptureRecorder) releaseFingerprint(fingerprint string, requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.captured[fingerprint] == requestID {
		delete(r.captured, fingerprint)
	}
}

func (r *llmRequestCaptureRecorder) countMessagesTokens(ctx context.Context, msgs []llm.Message) int {
	if r.tokenizer != nil {
		if n, err := r.tokenizer.CountMessagesTokens(ctx, msgs); err == nil {
			return n
		}
	}
	if tokenizable, ok := r.provider.(llm.TokenizableProvider); ok {
		if tok := tokenizable.Tokenizer(); tok != nil {
			if n, err := tok.CountMessagesTokens(ctx, msgs); err == nil {
				return n
			}
		}
	}
	return llm.EstimateTokensForMessages(msgs)
}

func (r *llmRequestCaptureRecorder) contextWindowForModel(model string) int {
	if r.contextWindowTokens > 0 {
		return r.contextWindowTokens
	}
	if size, _ := llm.ContextSize(model); size > 0 {
		return size
	}
	if size, _ := llm.ContextSize(r.defaultModel); size > 0 {
		return size
	}
	return 0
}

func llmProviderName(provider llm.Provider) string {
	if provider == nil {
		return ""
	}
	t := reflect.TypeOf(provider)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() != "" {
		return t.Name()
	}
	return t.String()
}

func llmRequestFingerprint(messages []llm.Message, tools []llm.ToolSchema, model string) string {
	b, err := json.Marshal(struct {
		Model    string           `json:"model"`
		Messages []llm.Message    `json:"messages"`
		Tools    []llm.ToolSchema `json:"tools"`
	}{
		Model:    strings.TrimSpace(model),
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return uuid.NewString()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func cloneLLMMessagesForCapture(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, msg := range msgs {
		out[i] = msg
		if len(msg.ToolCalls) > 0 {
			out[i].ToolCalls = append([]llm.ToolCall(nil), msg.ToolCalls...)
			for j := range out[i].ToolCalls {
				if msg.ToolCalls[j].Args != nil {
					out[i].ToolCalls[j].Args = append([]byte(nil), msg.ToolCalls[j].Args...)
				}
			}
		}
		if len(msg.Images) > 0 {
			out[i].Images = append([]llm.GeneratedImage(nil), msg.Images...)
			for j := range out[i].Images {
				if msg.Images[j].Data != nil {
					out[i].Images[j].Data = append([]byte(nil), msg.Images[j].Data...)
				}
			}
		}
		if len(msg.Videos) > 0 {
			out[i].Videos = append([]llm.GeneratedVideo(nil), msg.Videos...)
			for j := range out[i].Videos {
				if msg.Videos[j].Data != nil {
					out[i].Videos[j].Data = append([]byte(nil), msg.Videos[j].Data...)
				}
			}
		}
		if msg.Compaction != nil {
			compaction := *msg.Compaction
			out[i].Compaction = &compaction
		}
	}
	return out
}

func cloneToolSchemasForCapture(schemas []llm.ToolSchema) []llm.ToolSchema {
	out := make([]llm.ToolSchema, len(schemas))
	for i, schema := range schemas {
		out[i] = schema
		if schema.Parameters != nil {
			out[i].Parameters = cloneCaptureMap(schema.Parameters)
		}
	}
	return out
}

func cloneCaptureMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneCaptureValue(v)
	}
	return out
}

func cloneCaptureValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneCaptureMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneCaptureValue(item)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case json.RawMessage:
		return append(json.RawMessage(nil), v...)
	case []byte:
		return append([]byte(nil), v...)
	default:
		return v
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
