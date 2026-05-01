package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
	logger *slog.Logger
}

func New(redisURL string, logger *slog.Logger) (*Cache, error) {
	if redisURL == "" {
		logger.Info("Redis not configured, caching disabled")
		return &Cache{logger: logger}, nil
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis connection failed, caching disabled", slog.String("error", err.Error()))
		return &Cache{logger: logger}, nil
	}

	logger.Info("Redis connected", slog.String("url", redisURL))
	return &Cache{client: client, logger: logger}, nil
}

func (c *Cache) IsEnabled() bool {
	return c.client != nil
}

func (c *Cache) Get(ctx context.Context, key string, dest any) bool {
	if !c.IsEnabled() {
		return false
	}

	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		c.logger.Debug("cache get error", slog.String("key", key), slog.String("error", err.Error()))
		return false
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		c.logger.Debug("cache unmarshal error", slog.String("key", key), slog.String("error", err.Error()))
		return false
	}
	return true
}

func (c *Cache) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	if !c.IsEnabled() {
		return
	}

	data, err := json.Marshal(value)
	if err != nil {
		c.logger.Debug("cache marshal error", slog.String("key", key), slog.String("error", err.Error()))
		return
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.logger.Debug("cache set error", slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (c *Cache) Delete(ctx context.Context, key string) {
	if !c.IsEnabled() {
		return
	}
	c.client.Del(ctx, key)
}

func (c *Cache) DeleteByPrefix(ctx context.Context, prefix string) {
	if !c.IsEnabled() {
		return
	}

	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			c.logger.Debug("cache scan error", slog.String("prefix", prefix), slog.String("error", err.Error()))
			return
		}

		if len(keys) > 0 {
			c.client.Del(ctx, keys...)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

func (c *Cache) Close() error {
	if c.IsEnabled() {
		return c.client.Close()
	}
	return nil
}
