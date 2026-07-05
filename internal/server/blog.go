package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) blogRouter() http.Handler {
	r := chi.NewRouter()

	// r.Get("/articles", s.handleGetArticles)
	// r.Post("/articles/create", s.handleCreateArticle)
	// r.Get("/articles/{slug}", s.handleGetArticleBySlug)
	// r.Patch("/articles/{slug}", s.handleUpdateArticleBySlug)
	// r.Delete("/articles/{slug}", s.handleDeleteArticleBySlug)

	return r
}

// TODO: implement
func (s *Server) handleGetArticles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleCreateArticle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleGetArticleBySlug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleUpdateArticleBySlug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}

// TODO: implement
func (s *Server) handleDeleteArticleBySlug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Not implemented",
	})
}
