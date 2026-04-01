package discovery

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"manifold/internal/observability"
	"manifold/internal/tools"
)

type promoter interface {
	Promote(names []string) []string
	IsActive(name string) bool
}

type searchTool struct {
	index    *ToolIndex
	registry promoter
}

type searchToolInput struct {
	Query string   `json:"query"`
	Names []string `json:"names"`
}

type searchToolResult struct {
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	ParametersSummary string  `json:"parameters_summary,omitempty"`
	Score             float64 `json:"score,omitempty"`
	Loaded            bool    `json:"loaded"`
}

func NewSearchTool(index *ToolIndex, registry promoter) tools.Tool {
	return &searchTool{index: index, registry: registry}
}

func (t *searchTool) Name() string { return "tool_search" }

func (t *searchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Search for tools by capability description or exact name. Matching tools become available for later steps in the same run.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural-language description of the capability you need, such as 'read files', 'fetch a web page', or 'call another agent'.",
				},
				"names": map[string]any{
					"type":        "array",
					"description": "Optional exact tool names to load directly if you already know them.",
					"items":       map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (t *searchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var input searchToolInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
	}
	resultsByName := map[string]ToolSearchResult{}
	if query := strings.TrimSpace(input.Query); query != "" {
		matches := t.index.Search(query, 10)
		for _, result := range matches {
			resultsByName[result.Name] = result
		}
		observability.LoggerWithTrace(ctx).Info().Str("query", query).Int("matches", len(matches)).Msg("tool_search_query")
	}
	for _, name := range input.Names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if result, ok := t.index.Lookup(name); ok {
			resultsByName[name] = result
		}
	}
	names := make([]string, 0, len(resultsByName))
	for name := range resultsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	t.registry.Promote(names)
	out := make([]searchToolResult, 0, len(names))
	for _, name := range names {
		result := resultsByName[name]
		out = append(out, searchToolResult{
			Name:              result.Name,
			Description:       result.Description,
			ParametersSummary: result.ParametersSummary,
			Score:             result.Score,
			Loaded:            t.registry.IsActive(name),
		})
	}
	return out, nil
}
