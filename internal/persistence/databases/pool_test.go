package databases

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenPool_InvalidDSN(t *testing.T) {
	t.Parallel()

	_, err := OpenPool(context.Background(), "postgres://user:pass@localhost:99999/db")

	require.Error(t, err)
}
