package skills

// InvalidateCacheForProject clears cached skills entries for the given project path.
// The cache keys are projectID:generation:skillsGen; here we invalidate using the
// project path as the projectID used by the prompt renderer.
func InvalidateCacheForProject(projectID string) {
	defaultCache := skillsCacheSingleton()
	if defaultCache != nil {
		defaultCache.Invalidate(projectID)
	}
}

var globalCache *Cache

// skillsCacheSingleton returns a process-wide cache, lazily initialized.
func skillsCacheSingleton() *Cache {
	if globalCache == nil {
		globalCache = NewCache()
	}
	return globalCache
}

// DefaultCache returns the process-wide skills cache singleton.
// This is the exported version for use by other packages.
func DefaultCache() *Cache {
	return skillsCacheSingleton()
}
