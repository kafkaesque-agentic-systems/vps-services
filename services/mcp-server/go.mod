// Module: mcp-server
//
// This is the module definition for the Custom-VPS-MCP-Engine — a remote
// Model Context Protocol (MCP) server written in Go and exposed over an
// HTTPS + Server-Sent Events (SSE) transport.
//
// Why Go 1.25:
//   - Aligns with the `golang:1.25-alpine` builder image used by the
//     Phase 5 Dockerfile, guaranteeing that the toolchain used locally matches
//     the toolchain used in CI/CD and production containers.
//   - Provides the modern standard-library HTTP server semantics we rely on for
//     long-lived SSE streams (per-request context cancellation, robust timeouts).
//
// Dependency policy (see agent directive 2.2) — AMENDED for the database skill:
//   - We deliberately minimize third-party dependencies. There are exactly TWO
//     sanctioned external dependencies, both first-party vendor SDKs:
//       1. The official MCP Go SDK (protocol layer).
//       2. The official MongoDB Go driver v2 (internal/skills/database) —
//          chosen over shelling out to `mongo --eval`, which is the
//          injection-prone legacy pattern the database skill retires
//          (Audit C-8).
//     Everything else — routing, middleware, JSON handling, transport
//     plumbing — is built on the standard library to keep the static binary
//     small and the attack surface narrow.
module mcp-server

go 1.25.0

require (
	github.com/modelcontextprotocol/go-sdk v1.6.1
	go.mongodb.org/mongo-driver/v2 v2.2.2
)

require (
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)
