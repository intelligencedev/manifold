package embeddedpg

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoctorReportsMissingRuntimeAsset(t *testing.T) {
	t.Parallel()

	result := Doctor(context.Background(), DoctorRequest{})
	require.False(t, result.OK)
	require.Equal(t, runtime.GOOS, result.OS)
	require.NotEmpty(t, result.Arch)
	require.Contains(t, result.Error, "no embedded PostgreSQL runtime is bundled")
}
