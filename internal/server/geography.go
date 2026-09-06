package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const (
	maxGeographyNameLength = 200
	// geographyWeight is the fill intensity (a "count" in the response) the
	// frontend uses to shade geography polygons. Higher values render darker.
	geographyWeight = 3
)

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
// @Description  Returns a GeoJSON FeatureCollection for every Geography node whose name matches the id.
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

	// A geography is split across many nodes, one per polygon/multipolygon.
	// Collect them all and emit each geometry as its own feature, labelled with
	// the polygon's own name from its stored properties (not the geography name).
	cypher := `
		MATCH (g:Geography {name: $id})
		RETURN g.geometryJSON AS json, g.properties AS props
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

	features := make([]feature, 0, len(result.Records))
	for _, record := range result.Records {
		geometryJSON, _ := record.Get("json")
		geometryStr, _ := geometryJSON.(string)

		var geometry map[string]any
		if err := json.Unmarshal([]byte(geometryStr), &geometry); err != nil {
			continue
		}

		props := map[string]any{}
		if propsJSON, _ := record.Get("props"); propsJSON != nil {
			if propsStr, ok := propsJSON.(string); ok {
				_ = json.Unmarshal([]byte(propsStr), &props)
			}
		}

		props["name"] = polygonName(props, id)
		props["count"] = geographyWeight

		features = append(features, feature{
			Type:       "Feature",
			Properties: props,
			Geometry:   geometry,
		})
	}

	response := geographyResponse{
		GeoJSON: geoJSON{
			Type:     "FeatureCollection",
			Features: features,
		},
	}

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

// polygonName resolves a polygon's display name from its source properties,
// falling back to the geography name when no name is present.
func polygonName(props map[string]any, fallback string) string {
	if v, ok := props["name"].(string); ok && v != "" {
		return v
	}
	if v, ok := props["ADMIN"].(string); ok && v != "" {
		return v
	}
	return fallback
}
