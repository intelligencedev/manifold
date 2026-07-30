package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	"manifold/internal/durable"
	"manifold/internal/fleet"
	"manifold/internal/llm"
)

type durableChatEventWriter struct {
	ctx       context.Context
	taskID    string
	sessionID string
	mu        sync.Mutex
	nextKey   int64
	prior     map[string]struct{}
}

func newDurableChatEventWriter(ctx context.Context, taskID, sessionID string) *durableChatEventWriter {
	writer := &durableChatEventWriter{ctx: ctx, taskID: strings.TrimSpace(taskID), sessionID: strings.TrimSpace(sessionID), prior: map[string]struct{}{}}
	if tc, ok := durable.FromContext(ctx); ok && tc.Store != nil {
		events, _, _, err := tc.Store.ListTaskEvents(ctx, tc.Task.UserID, tc.Task.ID, 0)
		if err == nil {
			for _, event := range events {
				if event.Sequence > writer.nextKey {
					writer.nextKey = event.Sequence
				}
				if eventPayload, ok := durableChatEventPayload(event); ok && eventPayload["type"] != "delta" {
					writer.prior[durableChatEventFingerprint(eventPayload)] = struct{}{}
				}
			}
		}
	}
	return writer
}

func (w *durableChatEventWriter) write(payload any) {
	if w == nil {
		return
	}
	eventPayload := durableChatPayloadMap(durableChatJSONSafe(payload))
	if strings.TrimSpace(w.sessionID) != "" {
		eventPayload["session_id"] = w.sessionID
	}
	if strings.TrimSpace(w.taskID) != "" {
		eventPayload["run_id"] = w.taskID
	}
	fingerprint := durableChatEventFingerprint(eventPayload)
	if typ, _ := eventPayload["type"].(string); typ != "delta" {
		w.mu.Lock()
		if _, ok := w.prior[fingerprint]; ok {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	w.nextKey++
	eventKey := fmt.Sprintf("event:%012d", w.nextKey)
	w.mu.Unlock()
	name := "chat.event"
	if typ, _ := eventPayload["type"].(string); strings.TrimSpace(typ) != "" {
		name = "chat." + strings.TrimSpace(typ)
	}
	if _, err := durable.RecordEventOnce(w.ctx, eventKey, name, map[string]any{"event": durableChatJSONSafe(eventPayload)}); err != nil {
		log.Warn().Err(err).Str("task_id", w.taskID).Str("event", name).Msg("durable_chat_record_event_failed")
	}
}

func (w *durableChatEventWriter) writeText(string) {}

func (w *durableChatEventWriter) Write(payload any) { w.write(payload) }

func (w *durableChatEventWriter) WriteText(text string) { w.writeText(text) }

type durableAgentTracer struct {
	writer  *durableChatEventWriter
	onTrace func(agent.AgentTrace)
}

func (t durableAgentTracer) Trace(ev agent.AgentTrace) {
	if t.onTrace != nil {
		t.onTrace(ev)
	}
	if t.writer == nil {
		return
	}
	t.writer.write(map[string]any{
		"type": ev.Type, "agent": ev.Agent, "team": ev.Team, "model": ev.Model,
		"call_id": ev.CallID, "parent_call_id": ev.ParentCallID, "depth": ev.Depth,
		"role": ev.Role, "content": ev.Content, "title": ev.Title,
		"tool_name": ev.ToolName, "tool_title": ev.ToolTitle, "args": ev.Args,
		"data": ev.Data, "tool_id": ev.ToolID, "error": ev.Error,
		"thought_summary": ev.ThoughtSummary,
	})
}

type durableInputRequester struct {
	writer  *durableChatEventWriter
	session string
	runID   string
	userID  *int64
	bus     *fleet.Bus
}

func (r durableInputRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	if strings.TrimSpace(req.ToolID) != "" {
		req.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.runID+":input:"+strings.TrimSpace(req.ToolID))).String()
	}
	if strings.TrimSpace(req.ID) == "" {
		return inputrequest.Response{}, errors.New("input request id is required")
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	if r.writer != nil {
		r.writer.write(inputRequestEventPayload(req, r.session, r.runID))
	}
	if r.bus != nil {
		r.bus.Publish(fleet.Event{Kind: fleet.EventInputRequest, RunID: r.runID, SessionID: r.session, CallID: req.CallID, ParentCallID: req.ParentCallID, ToolID: req.ToolID, Agent: req.Agent, Depth: req.Depth, UserID: derefInputUserID(r.userID), Data: map[string]any{"request_id": req.ID, "question": req.Question, "reason": req.Reason}})
	}
	resp, err := durable.AwaitEvent[inputrequest.Response](ctx, durableChatInputAnswerEvent(req.ID), 0)
	if err != nil {
		return inputrequest.Response{}, err
	}
	if strings.TrimSpace(resp.RequestID) == "" {
		resp.RequestID = req.ID
	}
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = time.Now().UTC()
	}
	return resp, nil
}

func durableChatInputAnswerEvent(requestID string) string {
	return "chat.input_answer." + strings.TrimSpace(requestID)
}

func durableChatStreamPayload(runID string, event durable.Event) map[string]any {
	payload, ok := durableChatEventPayload(event)
	if !ok {
		payload = durableChatPayloadMap(event.Payload)
	}
	payload["sequence"] = event.Sequence
	payload["run_id"] = runID
	return payload
}

func durableChatEventPayload(event durable.Event) (map[string]any, bool) {
	raw, ok := event.Payload["event"]
	if !ok {
		return nil, false
	}
	return durableChatPayloadMap(raw), true
}

func durableChatPayloadMap(payload any) map[string]any {
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"type": "event"}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"type": "event", "data": string(raw)}
	}
	if out == nil {
		out = map[string]any{"type": "event"}
	}
	return out
}

func durableChatJSONSafe(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return durableChatSafeRawMessage(v)
	case []json.RawMessage:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatSafeRawMessage(item))
		}
		return out
	case []llm.Message:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	case llm.Message:
		return map[string]any{"role": v.Role, "content": v.Content, "tool_id": v.ToolID, "tool_calls": durableChatJSONSafe(v.ToolCalls), "images": v.Images, "compaction": v.Compaction, "thought_signature": v.ThoughtSignature}
	case []llm.ToolCall:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	case llm.ToolCall:
		return map[string]any{"name": v.Name, "args": durableChatSafeRawMessage(v.Args), "id": v.ID, "thought_signature": v.ThoughtSignature}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = durableChatJSONSafe(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	default:
		return value
	}
}

func durableCheckpointJSONSafe(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return durableChatSafeRawMessage(v)
	case []json.RawMessage:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatSafeRawMessage(item))
		}
		return out
	case []llm.Message:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	case llm.Message:
		return map[string]any{"Role": v.Role, "Content": v.Content, "ToolID": v.ToolID, "ToolCalls": durableCheckpointJSONSafe(v.ToolCalls), "Images": v.Images, "Compaction": v.Compaction, "ThoughtSignature": v.ThoughtSignature}
	case []llm.ToolCall:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	case llm.ToolCall:
		return map[string]any{"Name": v.Name, "Args": durableChatSafeRawMessage(v.Args), "ID": v.ID, "ThoughtSignature": v.ThoughtSignature}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = durableCheckpointJSONSafe(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	default:
		return value
	}
}

func durableChatSafeRawMessage(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return map[string]any{}
	}
	if json.Valid([]byte(trimmed)) {
		var out any
		if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
			return out
		}
	}
	return trimmed
}

func durableChatEventFingerprint(payload map[string]any) string {
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		if key != "sequence" {
			clone[key] = value
		}
	}
	if typ, _ := clone["type"].(string); typ == "input_request" {
		delete(clone, "created_at")
	}
	raw, _ := json.Marshal(clone)
	return string(raw)
}
