package llm

import "sync"

// TokenCache memoizes token counts for repeated tokenizer requests.
type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]int
}

// Get returns a cached token count.
func (c *TokenCache) Get(text string) (int, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	count, ok := c.entries[text]
	return count, ok
}

// Set stores a token count for later reuse.
func (c *TokenCache) Set(text string, count int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]int)
	}
	c.entries[text] = count
}
