package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderTemplateReplacesPlaceholders(t *testing.T) {
	t.Parallel()

	rendered, err := renderTemplate("Hello {{name}}", map[string]any{"name": "Ada"})
	require.NoError(t, err)
	require.Equal(t, "Hello Ada", rendered)
}

func TestRenderTemplateDetectsMissingPlaceholder(t *testing.T) {
	t.Parallel()

	rendered, err := renderTemplate("Hello {{name}} {{missing}}", map[string]any{"name": "Ada"})
	require.Error(t, err)
	require.Empty(t, rendered)
}
