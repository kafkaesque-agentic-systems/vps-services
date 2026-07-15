// Package snapshot implements the "snapshot" skill: a domain module providing
// VPS codebase backup and restore tooling for the Custom-VPS-MCP-Engine.
//
// # Intended capabilities
//
// This skill exposes operational tooling that creates point-in-time archives
// of the active services directory on the VPS before major AI-driven changes
// are applied, and restores a prior archive when a deployment must be reverted.
// Archives are gzip-compressed tar files under a dedicated snapshots directory.
//
// # Tools
//
//   - snapshot_create — archives /opt/micro-services.d (excluding image, vol,
//     the snapshots store itself, and .bak-* restore backups) into
//     /opt/micro-services.d/snapshots/.
//   - snapshot_restore — CONTENTS-SWAP restore: moves the tree's current
//     top-level entries into a .bak-<timestamp> backup directory INSIDE the
//     tree, preserving the entire archive-exclusion set in place (snapshots,
//     image, vol, .bak-*) — excluded entries cannot be re-created by
//     extraction, so swapping them out would lose them — then extracts the
//     named archive into the root; rolls back automatically on failure.
//
// # Paths and exclusions (2026-07-14 layout migration)
//
// The default source directory is /opt/micro-services.d — the codebase now
// lives at the root of that tree (the former services/ level was removed).
// Two consequences shape this package's design:
//
//  1. The snapshot store /opt/micro-services.d/snapshots is NESTED inside the
//     source, so archives exclude "snapshots" (correctness: prevents each
//     archive from recursively containing all previous archives) alongside
//     image, vol (disk economy), and .bak-* (restore backups).
//  2. The source root is the go-mcp container's BIND-MOUNT POINT, which
//     cannot be renamed from inside the container (EBUSY). snapshot_restore
//     therefore swaps the tree's CONTENTS rather than the tree itself; see
//     restore.go for the full rationale.
//
// Archives created before the migration contain the old services/-rooted
// layout and are NOT compatible with snapshot_restore against the new root.
//
// # Self-healing contract
//
// Per the engine's core directives, tools in this package must never panic on
// bad input. They validate preconditions (source exists, tar available,
// destination writable) and, on failure, return descriptive MCP tool-error
// results whose text tells the calling LLM exactly what went wrong — enabling
// caller-side self-healing rather than opaque protocol failures.
//
// # Structure of a skill package
//
// A skill package is expected to provide:
//
//   - Typed input/output structs with schema tags for each tool.
//   - Pure handler functions containing the business logic.
//   - A Register export that the central registrar calls to attach the skill's
//     tools to the core MCP server.
package snapshot
