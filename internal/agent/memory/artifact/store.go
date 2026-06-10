package artifact

import "context"

// Store persists immutable captured artifacts.
type Store interface {
	Init(ctx context.Context) error
	UpsertArtifact(ctx context.Context, artifact Artifact) (Artifact, error)
	GetArtifact(ctx context.Context, tenantID int64, id string) (Artifact, bool, error)
	FindByExternalID(ctx context.Context, tenantID int64, kind ArtifactKind, externalID string) ([]Artifact, error)
	SearchArtifacts(ctx context.Context, query SearchQuery) ([]SearchResult, error)
}

// NoopStore is a disabled artifact store implementation.
type NoopStore struct{}

func (NoopStore) Init(context.Context) error { return nil }
func (NoopStore) UpsertArtifact(_ context.Context, artifact Artifact) (Artifact, error) {
	return artifact, nil
}
func (NoopStore) GetArtifact(context.Context, int64, string) (Artifact, bool, error) {
	return Artifact{}, false, nil
}
func (NoopStore) FindByExternalID(context.Context, int64, ArtifactKind, string) ([]Artifact, error) {
	return nil, nil
}
func (NoopStore) SearchArtifacts(context.Context, SearchQuery) ([]SearchResult, error) {
	return nil, nil
}
