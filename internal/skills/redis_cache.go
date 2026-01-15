//go:build enterprise
// +build enterprise

package skills

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

// RedisSkillsCache provides Redis-backed caching for rendered skills prompts.
// It caches prompts keyed by tenant, project, and skills generation to avoid
// repeated S3 fetches and skill rendering.
type RedisSkillsCache struct {
	client redis.UniversalClient
	ttl    time.Duration
}

// NewRedisSkillsCache builds a Redis-backed skills cache when enabled.
// Returns nil when disabled.
func NewRedisSkillsCache(cfg config.RedisConfig, ttl time.Duration) (*RedisSkillsCache, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLSInsecureSkipVerify {
		opts.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis skills cache ping: %w", err)
	}
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	return &RedisSkillsCache{client: client, ttl: ttl}, nil
}

func (c *RedisSkillsCache) promptKey(tenantID, projectID string, skillsGen int64) string {
	return fmt.Sprintf("skills:%s:%s:%d:prompt", tenantID, projectID, skillsGen)
}

func (c *RedisSkillsCache) metadataKey(tenantID, projectID string, skillsGen int64) string {
	return fmt.Sprintf("skills:%s:%s:%d:metadata", tenantID, projectID, skillsGen)
}

// GetRenderedPrompt retrieves a cached rendered skills prompt.
// Returns empty string and false if not cached.
func (c *RedisSkillsCache) GetRenderedPrompt(ctx context.Context, tenantID, projectID string, skillsGen int64) (string, bool) {
	if c == nil || c.client == nil {
		return "", false
	}
	key := c.promptKey(tenantID, projectID, skillsGen)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Debug().Err(err).Str("key", key).Msg("redis_skills_cache_get_prompt_error")
		}
		return "", false
	}
	return val, true
}

// SetRenderedPrompt caches a rendered skills prompt.
func (c *RedisSkillsCache) SetRenderedPrompt(ctx context.Context, tenantID, projectID string, skillsGen int64, prompt string) error {
	if c == nil || c.client == nil {
		return nil
	}
	key := c.promptKey(tenantID, projectID, skillsGen)
	if err := c.client.Set(ctx, key, prompt, c.ttl).Err(); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("redis_skills_cache_set_prompt_error")
		return err
	}
	return nil
}

// CachedSkillsMetadata holds cached skill metadata for a project.
type CachedSkillsMetadata struct {
	Skills   []Metadata `json:"skills"`
	CachedAt time.Time  `json:"cachedAt"`
}

// GetSkillsMetadata retrieves cached skills metadata.
// Returns nil and false if not cached.
func (c *RedisSkillsCache) GetSkillsMetadata(ctx context.Context, tenantID, projectID string, skillsGen int64) (*CachedSkillsMetadata, bool) {
	if c == nil || c.client == nil {
		return nil, false
	}
	key := c.metadataKey(tenantID, projectID, skillsGen)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Debug().Err(err).Str("key", key).Msg("redis_skills_cache_get_metadata_error")
		}
		return nil, false
	}
	var meta CachedSkillsMetadata
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("redis_skills_cache_unmarshal_error")
		return nil, false
	}
	return &meta, true
}

// SetSkillsMetadata caches skills metadata.
func (c *RedisSkillsCache) SetSkillsMetadata(ctx context.Context, tenantID, projectID string, skillsGen int64, skills []Metadata) error {
	if c == nil || c.client == nil {
		return nil
	}
	key := c.metadataKey(tenantID, projectID, skillsGen)
	data, err := json.Marshal(CachedSkillsMetadata{Skills: skills, CachedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		log.Debug().Err(err).Str("key", key).Msg("redis_skills_cache_set_metadata_error")
		return err
	}
	return nil
}

// Invalidate removes cached entries for a project (all generations).
func (c *RedisSkillsCache) Invalidate(ctx context.Context, tenantID, projectID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	pattern := fmt.Sprintf("skills:%s:%s:*", tenantID, projectID)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			log.Debug().Err(err).Str("key", iter.Val()).Msg("redis_skills_cache_invalidate_error")
		}
	}
	return iter.Err()
}

// Close closes the Redis client connection.
func (c *RedisSkillsCache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
