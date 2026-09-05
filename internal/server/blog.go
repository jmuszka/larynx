package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
)

type article struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Published   string `json:"published"`
	Modified    string `json:"modified"`
}

type articleSummary struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Published   string `json:"published"`
	Modified    string `json:"modified"`
}

type articlesResponse struct {
	Articles []articleSummary `json:"articles"`
}

type createArticleRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type updateArticleRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Content     *string `json:"content"`
}

type messageResponse struct {
	Message string `json:"message"`
}

const (
	maxTitleLength       = 200
	maxDescriptionLength = 500
	maxContentLength     = 100000
	maxSlugLength        = 200
)

// slugify lowercases the title, strips all punctuation, and collapses the
// remaining whitespace into single hyphens (e.g. "Hello, World!" ->
// "hello-world").
func slugify(title string) string {
	title = strings.ToLower(title)
	title = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, title)
	return strings.Join(strings.Fields(title), "-")
}

func (s *Server) blogRouter() http.Handler {
	r := chi.NewRouter()

	r.Get("/articles", s.handleGetArticles)
	r.Get("/articles/{slug}", s.handleGetArticleBySlug)

	r.Group(func(r chi.Router) {
		r.Use(s.adminJWTAuth)
		r.Use(httprate.LimitBy(rateLimitBlogWritePerUser, rateLimitWindow, adminUserKey, httprate.WithLimitHandler(rateLimitHandler)))
		r.Post("/articles/create", s.handleCreateArticle)
		r.Patch("/articles/{slug}", s.handleUpdateArticleBySlug)
		r.Delete("/articles/{slug}", s.handleDeleteArticleBySlug)
	})

	return r
}

// handleGetArticles godoc
// @Summary      List articles
// @Description  Returns all articles ordered by most recently modified.
// @Tags         blog
// @Produce      json
// @Success      200  {object}  articlesResponse
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /blog/articles [get]
func (s *Server) handleGetArticles(w http.ResponseWriter, r *http.Request) {
	// Retrieve blogposts
	const sqlQuery = "SELECT slug, title, description, published, modified FROM articles ORDER BY modified DESC"
	s.logger.Debug("SQL: " + sqlQuery)
	rows, err := s.db.Query(sqlQuery)
	if err != nil {
		s.logger.Error("query failed", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query database")
		return
	}
	defer rows.Close()

	// Parse results
	articles := []articleSummary{}
	for rows.Next() {
		var a articleSummary
		// Scan targets MUST match the order of columns in SELECT statement
		err := rows.Scan(&a.Slug, &a.Title, &a.Description, &a.Published, &a.Modified)
		if err != nil {
			s.logger.Error("row scan failed", "error", err)
			s.writeJSONError(w, http.StatusInternalServerError, "Failed to process data")
			return
		}
		articles = append(articles, a)
	}
	if err = rows.Err(); err != nil {
		s.logger.Error("iteration error", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Database cursor error")
		return
	}

	s.writeJSON(w, http.StatusOK, articlesResponse{
		Articles: articles,
	})
}

// handleCreateArticle godoc
// @Summary      Create an article
// @Description  Creates a new article from the provided title, description, and content.
// @Tags         blog
// @Accept       json
// @Produce      json
// @Param        body  body      createArticleRequest  true  "Article to create"
// @Success      200   {object}  messageResponse
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Security     AdminJWTAuth
// @Router       /blog/articles/create [post]
func (s *Server) handleCreateArticle(w http.ResponseWriter, r *http.Request) {
	// Parse input
	var req createArticleRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	defer r.Body.Close()

	// Input validation
	if len(req.Title) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(req.Description) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "description is required")
		return
	}
	if len(req.Content) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}
	if len(req.Title) > maxTitleLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length of %d characters", maxTitleLength))
		return
	}
	if len(req.Description) > maxDescriptionLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length of %d characters", maxDescriptionLength))
		return
	}
	if len(req.Content) > maxContentLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("content exceeds maximum length of %d characters", maxContentLength))
		return
	}

	// Write new article to database
	slug := slugify(req.Title)
	const insertQuery = "INSERT INTO articles (title, description, content, slug) VALUES (?, ?, ?, ?)"
	s.logger.Debug("SQL: " + renderSQL(insertQuery, []any{req.Title, req.Description, req.Content, slug}))
	_, err = s.db.Exec(insertQuery, req.Title, req.Description, req.Content, slug)
	if err != nil {
		s.logger.Error("failed to create article", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create article")
		return
	}

	s.writeJSON(w, http.StatusOK, messageResponse{
		Message: "Article created successfully",
	})
}

// handleGetArticleBySlug godoc
// @Summary      Get an article by slug
// @Description  Returns a single article identified by its slug.
// @Tags         blog
// @Produce      json
// @Param        slug  path      string  true  "Article slug"
// @Success      200   {object}  article
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Router       /blog/articles/{slug} [get]
func (s *Server) handleGetArticleBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Input validation
	if len(slug) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}
	if len(slug) > maxSlugLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("slug exceeds maximum length of %d characters", maxSlugLength))
		return
	}

	var a article

	// Retrieve blogpost
	const sqlQuery = "SELECT slug, title, description, content, published, modified FROM articles WHERE slug LIKE ?"
	s.logger.Debug("SQL: " + renderSQL(sqlQuery, []any{slug}))
	err := s.db.QueryRow(
		sqlQuery,
		slug,
	).Scan(&a.Slug, &a.Title, &a.Description, &a.Content, &a.Published, &a.Modified)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.writeJSONError(w, http.StatusNotFound, "article not found")
			return
		}
		s.logger.Error("query failed", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query database")
		return
	}

	s.writeJSON(w, http.StatusOK, a)
}

// handleUpdateArticleBySlug godoc
// @Summary      Update an article
// @Description  Updates one or more fields of the article identified by its slug.
// @Tags         blog
// @Accept       json
// @Produce      json
// @Param        slug  path      string                true  "Article slug"
// @Param        body  body      updateArticleRequest  true  "Fields to update"
// @Success      200   {object}  messageResponse
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Security     AdminJWTAuth
// @Router       /blog/articles/{slug} [patch]
func (s *Server) handleUpdateArticleBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Input validation
	if len(slug) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}
	if len(slug) > maxSlugLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("slug exceeds maximum length of %d characters", maxSlugLength))
		return
	}

	// Parse input
	var req updateArticleRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	defer r.Body.Close()

	// Input validation
	if req.Title != nil {
		if len(*req.Title) == 0 {
			s.writeJSONError(w, http.StatusBadRequest, "title is required")
			return
		}
		if len(*req.Title) > maxTitleLength {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("title exceeds maximum length of %d characters", maxTitleLength))
			return
		}
	}
	if req.Description != nil {
		if len(*req.Description) == 0 {
			s.writeJSONError(w, http.StatusBadRequest, "description is required")
			return
		}
		if len(*req.Description) > maxDescriptionLength {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("description exceeds maximum length of %d characters", maxDescriptionLength))
			return
		}
	}
	if req.Content != nil {
		if len(*req.Content) == 0 {
			s.writeJSONError(w, http.StatusBadRequest, "content is required")
			return
		}
		if len(*req.Content) > maxContentLength {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("content exceeds maximum length of %d characters", maxContentLength))
			return
		}
	}

	var queryParts []string
	var args []any

	if req.Title != nil {
		queryParts = append(queryParts, "title = ?")
		args = append(args, *req.Title) // De-reference the pointer to get the actual string
	}
	if req.Description != nil {
		queryParts = append(queryParts, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Content != nil {
		queryParts = append(queryParts, "content = ?")
		args = append(args, *req.Content)
	}

	if len(queryParts) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "No fields provided for update")
		return
	}

	queryParts = append(queryParts, "modified = CURRENT_TIMESTAMP")

	query := fmt.Sprintf("UPDATE articles SET %s WHERE slug = ?", strings.Join(queryParts, ", "))
	args = append(args, slug)

	// Update article in database
	s.logger.Debug("SQL: " + renderSQL(query, args))
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.logger.Error("failed to update article", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to update article")
		return
	}

	s.writeJSON(w, http.StatusOK, messageResponse{
		Message: "Article updated successfully",
	})
}

// handleDeleteArticleBySlug godoc
// @Summary      Delete an article
// @Description  Deletes the article identified by its slug.
// @Tags         blog
// @Produce      json
// @Param        slug  path      string  true  "Article slug"
// @Success      200   {object}  messageResponse
// @Failure      500   {object}  map[string]string
// @Security     BearerAuth
// @Security     AdminJWTAuth
// @Router       /blog/articles/{slug} [delete]
func (s *Server) handleDeleteArticleBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Input validation
	if len(slug) == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}
	if len(slug) > maxSlugLength {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("slug exceeds maximum length of %d characters", maxSlugLength))
		return
	}

	// Delete article from database
	const deleteQuery = "DELETE FROM articles WHERE slug = ?"
	s.logger.Debug("SQL: " + renderSQL(deleteQuery, []any{slug}))
	_, err := s.db.Exec(deleteQuery, slug)
	if err != nil {
		s.logger.Error("failed to delete article", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to delete article")
		return
	}

	s.writeJSON(w, http.StatusOK, messageResponse{
		Message: "Article deleted successfully",
	})
}
