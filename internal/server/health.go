package server

import (
	"context"
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

// handleHealth godoc
// @Summary      Health check
// @Description  Returns the server version and the status of its services.
// @Tags         health
// @Produce      json
// @Success      200  {object}  healthResponse
// @Failure      503  {object}  healthResponse
// @Security     BearerAuth
// @Router       /health [get]
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	services := map[string]string{
		"server":   "ok",
		"database": "ok",
	}

	if err := s.graph.VerifyConnectivity(ctx); err != nil {
		services["database"] = "error"
	}

	overall := "ok"
	for _, status := range services {
		if status != "ok" {
			overall = "degraded"
			break
		}
	}

	status := http.StatusOK
	if overall != "ok" {
		status = http.StatusServiceUnavailable
	}

	s.writeJSON(w, status, healthResponse{
		Version:  s.version,
		Status:   overall,
		Services: services,
	})
}
