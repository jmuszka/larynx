package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const maxGeographyNameLength = 200

type geographyResponse struct {
	GeoJSON geoJSON `json:"geojson"`
}

func (s *Server) geographyRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/{id}", s.handleGetGeography)
	return r
}

// handleGetGeography godoc
// @Summary      Get geography by name
// @Description  Returns a GeoJSON FeatureCollection for the Geography node whose name matches the id.
// @Tags         geography
// @Produce      json
// @Param        id   path      string  true  "The geography name to look up"
// @Success      200  {object}  geographyResponse
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geography/{id} [get]
func (s *Server) handleGetGeography(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if response exists in cache
	val, err := s.cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		w.Write([]byte(val))
		return
	}

	id := unescapeParam(r, "id")
	if len(id) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "id is required")
		return
	}
	if len(id) > maxGeographyNameLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("id exceeds maximum length of %d characters", maxGeographyNameLength))
		return
	}

	cypher := `
		MATCH (g:Geography {name: $id})
		RETURN g.geometryJSON AS json
	`
	params := map[string]any{"id": id}
	s.logger.Debug("CYPHER: " + renderCypher(cypher, params))
	result, err := s.graph.ExecuteQuery(r.Context(), cypher,
		params, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.logger.Error("failed to execute geography query", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to execute query")
		return
	}

	if len(result.Records) == 0 {
		s.writeJSONError(w, http.StatusNotFound, "geography not found")
		return
	}

	// geometryJSON stores the full FeatureCollection for the geography.
	var fc geoJSON
	for _, record := range result.Records {
		geometryJSON, _ := record.Get("json")
		geometryStr, _ := geometryJSON.(string)

		if err := json.Unmarshal([]byte(geometryStr), &fc); err != nil {
			s.logger.Error("failed to parse geography geometry", "error", err)
			s.writeJSONError(w, http.StatusInternalServerError, "failed to parse geometry")
			return
		}
		break
	}

	response := geographyResponse{GeoJSON: fc}

	// Write to cache so that future queries are quick
	encoded, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	w.Write(encoded)
	s.cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
}
