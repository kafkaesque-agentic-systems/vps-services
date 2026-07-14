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
//   - snapshot_create — archives /opt/micro-services.d/services (excluding image
//     and vol) into /opt/micro-services.d/snapshots/.
//   - snapshot_restore — extracts a named archive back into the services
//     directory after renaming the active tree to services.bak-<timestamp>.
//
// # Paths and exclusions
//
// The default source directory is /opt/micro-services.d/services. The default
// snapshot directory is /opt/micro-services.d/snapshots. Two top-level
// subdirectories under the source are excluded from every archive to conserve
// disk space: image and vol.
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
