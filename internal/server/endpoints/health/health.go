package health

import (
	"context"
	"github.com/jmuszka/larynx/internal/server/endpoints"
	"net/http"
	"time"
)

type healthResponse struct {
	Version  string            `json:"version"`
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

// HandleHealth godoc
// @Summary      Health check
// @Description  Returns the server version and the status of its services.
// @Tags         health
// @Produce      json
// @Success      200  {object}  healthResponse
// @Failure      503  {object}  healthResponse
// @Security     BearerAuth
// @Router       /health [get]
func HandleHealth(s *endpoints.Server, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	services := map[string]string{
		"server":   "ok",
		"database": "ok",
	}

	if err := s.Graph.VerifyConnectivity(ctx); err != nil {
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

	s.WriteJSON(w, status, healthResponse{
		Version:  s.Version,
		Status:   overall,
		Services: services,
	})
}
