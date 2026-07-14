// Package skills is the aggregation point for the server's domain-driven
// capability modules ("skills").
//
// # Concept
//
// Following Domain-Driven Design, every logical capability the MCP server
// exposes is packaged as a self-contained "skill" in its own subpackage under
// internal/skills/ (for example: internal/skills/system, internal/skills/
// snapshot, internal/skills/docker, internal/skills/deploy, internal/skills/weather).
// A skill fully owns its:
//
//   - Tool definitions and their JSON input/output schemas.
//   - Business logic / handlers.
//   - Prompts, where applicable.
//   - Input validation and self-healing error messages.
//
// # Why this boundary matters
//
// Bundling everything a domain needs into one isolated package means a new
// capability can be added or removed without touching unrelated code, and each
// skill can be reasoned about, tested, and reviewed independently. The
// composition root (cmd/server) and the registration routine (delivered in
// Phase 4) simply enumerate the available skills and register each one's tools
// with the core MCP server.
//
// This top-level package documents the convention. Concrete skills include
// internal/skills/system (operational probes), internal/skills/snapshot
// (VPS codebase archives), internal/skills/docker (compose lifecycle),
// internal/skills/deploy (local-to-VPS rsync push), and
// internal/skills/database (native MongoDB tooling with a fail-closed write
// switch). The composition root registers each via Register().
//
// One deliberate exception to the one-package-per-skill rule: internal/skills/
// quote holds only the ThirdEye API client (business logic) for the
// quote_random tool, whose tool definition and registration live in
// internal/skills/system. Any future growth of the quote domain should move
// registration into the quote package itself to restore the standard shape.
package skills
