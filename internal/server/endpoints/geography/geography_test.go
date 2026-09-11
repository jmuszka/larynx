package geography

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmuszka/larynx/internal/server/endpoints"
	"github.com/jmuszka/larynx/internal/server/testutil"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGeographyGraph(t *testing.T) *testutil.FakeGraphStore {
	t.Helper()
	return &testutil.FakeGraphStore{
		ExecuteFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				testutil.FakeRecord([]string{"json", "props"}, []any{`{"type":"Point","coordinates":[0,0]}`, `{"name":"Arawakan"}`}),
			}}, nil
		},
	}
}

func TestHandleGetGeography(t *testing.T) {
	newSrv := func(t *testing.T, graph endpoints.GraphStore) *endpoints.Server {
		return endpoints.New(endpoints.Config{Logger: testutil.TestLogger(t), Graph: graph, Cache: testutil.NewServerCache(t)})
	}

	t.Run("missing id", func(t *testing.T) {
		s := newSrv(t, &testutil.FakeGraphStore{})
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"ID is required"}`, w.Body.String())
	})

	t.Run("id too long", func(t *testing.T) {
		s := newSrv(t, &testutil.FakeGraphStore{})
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", strings.Repeat("a", maxGeographyNameLength+1))
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cache hit", func(t *testing.T) {
		graph := &testutil.FakeGraphStore{}
		s := newSrv(t, graph)
		require.NoError(t, s.Cache.Set(t.Context(), "/geography/test", `{"cached":true}`, 0))

		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/test", nil), "id", "test")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"cached":true}`, w.Body.String())
		assert.Empty(t, graph.Queries)
	})

	t.Run("geography not found", func(t *testing.T) {
		s := newSrv(t, &testutil.FakeGraphStore{})
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/nope", nil), "id", "nope")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"Geography not found"}`, w.Body.String())
	})

	t.Run("success", func(t *testing.T) {
		s := newSrv(t, newGeographyGraph(t))
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/Arawakan", nil), "id", "Arawakan")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)

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
		assert.Equal(t, float64(geographyWeight), props["count"])

		geom, ok := feat["geometry"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Point", geom["type"])
	})

	t.Run("rounds up all matching nodes", func(t *testing.T) {
		graph := &testutil.FakeGraphStore{ExecuteFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				testutil.FakeRecord([]string{"json", "props"}, []any{`{"type":"Point","coordinates":[0,0]}`, `{"name":"a"}`}),
				testutil.FakeRecord([]string{"json", "props"}, []any{`{"type":"Point","coordinates":[1,1]}`, `{"name":"b"}`}),
			}}, nil
		}}
		s := newSrv(t, graph)
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/Multi", nil), "id", "Multi")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		gj := resp["geojson"].(map[string]any)
		features := gj["features"].([]any)
		require.Len(t, features, 2)
	})

	t.Run("uses ADMIN as name when no name present", func(t *testing.T) {
		graph := &testutil.FakeGraphStore{ExecuteFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				testutil.FakeRecord([]string{"json", "props"}, []any{`{"type":"Point","coordinates":[0,0]}`, `{"ADMIN":"Zimbabwe"}`}),
			}}, nil
		}}
		s := newSrv(t, graph)
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/BritishEmpire", nil), "id", "BritishEmpire")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		feat := resp["geojson"].(map[string]any)["features"].([]any)[0].(map[string]any)
		props := feat["properties"].(map[string]any)
		assert.Equal(t, "Zimbabwe", props["name"])
	})

	t.Run("falls back to id when properties empty", func(t *testing.T) {
		graph := &testutil.FakeGraphStore{ExecuteFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				testutil.FakeRecord([]string{"json", "props"}, []any{`{"type":"Point","coordinates":[0,0]}`, `{}`}),
			}}, nil
		}}
		s := newSrv(t, graph)
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/England", nil), "id", "England")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		feat := resp["geojson"].(map[string]any)["features"].([]any)[0].(map[string]any)
		props := feat["properties"].(map[string]any)
		assert.Equal(t, "England", props["name"])
	})

	t.Run("query error", func(t *testing.T) {
		graph := &testutil.FakeGraphStore{ExecuteFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return nil, assert.AnError
		}}
		s := newSrv(t, graph)
		r := testutil.WithURLParam(httptest.NewRequest(http.MethodGet, "/geography/test", nil), "id", "test")
		w := httptest.NewRecorder()
		HandleGetGeography(s, w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
