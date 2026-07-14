// Package auth provides the transport-level security layer for the
// Custom-VPS-MCP-Engine.
//
// # Purpose
//
// Because the MCP server is exposed remotely over the public internet (via an
// Nginx reverse proxy), every request that reaches the SSE endpoints must be
// authenticated BEFORE it is allowed to touch the MCP engine or any skill.
// This package implements that gate as standard net/http middleware.
//
// # Design (delivered in Phase 2)
//
//   - TokenAuthMiddleware wraps any http.Handler and enforces a shared-secret
//     Bearer token check.
//   - The expected secret is sourced from the MCP_SECRET_TOKEN environment
//     variable so that credentials never live in source control or images.
//   - Two credential-presentation strategies are supported to maximize client
//     compatibility:
//     1. The canonical `Authorization: Bearer <token>` header.
//     2. A `?token=<token>` query-parameter fallback, required because some MCP
//     clients cannot attach custom headers during the initial SSE handshake.
//
// # Why middleware and not per-handler checks
//
// Centralizing authentication in one composable wrapper guarantees that a new
// route can never be accidentally exposed unauthenticated: the whole mux is
// wrapped once at the composition root (see cmd/server). This is a defense-in-
// depth posture — the security decision is made in exactly one place.
package auth
