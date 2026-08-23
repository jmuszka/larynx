package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func (s *Server) apiRouter() http.Handler {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(s.bearerAuth)
		r.Use(middleware.ClientIPFromRemoteAddr)
		r.Use(httprate.LimitBy(rateLimitDefaultPerIP, rateLimitWindow, clientIPKey, httprate.WithLimitHandler(rateLimitHandler)))
		r.Mount("/health", s.healthRouter())
		r.Mount("/words", s.wordsRouter())
		r.Mount("/games", s.gamesRouter())
		r.Mount("/blog", s.blogRouter())
	})

	// Mount swagger API docs if in development mode
	if s.cfg.DebugMode {
		r.Mount("/docs", httpSwagger.Handler(
			httpSwagger.URL("/api/v1/docs/doc.json"),
		))
	}

	return r
}
