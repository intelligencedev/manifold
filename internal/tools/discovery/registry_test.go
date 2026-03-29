package discovery

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/tools"
)

type fakeTool struct {
	name        string
	description string
	called      bool
}

func (t *fakeTool) Name() string { return t.name }

func (t *fakeTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.description,
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *fakeTool) Call(context.Context, json.RawMessage) (any, error) {
	t.called = true
	return map[string]any{"tool": t.name}, nil
}

func TestDiscoverableRegistryExposesBootstrapAndSearch(t *testing.T) {
	t.Parallel()

	base := tools.NewRegistry()
	base.Register(&fakeTool{name: "file_read", description: "Read files"})
	base.Register(&fakeTool{name: "web_fetch", description: "Fetch web pages"})
	reg := NewDiscoverableRegistry(base, NewToolIndex(base.Schemas()), []string{"file_read"}, 5)

	schemas := tools.SchemaNames(reg)
	require.ElementsMatch(t, []string{"file_read", "tool_search"}, schemas)
}

func TestDiscoverableRegistryPromotesViaSearch(t *testing.T) {
	t.Parallel()

	base := tools.NewRegistry()
	webTool := &fakeTool{name: "web_fetch", description: "Fetch web pages"}
	base.Register(&fakeTool{name: "file_read", description: "Read files"})
	base.Register(webTool)
	reg := NewDiscoverableRegistry(base, NewToolIndex(base.Schemas()), []string{"file_read"}, 5)

	_, err := reg.Dispatch(context.Background(), "tool_search", json.RawMessage(`{"query":"fetch web pages"}`))
	require.NoError(t, err)
	require.Contains(t, tools.SchemaNames(reg), "web_fetch")

	_, err = reg.Dispatch(context.Background(), "web_fetch", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, webTool.called)
}

func TestDiscoverableRegistryRespectsCap(t *testing.T) {
	t.Parallel()

	base := tools.NewRegistry()
	base.Register(&fakeTool{name: "file_read", description: "Read files"})
	base.Register(&fakeTool{name: "web_fetch", description: "Fetch web pages"})
	base.Register(&fakeTool{name: "apply_patch", description: "Patch files"})
	reg := NewDiscoverableRegistry(base, NewToolIndex(base.Schemas()), []string{"file_read"}, 1)

	promoted := reg.Promote([]string{"web_fetch", "apply_patch"})
	require.Len(t, promoted, 1)
}
