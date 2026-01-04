package workspaces

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

// InvalidationEvent represents a generation bump notification.
type InvalidationEvent struct {
	Generation       int64    `json:"generation"`
	SkillsGeneration int64    `json:"skills_generation"`
	ChangedPaths     []string `json:"changed_paths"`
}

// GenerationCache coordinates project generation state via Redis.
type GenerationCache interface {
	GetGeneration(ctx context.Context, tenantID, projectID string) (int64, error)
	GetSkillsGeneration(ctx context.Context, tenantID, projectID string) (int64, error)
	SetGenerations(ctx context.Context, tenantID, projectID string, gen, skillsGen int64) error
	PublishInvalidation(ctx context.Context, tenantID, projectID string, ev InvalidationEvent) error
	SubscribeInvalidations(ctx context.Context, tenantID, projectID string) (<-chan InvalidationEvent, func())
	AcquireCommitLock(ctx context.Context, tenantID, projectID, sessionID string, ttl time.Duration) (bool, error)
}

// RedisGenerationCache is a Redis-backed GenerationCache.
type RedisGenerationCache struct {
	client redis.UniversalClient
}

// NewRedisGenerationCache builds a Redis cache when enabled; returns nil when disabled.
func NewRedisGenerationCache(cfg config.RedisConfig) (*RedisGenerationCache, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	opts := &redis.Options{ // single-node by default
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLSInsecureSkipVerify {
		opts.TLSConfig = insecureTLS()
	}

	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &RedisGenerationCache{client: client}, nil
}

func (c *RedisGenerationCache) keyGen(tenantID, projectID string) string {
	return "project:" + tenantID + ":" + projectID + ":generation"
}

func (c *RedisGenerationCache) keySkills(tenantID, projectID string) string {
	return "project:" + tenantID + ":" + projectID + ":skills_gen"
}

func (c *RedisGenerationCache) keyLock(tenantID, projectID string) string {
	return "project:" + tenantID + ":" + projectID + ":lock"
}

func (c *RedisGenerationCache) keyChannel(tenantID, projectID string) string {
	return "project:" + tenantID + ":" + projectID + ":invalidate"
}

func (c *RedisGenerationCache) GetGeneration(ctx context.Context, tenantID, projectID string) (int64, error) {
	return c.client.Get(ctx, c.keyGen(tenantID, projectID)).Int64()
}

func (c *RedisGenerationCache) GetSkillsGeneration(ctx context.Context, tenantID, projectID string) (int64, error) {
	return c.client.Get(ctx, c.keySkills(tenantID, projectID)).Int64()
}

func (c *RedisGenerationCache) SetGenerations(ctx context.Context, tenantID, projectID string, gen, skillsGen int64) error {
	pipe := c.client.TxPipeline()
	pipe.Set(ctx, c.keyGen(tenantID, projectID), gen, 0)
	pipe.Set(ctx, c.keySkills(tenantID, projectID), skillsGen, 0)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisGenerationCache) PublishInvalidation(ctx context.Context, tenantID, projectID string, ev InvalidationEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, c.keyChannel(tenantID, projectID), data).Err()
}

func (c *RedisGenerationCache) SubscribeInvalidations(ctx context.Context, tenantID, projectID string) (<-chan InvalidationEvent, func()) {
	ch := make(chan InvalidationEvent, 1)
	sub := c.client.Subscribe(ctx, c.keyChannel(tenantID, projectID))
	go func() {
		for msg := range sub.Channel() {
			var ev InvalidationEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				log.Warn().Err(err).Msg("redis_invalidation_decode_failed")
				continue
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}()
	cancel := func() {
		_ = sub.Close()
		close(ch)
	}
	return ch, cancel
}

func (c *RedisGenerationCache) AcquireCommitLock(ctx context.Context, tenantID, projectID, sessionID string, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, c.keyLock(tenantID, projectID), sessionID, ttl).Result()
}

func insecureTLS() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}
