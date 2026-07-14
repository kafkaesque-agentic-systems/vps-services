package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// Operation time budgets. The v2 driver propagates the context deadline to the
// server (maxTimeMS semantics), so a pathological unindexed scan is killed in
// MongoDB itself, not merely abandoned by a hung handler.
const (
	// connectTimeout bounds initial connection + primary ping.
	connectTimeout = 10 * time.Second

	// readTimeout bounds every find/count/aggregate operation.
	readTimeout = 10 * time.Second

	// writeTimeout bounds every insert/update/delete operation (slightly wider
	// than reads to absorb majority write-concern acknowledgement).
	writeTimeout = 15 * time.Second
)

// shared holds the process-wide MongoDB client.
//
// DESIGN: a mutex-guarded lazy singleton, deliberately NOT sync.Once. Once
// would cache a *failed* first connection forever; with the mutex pattern a
// failure leaves shared.client nil, so the next tool call retries. This gives
// the MCP server two crucial properties: (1) it boots and serves all other
// skills even when MongoDB is down, and (2) database tools self-heal the
// moment the dbs container returns.
var shared struct {
	mu     sync.Mutex
	client *mongo.Client
}

// getClient returns the shared connected client, establishing and verifying the
// connection on first use.
//
// The driver's internal connection pool makes the single *mongo.Client safe for
// concurrent use across simultaneous SSE tool calls; the mutex here only
// serializes the connect-or-reuse decision, not the operations themselves.
func getClient(ctx context.Context) (*mongo.Client, error) {
	shared.mu.Lock()
	defer shared.mu.Unlock()

	if shared.client != nil {
		return shared.client, nil
	}

	cfg, err := resolveMongoConfig()
	if err != nil {
		return nil, err
	}
	if cfg.usingRootCredentials {
		// Loud, actionable nudge toward least privilege (approved decision #2).
		log.Printf(
			"skills/database: WARNING: connecting with ROOT credentials (%s); create the scoped "+
				"mcp_agent user and set %s/%s instead — runbook in ARCHITECTURE.md",
			envRootUser, envMongoUser, envMongoPass,
		)
	}

	client, err := mongo.Connect(options.Client().
		ApplyURI(cfg.uri).
		SetConnectTimeout(connectTimeout).
		SetServerSelectionTimeout(connectTimeout).
		// Majority write concern: a write is acknowledged only once durable on
		// a majority of the (single-node) deployment — matches the safety
		// posture expected of agent-driven mutations.
		SetWriteConcern(writeconcern.Majority()))
	if err != nil {
		return nil, fmt.Errorf("create MongoDB client for %s: %w", cfg.host, err)
	}

	// Verify reachability NOW so the tool call that triggered the connect gets
	// an immediate, specific error instead of a later mid-operation timeout.
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		// Best-effort teardown; the primary error is what matters to the caller.
		if dcErr := client.Disconnect(context.WithoutCancel(ctx)); dcErr != nil {
			log.Printf("skills/database: disconnect after failed ping: %v", dcErr)
		}
		return nil, fmt.Errorf(
			"reach MongoDB primary at %s (is the dbs container running? check with system_logs service=dbs): %w",
			cfg.host, err,
		)
	}

	log.Printf("skills/database: connected to MongoDB at %s", cfg.host)
	shared.client = client
	return shared.client, nil
}

// collectionFor resolves an exposed collection name through the allowlist and
// returns a live handle plus the resolved namespace for reporting.
//
// This is the ONLY way handlers in this package obtain a *mongo.Collection;
// funneling every operation through it guarantees the allowlist cannot be
// bypassed by a future handler forgetting to validate.
func collectionFor(ctx context.Context, name string) (*mongo.Collection, namespace, error) {
	ns, err := resolveNamespace(name)
	if err != nil {
		return nil, namespace{}, err
	}
	client, err := getClient(ctx)
	if err != nil {
		return nil, namespace{}, err
	}
	return client.Database(ns.database).Collection(ns.collection), ns, nil
}
