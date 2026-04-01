package discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/llm"
)

func TestToolIndexSearchPrefersExactName(t *testing.T) {
	t.Parallel()

	idx := NewToolIndex([]llm.ToolSchema{
		{Name: "file_read", Description: "Read files from the workspace"},
		{Name: "web_fetch", Description: "Fetch a web page"},
	})

	results := idx.Search("file_read", 5)
	require.NotEmpty(t, results)
	require.Equal(t, "file_read", results[0].Name)
}

func TestToolIndexSearchUsesParameterDescriptions(t *testing.T) {
	t.Parallel()

	idx := NewToolIndex([]llm.ToolSchema{
		{
			Name:        "apply_patch",
			Description: "Update files using a patch",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string", "description": "Diff patch content to apply"},
				},
			},
		},
	})

	results := idx.Search("diff patch", 5)
	require.Len(t, results, 1)
	require.Equal(t, "apply_patch", results[0].Name)
}

func TestToolIndexLookup(t *testing.T) {
	t.Parallel()

	idx := NewToolIndex([]llm.ToolSchema{{Name: "web_fetch", Description: "Fetch content"}})
	result, ok := idx.Lookup("web_fetch")
	require.True(t, ok)
	require.Equal(t, "web_fetch", result.Name)
}
