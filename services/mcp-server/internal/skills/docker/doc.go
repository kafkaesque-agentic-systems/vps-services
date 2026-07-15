// Package docker implements the "docker" skill: MCP tools for lifecycle management
// and log inspection of the VPS micro-services Compose stack.
//
// # Tools
//
//   - system_down — runs `docker compose down` (never removes volumes).
//   - system_up — runs `docker compose up -d`, optionally with `--build`, after
//     loading variables from .environs.
//   - system_logs — returns a static snapshot of container logs (no follow).
//
// # Compose project
//
// All commands execute with working directory /opt/micro-services.d (the
// codebase root since the 2026-07-14 layout migration), which must contain
// docker-compose.yml and .environs on the VPS host.
//
// # Infrastructure requirements
//
// The go-mcp container must have the Docker CLI, Compose plugin, and read/write
// access to /var/run/docker.sock. These are provisioned by this module's
// Dockerfile and by the COMPOSITE stack descriptor in the PARENT services
// repository (this module is a nested git repo): see ../docker-compose.yml and
// ../ARCHITECTURE.md relative to this module's root.
//
// # Self-termination caveat (system_down)
//
// The compose client for system_down runs INSIDE the go-mcp container, which is
// itself part of the stack being torn down. Once docker stops go-mcp, the
// client dies mid-orchestration: the teardown may complete only partially and
// the tool's success report may never reach the caller. Operators must verify
// and finish the shutdown on the VPS host.
//
// # Self-healing contract
//
// Tools validate inputs (service allowlist, .environs presence for system_up)
// and return descriptive MCP tool-error results on failure.
package docker
