package endpoints

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jmuszka/larynx/internal/ai"
	"github.com/jmuszka/larynx/internal/cache"
	"github.com/jmuszka/larynx/internal/logging"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// GraphStore is the subset of graph-database capabilities the handlers rely
// on. It exists so tests can inject a fake without implementing the full
// neo4j.Driver/session/transaction stack.
type GraphStore interface {
	ExecuteQuery(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error)
	VerifyConnectivity(ctx context.Context) error
}

// GeoJSON is the standard FeatureCollection response envelope shared by the
// geography endpoints.
type GeoJSON struct {
	Type     string    `json:"type" example:"FeatureCollection"`
	Features []Feature `json:"features"`
}

// Feature is a single GeoJSON feature.
type Feature struct {
	Type       string         `json:"type" example:"Feature"`
	Properties map[string]any `json:"properties"`
	Geometry   map[string]any `json:"geometry"`
}

// Config carries the dependencies the endpoint handlers need.
type Config struct {
	Logger           *logging.Service
	Graph            GraphStore
	DB               *sql.DB
	Cache            *cache.Cache
	AI               *ai.Service
	HTTPClient       *http.Client
	Version          string
	EtymologyBaseURL string
}

// Server holds the shared dependencies of every endpoint handler. Fields are
// exported because the handlers live in subpackages.
type Server struct {
	Logger           *logging.Service
	Graph            GraphStore
	DB               *sql.DB
	Cache            *cache.Cache
	AI               *ai.Service
	HTTPClient       *http.Client
	Version          string
	EtymologyBaseURL string
}

func New(cfg Config) *Server {
	return &Server{
		Logger:           cfg.Logger,
		Graph:            cfg.Graph,
		DB:               cfg.DB,
		Cache:            cfg.Cache,
		AI:               cfg.AI,
		HTTPClient:       cfg.HTTPClient,
		Version:          cfg.Version,
		EtymologyBaseURL: cfg.EtymologyBaseURL,
	}
}

// UnescapeParam decodes a URL path parameter (e.g. "caf%C3%A9" -> "café").
func UnescapeParam(r *http.Request, param string) string {
	word := chi.URLParam(r, param)
	if decoded, err := url.PathUnescape(word); err == nil {
		return decoded
	}
	return word
}
