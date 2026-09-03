package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminUserKey(t *testing.T) {
	t.Run("with subject", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), adminSubjectKey, "blog-admin")
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		k, err := adminUserKey(r)
		require.NoError(t, err)
		assert.Equal(t, "blog-admin", k)
	})
	t.Run("without subject", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		k, err := adminUserKey(r)
		require.NoError(t, err)
		assert.Equal(t, "unknown", k)
	})
}

func TestClientIPKey(t *testing.T) {
	var got string
	var gotErr error
	h := middleware.ClientIPFromRemoteAddr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, gotErr = clientIPKey(r)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)

	require.NoError(t, gotErr)
	assert.Equal(t, "192.0.2.1", got)
}

func TestClientIPKeyNoIP(t *testing.T) {
	var got string
	h := middleware.ClientIPFromRemoteAddr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = clientIPKey(r)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "not-an-ip"
	h.ServeHTTP(httptest.NewRecorder(), r)

	assert.Equal(t, "", got)
}

func TestRateLimitHandler(t *testing.T) {
	w := httptest.NewRecorder()
	rateLimitHandler(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.JSONEq(t, `{"error":"rate limit exceeded"}`, w.Body.String())
}
