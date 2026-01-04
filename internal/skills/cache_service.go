package skills

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

// SkillsFileEntry represents a file or directory for skills loading.
// This mirrors the projects.FileEntry structure to avoid import cycles.
type SkillsFileEntry struct {
	Path string
	Name string
	Type string // "file" or "dir"
	Size int64
}

// SkillsProjectService is the subset of projects.ProjectService needed for skills loading.
// This interface allows skills to load from S3 without creating an import cycle.
type SkillsProjectService interface {
	ListTreeForSkills(ctx context.Context, userID int64, projectID, path string) ([]SkillsFileEntry, error)
	ReadFile(ctx context.Context, userID int64, projectID, path string) (io.ReadCloser, error)
}

// CacheService coordinates local and Redis-backed skills caching.
// It provides a unified interface for fetching skills with multi-tier caching:
// 1. In-memory cache (process-local, fastest)
// 2. Redis cache (distributed, shared across nodes)
// 3. S3 direct fetch (when available, avoids full workspace hydration)
// 4. Filesystem fallback (for legacy/local deployments)
type CacheService struct {
	localCache *Cache
	redisCache *RedisSkillsCache
	s3Loader   *S3SkillsLoader
}

// CacheServiceConfig configures the skills cache service.
type CacheServiceConfig struct {
	RedisConfig config.RedisConfig
	RedisTTL    time.Duration
	ProjectSvc  SkillsProjectService
}

// NewCacheService creates a skills cache service with optional Redis backing.
func NewCacheService(cfg CacheServiceConfig) (*CacheService, error) {
	svc := &CacheService{
		localCache: NewCache(),
	}

	// Initialize Redis cache if enabled
	if cfg.RedisConfig.Enabled {
		redisCache, err := NewRedisSkillsCache(cfg.RedisConfig, cfg.RedisTTL)
		if err != nil {
			log.Warn().Err(err).Msg("skills_cache_service: redis cache init failed, falling back to local-only")
		} else if redisCache != nil {
			svc.redisCache = redisCache
			log.Info().Msg("skills_cache_service: redis cache enabled")
		}
	}

	// Initialize S3 loader if project service is provided
	if cfg.ProjectSvc != nil {
		svc.s3Loader = NewS3SkillsLoader(cfg.ProjectSvc)
		log.Info().Msg("skills_cache_service: s3 skills loader enabled")
	}

	return svc, nil
}

// GetOrLoad retrieves skills for a project, checking caches in order:
// local -> Redis -> S3/filesystem. Results are cached at each tier.
func (s *CacheService) GetOrLoad(ctx context.Context, tenantID, projectID, projectDir string, generation, skillsGen int64) (*CachedSkills, error) {
	cacheKey := projectID
	if cacheKey == "" {
		cacheKey = projectDir
	}

	// 1. Check local in-memory cache first
	cached, err := s.localCache.GetOrLoad(cacheKey, generation, skillsGen, func() (*CachedSkills, error) {
		// 2. Check Redis cache
		if s.redisCache != nil {
			if prompt, ok := s.redisCache.GetRenderedPrompt(ctx, tenantID, projectID, skillsGen); ok {
				meta, _ := s.redisCache.GetSkillsMetadata(ctx, tenantID, projectID, skillsGen)
				var skillsList []Metadata
				if meta != nil {
					skillsList = meta.Skills
				}
				log.Debug().Str("projectID", projectID).Int64("skillsGen", skillsGen).Msg("skills_cache_service: redis hit")
				return &CachedSkills{
					Generation:       generation,
					SkillsGeneration: skillsGen,
					Skills:           skillsList,
					RenderedPrompt:   prompt,
					CachedAt:         time.Now().UTC(),
				}, nil
			}
		}

		// 3. Load from S3 (if available) or filesystem
		var outcome LoadOutcome
		if s.s3Loader != nil && projectID != "" {
			userID := parseUserIDFromTenant(tenantID)
			outcome = s.s3Loader.LoadSkillsOnly(ctx, userID, projectID)
			log.Debug().Str("projectID", projectID).Int("skills", len(outcome.Skills)).Msg("skills_cache_service: s3 load")
		} else if projectDir != "" {
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

		// 4. Populate Redis cache for future nodes
		if s.redisCache != nil && projectID != "" {
			if err := s.redisCache.SetRenderedPrompt(ctx, tenantID, projectID, skillsGen, prompt); err != nil {
				log.Debug().Err(err).Msg("skills_cache_service: redis set prompt failed")
			}
			if err := s.redisCache.SetSkillsMetadata(ctx, tenantID, projectID, skillsGen, outcome.Skills); err != nil {
				log.Debug().Err(err).Msg("skills_cache_service: redis set metadata failed")
			}
		}

		return result, nil
	})

	return cached, err
}

// Invalidate clears cached entries for a project from all cache tiers.
func (s *CacheService) Invalidate(ctx context.Context, tenantID, projectID string) {
	// Invalidate local cache
	s.localCache.Invalidate(projectID)

	// Invalidate Redis cache
	if s.redisCache != nil {
		if err := s.redisCache.Invalidate(ctx, tenantID, projectID); err != nil {
			log.Debug().Err(err).Str("projectID", projectID).Msg("skills_cache_service: redis invalidate failed")
		}
	}
}

// Close shuts down cache connections.
func (s *CacheService) Close() error {
	if s.redisCache != nil {
		return s.redisCache.Close()
	}
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

// parseUserIDFromTenant extracts userID from tenant string.
// Tenant format is typically the string representation of userID.
func parseUserIDFromTenant(tenantID string) int64 {
	var userID int64
	_, _ = parseIntFromString(tenantID, &userID)
	return userID
}

func parseIntFromString(s string, out *int64) (bool, error) {
	if s == "" {
		return false, nil
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, nil
		}
		n = n*10 + int64(c-'0')
	}
	*out = n
	return true, nil
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
		// Call InitCacheService to enable Redis/S3 backing.
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
