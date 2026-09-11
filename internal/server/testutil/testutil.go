// Package testutil provides shared test helpers for the server package and
// its endpoint handlers.
package testutil

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmuszka/larynx/internal/cache"
	"github.com/jmuszka/larynx/internal/logging"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestLogger(t *testing.T) *logging.Service {
	t.Helper()
	l, err := logging.New(logging.Config{Level: logging.LevelError})
	require.NoError(t, err)
	t.Cleanup(l.Close)
	return l
}

func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS articles (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			slug        TEXT NOT NULL UNIQUE,
			title       TEXT NOT NULL,
			content     TEXT NOT NULL,
			description TEXT NOT NULL,
			published 	DATETIME DEFAULT CURRENT_TIMESTAMP,
			modified    DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)
	return db
}

type FakeGraphStore struct {
	ExecuteFn func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error)
	ConnErr   error
	Queries   []string
	ParamSets []map[string]any
}

func (f *FakeGraphStore) ExecuteQuery(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
	f.Queries = append(f.Queries, query)
	f.ParamSets = append(f.ParamSets, params)
	if f.ExecuteFn != nil {
		return f.ExecuteFn(ctx, query, params, opts...)
	}
	return &neo4j.EagerResult{}, nil
}

func (f *FakeGraphStore) VerifyConnectivity(ctx context.Context) error {
	return f.ConnErr
}

func FakeRecord(keys []string, values []any) *neo4j.Record {
	return &neo4j.Record{Keys: keys, Values: values}
}

func WithURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func NewServerCache(t *testing.T) *cache.Cache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return cache.New(client, nil)
}
