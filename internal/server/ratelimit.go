package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

const (
	rateLimitWindow           = time.Minute
	rateLimitDefaultPerIP     = 1000
	rateLimitEtymologyPerIP   = 20
	rateLimitHistoryPerIP     = 20
	rateLimitBlogWritePerUser = 20
)

func clientIPKey(r *http.Request) (string, error) {
	return httprate.CanonicalizeIP(middleware.GetClientIP(r.Context())), nil
}

func adminUserKey(r *http.Request) (string, error) {
	sub, _ := r.Context().Value(adminSubjectKey).(string)
	if sub == "" {
		sub = "unknown"
	}
	return sub, nil
}

func rateLimitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
}
