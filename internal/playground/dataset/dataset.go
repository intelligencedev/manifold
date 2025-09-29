package dataset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Dataset groups rows under a named collection.
type Dataset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	CreatedAt   time.Time         `json:"createdAt"`
	Metadata    map[string]string `json:"metadata"`
}

// Snapshot captures a frozen view of a dataset's rows.
type Snapshot struct {
	ID        string    `json:"id"`
	DatasetID string    `json:"datasetId"`
	CreatedAt time.Time `json:"createdAt"`
	Filter    string    `json:"filter"`
}

// Row is a generic representation of model inputs and expected outputs.
type Row struct {
	ID       string         `json:"id"`
	Inputs   map[string]any `json:"inputs"`
	Expected any            `json:"expected"`
	Meta     map[string]any `json:"meta"`
	Split    string         `json:"split"`
}

// Store defines how datasets and rows are persisted.
type Store interface {
	CreateDataset(ctx context.Context, ds Dataset) (Dataset, error)
	UpdateDataset(ctx context.Context, ds Dataset) (Dataset, error)
	GetDataset(ctx context.Context, id string) (Dataset, bool, error)
	ListDatasets(ctx context.Context) ([]Dataset, error)
	CreateSnapshot(ctx context.Context, snapshot Snapshot, rows []Row) (Snapshot, error)
	ListSnapshotRows(ctx context.Context, datasetID, snapshotID string) ([]Row, error)
	DeleteDataset(ctx context.Context, id string) error
}

// Service offers convenience helpers on top of the persistence store.
type Service struct {
	store Store
	clock Clock
}

// Clock makes dataset services testable.
type Clock interface {
	Now() time.Time
}

// SystemClock implements Clock using time.Now().
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// NewService constructs a dataset service.
func NewService(store Store) *Service {
	return &Service{store: store, clock: SystemClock{}}
}

// WithClock sets a custom clock for tests and determinism.
func (s *Service) WithClock(clock Clock) *Service {
	s.clock = clock
	return s
}

// DeleteDataset removes a dataset and associated rows/snapshots.
func (s *Service) DeleteDataset(ctx context.Context, id string) error {
	return s.store.DeleteDataset(ctx, id)
}

var (
	// ErrDatasetExists indicates an attempt to re-use a dataset ID.
	ErrDatasetExists = errors.New("playground/dataset: dataset already exists")
	// ErrDatasetNotFound indicates a dataset cannot be found.
	ErrDatasetNotFound = errors.New("playground/dataset: dataset not found")
)

// CreateDataset registers a dataset with optional initial rows via snapshot.
func (s *Service) CreateDataset(ctx context.Context, ds Dataset, rows []Row) (Dataset, error) {
	ds.CreatedAt = s.clock.Now()
	created, err := s.store.CreateDataset(ctx, ds)
	if err != nil {
		return Dataset{}, err
	}
	if len(rows) > 0 {
		initialSnapshot := Snapshot{
			ID:        fmt.Sprintf("%s-initial", ds.ID),
			DatasetID: ds.ID,
			CreatedAt: s.clock.Now(),
			Filter:    "",
		}
		if _, err := s.store.CreateSnapshot(ctx, initialSnapshot, rows); err != nil {
			return Dataset{}, fmt.Errorf("create initial snapshot: %w", err)
		}
	}
	return created, nil
}

// UpdateDataset merges new metadata and optionally replaces the initial snapshot rows.
func (s *Service) UpdateDataset(ctx context.Context, ds Dataset, rows []Row) (Dataset, error) {
	if strings.TrimSpace(ds.ID) == "" {
		return Dataset{}, fmt.Errorf("dataset ID is required")
	}
	if strings.TrimSpace(ds.Name) == "" {
		return Dataset{}, fmt.Errorf("dataset name is required")
	}
	existing, ok, err := s.store.GetDataset(ctx, ds.ID)
	if err != nil {
		return Dataset{}, err
	}
	if !ok {
		return Dataset{}, ErrDatasetNotFound
	}
	updated := existing
	updated.Name = ds.Name
	updated.Description = ds.Description
	if ds.Tags != nil {
		updated.Tags = append([]string(nil), ds.Tags...)
	}
	if ds.Metadata != nil {
		updated.Metadata = ds.Metadata
	}
	saved, err := s.store.UpdateDataset(ctx, updated)
	if err != nil {
		return Dataset{}, err
	}
	if rows != nil {
		snapshot := Snapshot{
			ID:        fmt.Sprintf("%s-initial", ds.ID),
			DatasetID: ds.ID,
			CreatedAt: s.clock.Now(),
		}
		if _, err := s.store.CreateSnapshot(ctx, snapshot, rows); err != nil {
			return Dataset{}, fmt.Errorf("update snapshot: %w", err)
		}
	}
	return saved, nil
}

// ListDatasets retrieves all dataset metadata from the store.
func (s *Service) ListDatasets(ctx context.Context) ([]Dataset, error) {
	return s.store.ListDatasets(ctx)
}

// GetDataset fetches a dataset by ID.
func (s *Service) GetDataset(ctx context.Context, id string) (Dataset, bool, error) {
	return s.store.GetDataset(ctx, id)
}

// ResolveSnapshotRows fetches rows from either a snapshot or the latest snapshot
// when snapshotID is empty. Applies a simple split filter when sliceExpr matches a split.
func (s *Service) ResolveSnapshotRows(ctx context.Context, datasetID, snapshotID, sliceExpr string) ([]Row, error) {
	snapshot := snapshotID
	if snapshot == "" {
		snapshot = fmt.Sprintf("%s-initial", datasetID)
	}
	rows, err := s.store.ListSnapshotRows(ctx, datasetID, snapshot)
	if err != nil {
		return nil, err
	}
	if sliceExpr == "" {
		return rows, nil
	}
	var filtered []Row
	for _, row := range rows {
		if matchSlice(row.Split, sliceExpr) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func matchSlice(split, expr string) bool {
	switch strings.ToLower(expr) {
	case "train", "validation", "test":
		return strings.EqualFold(split, expr)
	case "eval":
		return strings.EqualFold(split, "validation") || strings.EqualFold(split, "test")
	default:
		return true
	}
}

// NewInMemoryStore offers a basic in-memory dataset store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		datasets: make(map[string]Dataset),
		rows:     make(map[string][]Row),
	}
}

// InMemoryStore implements Store backed by maps.
type InMemoryStore struct {
	datasets map[string]Dataset
	rows     map[string][]Row
}

// CreateDataset adds the dataset if the ID is unused.
func (s *InMemoryStore) CreateDataset(_ context.Context, ds Dataset) (Dataset, error) {
	if _, ok := s.datasets[ds.ID]; ok {
		return Dataset{}, ErrDatasetExists
	}
	s.datasets[ds.ID] = ds
	return ds, nil
}

// UpdateDataset updates metadata for an existing dataset.
func (s *InMemoryStore) UpdateDataset(_ context.Context, ds Dataset) (Dataset, error) {
	if _, ok := s.datasets[ds.ID]; !ok {
		return Dataset{}, ErrDatasetNotFound
	}
	s.datasets[ds.ID] = ds
	return ds, nil
}

// GetDataset retrieves a dataset.
func (s *InMemoryStore) GetDataset(_ context.Context, id string) (Dataset, bool, error) {
	ds, ok := s.datasets[id]
	return ds, ok, nil
}

// ListDatasets returns all dataset metadata sorted by creation time descending.
func (s *InMemoryStore) ListDatasets(_ context.Context) ([]Dataset, error) {
	items := make([]Dataset, 0, len(s.datasets))
	for _, ds := range s.datasets {
		items = append(items, ds)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// CreateSnapshot replaces the rows for the snapshot identifier.
func (s *InMemoryStore) CreateSnapshot(_ context.Context, snapshot Snapshot, rows []Row) (Snapshot, error) {
	key := snapshotKey(snapshot.DatasetID, snapshot.ID)
	copyRows := append([]Row(nil), rows...)
	s.rows[key] = copyRows
	return snapshot, nil
}

// ListSnapshotRows returns rows sorted by ID.
func (s *InMemoryStore) ListSnapshotRows(_ context.Context, datasetID, snapshotID string) ([]Row, error) {
	key := snapshotKey(datasetID, snapshotID)
	rows := append([]Row(nil), s.rows[key]...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// DeleteDataset removes dataset and its rows.
func (s *InMemoryStore) DeleteDataset(_ context.Context, id string) error {
	delete(s.datasets, id)
	// remove snapshots/rows for initial snapshot key if present
	for k := range s.rows {
		if strings.HasPrefix(k, id+":") {
			delete(s.rows, k)
		}
	}
	return nil
}

func snapshotKey(datasetID, snapshotID string) string {
	return datasetID + ":" + snapshotID
}
