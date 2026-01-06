package databases

import (
	"context"
	"testing"
)

func TestMemUserPreferencesStore_GetEmpty(t *testing.T) {
	store := NewUserPreferencesStore(nil)
	ctx := context.Background()

	prefs, err := store.Get(ctx, 123)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if prefs.UserID != 123 {
		t.Errorf("expected UserID=123, got %d", prefs.UserID)
	}
	if prefs.ActiveProjectID != "" {
		t.Errorf("expected empty ActiveProjectID, got %q", prefs.ActiveProjectID)
	}
}

func TestMemUserPreferencesStore_SetAndGet(t *testing.T) {
	store := NewUserPreferencesStore(nil)
	ctx := context.Background()

	err := store.SetActiveProject(ctx, 42, "project-abc")
	if err != nil {
		t.Fatalf("SetActiveProject error: %v", err)
	}

	prefs, err := store.Get(ctx, 42)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if prefs.UserID != 42 {
		t.Errorf("expected UserID=42, got %d", prefs.UserID)
	}
	if prefs.ActiveProjectID != "project-abc" {
		t.Errorf("expected ActiveProjectID='project-abc', got %q", prefs.ActiveProjectID)
	}
	if prefs.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
}

func TestMemUserPreferencesStore_UpdateProject(t *testing.T) {
	store := NewUserPreferencesStore(nil)
	ctx := context.Background()

	_ = store.SetActiveProject(ctx, 1, "proj-1")
	_ = store.SetActiveProject(ctx, 1, "proj-2")

	prefs, _ := store.Get(ctx, 1)
	if prefs.ActiveProjectID != "proj-2" {
		t.Errorf("expected ActiveProjectID='proj-2', got %q", prefs.ActiveProjectID)
	}
}

func TestMemUserPreferencesStore_ClearProject(t *testing.T) {
	store := NewUserPreferencesStore(nil)
	ctx := context.Background()

	_ = store.SetActiveProject(ctx, 1, "proj-1")
	_ = store.SetActiveProject(ctx, 1, "") // Clear

	prefs, _ := store.Get(ctx, 1)
	if prefs.ActiveProjectID != "" {
		t.Errorf("expected empty ActiveProjectID, got %q", prefs.ActiveProjectID)
	}
}

func TestMemUserPreferencesStore_MultiUser(t *testing.T) {
	store := NewUserPreferencesStore(nil)
	ctx := context.Background()

	_ = store.SetActiveProject(ctx, 1, "user1-proj")
	_ = store.SetActiveProject(ctx, 2, "user2-proj")

	prefs1, _ := store.Get(ctx, 1)
	prefs2, _ := store.Get(ctx, 2)

	if prefs1.ActiveProjectID != "user1-proj" {
		t.Errorf("user1: expected 'user1-proj', got %q", prefs1.ActiveProjectID)
	}
	if prefs2.ActiveProjectID != "user2-proj" {
		t.Errorf("user2: expected 'user2-proj', got %q", prefs2.ActiveProjectID)
	}
}
