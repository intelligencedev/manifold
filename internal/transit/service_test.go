package transit_test

import (
	"context"
	"testing"

	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	transit "manifold/internal/transit"
)

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
