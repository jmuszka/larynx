package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("missing BaseURL", func(t *testing.T) {
		_, err := New(Config{Model: "m"})
		assert.Error(t, err)
	})
	t.Run("missing Model", func(t *testing.T) {
		_, err := New(Config{BaseURL: "http://localhost"})
		assert.Error(t, err)
	})
	t.Run("default http client", func(t *testing.T) {
		s, err := New(Config{BaseURL: "http://localhost", Model: "m"})
		require.NoError(t, err)
		assert.NotNil(t, s.client)
	})
	t.Run("custom http client", func(t *testing.T) {
		c := &http.Client{}
		s, err := New(Config{BaseURL: "http://localhost", Model: "m", HTTPClient: c})
		require.NoError(t, err)
		assert.Same(t, c, s.client)
	})
}

type mockAI struct {
	server      *httptest.Server
	requests    int32
	lastAuth    string
	lastBody    chatRequest
	responses   []string // JSON bodies to serve in order
	statusCodes []int
}

func newMockAI(t *testing.T, handler http.HandlerFunc) *mockAI {
	m := &mockAI{server: httptest.NewServer(handler)}
	t.Cleanup(m.server.Close)
	return m
}

func chatBody(choices []map[string]any, apiErr string) string {
	resp := map[string]any{"choices": choices}
	if apiErr != "" {
		resp["error"] = map[string]string{"message": apiErr}
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func choice(content, finish string) map[string]any {
	return map[string]any{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": finish}
}

func TestPrompt(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotAuth string
		var gotBody chatRequest
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			gotAuth = r.Header.Get("Authorization")
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(chatBody([]map[string]any{choice("hello world", "stop")}, "")))
		})

		s, err := New(Config{BaseURL: m.server.URL + "/", Model: "test-model", APIKey: "secret"})
		require.NoError(t, err)

		got, err := s.Prompt(context.Background(), "say hi")
		require.NoError(t, err)
		assert.Equal(t, "hello world", got)
		assert.Equal(t, "Bearer secret", gotAuth)
		assert.Equal(t, "test-model", gotBody.Model)
		assert.Equal(t, "user", gotBody.Messages[0].Role)
		assert.Equal(t, "say hi", gotBody.Messages[0].Content)
	})

	t.Run("non-200", func(t *testing.T) {
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		})
		s, _ := New(Config{BaseURL: m.server.URL, Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.ErrorContains(t, err, "HTTP 500")
	})

	t.Run("api error", func(t *testing.T) {
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(chatBody(nil, "bad request")))
		})
		s, _ := New(Config{BaseURL: m.server.URL, Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.ErrorContains(t, err, "API error: bad request")
	})

	t.Run("no choices", func(t *testing.T) {
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(chatBody(nil, "")))
		})
		s, _ := New(Config{BaseURL: m.server.URL, Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.ErrorContains(t, err, "no choices")
	})

	t.Run("continuation on length", func(t *testing.T) {
		var n atomic.Int32
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			var body chatRequest
			json.NewDecoder(r.Body).Decode(&body)
			if n.Add(1) == 1 {
				assert.Len(t, body.Messages, 1)
				w.Write([]byte(chatBody([]map[string]any{choice("part1", "length")}, "")))
				return
			}
			assert.Len(t, body.Messages, 2)
			assert.Equal(t, "assistant", body.Messages[1].Role)
			assert.Equal(t, "part1", body.Messages[1].Content)
			w.Write([]byte(chatBody([]map[string]any{choice("part2", "stop")}, "")))
		})
		s, _ := New(Config{BaseURL: m.server.URL, Model: "m"})
		got, err := s.Prompt(context.Background(), "x")
		require.NoError(t, err)
		assert.Equal(t, "part1part2", got)
	})

	t.Run("exceeds max continuations", func(t *testing.T) {
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(chatBody([]map[string]any{choice("chunk", "length")}, "")))
		})
		s, _ := New(Config{BaseURL: m.server.URL, Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.ErrorContains(t, err, "continuations")
	})

	t.Run("invalid base url", func(t *testing.T) {
		s, _ := New(Config{BaseURL: "://bad", Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.Error(t, err)
	})

	t.Run("server unreachable", func(t *testing.T) {
		s, _ := New(Config{BaseURL: "http://127.0.0.1:1", Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		assert.Error(t, err)
	})
}

func TestPromptURLNormalization(t *testing.T) {
	t.Run("already has /v1", func(t *testing.T) {
		m := newMockAI(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			w.Write([]byte(chatBody([]map[string]any{choice("ok", "stop")}, "")))
		})
		s, _ := New(Config{BaseURL: m.server.URL + "/v1", Model: "m"})
		_, err := s.Prompt(context.Background(), "x")
		require.NoError(t, err)
	})
}
