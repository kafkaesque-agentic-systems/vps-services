// Package mcpengine encapsulates the construction and configuration of the core
// MCP server and its Server-Sent Events (SSE) transport.
//
// # Purpose
//
// This package is the seam between our application and the official MCP Go SDK
// (github.com/modelcontextprotocol/go-sdk). By funneling all SDK usage through
// here, we:
//
//   - Isolate the rest of the codebase from SDK API churn. If the SDK's
//     construction API changes, only this package needs to adapt.
//   - Provide a single, well-documented place where the ServerInfo (name,
//     version) and transport wiring are defined.
//
// # Responsibilities (delivered in Phase 3)
//
//   - Initialize the MCP ServerInfo:
//     Name    = "Custom-VPS-MCP-Engine"
//     Version = "1.0.0"
//   - Instantiate the core server via the SDK (mcp.NewServer).
//   - Build the SSE transport handler and expose the `/sse` and `/message`
//     HTTP routes that MCP clients connect to.
//
// # Why SSE (not stdio)
//
// The standard MCP stdio transport assumes a local child process. Our server is
// remote and long-lived, so we use HTTP + SSE instead: the client opens a
// persistent `/sse` stream to receive server-to-client events, and POSTs
// client-to-server messages to `/message`. This is what allows a single Go
// binary behind Nginx to serve many concurrent remote MCP sessions.
package mcpengine
