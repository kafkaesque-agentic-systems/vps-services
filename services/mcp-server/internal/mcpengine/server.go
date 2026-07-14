package mcpengine

import (
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These constants define the identity this server advertises to every MCP
// client during the protocol's initialize handshake. Centralizing them here
// (rather than scattering literals across the codebase) gives us one
// authoritative place to bump the version and keeps the identity consistent
// across logs, handshakes, and future telemetry.
const (
	// serverName is the human-readable product name reported in the MCP
	// ServerInfo. Clients (and the LLMs behind them) may surface this string,
	// so it should remain stable and descriptive.
	serverName = "Custom-VPS-MCP-Engine"

	// serverVersion is the semantic version of this server implementation.
	// Bump it whenever the exposed tool/prompt surface changes in a way clients
	// might care about.
	serverVersion = "1.0.0"

	// sessionKeepAlive is the interval at which the server pings each connected
	// MCP session to keep it demonstrably alive and to detect dead peers
	// (Audit P-1).
	//
	// # Why this prevents zombie sessions
	//
	// The SDK's KeepAlive is a session-level "ping" mechanism: at this interval
	// the server sends a ping and, if the peer fails to respond, the session is
	// automatically closed. Without it, an idle SSE stream whose client has
	// silently vanished (dropped by a NAT/firewall/proxy, or crashed) would
	// linger as a "zombie", holding a goroutine and socket indefinitely. The
	// ping both keeps healthy connections warm and reaps dead ones.
	//
	// 25s sits comfortably under common 30–60s idle-timeout thresholds
	// (including our Nginx proxy_read_timeout and typical load-balancer idle
	// limits), so a keep-alive always fires before any such timer can expire.
	sessionKeepAlive = 25 * time.Second
)

// NewServer constructs and returns the core MCP server for the
// Custom-VPS-MCP-Engine.
//
// # Why this function exists
//
// This is the ONLY place in the codebase that calls into the SDK's server
// constructor. By funneling construction through mcpengine, the rest of the
// application depends on our narrow, documented surface rather than directly on
// the SDK. If the SDK's construction API shifts between releases, the blast
// radius is confined to this package.
//
// # What it does
//
//   - Declares the server's identity (ServerInfo) via mcp.Implementation using
//     the serverName/serverVersion constants above.
//   - Instantiates the server with mcp.NewServer, passing *mcp.ServerOptions
//     that enable session keep-alive pings (Audit P-1; see sessionKeepAlive).
//     This is also the extension point for any future behavior (instructions,
//     logging hooks, etc.).
//
// # What it deliberately does NOT do
//
// It does not register any tools or prompts. Capability registration is the
// responsibility of the skills layer (internal/skills), wired up in Phase 4 via
// a dedicated registrar. Keeping construction and registration separate means a
// caller can build a bare server for testing without dragging in every domain.
//
// The returned *mcp.Server is not yet connected to any transport; the caller is
// expected to hand it to the SSE transport layer (see NewSSETransport) to make
// it reachable over HTTP.
func NewServer() *mcp.Server {
	// mcp.Implementation carries the identity fields transmitted during the MCP
	// initialize handshake. Name and Version are the two fields mandated by the
	// architecture blueprint (Task 3.2).
	info := &mcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}

	// Enable session keep-alive so idle-but-dead SSE sessions are reaped rather
	// than lingering as zombies (Audit P-1).
	opts := &mcp.ServerOptions{
		KeepAlive: sessionKeepAlive,
	}

	return mcp.NewServer(info, opts)
}
