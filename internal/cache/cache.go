package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("key not found")

type Cache struct {
	client *redis.Client
	logger *slog.Logger
}

func New(client *redis.Client, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cache{client: client, logger: logger}
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		c.logger.Error("cache get failed", "key", key, "error", err)
	}
	return val, err
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	err := c.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		c.logger.Error("cache set failed", "key", key, "error", err)
	}
	return err
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		c.logger.Error("cache delete failed", "key", key, "error", err)
	}
	return err
}

func (c *Cache) Close() error {
	return c.client.Close()
}
