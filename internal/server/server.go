package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmuszka/larynx/internal/ai"
	"github.com/jmuszka/larynx/internal/cache"
	"github.com/jmuszka/larynx/internal/logging"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

type Config struct {
	Addr              string
	Neo4jUri          string
	Neo4jUser         string
	Neo4jPassword     string
	SqlitePath        string
	Version           string
	AIBaseURL         string
	AIKey             string
	AIModel           string
	EtymologyBaseURL  string
	Logger            *logging.Service
	AllowedOrigins    []string
	BearerTokens      []string
	AdminJWTSecret    string
	AdminJWTSubject   string
	DebugMode         bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type Server struct {
	*http.Server
	cfg        Config
	logger     *logging.Service
	graph      graphStore
	db         *sql.DB
	cache      *cache.Cache
	ai         *ai.Service
	httpClient *http.Client
	version    string
}

func New(cfg Config) *Server {
	if cfg.EtymologyBaseURL == "" {
		cfg.EtymologyBaseURL = "https://www.etymonline.com"
	}

	// Connect to Neo4j database
	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jUri,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""))
	if err != nil {
		cfg.Logger.Fatal("failed to create neo4j driver", "error", err)
	}
	if err = driver.VerifyConnectivity(ctx); err != nil {
		cfg.Logger.Fatal("failed to verify neo4j connectivity", "error", err)
	}
	cfg.Logger.Info("neo4j connection established")

	db, err := sql.Open("sqlite", cfg.SqlitePath)
	if err != nil {
		cfg.Logger.Fatal("failed to open sqlite database", "error", err)
	}
	if err = db.PingContext(ctx); err != nil {
		cfg.Logger.Fatal("failed to ping sqlite database", "error", err)
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
		cfg.Logger.Fatal("failed to create articles table", "error", err)
	}
	cfg.Logger.Info("sqlite connection established")

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	cache := cache.New(rdb, cfg.Logger.Logger())
	cfg.Logger.Info("redis connection established")

	// For web scraping
	httpClient := &http.Client{Timeout: 30 * time.Second}

	aiService, err := ai.New(ai.Config{
		APIKey:     cfg.AIKey,
		BaseURL:    cfg.AIBaseURL,
		Model:      cfg.AIModel,
		HTTPClient: httpClient,
	})
	if err != nil {
		cfg.Logger.Fatal("failed to initialize ai service", "error", err)
	}
	cfg.Logger.Info("ai service initialized")

	s := &Server{cfg: cfg, logger: cfg.Logger, graph: &neo4jStore{driver: driver}, db: db, version: cfg.Version, cache: cache, ai: aiService, httpClient: httpClient}

	// Routing
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(30 * time.Second))

	// CORS
	if cfg.DebugMode {
		r.Use(cors.AllowAll().Handler)
	} else {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.AllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Admin-JWT"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r.Mount("/api/v1", s.apiRouter())

	// Start server
	s.Server = &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}
	return s
}
