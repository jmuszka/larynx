package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleHealth(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		s := &Server{logger: testLogger(t), graph: &fakeGraphStore{}, version: "preview"}
		w := httptest.NewRecorder()
		s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		var resp healthResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "preview", resp.Version)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "ok", resp.Services["database"])
		assert.Equal(t, "ok", resp.Services["server"])
	})

	t.Run("degraded", func(t *testing.T) {
		s := &Server{logger: testLogger(t), graph: &fakeGraphStore{connErr: errors.New("down")}, version: "preview"}
		w := httptest.NewRecorder()
		s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		var resp healthResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "degraded", resp.Status)
		assert.Equal(t, "error", resp.Services["database"])
	})
}
