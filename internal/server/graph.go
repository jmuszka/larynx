package server

import (
	"context"
	"time"

	"github.com/jmuszka/larynx/internal/logging"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// neo4jStore adapts a real neo4j.Driver to the endpoints.GraphStore interface.
type neo4jStore struct {
	driver neo4j.Driver
	logger *logging.Service
}

func (n *neo4jStore) ExecuteQuery(ctx context.Context, query string, params map[string]any, opts ...neo4j.ExecuteQueryConfigurationOption) (*neo4j.EagerResult, error) {
	start := time.Now()
	result, err := neo4j.ExecuteQuery(ctx, n.driver, query, params, neo4j.EagerResultTransformer, opts...)
	n.logger.Debug("graph query completed", "duration", time.Since(start).Round(time.Microsecond).String())
	return result, err
}

func (n *neo4jStore) VerifyConnectivity(ctx context.Context) error {
	return n.driver.VerifyConnectivity(ctx)
}
