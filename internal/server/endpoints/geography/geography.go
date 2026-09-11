package geography

import (
	"encoding/json"
	"fmt"
	"github.com/jmuszka/larynx/internal/server/endpoints"
	"net/http"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const (
	maxGeographyNameLength = 200
	// geographyWeight is the fill intensity (a "count" in the response) the
	// frontend uses to shade geography polygons. Higher values render darker.
	geographyWeight = 3
)

type geographyResponse struct {
	GeoJSON endpoints.GeoJSON `json:"geojson"`
}

// HandleGetGeography godoc
// @Summary      Get geography by name
// @Description  Returns a GeoJSON FeatureCollection for every Geography node whose name matches the id.
// @Tags         geography
// @Produce      json
// @Param        id   path      string  true  "The geography name to look up"
// @Success      200  {object}  geographyResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /geography/{id} [get]
func HandleGetGeography(s *endpoints.Server, w http.ResponseWriter, r *http.Request) {
	id := endpoints.UnescapeParam(r, "id")
	if len(id) == 0 {
		s.WriteJSONError(w, http.StatusBadRequest, "ID is required")
		return
	}
	if len(id) > maxGeographyNameLength {
		s.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("ID exceeds maximum length of %d characters", maxGeographyNameLength))
		return
	}

	// Check if response exists in cache
	val, err := s.Cache.Get(r.Context(), r.RequestURI)
	if err == nil {
		s.WriteRawJSON(w, http.StatusOK, []byte(val))
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
	s.Logger.Debug("CYPHER: " + endpoints.RenderCypher(cypher, params))
	result, err := s.Graph.ExecuteQuery(r.Context(), cypher,
		params, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		s.Logger.Error("failed to execute geography query", "error", err)
		s.WriteJSONError(w, http.StatusInternalServerError, "Failed to execute query")
		return
	}

	if len(result.Records) == 0 {
		s.WriteJSONError(w, http.StatusNotFound, "Geography not found")
		return
	}

	features := make([]endpoints.Feature, 0, len(result.Records))
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

		features = append(features, endpoints.Feature{
			Type:       "Feature",
			Properties: props,
			Geometry:   geometry,
		})
	}

	response := geographyResponse{
		GeoJSON: endpoints.GeoJSON{
			Type:     "FeatureCollection",
			Features: features,
		},
	}

	// Write to cache so that future queries are quick
	encoded, err := json.Marshal(response)
	if err != nil {
		s.Logger.Error("failed to marshal response", "error", err)
		s.WriteJSONError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
	s.WriteRawJSON(w, http.StatusOK, encoded)
	s.Cache.Set(r.Context(), r.RequestURI, string(encoded), 0)
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
