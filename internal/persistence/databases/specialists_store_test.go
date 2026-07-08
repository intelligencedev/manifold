package databases

import (
	"context"
	"testing"

	"manifold/internal/observability"
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

func TestMemSpecStoreUpsertPreservesExistingAPIKeyWhenRedactedPlaceholder(t *testing.T) {
	t.Parallel()

	store := &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	ctx := context.Background()
	userID := int64(10)

	_, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "writer",
		Provider: "openai",
		APIKey:   "key-1",
		Model:    "gpt-5-mini",
		ExtraHeaders: map[string]string{
			"Authorization": "Bearer real-header",
			"X-Trace":       "trace-1",
		},
		ExtraParams: map[string]any{
			"password": "real-password",
			"visible":  "shown",
		},
	})
	if err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}

	saved, err := store.Upsert(ctx, userID, persistence.Specialist{
		Name:     "writer",
		Provider: "openai",
		APIKey:   observability.RedactedValue,
		Paused:   true,
		Model:    "gpt-5",
		ExtraHeaders: map[string]string{
			"Authorization": observability.RedactedValue,
			"X-Trace":       "trace-2",
		},
		ExtraParams: map[string]any{
			"password": observability.RedactedValue,
			"visible":  "also-shown",
		},
	})
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}

	if saved.APIKey != "key-1" {
		t.Fatalf("expected redacted api key placeholder to preserve existing key, got %q", saved.APIKey)
	}
	if !saved.Paused {
		t.Fatalf("expected paused flag to update")
	}
	if saved.Model != "gpt-5" {
		t.Fatalf("expected model to update, got %q", saved.Model)
	}
	if saved.ExtraHeaders["Authorization"] != "Bearer real-header" {
		t.Fatalf("expected authorization header preserved, got %#v", saved.ExtraHeaders)
	}
	if saved.ExtraHeaders["X-Trace"] != "trace-2" {
		t.Fatalf("expected non-secret header to update, got %#v", saved.ExtraHeaders)
	}
	if saved.ExtraParams["password"] != "real-password" {
		t.Fatalf("expected password param preserved, got %#v", saved.ExtraParams)
	}
	if saved.ExtraParams["visible"] != "also-shown" {
		t.Fatalf("expected visible param to update, got %#v", saved.ExtraParams)
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
