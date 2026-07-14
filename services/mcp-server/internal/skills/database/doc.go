// Package database implements the "database" skill: native MongoDB tooling for
// the Custom-VPS-MCP-Engine, built on the official Go driver
// (go.mongodb.org/mongo-driver/v2) — the second and final sanctioned external
// dependency of this module (see go.mod for the amended policy).
//
// # Why native, not scripts
//
// This skill replaces the legacy scripts/ workflow (docker exec + `mongo --eval`
// with interpolated JavaScript run as root). The native driver eliminates the
// entire eval-injection vulnerability class (Audit C-8): arguments are parsed as
// MongoDB Extended JSON into BSON documents — there is no shell, no JavaScript,
// and no string interpolation anywhere in the execution path. There is
// deliberately NO db_run_script fallback tool; the legacy scripts are deprecated
// (see ARCHITECTURE.md §7 in the parent repository).
//
// # Tools
//
// Read tools (always registered):
//
//   - db_collections      — allowlisted namespaces + estimated counts + limits.
//   - db_find             — bounded query with filter/projection/sort/limit/skip.
//   - db_count            — countDocuments for a filter (also the first half of
//     the destructive-write handshake).
//   - db_aggregate        — bounded aggregation with stage validation.
//   - user_list           — bounded, redacted listing of API users
//     (replaces scripts/list_users.sh).
//   - quote_owner_lookup  — quote ObjectId → owner uid
//     (replaces scripts/find_user_by_post_id.sh).
//
// Write tools (registered ONLY when MCP_DB_ALLOW_WRITES=true — fail-closed,
// mirroring the auth middleware philosophy):
//
//   - db_insert       — bounded ordered insert (max 25 documents).
//   - db_update       — single-doc by default; multi requires a count handshake.
//   - db_delete       — single-doc by default; multi requires a count handshake.
//   - user_provision  — atomic user creation with crypto/rand credentials
//     (replaces scripts/add_user.sh, fixing its check-then-insert race).
//   - user_revoke     — single-user removal (replaces scripts/remove_user.sh).
//
// # Guardrail contract (non-negotiable)
//
//  1. NAMESPACE ALLOWLIST: every tool resolves its collection through
//     resolveNamespace; only qdata, users, tokens (in $MONGO_DATABASE, default
//     qdb) and tdata (in tarotdb) are reachable. admin/config/local and
//     arbitrary databases are unreachable even when connected as root.
//  2. RESULT BOUNDING: reads are clamped to a hard cap of 50 documents
//     (default 20), a 48 KiB response byte budget, a 10 000 skip ceiling, and a
//     10s operation timeout (the driver propagates the context deadline to the
//     server). An empty filter on a huge collection is safe by construction.
//  3. EMPTY-FILTER REJECTION: db_update and db_delete refuse a {} filter with
//     no bypass flag. Collection-wide destruction stays a host-only operation.
//  4. MANY-WRITE HANDSHAKE: many=true requires expected_matches; the handler
//     re-counts and aborts on mismatch, and refuses any multi-write touching
//     more than 100 documents.
//  5. OPERATOR BANS: $where/$function/$accumulator are rejected everywhere
//     (server-side JavaScript is the eval hole reborn); $out/$merge are
//     rejected in pipelines (writes masquerading as reads); $lookup/$unionWith/
//     $graphLookup targets must be allowlisted and same-database.
//  6. SECRET REDACTION: any field named "authorization" is redacted to a
//     sha256-prefix marker in ALL tool output unless the caller explicitly
//     opts in (include_secrets / include_tokens). user_provision returns the
//     new token exactly once, by design.
//  7. AUDIT TRAIL: every write emits a structured log line (tool, namespace,
//     filter digest, affected counts).
//
// # Connection design
//
// Credentials resolve in precedence order: MCP_MONGO_URI (full override) →
// MCP_MONGO_USERNAME/MCP_MONGO_PASSWORD (preferred: the scoped mcp_agent user,
// readWrite on qdb+tarotdb only — creation runbook in ARCHITECTURE.md) →
// MONGO_INITDB_ROOT_USERNAME/MONGO_INITDB_ROOT_PASSWORD (fallback, logged as a
// WARNING). The host defaults to the stack's static IP 172.255.255.2:27017
// with authSource=admin, mirroring the api service's connection logic.
//
// The client connects LAZILY on first tool use (the MCP server must boot and
// serve its other skills even when MongoDB is down) behind a mutex — not
// sync.Once, so a failed connect is retried on the next call. On failure every
// tool returns a self-healing IsError result pointing the agent at
// system_logs service=dbs.
//
// # Self-healing contract
//
// Per the engine's core directives, tools never panic and never surface vague
// protocol errors: every validation failure returns IsError:true with a nil Go
// error and a message that names the offending argument, explains why it was
// rejected, and shows a corrected example the LLM can imitate on its next call.
package database
