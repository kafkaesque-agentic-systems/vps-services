package main

import (
	"cmp"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server/internal/auth"
	"mcp-server/internal/mcpengine"
	"mcp-server/internal/skills/database"
	"mcp-server/internal/skills/deploy"
	"mcp-server/internal/skills/docker"
	"mcp-server/internal/skills/snapshot"
	"mcp-server/internal/skills/system"
)

// Configuration constants for the process. Only PORT and MCP_SECRET_TOKEN are
// externally configurable; the timeouts and limits below are internal safety
// defaults hardened per the production security audit.
const (
	// portEnvVar names the environment variable that selects the listen port.
	portEnvVar = "PORT"

	// defaultPort is used when PORT is unset. 8080 is the conventional
	// container-internal HTTP port and matches the Nginx upstream configured in
	// Phase 5.
	defaultPort = "8080"

	// healthzPath is the unauthenticated liveness endpoint. It is served from
	// the ROOT mux (outside the auth middleware) so container orchestrators and
	// load balancers can probe liveness without holding the shared secret
	// (Audit C-3). It is intentionally NOT proxied publicly by Nginx.
	healthzPath = "/healthz"

	// shutdownGracePeriod bounds how long we wait for in-flight requests to
	// drain on shutdown before forcing exit. Combined with base-context
	// cancellation (see main), long-lived SSE streams are severed first, so this
	// window is only ever consumed by genuinely short in-flight requests.
	shutdownGracePeriod = 15 * time.Second

	// readHeaderTimeout caps how long a client may take to send its request
	// headers (Slowloris mitigation).
	readHeaderTimeout = 10 * time.Second

	// readTimeout caps how long reading the ENTIRE request (headers + body) may
	// take (Audit C-1). This bounds slow-body attacks on POST /message. It does
	// not affect the SSE response duration, which is governed by the (unset)
	// WriteTimeout.
	readTimeout = 30 * time.Second

	// idleTimeout caps how long an idle keep-alive connection is kept open
	// (Audit C-1), preventing connection accumulation from clients that open
	// sockets but send nothing.
	idleTimeout = 120 * time.Second

	// maxHeaderBytes caps the size of request headers at 1 MiB (Audit C-1),
	// bounding memory a single request's headers can consume.
	maxHeaderBytes = 1 << 20 // 1 MiB
)

// main is the composition root: it wires configuration, the MCP engine, the SSE
// transport, the authentication middleware, and an unauthenticated liveness
// probe into a single hardened HTTP server, then runs that server with prompt,
// stream-aware graceful shutdown.
//
// Request routing (outermost to innermost):
//
//	rootMux
//	  ├── GET /healthz            -> handleHealthz            (UNAUTHENTICATED)
//	  └── /                       -> auth.TokenAuthMiddleware -> mcpMux
//	                                                              ├── GET  /sse
//	                                                              └── POST /message
func main() {
	// --- Configuration ------------------------------------------------------
	//
	// cmp.Or returns the first non-zero (non-empty) argument, replacing the old
	// bespoke getEnvOrDefault helper with a standard-library one-liner (Audit
	// O-1).
	port := cmp.Or(os.Getenv(portEnvVar), defaultPort)
	if os.Getenv("MCP_SECRET_TOKEN") == "" {
		// The auth middleware fails closed (rejecting all traffic) until the
		// secret is set, so this is a highly visible warning rather than a fatal
		// error — it is the single most common misconfiguration.
		log.Printf("WARNING: MCP_SECRET_TOKEN is not set; all requests will be rejected until it is configured")
	}

	// --- MCP engine + skills ------------------------------------------------
	server := mcpengine.NewServer()
	registerCustomSkills(server)

	// --- Transport + routing ------------------------------------------------
	transport := mcpengine.NewSSETransport(server)

	// Inner mux: the MCP transport routes. Everything mounted here sits BEHIND
	// the auth middleware.
	mcpMux := http.NewServeMux()
	transport.RegisterRoutes(mcpMux)

	// Root mux: an unauthenticated liveness probe plus the auth-protected
	// application. The "/" pattern is the catch-all, so /sse and /message flow
	// through auth into mcpMux, while the more specific "GET /healthz" pattern
	// bypasses auth entirely (Audit C-3).
	rootMux := http.NewServeMux()
	rootMux.HandleFunc("GET "+healthzPath, handleHealthz)
	rootMux.Handle("/", auth.TokenAuthMiddleware(mcpMux))

	// --- Base context (stream lifecycle) ------------------------------------
	//
	// Every inbound connection — and therefore every request context — derives
	// from baseCtx via the server's BaseContext hook. Cancelling baseCtx on
	// shutdown immediately propagates cancellation into every in-flight SSE
	// handler, causing the transport pumps to return at once (Audit C-2). This
	// is what prevents Shutdown() from stalling for the full grace period on
	// streams that would otherwise stay open indefinitely.
	baseCtx, cancelBaseCtx := context.WithCancel(context.Background())
	defer cancelBaseCtx()

	// --- HTTP server (hardened) ---------------------------------------------
	httpServer := &http.Server{
		Addr:        ":" + port,
		Handler:     rootMux,
		BaseContext: func(net.Listener) context.Context { return baseCtx },

		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		// WriteTimeout is intentionally left at its zero value (no timeout). SSE
		// streams live for the duration of a client session; a write deadline
		// would kill them mid-stream. Stream teardown is instead handled by
		// base-context cancellation above.
	}

	// --- Graceful shutdown --------------------------------------------------
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Custom-VPS-MCP-Engine listening on :%s (routes: %s, %s; liveness: %s)",
			port, mcpengine.SSEPath, mcpengine.MessagePath, healthzPath)
		// ListenAndServe blocks until Shutdown/Close, then returns
		// http.ErrServerClosed (an expected, non-error condition).
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	case <-signalCtx.Done():
		log.Printf("shutdown signal received; severing active streams and draining (grace: %s)", shutdownGracePeriod)

		// C-2: cancel the base context FIRST. This unblocks every active SSE
		// handler immediately so the subsequent Shutdown drains quickly instead
		// of waiting the full grace period for long-lived streams.
		cancelBaseCtx()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown incomplete: %v", err)
		} else {
			log.Printf("shutdown complete")
		}
	}
}

// handleHealthz is the unauthenticated liveness probe (Audit C-3).
//
// It performs no dependency checks — it simply proves the process is up and
// accepting connections, which is exactly what a container HEALTHCHECK / load
// balancer needs. Deep health (uptime, runtime metrics) is available through the
// authenticated system_health MCP tool instead, so this endpoint leaks nothing
// useful to an unauthenticated caller.
//
// It is mounted on the ROOT mux, deliberately outside the auth middleware, and
// is not exposed through the public Nginx proxy — only the container-internal
// healthcheck reaches it.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// Body is best-effort; a failed write here is inconsequential for a probe.
	_, _ = w.Write([]byte("ok\n"))
}

// registerCustomSkills attaches this server's domain skills to the MCP engine.
//
// # Role
//
// This function is the aggregation seam between the composition root and the
// domain layer (internal/skills/...). It is the single place that knows which
// skills exist; each skill package, in turn, owns the details of its own tools.
// Adding a new capability to the server is therefore a two-line change: import
// the skill package and add one Register call below — the transport, auth, and
// server-lifecycle code never change.
//
// # Ordering guarantee
//
// Skills MUST be registered here, before the transport begins serving, so that
// the very first MCP initialize handshake advertises the complete tool set to
// connecting clients. main() calls this immediately after constructing the
// server and before wiring the transport, satisfying that requirement.
//
// The parameter is the constructed but not-yet-connected *mcp.Server.
func registerCustomSkills(server *mcp.Server) {
	// The "system" skill: operational tools (health/uptime probe, server time).
	system.Register(server)

	// The "snapshot" skill: VPS codebase archive before AI-driven changes.
	snapshot.Register(server)

	// The "docker" skill: compose lifecycle (system_down / system_up / system_logs).
	docker.Register(server)

	// The "deploy" skill: local-to-VPS rsync push (push_codebase; local MCP only).
	deploy.Register(server)

	// The "database" skill: native MongoDB tooling (db_* / user_* tools).
	// Read tools always register; write tools require MCP_DB_ALLOW_WRITES=true
	// (fail-closed, approved decision #3).
	database.Register(server)

	// Register additional skill domains here as they are implemented, e.g.:
	//   weather.Register(server)
}
