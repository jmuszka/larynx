package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) gamesRouter() http.Handler {
	r := chi.NewRouter()

	// r.Get("/guess/history", s.handleGuessHistory)
	// r.Get("/guess/word", s.handleGuessWord)
	// r.Get("/guess/ancestors", s.handleGuessAncestors)
	// r.Get("/guess/tree", s.handleGuessTree)

	return r
}

// TODO: implement
func (s *Server) handleGuessHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGuessWord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGuessAncestors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGuessTree(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}
