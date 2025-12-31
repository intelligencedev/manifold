package skills

import "sync"

// Manager caches skills per workdir.
//
// For now, Manifold only needs skills metadata injection into prompts, but this
// provides a safe place to add caching and reload behavior.
type Manager struct {
	base Loader

	mu    sync.RWMutex
	cache map[string]LoadOutcome
}

func NewManager(base Loader) *Manager {
	return &Manager{base: base, cache: make(map[string]LoadOutcome)}
}

func (m *Manager) ForWorkdir(workdir string, forceReload bool) LoadOutcome {
	if !forceReload {
		m.mu.RLock()
		if out, ok := m.cache[workdir]; ok {
			m.mu.RUnlock()
			return out
		}
		m.mu.RUnlock()
	}

	loader := m.base
	loader.Workdir = workdir
	out := loader.Load()

	m.mu.Lock()
	m.cache[workdir] = out
	m.mu.Unlock()
	return out
}
