package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGeographyGraph(t *testing.T) *fakeGraphStore {
	t.Helper()
	return &fakeGraphStore{
		executeFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				fakeRecord([]string{"json"}, []any{`{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"name":"Arawakan"},"geometry":{"type":"Point","coordinates":[0,0]}}]}`}),
			}}, nil
		},
	}
}

func TestHandleGetGeography(t *testing.T) {
	newSrv := func(t *testing.T, graph graphStore) *Server {
		return &Server{logger: testLogger(t), graph: graph, cache: newServerCache(t)}
	}

	t.Run("missing id", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "")
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"id is required"}`, w.Body.String())
	})

	t.Run("id too long", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", strings.Repeat("a", maxGeographyNameLength+1))
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cache hit", func(t *testing.T) {
		graph := &fakeGraphStore{}
		s := newSrv(t, graph)
		require.NoError(t, s.cache.Set(t.Context(), "/geography/test", `{"cached":true}`, 0))

		r := withURLParam(httptest.NewRequest(http.MethodGet, "/geography/test", nil), "id", "test")
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"cached":true}`, w.Body.String())
		assert.Empty(t, graph.queries)
	})

	t.Run("geography not found", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/geography/nope", nil), "id", "nope")
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"geography not found"}`, w.Body.String())
	})

	t.Run("success", func(t *testing.T) {
		s := newSrv(t, newGeographyGraph(t))
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/geography/Arawakan", nil), "id", "Arawakan")
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		gj, ok := resp["geojson"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "FeatureCollection", gj["type"])

		features, ok := gj["features"].([]any)
		require.True(t, ok)
		require.Len(t, features, 1)

		feat, ok := features[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Feature", feat["type"])

		props, ok := feat["properties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Arawakan", props["name"])

		geom, ok := feat["geometry"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Point", geom["type"])
	})

	t.Run("query error", func(t *testing.T) {
		graph := &fakeGraphStore{executeFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return nil, assert.AnError
		}}
		s := newSrv(t, graph)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/geography/test", nil), "id", "test")
		w := httptest.NewRecorder()
		s.handleGetGeography(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
