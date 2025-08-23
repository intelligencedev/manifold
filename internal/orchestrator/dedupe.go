package orchestrator

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// DedupeStore is a minimal interface for idempotency storage used by the orchestrator.
// Implementations should store a value under a correlation key with a TTL.
type DedupeStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// RedisDedupeStore is a Redis-backed implementation of DedupeStore.
type RedisDedupeStore struct {
	client *redis.Client
}

// NewRedisDedupeStore creates a new RedisDedupeStore using the given address
// (e.g., "localhost:6379") and pings the server to validate the connection.
func NewRedisDedupeStore(addr string) (*RedisDedupeStore, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return &RedisDedupeStore{client: c}, nil
}

// Get returns the value for the given key or "" when the key is missing.
func (s *RedisDedupeStore) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// Set stores the given value under key with the provided TTL.
func (s *RedisDedupeStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

// Close closes the underlying Redis client. This is not part of the DedupeStore
// interface but is provided for graceful shutdown in main.
func (s *RedisDedupeStore) Close() error {
	return s.client.Close()
}
