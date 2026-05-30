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

func TestMemSpecStoreRoundTripsHarnessOverride(t *testing.T) {
	t.Parallel()

	store := &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	ctx := context.Background()
	userID := int64(9)
	_, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "planner",
		Provider: "openai",
		Harness: &persistence.SpecialistHarness{
			Enabled:       true,
			Mode:          "workflow",
			TerminalTools: []string{"agent_response"},
			RequiredSteps: []string{"search"},
			ToolPrerequisites: map[string][]persistence.SpecialistHarnessPrerequisite{
				"fetch": {{Tool: "search", MatchArg: "query"}},
			},
			Compact: persistence.SpecialistHarnessCompact{Enabled: true, KeepRecentSteps: 5},
		},
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	got, ok, err := store.GetByName(ctx, userID, "planner")
	if err != nil || !ok {
		t.Fatalf("get failed: ok=%v err=%v", ok, err)
	}
	if got.Harness == nil || !got.Harness.Enabled || got.Harness.Mode != "workflow" {
		t.Fatalf("unexpected harness config: %+v", got.Harness)
	}
	if got.Harness.ToolPrerequisites["fetch"][0].MatchArg != "query" {
		t.Fatalf("unexpected prerequisites: %+v", got.Harness.ToolPrerequisites)
	}
}
