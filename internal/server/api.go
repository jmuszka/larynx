package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func (s *Server) apiRouter() http.Handler {
	r := chi.NewRouter()

	r.Mount("/health", s.healthRouter())
	r.Mount("/words", s.wordsRouter())
	r.Mount("/games", s.gamesRouter())
	r.Mount("/blog", s.blogRouter())
	r.Mount("/docs", httpSwagger.Handler(
		httpSwagger.URL("/api/v1/docs/doc.json"),
	))

	return r
}
