package web

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucket_TakeAndRefill(t *testing.T) {
	// Small capacity and fast refill for test
	tb := newTokenBucket(1, 5*time.Millisecond)
	if !tb.takeToken() {
		t.Fatalf("expected first take to succeed")
	}
	if tb.takeToken() {
		t.Fatalf("expected second take to fail")
	}
	// Wait for refill
	time.Sleep(10 * time.Millisecond)
	if !tb.takeToken() {
		t.Fatalf("expected take after refill to succeed")
	}
}

func TestTokenBucket_WaitForToken_Canceled(t *testing.T) {
	tb := newTokenBucket(1, 100*time.Millisecond)
	// drain token
	if !tb.takeToken() {
		t.Fatalf("expected initial token")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tb.waitForToken(ctx); err == nil {
		t.Fatalf("expected error when context canceled")
	}
}
