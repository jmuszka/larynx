package endpoints

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes v to the response as JSON with the given status code.
// Any encoding error is logged since the status code can no longer change.
func (s *Server) WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.Logger.Error("failed to encode response", "error", err)
	}
}

// WriteJSONError writes a JSON error payload in the shape {"error": "<msg>"}
// with the given status code.
func (s *Server) WriteJSONError(w http.ResponseWriter, status int, msg string) {
	s.WriteJSON(w, status, map[string]string{"error": msg})
}

// WriteRawJSON writes pre-encoded JSON bytes (e.g. a cached response) with the
// given status code, so cached payloads go through the same Content-Type and
// status handling as freshly encoded responses.
func (s *Server) WriteRawJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		s.Logger.Error("failed to write response", "error", err)
	}
}
