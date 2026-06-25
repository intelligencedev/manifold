package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestMCPServerHasProjectPlaceholderChecksWorkdirAndToolDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.MCPServerConfig
	}{
		{
			name: "workdir",
			cfg:  config.MCPServerConfig{Workdir: "{{PROJECT_DIR}}"},
		},
		{
			name: "tool default string",
			cfg: config.MCPServerConfig{ToolDefaults: map[string]map[string]any{
				"take_screenshot": {"filePath": "{{PROJECT_DIR}}/.manifold/screenshot.png"},
			}},
		},
		{
			name: "nested tool default",
			cfg: config.MCPServerConfig{ToolDefaults: map[string]map[string]any{
				"example": {"payload": map[string]any{"path": "{{PROJECT_DIR}}/out"}},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if !mcpServerHasProjectPlaceholder(test.cfg) {
				t.Fatalf("expected project placeholder in %+v", test.cfg)
			}
		})
	}
}
