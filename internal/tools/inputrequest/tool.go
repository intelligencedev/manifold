package inputrequest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agentinput "manifold/internal/agent/inputrequest"
)

// ToolName is the registry name exposed to agents.
const ToolName = "request_info"

// Tool lets an agent pause and ask its caller/user for information.
type Tool struct{}

// New constructs a request_info tool.
func New() *Tool {
	return &Tool{}
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Ask the current caller or user for information needed before continuing. This tool blocks until the request is answered. Use it only when you cannot proceed safely from the available context.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The concise question the caller/user must answer.",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Optional brief reason explaining why the information is needed.",
				},
				"choices": map[string]any{
					"type":        "array",
					"description": "Optional answer choices. Items may be strings or objects with id, label, and description.",
					"items": map[string]any{
						"oneOf": []map[string]any{
							{"type": "string"},
							{
								"type": "object",
								"properties": map[string]any{
									"id":          map[string]any{"type": "string"},
									"label":       map[string]any{"type": "string"},
									"description": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"allow_free_text": map[string]any{
					"type":        "boolean",
					"description": "Whether the caller/user may answer with free text. Defaults to true when no choices are provided, otherwise false.",
				},
				"multiple": map[string]any{
					"type":        "boolean",
					"description": "Whether more than one choice may be selected.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Optional request-specific timeout. The run timeout still applies.",
				},
			},
			"required": []string{"question"},
		},
	}
}

type args struct {
	Question      string          `json:"question"`
	Reason        string          `json:"reason"`
	Choices       json.RawMessage `json:"choices"`
	AllowFreeText *bool           `json:"allow_free_text"`
	Multiple      bool            `json:"multiple"`
	Timeout       int             `json:"timeout_seconds"`
}

type choiceArg struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var in args
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	question := strings.TrimSpace(in.Question)
	if question == "" {
		return map[string]any{"ok": false, "error": "question is required"}, nil
	}
	requester := agentinput.RequesterFromContext(ctx)
	if requester == nil {
		return map[string]any{"ok": false, "error": "request_info is unavailable in this run context"}, nil
	}

	choices, err := decodeChoices(in.Choices)
	if err != nil {
		return nil, err
	}
	allowFreeText := len(choices) == 0
	if in.AllowFreeText != nil {
		allowFreeText = *in.AllowFreeText
	}

	meta := agentinput.RunMetadataFromContext(ctx)
	req := agentinput.Request{
		ID:            uuid.NewString(),
		Question:      question,
		Reason:        strings.TrimSpace(in.Reason),
		Choices:       choices,
		AllowFreeText: allowFreeText,
		Multiple:      in.Multiple,
		Agent:         strings.TrimSpace(meta.Agent),
		Model:         strings.TrimSpace(meta.Model),
		CallID:        strings.TrimSpace(meta.CallID),
		ParentCallID:  strings.TrimSpace(meta.ParentCallID),
		Depth:         meta.Depth,
		CreatedAt:     time.Now().UTC(),
	}

	requestCtx := ctx
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(in.Timeout)*time.Second)
		defer cancel()
	}

	resp, err := requester.RequestInfo(requestCtx, req)
	if err != nil {
		return map[string]any{"ok": false, "request_id": req.ID, "error": err.Error()}, nil
	}
	return map[string]any{
		"ok":           true,
		"request_id":   resp.RequestID,
		"answer":       resp.Answer,
		"choice_ids":   resp.ChoiceIDs,
		"responded_at": resp.RespondedAt,
	}, nil
}

func decodeChoices(raw json.RawMessage) ([]agentinput.Choice, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var nodes []json.RawMessage
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, err
	}
	out := make([]agentinput.Choice, 0, len(nodes))
	for i, node := range nodes {
		var label string
		if err := json.Unmarshal(node, &label); err == nil {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			out = append(out, agentinput.Choice{ID: fmt.Sprintf("choice_%d", i+1), Label: label})
			continue
		}
		var choice choiceArg
		if err := json.Unmarshal(node, &choice); err != nil {
			return nil, err
		}
		label = strings.TrimSpace(choice.Label)
		if label == "" {
			label = strings.TrimSpace(choice.ID)
		}
		if label == "" {
			continue
		}
		id := strings.TrimSpace(choice.ID)
		if id == "" {
			id = fmt.Sprintf("choice_%d", i+1)
		}
		out = append(out, agentinput.Choice{ID: id, Label: label, Description: strings.TrimSpace(choice.Description)})
	}
	return out, nil
}
