package transit_test

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/persistence/databases"
	transit "manifold/internal/transit"
)

type recordingTransitEmbedder struct {
	texts []string
}

type memoryTransitVector struct {
	inner databases.VectorStore
}

func (r *recordingTransitEmbedder) embed(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
	r.texts = append(r.texts, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (m memoryTransitVector) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	return m.inner.Upsert(ctx, id, vector, metadata)
}

func (m memoryTransitVector) Delete(ctx context.Context, id string) error {
	return m.inner.Delete(ctx, id)
}

func (m memoryTransitVector) SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]transit.VectorIndexResult, error) {
	results, err := m.inner.SimilaritySearch(ctx, vector, k, filter)
	if err != nil {
		return nil, err
	}
	out := make([]transit.VectorIndexResult, 0, len(results))
	for _, result := range results {
		out = append(out, transit.VectorIndexResult{ID: result.ID, Score: result.Score, Metadata: result.Metadata})
	}
	return out, nil
}

func TestServiceCRUDAndSearch(t *testing.T) {
	t.Parallel()
	store := databases.NewMemoryTransitStore()
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	service := transit.NewService(transit.ServiceConfig{
		Store:              store,
		Search:             nil,
		Vector:             nil,
		DefaultSearchLimit: 10,
		DefaultListLimit:   10,
		MaxBatchSize:       10,
		EnableVectorSearch: false,
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return [][]float32{{1, 0, 0}}, nil
		},
	})

	created, err := service.CreateMemory(context.Background(), 1, 1, []transit.CreateMemoryItem{{
		KeyName:     "project/demo/brief",
		Description: "Demo brief",
		Value:       "Transit stores durable shared project notes",
	}})
	if err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("CreateMemory() len = %d, want 1", len(created))
	}

	updated, err := service.UpdateMemory(context.Background(), 1, 1, transit.UpdateMemoryRequest{
		KeyName:   "project/demo/brief",
		Value:     "Transit stores durable shared project memory",
		IfVersion: created[0].Version,
	})
	if err != nil {
		t.Fatalf("UpdateMemory() error = %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("UpdateMemory() version = %d, want 2", updated.Version)
	}

	hits, err := service.SearchMemories(context.Background(), 1, transit.SearchRequest{Query: "durable shared", Limit: 5})
	if err != nil {
		t.Fatalf("SearchMemories() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("SearchMemories() len = %d, want 1", len(hits))
	}

	items, err := service.ListKeys(context.Background(), 1, transit.ListRequest{Prefix: "project/demo", Limit: 5})
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListKeys() len = %d, want 1", len(items))
	}

	if err := service.DeleteMemory(context.Background(), 1, []string{"project/demo/brief"}); err != nil {
		t.Fatalf("DeleteMemory() error = %v", err)
	}

	records, err := service.GetMemory(context.Background(), 1, []string{"project/demo/brief"})
	if err != nil {
		t.Fatalf("GetMemory() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("GetMemory() len = %d, want 0 after delete", len(records))
	}
}

func TestServiceNormalizesKeyNames(t *testing.T) {
	t.Parallel()
	store := databases.NewMemoryTransitStore()
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	service := transit.NewService(transit.ServiceConfig{
		Store:              store,
		DefaultSearchLimit: 10,
		DefaultListLimit:   10,
		MaxBatchSize:       10,
		EnableVectorSearch: false,
	})

	created, err := service.CreateMemory(context.Background(), 1, 1, []transit.CreateMemoryItem{{
		KeyName:     "Project Demo: Brief!",
		Description: "Demo brief",
		Value:       "Transit stores durable shared project notes",
	}})
	if err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
	if created[0].KeyName != "Project-Demo-Brief-" {
		t.Fatalf("CreateMemory() keyName = %q, want %q", created[0].KeyName, "Project-Demo-Brief-")
	}

	records, err := service.GetMemory(context.Background(), 1, []string{"Project Demo: Brief!"})
	if err != nil {
		t.Fatalf("GetMemory() error = %v", err)
	}
	if len(records) != 1 || records[0].KeyName != "Project-Demo-Brief-" {
		t.Fatalf("GetMemory() records = %#v, want normalized key record", records)
	}

	updated, err := service.UpdateMemory(context.Background(), 1, 1, transit.UpdateMemoryRequest{
		KeyName: "Project Demo: Brief!",
		Value:   "Updated value",
	})
	if err != nil {
		t.Fatalf("UpdateMemory() error = %v", err)
	}
	if updated.KeyName != "Project-Demo-Brief-" || updated.Value != "Updated value" {
		t.Fatalf("UpdateMemory() record = %#v, want normalized key and updated value", updated)
	}

	if err := service.DeleteMemory(context.Background(), 1, []string{"Project Demo: Brief!"}); err != nil {
		t.Fatalf("DeleteMemory() error = %v", err)
	}
	records, err = service.GetMemory(context.Background(), 1, []string{"Project-Demo-Brief-"})
	if err != nil {
		t.Fatalf("GetMemory() after delete error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("GetMemory() len after delete = %d, want 0", len(records))
	}
}

func TestServiceVectorSearchUsesQueryInstructionAndRawRecordEmbedding(t *testing.T) {
	t.Parallel()
	store := databases.NewMemoryTransitStore()
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	recorder := &recordingTransitEmbedder{}
	service := transit.NewService(transit.ServiceConfig{
		Store:  store,
		Vector: memoryTransitVector{inner: databases.NewMemoryVector()},
		EmbeddingConfig: config.EmbeddingConfig{
			Model: "Qwen3-Embedding-0.6B-f16.gguf",
			Instructions: config.EmbeddingInstructionConfig{
				Mode:   "auto",
				Format: "qwen",
			},
		},
		DefaultSearchLimit: 10,
		DefaultListLimit:   10,
		MaxBatchSize:       10,
		EnableVectorSearch: true,
		EmbedFn:            recorder.embed,
	})

	_, err := service.CreateMemory(context.Background(), 1, 1, []transit.CreateMemoryItem{{
		KeyName:     "project/demo/brief",
		Description: "Demo brief",
		Value:       "Transit stores durable shared project notes",
	}})
	if err != nil {
		t.Fatalf("CreateMemory() error = %v", err)
	}
	if len(recorder.texts) != 1 || recorder.texts[0] != "Transit stores durable shared project notes" {
		t.Fatalf("expected raw record embedding text, got %#v", recorder.texts)
	}

	_, err = service.SearchMemories(context.Background(), 1, transit.SearchRequest{Query: "durable shared", Limit: 5})
	if err != nil {
		t.Fatalf("SearchMemories() error = %v", err)
	}
	if len(recorder.texts) != 2 {
		t.Fatalf("expected record and query embedding calls, got %#v", recorder.texts)
	}
	if !strings.HasPrefix(recorder.texts[1], "Instruct: "+embedding.DefaultTransitQueryInstruction+"\nQuery: durable shared") {
		t.Fatalf("expected formatted transit query embedding, got %q", recorder.texts[1])
	}
}
