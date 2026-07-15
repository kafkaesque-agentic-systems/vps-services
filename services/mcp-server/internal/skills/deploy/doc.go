// Package deploy implements the push_codebase MCP tool for synchronizing a local
// development checkout to the production VPS over rsync+SSH.
//
// # Local-only execution
//
// push_codebase MUST run on a locally started mcp-server instance that has
// access to the developer's working tree and the rsync binary. The production
// go-mcp container on the VPS does not hold a local checkout and cannot invoke
// rsync in the local→remote direction. When DEPLOY_SSH_TARGET is unset or
// rsync is unavailable, the handler fails fast with a self-healing message.
//
// # Deployment lifecycle
//
//  1. Pre-flight: connect to the remote production MCP server over HTTPS+SSE
//     and call snapshot_create to archive the current VPS codebase.
//  2. Sync: run rsync -az --delete -i from DEPLOY_LOCAL_ROOT to the remote
//     services directory, writing an itemized ledger to deploy_ledgers/.
//  3. Response: return the snapshot summary plus the full itemized ledger so
//     the calling agent can audit every file change.
//
// # Itemized ledger legend (rsync -i / --itemize-changes)
//
// Each line describes one filesystem change. Common prefixes:
//
//   - >f+++++++++ — new regular file transferred to the remote
//   - >f..T...... — existing file updated (timestamp and/or size changed)
//   - >f..T...... — file content replaced (checksum differs; dots vary by change type)
//   - cd+++++++++ — new directory created on the remote
//   - *deleting   — stale remote file or directory removed (--delete)
//
// See rsync(1) for the full YXcstpoguax field legend. The deploy tool persists
// this output locally via --log-file under deploy_ledgers/ and returns it in
// the tool response.
//
// # Safety overrides
//
// rsync runs with --delete so files absent locally are removed on the VPS
// (stale code cleanup). Persistent runtime data is protected by exclusions:
// image/, vol/, snapshots/, .env, .environs, and local-only artifacts (.git/,
// node_modules/, .venv/, __pycache__/, deploy_ledgers/). The snapshots/
// exclusion is load-bearing: since the 2026-07-14 layout migration the VPS
// snapshot store lives inside the sync root (/opt/micro-services.d/snapshots)
// and would otherwise be erased by --delete on every push.
package deploy
