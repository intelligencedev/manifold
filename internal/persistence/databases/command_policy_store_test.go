package databases

import (
	"context"
	"testing"

	"manifold/internal/config"
)

func TestMemCommandPolicyStoreUpsertAndList(t *testing.T) {
	store := NewCommandPolicyStore(nil)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	stored, err := store.UpsertRule(ctx, 42, config.ExecCommandRule{
		Decision:      " ALLOW ",
		Pattern:       []string{" go ", " test "},
		Contexts:      []string{" CLI "},
		Justification: " approved ",
	})
	if err != nil {
		t.Fatalf("UpsertRule error: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("expected generated rule ID")
	}
	if stored.Decision != "allow" || stored.Pattern[0] != "go" || stored.Contexts[0] != "cli" || stored.Justification != "approved" {
		t.Fatalf("expected normalized stored rule, got %+v", stored)
	}

	rules, err := store.ListRules(ctx, 42)
	if err != nil {
		t.Fatalf("ListRules error: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != stored.ID {
		t.Fatalf("expected stored rule, got %+v", rules)
	}
}

func TestMemCommandPolicyStoreDefensiveCopies(t *testing.T) {
	store := NewCommandPolicyStore(nil)
	ctx := context.Background()
	rule, err := store.UpsertRule(ctx, 0, config.ExecCommandRule{
		ID:       "allow-echo",
		Decision: "allow",
		Pattern:  []string{"echo", "hi"},
		Contexts: []string{"cli"},
	})
	if err != nil {
		t.Fatalf("UpsertRule error: %v", err)
	}
	rule.Pattern[0] = "mutated"

	rules, err := store.ListRules(ctx, 0)
	if err != nil {
		t.Fatalf("ListRules error: %v", err)
	}
	rules[0].Contexts[0] = "mutated"

	again, err := store.ListRules(ctx, 0)
	if err != nil {
		t.Fatalf("ListRules second error: %v", err)
	}
	if again[0].Pattern[0] != "echo" || again[0].Contexts[0] != "cli" {
		t.Fatalf("store exposed mutable slices: %+v", again[0])
	}
}

func TestMemCommandPolicyStoreSessionAllowAll(t *testing.T) {
	store := NewCommandPolicyStore(nil)
	ctx := context.Background()

	if override, ok, err := store.GetSessionOverride(ctx, 7, "session-a"); err != nil || ok || override.AllowAllCommands {
		t.Fatalf("expected empty session override, override=%+v ok=%v err=%v", override, ok, err)
	}
	if err := store.SetSessionAllowAll(ctx, 7, "session-a", true); err != nil {
		t.Fatalf("SetSessionAllowAll true error: %v", err)
	}
	override, ok, err := store.GetSessionOverride(ctx, 7, "session-a")
	if err != nil {
		t.Fatalf("GetSessionOverride error: %v", err)
	}
	if !ok || !override.AllowAllCommands || override.UserID != 7 || override.SessionID != "session-a" {
		t.Fatalf("unexpected session override: %+v ok=%v", override, ok)
	}
	if _, ok, err := store.GetSessionOverride(ctx, 8, "session-a"); err != nil || ok {
		t.Fatalf("expected user-scoped session override miss, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetSessionOverride(ctx, 7, "session-b"); err != nil || ok {
		t.Fatalf("expected session-scoped override miss, ok=%v err=%v", ok, err)
	}
	if err := store.SetSessionAllowAll(ctx, 7, "session-a", false); err != nil {
		t.Fatalf("SetSessionAllowAll false error: %v", err)
	}
	if _, ok, err := store.GetSessionOverride(ctx, 7, "session-a"); err != nil || ok {
		t.Fatalf("expected disabled session override to be removed, ok=%v err=%v", ok, err)
	}
}

func TestMemCommandPolicyStoreDeleteSessionOverride(t *testing.T) {
	store := NewCommandPolicyStore(nil)
	ctx := context.Background()
	if err := store.SetSessionAllowAll(ctx, 0, "session-a", true); err != nil {
		t.Fatalf("SetSessionAllowAll error: %v", err)
	}
	if err := store.DeleteSessionOverride(ctx, 0, "session-a"); err != nil {
		t.Fatalf("DeleteSessionOverride error: %v", err)
	}
	if _, ok, err := store.GetSessionOverride(ctx, 0, "session-a"); err != nil || ok {
		t.Fatalf("expected deleted session override, ok=%v err=%v", ok, err)
	}
}
