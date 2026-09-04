package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBlogServer(t *testing.T) *Server {
	t.Helper()
	return &Server{logger: testLogger(t), db: newTestDB(t)}
}

func seedArticle(t *testing.T, db *sql.DB, slug, title, desc, content string) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO articles (slug, title, description, content) VALUES (?, ?, ?, ?)",
		slug, title, desc, content,
	)
	require.NoError(t, err)
}

func doRequest(t *testing.T, h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, reader)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Hello, World!", "hello-world"},
		{"What's Up?", "whats-up"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Numbers 123 OK", "numbers-123-ok"},
		{"Café & Crème", "café-crème"},
		{"...punctuation!!!", "punctuation"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, slugify(tt.in))
	}
}

func TestHandleCreateArticle(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/",
			`{"title":"Hello World","description":"A test","content":"Body"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"message":"Article created successfully"}`, w.Body.String())

		var slug string
		err := s.db.QueryRow("SELECT slug FROM articles WHERE title = 'Hello World'").Scan(&slug)
		require.NoError(t, err)
		assert.Equal(t, "hello-world", slug)
	})

	t.Run("invalid json", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/", `{bad`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing title", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/",
			`{"description":"d","content":"c"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"title is required"}`, w.Body.String())
	})

	t.Run("title too long", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/",
			`{"title":"`+strings.Repeat("a", maxTitleLength+1)+`","description":"d","content":"c"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("description too long", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/",
			`{"title":"t","description":"`+strings.Repeat("a", maxDescriptionLength+1)+`","content":"c"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("content too long", func(t *testing.T) {
		s := newBlogServer(t)
		w := doRequest(t, s.handleCreateArticle, http.MethodPost, "/",
			`{"title":"t","description":"d","content":"`+strings.Repeat("a", maxContentLength+1)+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleGetArticles(t *testing.T) {
	s := newBlogServer(t)
	seedArticle(t, s.db, "b", "B Title", "B desc", "B content")
	seedArticle(t, s.db, "a", "A Title", "A desc", "A content")
	// Give "a" a newer modified timestamp so ordering is deterministic.
	_, err := s.db.Exec("UPDATE articles SET modified = '2024-01-02 00:00:00' WHERE slug = 'a'")
	require.NoError(t, err)
	_, err = s.db.Exec("UPDATE articles SET modified = '2024-01-01 00:00:00' WHERE slug = 'b'")
	require.NoError(t, err)

	w := doRequest(t, s.handleGetArticles, http.MethodGet, "/", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var resp articlesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Articles, 2)
	assert.Equal(t, "a", resp.Articles[0].Slug) // most recently modified first
	assert.Equal(t, "b", resp.Articles[1].Slug)
}

func TestHandleGetArticleBySlug(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		s := newBlogServer(t)
		seedArticle(t, s.db, "my-slug", "My Title", "desc", "content")

		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "slug", "my-slug")
		w := httptest.NewRecorder()
		s.handleGetArticleBySlug(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var a article
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &a))
		assert.Equal(t, "my-slug", a.Slug)
		assert.Equal(t, "My Title", a.Title)
	})

	t.Run("not found", func(t *testing.T) {
		s := newBlogServer(t)
		r := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "slug", "nope")
		w := httptest.NewRecorder()
		s.handleGetArticleBySlug(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.JSONEq(t, `{"error":"article not found"}`, w.Body.String())
	})
}

func TestHandleUpdateArticleBySlug(t *testing.T) {
	t.Run("success partial", func(t *testing.T) {
		s := newBlogServer(t)
		seedArticle(t, s.db, "my-slug", "Old", "Old desc", "Old content")

		r := withURLParam(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"title":"New Title"}`)), "slug", "my-slug")
		w := httptest.NewRecorder()
		s.handleUpdateArticleBySlug(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var title string
		err := s.db.QueryRow("SELECT title FROM articles WHERE slug = 'my-slug'").Scan(&title)
		require.NoError(t, err)
		assert.Equal(t, "New Title", title)
	})

	t.Run("no fields", func(t *testing.T) {
		s := newBlogServer(t)
		seedArticle(t, s.db, "my-slug", "Old", "Old desc", "Old content")

		r := withURLParam(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{}`)), "slug", "my-slug")
		w := httptest.NewRecorder()
		s.handleUpdateArticleBySlug(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":"No fields provided for update"}`, w.Body.String())
	})
}

func TestHandleDeleteArticleBySlug(t *testing.T) {
	s := newBlogServer(t)
	seedArticle(t, s.db, "my-slug", "Title", "desc", "content")

	r := withURLParam(httptest.NewRequest(http.MethodDelete, "/", nil), "slug", "my-slug")
	w := httptest.NewRecorder()
	s.handleDeleteArticleBySlug(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM articles WHERE slug = 'my-slug'").Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count)
}
