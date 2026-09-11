package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/jmuszka/larynx/internal/server/endpoints"
	"github.com/jmuszka/larynx/internal/server/endpoints/blog"
	"github.com/jmuszka/larynx/internal/server/endpoints/geography"
	"github.com/jmuszka/larynx/internal/server/endpoints/health"
	"github.com/jmuszka/larynx/internal/server/endpoints/words"
)

// The endpoint handlers live in internal/server/endpoints/<resource>; this
// file wires them to routes and applies auth and rate limiting.

// handle adapts an endpoint handler (which takes the shared dependency struct
// as its first argument) into a standard http.HandlerFunc.
func (s *Server) handle(f func(e *endpoints.Server, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f(s.eps, w, r)
	}
}

func (s *Server) wordsRouter() http.Handler {
	r := chi.NewRouter()

	r.With(httprate.LimitBy(rateLimitEtymologyPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/etymology", s.handle(words.HandleGetEtymology))
	r.With(httprate.LimitBy(rateLimitHistoryPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler))).Get("/{word}/history", s.handle(words.HandleGetHistory))
	r.Get("/", s.handle(words.HandleSearchWords))
	return r
}

func (s *Server) blogRouter() http.Handler {
	r := chi.NewRouter()

	r.Get("/articles", s.handle(blog.HandleGetArticles))
	r.Get("/articles/{slug}", s.handle(blog.HandleGetArticleBySlug))

	r.Group(func(r chi.Router) {
		r.Use(s.adminJWTAuth)
		r.Use(httprate.LimitBy(rateLimitBlogWritePerUser, rateLimitWindow, adminUserKey, httprate.WithLimitHandler(rateLimitHandler)))
		r.Post("/articles/create", s.handle(blog.HandleCreateArticle))
		r.Patch("/articles/{slug}", s.handle(blog.HandleUpdateArticleBySlug))
		r.Delete("/articles/{slug}", s.handle(blog.HandleDeleteArticleBySlug))
	})

	return r
}

func (s *Server) geographyRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/{id}", s.handle(geography.HandleGetGeography))
	return r
}

func (s *Server) healthRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", s.handle(health.HandleHealth))
	return r
}
