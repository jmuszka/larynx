package server

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const bearerPrefix = "Bearer "

type contextKey int

const adminSubjectKey contextKey = iota

func (s *Server) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, bearerPrefix) {
			s.unauthorized(w)
			return
		}

		token := strings.TrimPrefix(authz, bearerPrefix)
		for _, allowed := range s.cfg.BearerTokens {
			if subtle.ConstantTimeCompare([]byte(token), []byte(allowed)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		s.unauthorized(w)
	})
}

func (s *Server) unauthorized(w http.ResponseWriter) {
	s.writeJSONError(w, http.StatusUnauthorized, "unauthorized")
}

func (s *Server) adminUnauthorized(w http.ResponseWriter) {
	s.writeJSONError(w, http.StatusUnauthorized, "invalid or missing admin token")
}

func (s *Server) adminJWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := strings.TrimSpace(r.Header.Get("X-Admin-JWT"))
		if tokenStr == "" {
			s.adminUnauthorized(w)
			return
		}

		token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{},
			func(t *jwt.Token) (any, error) {
				return []byte(s.cfg.AdminJWTSecret), nil
			},
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !token.Valid {
			s.adminUnauthorized(w)
			return
		}

		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok || claims.Subject != s.cfg.AdminJWTSubject {
			s.adminUnauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), adminSubjectKey, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
