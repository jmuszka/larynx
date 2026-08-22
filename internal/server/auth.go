package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const bearerPrefix = "Bearer "

func (s *Server) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, bearerPrefix) {
			unauthorized(w)
			return
		}

		token := strings.TrimPrefix(authz, bearerPrefix)
		for _, allowed := range s.cfg.BearerTokens {
			if subtle.ConstantTimeCompare([]byte(token), []byte(allowed)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		unauthorized(w)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

func adminUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "invalid or missing admin token"})
}

func (s *Server) adminJWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := strings.TrimSpace(r.Header.Get("X-Admin-JWT"))
		if tokenStr == "" {
			adminUnauthorized(w)
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
			adminUnauthorized(w)
			return
		}

		claims, ok := token.Claims.(*jwt.RegisteredClaims)
		if !ok || claims.Subject != s.cfg.AdminJWTSubject {
			adminUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}
