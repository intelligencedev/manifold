package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunStorageCommandJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dbPath := filepath.Join(t.TempDir(), "doctor.db")
	handled, code := runStorageCommand([]string{"storage", "doctor", "--json", "--path", dbPath}, &stdout, &stderr)
	require.True(t, handled)
	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), `"ok": true`)
	require.Contains(t, stdout.String(), `"fts5": true`)
	require.Contains(t, stdout.String(), `"vec1Info": true`)
	require.Contains(t, stdout.String(), `"tempVectorQuery": true`)
	require.Empty(t, stderr.String())
}

func TestRunStorageCommandIgnoresOtherArgs(t *testing.T) {
	t.Parallel()

	handled, code := runStorageCommand([]string{"serve"}, &bytes.Buffer{}, &bytes.Buffer{})
	require.False(t, handled)
	require.Zero(t, code)
}
