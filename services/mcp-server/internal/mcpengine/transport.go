package mcpengine

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Route constants for the HTTP + SSE transport. Defining them in one place
// documents the public wire contract of the server and keeps route registration
// consistent.
const (
	// SSEPath is the endpoint a client opens (via GET) to establish the
	// long-lived, server-to-client event stream. This is the entry point of an
	// MCP session over SSE.
	SSEPath = "/sse"

	// MessagePath is the endpoint a client POSTs to for every client-to-server
	// message. The SDK's SSEHandler correlates these POSTs to the right session
	// internally, so we simply route both legs to the same handler.
	MessagePath = "/message"
)

// SSETransport is the HTTP-facing adapter that exposes a single *mcp.Server over
// the Model Context Protocol's HTTP + Server-Sent Events transport.
//
// # Why this is now a thin wrapper
//
// Earlier iterations hand-rolled the SSE multiplexer: a session map, a mutex to
// guard it, and self-minted session IDs to correlate message POSTs to their
// streams. The current official SDK provides mcp.SSEHandler, which performs ALL
// of that internally — session creation, ID generation, endpoint advertisement,
// and POST-to-session routing — and returns a ready-to-serve http.Handler.
//
// Delegating to the SDK removes an entire class of concurrency bugs from our
// codebase and keeps us aligned with the transport semantics the SDK maintainers
// test and support. This type therefore exists only to (a) construct that
// handler with our shared server and (b) attach it to our routes.
type SSETransport struct {
	// handler is the SDK-provided http.Handler that natively implements both the
	// SSE stream (GET) and the message endpoint (POST). It is safe for
	// concurrent use across many simultaneous sessions.
	handler *mcp.SSEHandler
}

// NewSSETransport builds an SSETransport that exposes the given MCP server over
// HTTP + SSE.
//
// It instantiates the SDK's SSEHandler, handing it a getServer closure. The SDK
// invokes that closure for each incoming request to obtain the *mcp.Server that
// should back the session; because we run a single shared engine (with all
// skills already registered per Phase 4), the closure returns that one server
// for every request.
//
// The closure's signature is dictated by the SDK: func(*http.Request) *mcp.Server.
// It receives the raw request (useful if a future implementation wanted to pick
// a server based on headers/path), and returns the server with no error channel.
//
// Session keep-alive (Audit P-1) is configured on the server itself, not here:
// in this SDK version KeepAlive is a *mcp.ServerOptions field (a session-level
// ping), so it lives in NewServer (see mcpengine/server.go). The SSE handler's
// own *mcp.SSEOptions only tunes transport-level concerns (e.g. DNS-rebinding
// protection), which we leave at their secure defaults, hence nil.
func NewSSETransport(server *mcp.Server) *SSETransport {
	handler := mcp.NewSSEHandler(func(request *http.Request) *mcp.Server {
		// Every request is served by the same shared engine. We don't branch on
		// the request because we run one multi-tenant server, not per-session
		// servers.
		return server
	}, nil)

	return &SSETransport{handler: handler}
}

// RegisterRoutes attaches the SDK's SSE handler to both transport routes on the
// supplied ServeMux.
//
// We use Go 1.22+ method-scoped patterns ("GET /sse", "POST /message") so the
// mux rejects wrong-method requests with 405 before the handler runs. Both
// patterns point at the SAME handler: the SDK's SSEHandler inspects the request
// (method + session correlation) and internally dispatches the GET as a new
// stream and the POST as a message into the matching session.
//
// This method does NOT apply authentication. Wrapping the resulting mux with
// auth.TokenAuthMiddleware is the composition root's job (cmd/server), which
// guarantees both routes are protected by a single, auditable decision.
func (t *SSETransport) RegisterRoutes(mux *http.ServeMux) {
	// Wrap the GET route to rewrite the path before the SDK sees it.
	// The SDK uses r.URL.Path to tell the client where to send POST messages.
	// By temporarily changing it to MessagePath ("/message"), the SDK will correctly
	// advertise "/message?sessionId=..." instead of "/sse?sessionId=...".
	mux.HandleFunc("GET "+SSEPath, func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = MessagePath
		t.handler.ServeHTTP(w, r)
	})

	// Handle the incoming POSTs on the dedicated message route.
	mux.Handle("POST "+MessagePath, t.handler)
}
