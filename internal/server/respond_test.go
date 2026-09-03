package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{logger: testLogger(t)}
}

func TestWriteJSON(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"key":"value"}`, w.Body.String())
}

func TestWriteJSONError(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.writeJSONError(w, http.StatusBadRequest, "bad input")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"bad input"}`, w.Body.String())
}

func TestUnauthorized(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.unauthorized(w)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, w.Body.String())
}

func TestAdminUnauthorized(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.adminUnauthorized(w)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.JSONEq(t, `{"error":"invalid or missing admin token"}`, w.Body.String())
}
