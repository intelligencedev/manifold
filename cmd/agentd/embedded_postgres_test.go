package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunEmbeddedPostgresCommandJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, code := runEmbeddedPostgresCommand([]string{"embedded-postgres", "doctor", "--json"}, &stdout, &stderr)
	require.True(t, handled)
	require.Equal(t, 1, code)
	require.Contains(t, stdout.String(), `"ok": false`)
	require.Contains(t, stdout.String(), `"error":`)
	require.Empty(t, stderr.String())
}

func TestRunEmbeddedPostgresCommandIgnoresOtherArgs(t *testing.T) {
	t.Parallel()

	handled, code := runEmbeddedPostgresCommand([]string{"serve"}, &bytes.Buffer{}, &bytes.Buffer{})
	require.False(t, handled)
	require.Zero(t, code)
}
