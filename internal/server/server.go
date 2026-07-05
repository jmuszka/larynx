package server

import (
	"context"
	"encoding/json"
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
}

type Server struct {
	*http.Server
	driver neo4j.DriverWithContext
	ctx    context.Context
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
	s := &Server{driver: driver, ctx: ctx}

	// Routing
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.AllowAll().Handler)

	// Create endpoints
	r.Get("/health", s.handleHealth)
	r.Get("/words/{word}", s.handleGetWord)
	r.Get("/words", s.handleSearchWords)

	// Start server
	s.Server = &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}
	return s
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "2026.07.04",
	})
}

func (s *Server) handleGetWord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, `
		MATCH (n:Word)
		WHERE n.lang = "English"
		AND n.term IS NOT NULL AND n.term =~ $word
		RETURN n
	`,
		map[string]any{
			"word": "(?i)" + chi.URLParam(r, "word"),
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	records := make([]map[string]any, len(result.Records))
	for i, record := range result.Records {
		records[i] = record.AsMap()
	}

	json.NewEncoder(w).Encode(records)
}

func (s *Server) handleSearchWords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	queryParams := r.URL.Query()

	prefix := queryParams.Get("prefix")

	result, err := neo4j.ExecuteQuery(s.ctx, s.driver, `
		MATCH (n:Word)
		WHERE n.lang = "English"
		AND n.term IS NOT NULL AND n.term STARTS WITH $prefix
		RETURN DISTINCT n.term AS term
	`,
		map[string]any{
			"prefix": prefix,
		}, neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		panic(err)
	}

	terms := make([]string, 0, len(result.Records))
	for _, record := range result.Records {
		if term, ok := record.Get("term"); ok {
			if s, ok := term.(string); ok {
				terms = append(terms, s)
			}
		}
	}

	json.NewEncoder(w).Encode(terms)
}
