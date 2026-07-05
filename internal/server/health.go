package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Version  string            `json:"version"`
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

func (s *Server) healthRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", s.handleHealth)
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	services := map[string]string{
		"server":   "ok",
		"database": "ok",
	}

	if err := s.driver.VerifyConnectivity(ctx); err != nil {
		services["database"] = "error"
	}

	overall := "ok"
	for _, status := range services {
		if status != "ok" {
			overall = "degraded"
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if overall != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(healthResponse{
		Version:  s.version,
		Status:   overall,
		Services: services,
	})
}
