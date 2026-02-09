package utility

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/tools"
)

const agentResponseToolName = "agent_response"

type agentResponseArgs struct {
	Text       string `json:"text"`
	RenderMode string `json:"render_mode"`
	OutputAttr string `json:"output_attr"`
	Label      string `json:"label"`
}

type agentResponseResponse struct {
	OK         bool   `json:"ok"`
	Text       string `json:"text"`
	RenderMode string `json:"render_mode"`
	OutputAttr string `json:"output_attr,omitempty"`
	Label      string `json:"label,omitempty"`
}

// AgentResponseTool renders a final text/markdown/html response in flow nodes.
type AgentResponseTool struct{}

// Name implements tools.Tool.
func (AgentResponseTool) Name() string { return agentResponseToolName }

// JSONSchema implements tools.Tool.
func (AgentResponseTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        agentResponseToolName,
		"description": "Render agent responses as text, markdown, or HTML.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"label": map[string]any{
					"type":        "string",
					"title":       "Label",
					"description": "Optional label for the response node.",
				},
				"text": map[string]any{
					"type":        "string",
					"title":       "Text",
					"description": "Content to render.",
					"default":     "",
				},
				"render_mode": map[string]any{
					"type":        "string",
					"title":       "Render Mode",
					"description": "Rendering mode: raw, markdown, or html.",
					"enum":        []string{"raw", "markdown", "html"},
					"default":     "markdown",
				},
				"output_attr": map[string]any{
					"type":        "string",
					"title":       "Output Attribute",
					"description": "Optional attribute key to expose rendered content downstream.",
				},
			},
		},
	}
}

// Call implements tools.Tool.
func (AgentResponseTool) Call(_ context.Context, raw json.RawMessage) (any, error) {
	var args agentResponseArgs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &args)
	}
	resp := agentResponseResponse{
		OK:         true,
		Text:       args.Text,
		RenderMode: normalizeRenderMode(args.RenderMode),
	}
	if args.OutputAttr != "" {
		resp.OutputAttr = args.OutputAttr
	}
	if args.Label != "" {
		resp.Label = args.Label
	}
	return resp, nil
}

func normalizeRenderMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "raw", "markdown", "html":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "markdown"
	}
}

// NewAgentResponseTool constructs the agent response utility tool.
func NewAgentResponseTool() tools.Tool {
	return AgentResponseTool{}
}
