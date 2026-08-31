package server

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v to the response as JSON with the given status code.
// Any encoding error is logged since the status code can no longer change.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

// writeJSONError writes a JSON error payload in the shape {"error": "<msg>"}
// with the given status code.
func (s *Server) writeJSONError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}
