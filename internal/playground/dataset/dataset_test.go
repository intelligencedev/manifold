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

func TestUpdateDatasetReplacesRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewInMemoryStore()
	svc := NewService(store)
	initialRows := []Row{{ID: "1", Inputs: map[string]any{"input": "foo"}, Split: "train"}}
	created, err := svc.CreateDataset(ctx, Dataset{ID: "ds", Name: "demo", Tags: []string{"alpha"}}, initialRows)
	require.NoError(t, err)
	require.NotEmpty(t, created.CreatedAt)

	updatedRows := []Row{{ID: "2", Inputs: map[string]any{"input": "bar"}, Split: "test"}}
	updated, err := svc.UpdateDataset(ctx, Dataset{ID: "ds", Name: "demo-updated", Description: "new", Tags: []string{"beta"}}, updatedRows)
	require.NoError(t, err)
	require.Equal(t, "demo-updated", updated.Name)
	require.Equal(t, "new", updated.Description)
	require.Equal(t, []string{"beta"}, updated.Tags)
	require.Equal(t, created.CreatedAt, updated.CreatedAt)

	snapshotRows, err := svc.ResolveSnapshotRows(ctx, "ds", "", "")
	require.NoError(t, err)
	require.Len(t, snapshotRows, 1)
	require.Equal(t, "2", snapshotRows[0].ID)
	require.Equal(t, "test", snapshotRows[0].Split)
}

func TestUpdateDatasetMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewInMemoryStore()
	svc := NewService(store)

	_, err := svc.UpdateDataset(ctx, Dataset{ID: "missing", Name: "ghost"}, nil)
	require.ErrorIs(t, err, ErrDatasetNotFound)
}
