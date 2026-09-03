package cache

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return New(client, slog.New(slog.NewTextHandler(nil, nil)))
}

func TestNewNilLogger(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	c := New(client, nil)
	assert.NotNil(t, c)
	assert.NotNil(t, c.logger)
}

func TestSetGet(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	err := c.Set(ctx, "k", "v", time.Minute)
	require.NoError(t, err)

	got, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestGetNotFound(t *testing.T) {
	c := newTestCache(t)
	_, err := c.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDelete(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k", "v", time.Minute))
	require.NoError(t, c.Delete(ctx, "k"))

	_, err := c.Get(ctx, "k")
	assert.ErrorIs(t, err, ErrNotFound)
}
