package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmuszka/larynx/internal/cache"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

type Config struct {
	Addr          string
	Neo4jUri      string
	Neo4jUser     string
	Neo4jPassword string
	SqlitePath    string
	Version       string
}

type Server struct {
	*http.Server
	driver  neo4j.DriverWithContext
	db      *sql.DB
	cache   *cache.Cache
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
	fmt.Println("Neo4j connection established.")

	// Connect to SQLite database
	db, err := sql.Open("sqlite", cfg.SqlitePath)
	if err != nil {
		panic(err)
	}
	if err = db.PingContext(ctx); err != nil {
		panic(err)
	}
	if _, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS articles (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			slug        TEXT NOT NULL UNIQUE,
			title       TEXT NOT NULL,
			content     TEXT NOT NULL,
			description TEXT NOT NULL,
			published 	DATETIME DEFAULT CURRENT_TIMESTAMP,
			modified    DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		panic(err)
	}
	fmt.Println("SQLite connection established.")

	// Connect to caching layer
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	cache := cache.New(rdb)
	fmt.Println("Redis connection established.")

	// Create server
	s := &Server{driver: driver, db: db, ctx: ctx, version: cfg.Version, cache: cache}

	// Routing
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.AllowAll().Handler)

	r.Mount("/api/v1", s.apiRouter())

	// Start server
	s.Server = &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}
	return s
}
