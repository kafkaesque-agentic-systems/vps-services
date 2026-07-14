// Package main is the composition root and executable entry point for the
// Custom-VPS-MCP-Engine.
//
// # Role in the architecture
//
// This package intentionally contains NO business logic. Its sole
// responsibilities are:
//
//  1. Dependency injection / wiring: read configuration from the environment,
//     construct the authentication middleware (internal/auth), the MCP engine
//     and SSE transport (internal/mcpengine), and the domain skill packages
//     (internal/skills/...), then compose them into a single HTTP handler.
//  2. Process lifecycle: start the HTTP server, and (in later phases) manage
//     graceful shutdown so that in-flight SSE streams are drained cleanly.
//
// Keeping the entry point thin is a deliberate clean-architecture choice: every
// unit of real behavior lives in a testable internal package, while `main`
// merely assembles those units. A junior developer reading this package should
// be able to understand the entire request pipeline at a glance without wading
// through domain code.
//
// # Request flow (for context)
//
//	Client -> HTTPS (Bearer Token) -> Nginx Reverse Proxy ->
//	  Docker network -> this server -> Auth Middleware -> MCP SSE transport ->
//	  Skills Router
//
// The concrete implementation of this package is delivered in Phase 3.
package main
