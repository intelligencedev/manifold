package skills

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// CacheService coordinates local skills caching.
// It provides a unified interface for fetching skills with multi-tier caching:
// 1. In-memory cache (process-local, fastest)
// 2. Filesystem fallback (for local deployments)
type CacheService struct {
	localCache *Cache
}

type CacheServiceConfig struct {
}

// NewCacheService creates a skills cache service.
func NewCacheService(cfg CacheServiceConfig) (*CacheService, error) {
	svc := &CacheService{
		localCache: NewCache(),
	}
	return svc, nil
}

// GetOrLoad retrieves skills for a project, checking caches in order:
// local -> filesystem. Results are cached in memory.
func (s *CacheService) GetOrLoad(ctx context.Context, tenantID, projectID, projectDir string, generation, skillsGen int64) (*CachedSkills, error) {
	cacheKey := projectID
	if cacheKey == "" {
		cacheKey = projectDir
	}

	// 1. Check local in-memory cache first
	cached, err := s.localCache.GetOrLoad(cacheKey, generation, skillsGen, func() (*CachedSkills, error) {
		// 2. Load from filesystem
		var outcome LoadOutcome
		if projectDir != "" {
			outcome = LoadFromDir(projectDir)
			log.Debug().Str("projectDir", projectDir).Int("skills", len(outcome.Skills)).Msg("skills_cache_service: filesystem load")
		}

		for _, e := range outcome.Errors {
			log.Debug().Str("path", e.Path).Str("error", e.Message).Msg("skills_cache_service: load error")
		}

		prompt := RenderSkillsSection(outcome.Skills)
		if prompt == "" && len(outcome.Skills) == 0 {
			return nil, nil
		}

		result := &CachedSkills{
			Generation:       generation,
			SkillsGeneration: skillsGen,
			Skills:           outcome.Skills,
			RenderedPrompt:   prompt,
			CachedAt:         time.Now().UTC(),
		}

		return result, nil
	})

	return cached, err
}

// Invalidate clears cached entries for a project from all cache tiers.
func (s *CacheService) Invalidate(ctx context.Context, tenantID, projectID string) {
	// Invalidate local cache
	s.localCache.Invalidate(projectID)

}

// Close shuts down cache connections.
func (s *CacheService) Close() error {
	return nil
}

// RenderSkillsSection builds a markdown "## Skills" section from skill metadata.
// Exported for use by the cache service and prompts package.
func RenderSkillsSection(skillsList []Metadata) string {
	if len(skillsList) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Skills\n")
	b.WriteString("These skills are discovered from the project's .manifold/skills folder. Each entry includes a name, description, and file path.\n")
	for _, s := range skillsList {
		desc := s.Description
		if strings.TrimSpace(s.ShortDescription) != "" {
			desc = s.ShortDescription
		}
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		b.WriteString(desc)
		b.WriteString(" (file: ")
		b.WriteString(s.Path)
		b.WriteString(")\n")
	}

	b.WriteString("- Trigger rules: If the user names a skill (with $skill-name or plain text) OR the task matches a skill description, use it for that turn. Multiple mentions mean use them all.\n")
	b.WriteString("- Progressive disclosure: After selecting a skill, open its SKILL.md; load additional files (references/, scripts/, assets/) only as needed.\n")
	b.WriteString("- Missing/blocked: If a named skill path cannot be read, say so briefly and continue with a fallback.\n")
	b.WriteString("- Context hygiene: Keep context small—summarize long files, avoid bulk-loading references, and only load variant-specific files when relevant.\n")
	return b.String()
}

// Global cache service singleton for the application.
var (
	globalCacheService *CacheService
	globalCacheOnce    sync.Once
)

// GetCacheService returns the global cache service instance.
// Lazy-initialized on first call.
func GetCacheService() *CacheService {
	globalCacheOnce.Do(func() {
		// Default initialization with local cache only.
		// Call InitCacheService to enable alternate backing stores.
		globalCacheService = &CacheService{
			localCache: NewCache(),
		}
	})
	return globalCacheService
}

// InitCacheService initializes the global cache service with the given configuration.
// Should be called early in application startup.
func InitCacheService(cfg CacheServiceConfig) error {
	svc, err := NewCacheService(cfg)
	if err != nil {
		return err
	}
	globalCacheService = svc
	return nil
}
