// Package system implements the "system" skill: a domain module providing
// operational, server-introspection tools for the Custom-VPS-MCP-Engine.
//
// # Intended capabilities (delivered in Phase 4)
//
// This skill will expose foundational operational tooling, such as a health /
// uptime probe, that lets an MCP client (and by extension an LLM) confirm the
// server is alive and inspect basic runtime state. It serves as the reference
// implementation that every future skill package should mirror in structure.
//
// # Self-healing contract
//
// Per the engine's core directives, tools in this package must never panic on
// bad input. They validate arguments strictly against their declared JSON
// schemas and, on failure, return descriptive MCP tool-error results whose text
// tells the calling LLM exactly how to fix its next invocation — enabling
// caller-side self-healing rather than opaque HTTP failures.
//
// # Structure of a skill package
//
// A skill package is expected to provide:
//
//   - Typed input/output structs with schema tags for each tool.
//   - Pure handler functions containing the business logic.
//   - A Register-style export that the central registrar (Phase 4) calls to
//     attach the skill's tools to the core MCP server.
package system
