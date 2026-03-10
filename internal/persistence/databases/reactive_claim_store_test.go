package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/persistence"

	"github.com/google/uuid"
)

func TestMemReactiveClaimStoreTryClaimAndRelease(t *testing.T) {
	t.Parallel()

	store := NewReactiveClaimStore(nil)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}

	firstToken := uuid.NewString()
	claimed, err := store.TryClaim(ctx, "!room:test", "@bot-a:test", firstToken, "$event-1", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("try claim: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim to succeed")
	}

	secondClaimed, err := store.TryClaim(ctx, "!room:test", "@bot-b:test", uuid.NewString(), "$event-2", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("second try claim: %v", err)
	}
	if secondClaimed {
		t.Fatalf("expected active lease to block competing claim")
	}

	claim, err := store.GetActiveClaim(ctx, "!room:test")
	if err != nil {
		t.Fatalf("get active claim: %v", err)
	}
	if claim.BotID != "@bot-a:test" {
		t.Fatalf("unexpected bot id: %q", claim.BotID)
	}

	if err := store.Release(ctx, "!room:test", firstToken); err != nil {
		t.Fatalf("release: %v", err)
	}

	_, err = store.GetActiveClaim(ctx, "!room:test")
	if err != persistence.ErrNotFound {
		t.Fatalf("expected claim to be gone, got %v", err)
	}
}
