package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signAdminJWT(t *testing.T, secret, subject string, opts ...func(*jwt.RegisteredClaims)) string {
	t.Helper()
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}
	for _, o := range opts {
		o(&claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestBearerAuth(t *testing.T) {
	s := &Server{logger: testLogger(t), cfg: Config{BearerTokens: []string{"token-a", "token-b"}}}

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "missing", header: "", wantStatus: http.StatusUnauthorized},
		{name: "wrong prefix", header: "Token abc", wantStatus: http.StatusUnauthorized},
		{name: "unknown token", header: "Bearer wrong", wantStatus: http.StatusUnauthorized},
		{name: "valid first", header: "Bearer token-a", wantStatus: http.StatusOK},
		{name: "valid second", header: "Bearer token-b", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			s.bearerAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestAdminJWTAuth(t *testing.T) {
	const secret = "test-secret"
	const subject = "blog-admin"

	newServer := func() *Server {
		return &Server{logger: testLogger(t), cfg: Config{AdminJWTSecret: secret, AdminJWTSubject: subject}}
	}

	t.Run("missing token", func(t *testing.T) {
		s := newServer()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		s.adminJWTAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		s := newServer()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Admin-JWT", "not-a-jwt")
		w := httptest.NewRecorder()
		s.adminJWTAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong secret", func(t *testing.T) {
		s := newServer()
		tok := signAdminJWT(t, "different-secret", subject)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Admin-JWT", tok)
		w := httptest.NewRecorder()
		s.adminJWTAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong subject", func(t *testing.T) {
		s := newServer()
		tok := signAdminJWT(t, secret, "someone-else")
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Admin-JWT", tok)
		w := httptest.NewRecorder()
		s.adminJWTAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("expired token", func(t *testing.T) {
		s := newServer()
		tok := signAdminJWT(t, secret, subject, func(c *jwt.RegisteredClaims) {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Admin-JWT", tok)
		w := httptest.NewRecorder()
		s.adminJWTAuth(http.HandlerFunc(okHandler)).ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("valid token sets context", func(t *testing.T) {
		s := newServer()
		tok := signAdminJWT(t, secret, subject)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Admin-JWT", tok)

		var gotSub string
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotSub, _ = r.Context().Value(adminSubjectKey).(string)
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		s.adminJWTAuth(h).ServeHTTP(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, subject, gotSub)
	})
}
