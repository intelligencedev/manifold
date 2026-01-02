package llm

import (
	"testing"
	"time"
)

func TestTokenCache_GetSet(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	// Initially empty
	count, ok := cache.Get("hello world")
	if ok {
		t.Error("expected cache miss for new entry")
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	// Set and retrieve
	cache.Set("hello world", 42)
	count, ok = cache.Get("hello world")
	if !ok {
		t.Error("expected cache hit after set")
	}
	if count != 42 {
		t.Errorf("expected count 42, got %d", count)
	}
}

func TestTokenCache_Expiration(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		MaxSize: 100,
		TTL:     10 * time.Millisecond,
	})

	cache.Set("test", 10)
	count, ok := cache.Get("test")
	if !ok || count != 10 {
		t.Error("expected cache hit immediately after set")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	count, ok = cache.Get("test")
	if ok {
		t.Error("expected cache miss after expiration")
	}
}

func TestTokenCache_Eviction(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		MaxSize: 3,
		TTL:     time.Hour,
	})

	// Fill cache
	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	// Access "a" to make it recently used
	cache.Get("a")

	// Add new entry, should evict oldest (b, since c was added after b but a was just accessed)
	cache.Set("d", 4)

	if cache.Size() != 3 {
		t.Errorf("expected size 3 after eviction, got %d", cache.Size())
	}

	// "a" should still be there (recently accessed)
	if _, ok := cache.Get("a"); !ok {
		t.Error("expected 'a' to be in cache (recently accessed)")
	}

	// "d" should be there (just added)
	if _, ok := cache.Get("d"); !ok {
		t.Error("expected 'd' to be in cache")
	}
}

func TestTokenCache_Stats(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cache.Set("test", 10)
	cache.Get("test")  // hit
	cache.Get("test")  // hit
	cache.Get("other") // miss

	hits, misses := cache.Stats()
	if hits != 2 {
		t.Errorf("expected 2 hits, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
}

func TestTokenCache_Clear(t *testing.T) {
	cache := NewTokenCache(TokenCacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cache.Set("a", 1)
	cache.Set("b", 2)

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"a", 1},           // 1/4 + 1 = 1
		{"hello", 2},       // 5/4 + 1 = 2
		{"hello world", 3}, // 11/4 + 1 = 3
		{"this is a longer sentence for testing", 10}, // 38/4 + 1 = 10
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := EstimateTokens(tt.input)
			if got != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEstimateTokensForMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}

	total := EstimateTokensForMessages(msgs)
	expected := EstimateTokens("You are a helpful assistant.") + EstimateTokens("Hello")

	if total != expected {
		t.Errorf("EstimateTokensForMessages() = %d, want %d", total, expected)
	}
}
