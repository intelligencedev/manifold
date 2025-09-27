package dataset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSnapshotRowsSliceFilter(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	svc := NewService(store)
	rows := []Row{
		{ID: "1", Split: "train"},
		{ID: "2", Split: "validation"},
		{ID: "3", Split: "test"},
	}
	_, err := svc.CreateDataset(context.Background(), Dataset{ID: "ds", Name: "demo"}, rows)
	require.NoError(t, err)

	filtered, err := svc.ResolveSnapshotRows(context.Background(), "ds", "", "eval")
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	require.Equal(t, []string{"2", "3"}, []string{filtered[0].ID, filtered[1].ID})
}
