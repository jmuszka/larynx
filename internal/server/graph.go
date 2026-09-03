package server

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// graphStore is the subset of graph-database capabilities the handlers rely on.
// It exists so tests can inject a fake without implementing the full
// neo4j.Driver/session/transaction stack.
type graphStore interface {
	ExecuteQuery(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error)
	VerifyConnectivity(ctx context.Context) error
}

// neo4jStore adapts a real neo4j.Driver to the graphStore interface.
type neo4jStore struct {
	driver neo4j.Driver
}

func (n *neo4jStore) ExecuteQuery(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
	return neo4j.ExecuteQuery(ctx, n.driver, query, params, neo4j.EagerResultTransformer, opts...)
}

func (n *neo4jStore) VerifyConnectivity(ctx context.Context) error {
	return n.driver.VerifyConnectivity(ctx)
}
