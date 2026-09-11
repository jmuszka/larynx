package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmuszka/larynx/internal/server/endpoints"
	"github.com/jmuszka/larynx/internal/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleHealth(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		s := endpoints.New(endpoints.Config{Logger: testutil.TestLogger(t), Graph: &testutil.FakeGraphStore{}, Version: "preview"})
		w := httptest.NewRecorder()
		HandleHealth(s, w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		var resp healthResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "preview", resp.Version)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "ok", resp.Services["database"])
		assert.Equal(t, "ok", resp.Services["server"])
	})

	t.Run("degraded", func(t *testing.T) {
		s := endpoints.New(endpoints.Config{Logger: testutil.TestLogger(t), Graph: &testutil.FakeGraphStore{ConnErr: errors.New("down")}, Version: "preview"})
		w := httptest.NewRecorder()
		HandleHealth(s, w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		var resp healthResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "degraded", resp.Status)
		assert.Equal(t, "error", resp.Services["database"])
	})
}
