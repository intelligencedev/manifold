package agentd

import (
	"context"
	"encoding/json"

	"manifold/internal/durable"
	"manifold/internal/llm"
)

type durableRunCheckpointer struct{}

func (durableRunCheckpointer) Load(ctx context.Context, key string, target any) (bool, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok || tc.Store == nil {
		return false, nil
	}
	raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key)
	if err != nil || !found {
		return found, err
	}
	if len(raw) == 0 {
		return true, nil
	}
	return true, unmarshalDurableCheckpoint(raw, target)
}

func (durableRunCheckpointer) Save(ctx context.Context, key string, value any) error {
	tc, ok := durable.FromContext(ctx)
	if !ok || tc.Store == nil {
		return nil
	}
	raw, err := json.Marshal(durableCheckpointJSONSafe(value))
	if err != nil {
		return err
	}
	_, err = tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, raw)
	return err
}

type durableCheckpointMessageCompat struct {
	Role             string                        `json:"role"`
	Content          string                        `json:"content"`
	ToolID           string                        `json:"tool_id"`
	ToolCalls        []durableCheckpointToolCompat `json:"tool_calls"`
	Images           []llm.GeneratedImage          `json:"images"`
	Compaction       *llm.CompactionItem           `json:"compaction"`
	ThoughtSignature string                        `json:"thought_signature"`
}

type durableCheckpointToolCompat struct {
	Name             string          `json:"name"`
	Args             json.RawMessage `json:"args"`
	ID               string          `json:"id"`
	ThoughtSignature string          `json:"thought_signature"`
}

func unmarshalDurableCheckpoint(raw []byte, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	switch out := target.(type) {
	case *llm.Message:
		applyDurableCheckpointCompatMessage(raw, out)
	case *[]llm.Message:
		applyDurableCheckpointCompatMessages(raw, out)
	}
	return nil
}

func applyDurableCheckpointCompatMessages(raw []byte, target *[]llm.Message) {
	if target == nil {
		return
	}
	var compat []durableCheckpointMessageCompat
	if err := json.Unmarshal(raw, &compat); err != nil || len(compat) != len(*target) {
		return
	}
	for i := range compat {
		applyDurableCheckpointCompat(&(*target)[i], compat[i])
	}
}

func applyDurableCheckpointCompatMessage(raw []byte, target *llm.Message) {
	if target == nil {
		return
	}
	var compat durableCheckpointMessageCompat
	if err := json.Unmarshal(raw, &compat); err != nil {
		return
	}
	applyDurableCheckpointCompat(target, compat)
}

func applyDurableCheckpointCompat(target *llm.Message, compat durableCheckpointMessageCompat) {
	if target.Role == "" {
		target.Role = compat.Role
	}
	if target.Content == "" {
		target.Content = compat.Content
	}
	if target.ToolID == "" {
		target.ToolID = compat.ToolID
	}
	if len(target.ToolCalls) == 0 && len(compat.ToolCalls) > 0 {
		target.ToolCalls = make([]llm.ToolCall, 0, len(compat.ToolCalls))
		for _, call := range compat.ToolCalls {
			target.ToolCalls = append(target.ToolCalls, llm.ToolCall{
				Name:             call.Name,
				Args:             normalizeDurableCheckpointRawMessage(call.Args),
				ID:               call.ID,
				ThoughtSignature: call.ThoughtSignature,
			})
		}
	}
	if len(target.Images) == 0 && len(compat.Images) > 0 {
		target.Images = compat.Images
	}
	if target.Compaction == nil {
		target.Compaction = compat.Compaction
	}
	if target.ThoughtSignature == "" {
		target.ThoughtSignature = compat.ThoughtSignature
	}
}

func normalizeDurableCheckpointRawMessage(raw json.RawMessage) json.RawMessage {
	if json.Valid(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}
