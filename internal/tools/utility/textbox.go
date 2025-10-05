package utility

import (
	"context"
	"encoding/json"

	"manifold/internal/tools"
)

const textboxToolName = "utility_textbox"

// textboxArgs captures the configuration for the utility textbox tool.
type textboxArgs struct {
	Text       string `json:"text"`
	OutputAttr string `json:"output_attr"`
	Label      string `json:"label"`
}

// textboxResponse echoes the configured textbox payload for WARPP executions.
type textboxResponse struct {
	OK         bool   `json:"ok"`
	Text       string `json:"text"`
	OutputAttr string `json:"output_attr,omitempty"`
	Label      string `json:"label,omitempty"`
}

// TextboxTool provides a simple passthrough utility node for WARPP flows.
type TextboxTool struct{}

// Name implements tools.Tool.
func (TextboxTool) Name() string { return textboxToolName }

// JSONSchema implements tools.Tool.
func (TextboxTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        textboxToolName,
		"description": "Utility textbox node for WARPP workflows.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{
					"type":        "string",
					"title":       "Label",
					"description": "Optional label rendered above the textbox in the editor.",
				},
				"text": map[string]any{
					"type":        "string",
					"title":       "Text",
					"description": "Initial text rendered in the textbox. Supports ${A.key} substitutions.",
					"default":     "",
				},
				"output_attr": map[string]any{
					"type":        "string",
					"title":       "Output Attribute",
					"description": "Optional attribute key that will receive the textbox value for downstream steps.",
				},
			},
		},
	}
}

// Call implements tools.Tool.
func (TextboxTool) Call(_ context.Context, raw json.RawMessage) (any, error) {
	var args textboxArgs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &args)
	}
	resp := textboxResponse{OK: true, Text: args.Text}
	if args.OutputAttr != "" {
		resp.OutputAttr = args.OutputAttr
	}
	if args.Label != "" {
		resp.Label = args.Label
	}
	return resp, nil
}

// NewTextboxTool constructs the utility textbox tool.
func NewTextboxTool() tools.Tool {
	return TextboxTool{}
}
