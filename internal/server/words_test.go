package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmuszka/larynx/internal/ai"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanEtymologyText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips html tags",
			in:   `<div class="word__defination">from Old French <em>test</em></div>`,
			want: "from Old French test",
		},
		{
			name: "decodes entities",
			in:   "Old English &amp; Middle English &#39;test&#39;",
			want: "Old English & Middle English 'test'",
		},
		{
			name: "collapses whitespace",
			in:   "line one\n\nline two\t\tline three",
			want: "line one line two line three",
		},
		{
			name: "removes ad noise",
			in:   "origin text Advertisement Remove Ads more text",
			want: "origin text more text",
		},
		{
			name: "preserves content",
			in:   "Borrowed from Latin testum meaning earthen pot.",
			want: "Borrowed from Latin testum meaning earthen pot.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanEtymologyText(tt.in))
		})
	}
}

func TestBuildFamilyTree(t *testing.T) {
	t.Run("single lineage", func(t *testing.T) {
		root := buildFamilyTree([][]familyRef{
			{
				{name: "Indo-European", glottocode: "indo1319"},
				{name: "Germanic", glottocode: "germ1287"},
			},
		})
		require.Equal(t, "root", root.ID)
		require.Len(t, root.Children, 1)
		assert.Equal(t, "Indo-European", root.Children[0].ID)
		assert.Equal(t, "indo1319", root.Children[0].Glottocode)
		assert.Equal(t, 1, root.Children[0].Value)
		require.Len(t, root.Children[0].Children, 1)
		assert.Equal(t, "Germanic", root.Children[0].Children[0].ID)
		assert.Equal(t, "germ1287", root.Children[0].Children[0].Glottocode)
		assert.Equal(t, 1, root.Children[0].Children[0].Value)
	})

	t.Run("merges shared prefix", func(t *testing.T) {
		root := buildFamilyTree([][]familyRef{
			{
				{name: "Indo-European", glottocode: "indo1319"},
				{name: "Germanic", glottocode: "germ1287"},
				{name: "West"},
			},
			{
				{name: "Indo-European", glottocode: "indo1319"},
				{name: "Germanic", glottocode: "germ1287"},
				{name: "North"},
			},
		})
		require.Len(t, root.Children, 1)
		assert.Equal(t, 2, root.Value)
		assert.Equal(t, 2, root.Children[0].Value)
		require.Len(t, root.Children[0].Children, 1)
		require.Len(t, root.Children[0].Children[0].Children, 2)
	})

	t.Run("fills in a missing code on a shared node", func(t *testing.T) {
		root := buildFamilyTree([][]familyRef{
			{{name: "Indo-European"}, {name: "Germanic", glottocode: "germ1287"}},
			{{name: "Indo-European", glottocode: "indo1319"}, {name: "Germanic", glottocode: "germ1287"}},
		})
		require.Len(t, root.Children, 1)
		assert.Equal(t, "indo1319", root.Children[0].Glottocode)
	})

	t.Run("empty lineages ignored", func(t *testing.T) {
		root := buildFamilyTree([][]familyRef{{}, {{name: "A"}}})
		require.Len(t, root.Children, 1)
		assert.Equal(t, "A", root.Children[0].ID)
	})
}

func TestFamilyDisplayName(t *testing.T) {
	assert.Equal(t, "Indo-European", familyDisplayName("Indo-European [indo1319]"))
	assert.Equal(t, "Indo-European", familyDisplayName("Indo-European"))
	assert.Equal(t, "Indo-European", familyDisplayName("'Indo-European [indo1319]'"))
	assert.Equal(t, "", familyDisplayName(""))
}

func TestFamilyMetadata(t *testing.T) {
	t.Run("map chain with glottocodes", func(t *testing.T) {
		family, code, ancestors := familyMetadata([]interface{}{
			map[string]any{"name": "Indo-European", "glottocode": "indo1319"},
			map[string]any{"name": "Germanic", "glottocode": "germ1287"},
			map[string]any{"name": "Anglic", "glottocode": "angc1293"},
		})
		assert.Equal(t, "Anglic", family)
		assert.Equal(t, "angc1293", code)
		assert.Equal(t, "|indo1319|germ1287|angc1293|", ancestors)
	})

	t.Run("legacy string chain", func(t *testing.T) {
		family, code, ancestors := familyMetadata([]interface{}{
			"Indo-European [indo1319]",
			"'Germanic [germ1287]'",
			"Anglic [angc1293]",
		})
		assert.Equal(t, "Anglic", family)
		assert.Equal(t, "angc1293", code)
		assert.Equal(t, "|indo1319|germ1287|angc1293|", ancestors)
	})

	t.Run("names without codes", func(t *testing.T) {
		family, code, ancestors := familyMetadata([]interface{}{
			map[string]any{"name": "Indo-European", "glottocode": nil},
			map[string]any{"name": "Germanic", "glottocode": ""},
		})
		assert.Equal(t, "Germanic", family)
		assert.Equal(t, "", code)
		assert.Equal(t, "", ancestors)
	})

	t.Run("empty chain", func(t *testing.T) {
		family, code, ancestors := familyMetadata(nil)
		assert.Equal(t, "", family)
		assert.Equal(t, "", code)
		assert.Equal(t, "", ancestors)
	})
}

func TestUnescapeParam(t *testing.T) {
	r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "word", "caf%C3%A9")
	assert.Equal(t, "café", unescapeParam(r, "word"))
}

func newEtymologyGraph(t *testing.T) *fakeGraphStore {
	t.Helper()
	calls := 0
	return &fakeGraphStore{
		executeFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			calls++
			switch calls {
			case 1:
				return &neo4j.EagerResult{Records: []*neo4j.Record{
					fakeRecord([]string{"path"}, []any{neo4j.Path{Nodes: []neo4j.Node{
						{Props: map[string]any{"lang": "English", "ipa": "/wɜːd/"}},
					}}}),
				}}, nil
			case 2:
				return &neo4j.EagerResult{Records: []*neo4j.Record{
					fakeRecord([]string{"lineage"}, []any{[]any{
						map[string]any{"name": "Indo-European", "glottocode": "indo1319"},
						map[string]any{"name": "Germanic", "glottocode": "germ1287"},
					}}),
				}}, nil
			case 3:
				return &neo4j.EagerResult{Records: []*neo4j.Record{
					fakeRecord(
						[]string{"id", "name", "json", "count", "chain"},
						[]any{
							"olde1238",
							"Old English (ca. 450-1100)",
							`{"type":"Point","coordinates":[0,0]}`,
							int64(3),
							[]any{
								map[string]any{"name": "Indo-European", "glottocode": "indo1319"},
								map[string]any{"name": "Germanic", "glottocode": "germ1287"},
								map[string]any{"name": "Anglic", "glottocode": "angc1293"},
							},
						},
					),
				}}, nil
			}
			return &neo4j.EagerResult{}, nil
		},
	}
}

func TestHandleGetEtymology(t *testing.T) {
	newSrv := func(t *testing.T, graph graphStore) *Server {
		return &Server{logger: testLogger(t), graph: graph, cache: newServerCache(t)}
	}

	t.Run("missing word", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "word", "")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"word is required"}`, w.Body.String())
	})

	t.Run("word too long", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "word", strings.Repeat("a", maxWordLength+1))
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("lang too long", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/?lang="+strings.Repeat("a", maxLangLength+1), nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cache hit", func(t *testing.T) {
		graph := &fakeGraphStore{}
		s := newSrv(t, graph)
		require.NoError(t, s.cache.Set(t.Context(), "/words/test/etymology", `{"cached":true}`, 0))

		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/etymology", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"cached":true}`, w.Body.String())
		assert.Empty(t, graph.queries)
	})

	t.Run("word not found", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/bluetooth/etymology", nil), "word", "bluetooth")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"word not found"}`, w.Body.String())
	})

	t.Run("success with geojson", func(t *testing.T) {
		graph := newEtymologyGraph(t)
		s := newSrv(t, graph)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/etymology", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "graph")
		assert.Contains(t, resp, "familyTree")
		assert.Equal(t, "/wɜːd/", resp["ipa"])

		ft, ok := resp["familyTree"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "root", ft["name"])

		children, ok := ft["children"].([]any)
		require.True(t, ok)
		require.Len(t, children, 1)
		ie, ok := children[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Indo-European", ie["name"])
		assert.Equal(t, "indo1319", ie["glottocode"])

		gj, ok := resp["geojson"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "FeatureCollection", gj["type"])

		feat := gj["features"].([]any)[0].(map[string]any)
		props := feat["properties"].(map[string]any)
		assert.Equal(t, "olde1238", props["id"])
		assert.Equal(t, "Old English (ca. 450-1100)", props["name"])
		assert.Equal(t, float64(3), props["count"])
		assert.Equal(t, "Old English (ca. 450-1100)", props["lang"])
		assert.Equal(t, "Anglic", props["family"])
		assert.Equal(t, "angc1293", props["familyCode"])
		assert.Equal(t, "|indo1319|germ1287|angc1293|", props["ancestors"])
	})

	t.Run("skip geojson", func(t *testing.T) {
		graph := newEtymologyGraph(t)
		s := newSrv(t, graph)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/etymology?geojson=false", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp["geojson"])
	})

	t.Run("skip family", func(t *testing.T) {
		graph := newEtymologyGraph(t)
		s := newSrv(t, graph)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/etymology?family=false", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp["familyTree"])
	})

	t.Run("query error", func(t *testing.T) {
		graph := &fakeGraphStore{executeFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return nil, assert.AnError
		}}
		s := newSrv(t, graph)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/etymology", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetEtymology(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleSearchWords(t *testing.T) {
	newSrv := func(t *testing.T, graph graphStore) *Server {
		return &Server{logger: testLogger(t), graph: graph, cache: newServerCache(t)}
	}

	t.Run("missing prefix", func(t *testing.T) {
		s := newSrv(t, &fakeGraphStore{})
		w := httptest.NewRecorder()
		s.handleSearchWords(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"prefix is required"}`, w.Body.String())
	})

	t.Run("success", func(t *testing.T) {
		graph := &fakeGraphStore{executeFn: func(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
			return &neo4j.EagerResult{Records: []*neo4j.Record{
				fakeRecord([]string{"term"}, []any{"cat"}),
				fakeRecord([]string{"term"}, []any{"catapult"}),
			}}, nil
		}}
		s := newSrv(t, graph)
		w := httptest.NewRecorder()
		s.handleSearchWords(w, httptest.NewRequest(http.MethodGet, "/?prefix=cat", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		var terms []string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &terms))
		assert.Equal(t, []string{"cat", "catapult"}, terms)
	})
}

func chatCompletionServer(t *testing.T, content string, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": "stop"}},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "boom"}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestHandleGetHistory(t *testing.T) {
	newSrv := func(t *testing.T, baseURL, aiURL string) *Server {
		s := &Server{logger: testLogger(t), cache: newServerCache(t), httpClient: &http.Client{}}
		s.cfg.EtymologyBaseURL = baseURL
		if aiURL != "" {
			svc, err := ai.New(ai.Config{BaseURL: aiURL, Model: "test"})
			require.NoError(t, err)
			s.ai = svc
		}
		return s
	}

	t.Run("missing word", func(t *testing.T) {
		s := newSrv(t, "http://x", "")
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "word", "")
		w := httptest.NewRecorder()
		s.handleGetHistory(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"word is required"}`, w.Body.String())
	})

	t.Run("non-english lang", func(t *testing.T) {
		s := newSrv(t, "http://x", "")
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/?lang=French", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetHistory(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html><body><section><h2 class="font-serif"><span lang="en">test</span></h2><p>Some etymology text.</p></section></body></html>`))
		}))
		t.Cleanup(src.Close)

		s := newSrv(t, src.URL, chatCompletionServer(t, "Test origin sentence.", http.StatusOK))
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/history", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetHistory(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "test", resp["word"])
		assert.Equal(t, "Test origin sentence.", resp["history"])
	})

	t.Run("ai error fallback", func(t *testing.T) {
		src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html><body><section><h2 class="font-serif"><span lang="en">test</span></h2><p>Some etymology text.</p></section></body></html>`))
		}))
		t.Cleanup(src.Close)

		s := newSrv(t, src.URL, chatCompletionServer(t, "", http.StatusInternalServerError))
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/words/test/history", nil), "word", "test")
		w := httptest.NewRecorder()
		s.handleGetHistory(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "test", resp["word"])
		assert.Contains(t, resp, "results")
	})
}
