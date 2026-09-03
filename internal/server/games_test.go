package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGamesRouterEmpty(t *testing.T) {
	s := &Server{logger: testLogger(t)}
	h := s.gamesRouter()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/guess/word", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGameStubs(t *testing.T) {
	s := &Server{logger: testLogger(t)}
	stubs := map[string]http.HandlerFunc{
		"history":   s.handleGuessHistory,
		"word":      s.handleGuessWord,
		"ancestors": s.handleGuessAncestors,
		"tree":      s.handleGuessTree,
	}
	for name, h := range stubs {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h(w, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, `{"status":"Not implemented"}`, w.Body.String())
		})
	}
}
