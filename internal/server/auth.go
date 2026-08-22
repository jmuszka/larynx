package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
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
