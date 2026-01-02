package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	// DefaultTokenCacheSize is the default maximum number of cached entries.
	DefaultTokenCacheSize = 1000
	// DefaultTokenCacheTTL is the default time-to-live for cache entries.
	DefaultTokenCacheTTL = 1 * time.Hour
)

// TokenCache provides an LRU-style cache for token counts.
type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]tokenCacheEntry
	maxSize int
	ttl     time.Duration

	// stats
	hits   int64
	misses int64
}

type tokenCacheEntry struct {
	count      int
	expiration time.Time
	lastAccess time.Time
}

// TokenCacheConfig configures the token cache.
type TokenCacheConfig struct {
	MaxSize int
	TTL     time.Duration
}

// NewTokenCache creates a new token cache with the given configuration.
func NewTokenCache(cfg TokenCacheConfig) *TokenCache {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = DefaultTokenCacheSize
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTokenCacheTTL
	}
	cache := &TokenCache{
		entries: make(map[string]tokenCacheEntry),
		maxSize: cfg.MaxSize,
		ttl:     cfg.TTL,
	}
	// Start background cleanup goroutine
	go cache.cleanupLoop()
	return cache
}

// Get retrieves a cached token count. Returns the count and true if found
// and not expired, otherwise returns 0 and false.
func (c *TokenCache) Get(text string) (int, bool) {
	key := hashText(text)
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return 0, false
	}

	if time.Now().After(entry.expiration) {
		delete(c.entries, key)
		c.misses++
		return 0, false
	}

	// Update last access time for LRU
	entry.lastAccess = time.Now()
	c.entries[key] = entry
	c.hits++
	return entry.count, true
}

// Set stores a token count in the cache.
func (c *TokenCache) Set(text string, count int) {
	key := hashText(text)
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldestLocked()
	}

	now := time.Now()
	c.entries[key] = tokenCacheEntry{
		count:      count,
		expiration: now.Add(c.ttl),
		lastAccess: now,
	}
}

// Stats returns cache hit/miss statistics.
func (c *TokenCache) Stats() (hits, misses int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

// Size returns the current number of entries in the cache.
func (c *TokenCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all entries from the cache.
func (c *TokenCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]tokenCacheEntry)
}

// evictOldestLocked removes the least recently accessed entry. Must be called with lock held.
func (c *TokenCache) evictOldestLocked() {
	if len(c.entries) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.lastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastAccess
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *TokenCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes all expired entries.
func (c *TokenCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiration) {
			delete(c.entries, key)
		}
	}
}

// hashText creates a short hash key from text to reduce memory usage.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16]) // Use first 16 bytes (32 hex chars)
}
