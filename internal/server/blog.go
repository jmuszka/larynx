package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) blogRouter() http.Handler {
	r := chi.NewRouter()

	r.Get("/articles", s.handleGetArticles)
	r.Post("/articles/create", s.handleCreateArticle)
	r.Get("/articles/{slug}", s.handleGetArticleBySlug)
	// r.Patch("/articles/{slug}", s.handleUpdateArticleBySlug)
	// r.Delete("/articles/{slug}", s.handleDeleteArticleBySlug)

	return r
}

func (s *Server) handleGetArticles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type Article struct {
		Slug        string `json:"slug"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Published   string `json:"published"` // Adjust to time.Time if your driver parses dates
		Modified    string `json:"modified"`
	}

	// Retrieve blogposts
	rows, err := s.db.Query("SELECT slug, title, description, published, modified FROM articles ORDER BY modified DESC")
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	// Parse results
	articles := []Article{}
	for rows.Next() {
		var a Article
		// Scan targets MUST match the order of columns in SELECT statement
		err := rows.Scan(&a.Slug, &a.Title, &a.Description, &a.Published, &a.Modified)
		if err != nil {
			http.Error(w, `{"error": "Failed to process data"}`, http.StatusInternalServerError)
			log.Printf("Row scan failed: %v", err)
			return
		}
		articles = append(articles, a)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, `{"error": "Database cursor error"}`, http.StatusInternalServerError)
		log.Printf("Iteration error: %v", err)
		return
	}

	json.NewEncoder(w).Encode(map[string][]Article{
		"articles": articles,
	})
}

func (s *Server) handleCreateArticle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type CreateArticleRequest struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}

	// Parse input
	var req CreateArticleRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}
	defer r.Body.Close()

	// Write new article to database
	slug := strings.Join(strings.Split(strings.ToLower(req.Title), " "), "-")
	_, err = s.db.Exec("INSERT INTO articles (title, description, content, slug) VALUES (?, ?, ?, ?)", req.Title, req.Description, req.Content, slug)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": "Article created successfully",
	})
}

func (s *Server) handleGetArticleBySlug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	slug := chi.URLParam(r, "slug")

	type Article struct {
		Slug        string `json:"slug"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Published   string `json:"published"` // Adjust to time.Time if your driver parses dates
		Modified    string `json:"modified"`
	}

	var a Article

	// Retrieve blogpost
	err := s.db.QueryRow(
		"SELECT slug, title, description, content, published, modified FROM articles WHERE slug LIKE ?",
		slug,
	).Scan(&a.Slug, &a.Title, &a.Description, &a.Content, &a.Published, &a.Modified)
	if err != nil {
		log.Printf("Query failed: %v", err)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(a)
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
