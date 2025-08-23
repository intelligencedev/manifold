package db

import (
	"context"
	"encoding/json"
	"testing"

	"singularityio/internal/config"
	"singularityio/internal/persistence/databases"
)

// stubPG simulates a postgres-backed vector store with fixed dimension.
type stubPG struct {
	wantDim int
	lastID  string
	lastVec []float32
}

func (s *stubPG) Upsert(_ context.Context, id string, vector []float32, metadata map[string]string) error {
	s.lastID = id
	// copy vector
	s.lastVec = make([]float32, len(vector))
	copy(s.lastVec, vector)
	return nil
}
func (s *stubPG) Delete(_ context.Context, id string) error { return nil }
func (s *stubPG) SimilaritySearch(_ context.Context, vector []float32, k int, filter map[string]string) ([]databases.VectorResult, error) {
	return nil, nil
}
func (s *stubPG) Dimension() int { return s.wantDim }

func TestVectorUpsert_HappyPath(t *testing.T) {
	v := databases.NewMemoryVector()
	up := NewVectorUpsertTool(v, config.EmbeddingConfig{}) // won't use emb config here
	ctx := context.Background()
	resp, err := up.Call(ctx, json.RawMessage(`{"id":"a","vector":[1,0],"metadata":{"k":"v"}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m := resp.(map[string]any)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok true, got %v", m)
	}
}

func TestVectorUpsert_DimensionMismatch(t *testing.T) {
	stub := &stubPG{wantDim: 3}
	up := NewVectorUpsertTool(stub, config.EmbeddingConfig{})
	ctx := context.Background()
	resp, err := up.Call(ctx, json.RawMessage(`{"id":"a","vector":[1,0],"metadata":{"k":"v"}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	m := resp.(map[string]any)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("expected ok false due to dimension mismatch, got %v", m)
	}
}
