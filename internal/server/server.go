package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

type Config struct {
	Addr          string
	Neo4jUri      string
	Neo4jUser     string
	Neo4jPassword string
	Version       string
}

type Server struct {
	*http.Server
	driver  neo4j.DriverWithContext
	ctx     context.Context
	version string
}

func New(cfg Config) *Server {
	// Connect to Neo4j database
	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jUri,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""))
	if err != nil {
		panic(err)
	}
	if err = driver.VerifyConnectivity(ctx); err != nil {
		panic(err)
	}
	fmt.Println("Connection established.")

	// Create server
	s := &Server{driver: driver, ctx: ctx, version: cfg.Version}

	// Routing
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.AllowAll().Handler)

	r.Mount("/health", s.healthRouter())
	r.Mount("/words", s.wordsRouter())
	r.Mount("/games", s.gamesRouter())
	r.Mount("/blog", s.blogRouter())

	// Start server
	s.Server = &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}
	return s
}
