package databases

import (
	"context"
	"testing"

	"manifold/internal/persistence"
)

func TestMemSpecStoreUpsertPreservesExistingAPIKeyWhenBlank(t *testing.T) {
	t.Parallel()

	store := &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	ctx := context.Background()
	userID := int64(7)

	_, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "writer",
		Provider: "openai",
		APIKey:   "key-1",
		Model:    "gpt-5-mini",
	})
	if err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}

	saved, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "writer",
		Provider: "openai",
		APIKey:   "",
		Model:    "gpt-5",
	})
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}

	if saved.APIKey != "key-1" {
		t.Fatalf("expected existing api key to be preserved, got %q", saved.APIKey)
	}
	if saved.Model != "gpt-5" {
		t.Fatalf("expected model to update, got %q", saved.Model)
	}
}

func TestMemSpecStoreUpsertReplacesAPIKeyWhenProvided(t *testing.T) {
	t.Parallel()

	store := &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	ctx := context.Background()
	userID := int64(8)

	_, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "analyst",
		Provider: "anthropic",
		APIKey:   "old-key",
		Model:    "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}

	saved, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "analyst",
		Provider: "anthropic",
		APIKey:   "new-key",
		Model:    "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}

	if saved.APIKey != "new-key" {
		t.Fatalf("expected api key to be replaced, got %q", saved.APIKey)
	}
}
