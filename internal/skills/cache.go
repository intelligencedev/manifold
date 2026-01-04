package skills

import (
	"fmt"
	"sync"
	"time"
)

// CachedSkills stores skills payload keyed by generation.
type CachedSkills struct {
	Generation       int64
	SkillsGeneration int64
	Skills           []Metadata
	RenderedPrompt   string
	CachedAt         time.Time
}

// Cache provides generation-aware skills caching per project.
type Cache struct {
	mu    sync.RWMutex
	cache map[string]*CachedSkills // key: projectID:generation:skillsGen
}

// NewCache builds a skills cache.
func NewCache() *Cache {
	return &Cache{cache: make(map[string]*CachedSkills)}
}

func cacheKey(projectID string, generation, skillsGen int64) string {
	return fmt.Sprintf("%s:%d:%d", projectID, generation, skillsGen)
}

// GetOrLoad returns cached skills for the provided generation or loads them via loader.
// loader should fetch and render skills for the provided project/generation tuple.
func (c *Cache) GetOrLoad(projectID string, generation, skillsGen int64, loader func() (*CachedSkills, error)) (*CachedSkills, error) {
	key := cacheKey(projectID, generation, skillsGen)

	c.mu.RLock()
	if out, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	loaded, err := loader()
	if err != nil {
		return nil, err
	}
	if loaded != nil {
		loaded.CachedAt = time.Now().UTC()
		c.mu.Lock()
		c.cache[key] = loaded
		c.mu.Unlock()
	}
	return loaded, nil
}

// Invalidate clears cached entries for a project (all generations).
func (c *Cache) Invalidate(projectID string) {
	c.mu.Lock()
	for k := range c.cache {
		if len(projectID) == 0 || (len(k) >= len(projectID) && k[:len(projectID)] == projectID && (len(k) == len(projectID) || k[len(projectID)] == ':')) {
			delete(c.cache, k)
		}
	}
	c.mu.Unlock()
}
