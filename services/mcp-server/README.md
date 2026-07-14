# Custom-VPS-MCP-Engine

![Language](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Transport](https://img.shields.io/badge/transport-HTTP%2BSSE-blue)
![Protocol](https://img.shields.io/badge/MCP-go--sdk%20v1.6.1-6E56CF)
![Container](https://img.shields.io/badge/Docker-multi--stage%20Alpine-2496ED?logo=docker&logoColor=white)
![Reverse%20Proxy](https://img.shields.io/badge/Nginx-TLS%20%2B%20SSE-009639?logo=nginx&logoColor=white)
![License](https://img.shields.io/badge/dependencies-stdlib%20%2B%20go--sdk%20%2B%20mongo--driver-brightgreen)

A **production-ready, remotely-hosted [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server** written in Go. It exposes tools ("skills") to LLM clients — Claude Desktop, Ollama, or any custom MCP client — over an **HTTP + Server-Sent Events (SSE)** transport, secured behind a strict Bearer-token gate and an Nginx TLS-terminating reverse proxy.

The engine is built on the **Go standard library plus the official [`go-sdk`](https://github.com/modelcontextprotocol/go-sdk) only** — no third-party web frameworks — and ships as a tiny, static, non-root Alpine container.

---

## Table of Contents

1. [Features](#1-features)
2. [Architecture](#2-architecture)
3. [Available Tools](#3-available-tools)
4. [Repository Anatomy](#4-repository-anatomy)
5. [Configuration Matrix](#5-configuration-matrix)
6. [Local Development](#6-local-development)
7. [Operational Verification (Testing & Health Checks)](#7-operational-verification-testing--health-checks)
8. [Deployment (Docker + Nginx Runbook)](#8-deployment-docker--nginx-runbook)
9. [Client Connection](#9-client-connection)
10. [Troubleshooting & Common Failure Modes](#10-troubleshooting--common-failure-modes)

---

## 1. Features

- **🔒 Secure by default (fail-closed auth).** A single custom middleware — [`auth.TokenAuthMiddleware`](internal/auth/middleware.go:71) — wraps the *entire* router at one choke point, so **no route can ever be exposed unauthenticated**. It:
  - Reads the expected secret from `MCP_SECRET_TOKEN` **per request** (no stale cache).
  - **Fails closed with HTTP 500** if the server secret is unset — never falls through to "allow".
  - Uses [`crypto/subtle.ConstantTimeCompare`](internal/auth/middleware.go:114) to defeat timing attacks.
  - Advertises the expected scheme via a `WWW-Authenticate` header for self-healing clients.
- **📡 Native HTTP + SSE transport.** The transport layer ([`SSETransport`](internal/mcpengine/transport.go:39)) is a thin wrapper over the SDK's [`mcp.NewSSEHandler`](internal/mcpengine/transport.go:63), which natively manages session creation, session-ID generation, endpoint advertisement, and POST-to-session routing — eliminating an entire class of hand-rolled concurrency bugs.
- **🧠 Self-healing tools.** Tools register via the generic [`mcp.AddTool`](internal/skills/system/system.go:79), which **auto-generates JSON input schemas via reflection**. On semantic failure (e.g. a bad timezone), a handler returns an `IsError: true` payload with a **`nil` Go error**, delivering actionable correction instructions directly to the LLM instead of a swallowed protocol failure.
- **🛑 Graceful shutdown.** [`signal.NotifyContext`](cmd/server/main.go:109) traps `SIGINT`/`SIGTERM` (the signals Docker sends on `stop`) and drains in-flight requests within a bounded grace period, so long-lived SSE streams close cleanly on container restart.
- **📦 Tiny, hardened runtime image.** Multi-stage `Dockerfile` builds a static binary (`CGO_ENABLED=0`) and copies **only that binary** into `alpine:3.20`, running as a dedicated **non-root** user.
- **🧱 Minimal dependency surface.** Standard library for routing, middleware, JSON, and transport plumbing; the official MCP SDK is the only sanctioned direct dependency.

---

## 2. Architecture

### 2.1 Request flow

```
                         ┌──────────────────────────────────────────────┐
                         │                   VPS Host                    │
                         │                                               │
  MCP Client             │   ┌─────────────┐        ┌────────────────┐   │
 (Claude Desktop,        │   │    Nginx    │        │   go-mcp       │   │
  Ollama, curl,   HTTPS  │   │  reverse    │  HTTP  │  container     │   │
  Python SDK) ───────────┼──▶│  proxy      ├───────▶│  (Go binary)   │   │
   Authorization:        │   │  :443 (TLS) │ 172.   │  :8080         │   │
   Bearer <token>        │   │             │ 255.   │                │   │
                         │   │ proxy_buffer│ 255.6  │                │   │
                         │   │  ing off    │        │                │   │
                         │   └─────────────┘        └────────────────┘   │
                         └──────────────────────────────────────────────┘

Inside the Go process (composition root: cmd/server/main.go):

  Request
     │
     ▼
  auth.TokenAuthMiddleware   ← Bearer-token gate (fail-closed, constant-time)
     │  (401 missing/invalid token · 500 if server secret unset)
     ▼
  http.ServeMux              ← method-scoped routes: "GET /sse", "POST /message"
     │
     ▼
  mcp.SSEHandler (SSETransport)   ← SDK-managed sessions + routing
     │
     ▼
  mcp.Server (mcpengine)     ← identity handshake + tool registry
     │
     ▼
  Skills (internal/skills/system)   ← system_health, system_time
```

**Client → HTTPS (Bearer) → Nginx → Docker → Go Server → Middleware → Mux → SSE Transport → MCP Engine → Skills.**

### 2.2 Wiring & lifecycle

The entire process lifecycle is traceable top-to-bottom in [`cmd/server/main.go`](cmd/server/main.go:55):

1. **Config resolution** — read `PORT` (default `8080`); warn loudly if `MCP_SECRET_TOKEN` is unset.
2. **Engine construction** — [`mcpengine.NewServer()`](internal/mcpengine/server.go:54) declares the server identity (`Custom-VPS-MCP-Engine` v`1.0.0`).
3. **Skill registration** — [`registerCustomSkills`](cmd/server/main.go:182) → [`system.Register`](internal/skills/system/system.go:77). This happens **before** the transport serves, so the very first MCP `initialize` handshake advertises the full tool set.
4. **Transport** — [`mcpengine.NewSSETransport`](internal/mcpengine/transport.go:62) wraps the engine; [`RegisterRoutes`](internal/mcpengine/transport.go:85) binds `GET /sse` and `POST /message`.
5. **Security** — the whole mux is wrapped exactly once by [`auth.TokenAuthMiddleware`](internal/auth/middleware.go:71).
6. **HTTP server** — `ReadHeaderTimeout` is set (Slowloris mitigation); **`WriteTimeout` is deliberately unset** so SSE streams are never severed mid-flight.
7. **Graceful shutdown** — on `SIGINT`/`SIGTERM`, drains within a **15s** grace window.

### 2.3 Architectural patterns & dependencies

| Concern | Choice | Rationale |
| :--- | :--- | :--- |
| Composition root | `cmd/server/main.go` | Single place that wires config, engine, transport, and auth. |
| Ports & adapters | `mcpengine` isolates the SDK | The only package that touches `mcp.NewServer`/`mcp.NewSSEHandler`; confines SDK API churn. |
| Skill registrar seam | `registerCustomSkills` + per-skill `Register()` | Adding a capability is a 2-line change; transport/auth/lifecycle never change. |
| Secure-by-default middleware | single-wrap decorator | A developer must go out of their way to *remove* protection, never to *add* it. |
| Self-healing tools | `IsError: true` + `nil` error | Error guidance reaches the LLM as tool output, not a protocol fault. |
| **Direct dependencies (exactly two)** | `github.com/modelcontextprotocol/go-sdk v1.6.1` + `go.mongodb.org/mongo-driver/v2` | See [`go.mod`](go.mod). The MCP SDK is the protocol layer; the official Mongo driver powers the database skill natively (no `mongo --eval` injection surface). Everything else is stdlib. |

**Indirect dependencies** (pulled in by the SDK, see [`go.mod`](go.mod:25)): `github.com/google/jsonschema-go`, `github.com/segmentio/asm`, `github.com/segmentio/encoding`, `github.com/yosida95/uritemplate/v3`, `golang.org/x/oauth2`, `golang.org/x/sys`.

---

## 3. Available Tools

Tools are namespaced `<domain>_<action>` and registered in skill packages under [`internal/skills/`](internal/skills/). The composition root calls each skill's `Register()` from [`cmd/server/main.go`](cmd/server/main.go).

### `system_health`

> Reports the operational health of the MCP server — liveness, process uptime, Go runtime version, active goroutine count, and current UTC time. **Takes no arguments.**

- **Input schema:** `HealthInput{}` (empty struct → object with no properties).
- **Handler:** [`handleHealth`](internal/skills/system/system.go:113) — never fails, never panics; snapshots cheap runtime metrics.
- **Sample text output:**

  ```
  status: ok
  uptime: 3m20s (200 seconds)
  go_version: go1.25.0
  num_goroutine: 8
  server_time_utc: 2026-07-06T14:03:12Z
  ```

### `system_time`

> Returns the current server time. Optionally accepts a `timezone` argument (an IANA name such as `America/New_York`); if omitted, UTC is used. Returns a descriptive, correctable error if the timezone is invalid.

- **Input schema:** `TimeInput{ Timezone string }` — optional, `json:"timezone,omitempty"`, with an LLM-facing `jsonschema` description ([system.go:51](internal/skills/system/system.go:51)).
- **Handler:** [`handleTime`](internal/skills/system/system.go:148) — validates the zone with `time.LoadLocation`.
- **Self-healing behavior:** an invalid zone yields `IsError: true` with a `nil` Go error and this actionable message:

  ```
  Invalid 'timezone' value "Mars/Phobos": unknown time zone Mars/Phobos.
  Provide a valid IANA timezone name, for example: "UTC", "America/New_York",
  "Europe/London", or "Asia/Tokyo". Alternatively, omit the 'timezone' argument
  entirely to receive the time in UTC.
  ```

  > ⚠️ **Runtime dependency:** non-UTC lookups require the IANA tz database. The runtime image installs `tzdata` for exactly this reason ([Dockerfile:77](Dockerfile:77)). Without it, every non-UTC lookup fails.

### `quote_random`

> Fetches a random quote from the ThirdEye API at `https://api.thirdeye.live/quote`. **Takes no arguments.**

- **Input schema:** `QuoteInput{}` (empty struct).
- **Handler:** [`handleQuote`](internal/skills/system/system.go:192) — delegates to [`quote.FetchRandomQuote`](internal/skills/quote/random.go).
- **Sample text output:** `"People say nothing is impossible... -aa milne"`

### `snapshot_create`

> Creates a gzip-compressed tar archive of the VPS services codebase at `/opt/micro-services.d/services`, writing the snapshot to `/opt/micro-services.d/snapshots/`. Excludes the top-level `image` and `vol` directories. **Takes no arguments.** Intended for use before major AI-driven changes.

- **Input schema:** `SnapshotInput{}` (empty struct → object with no properties).
- **Handler:** [`handleSnapshotCreate`](internal/skills/snapshot/snapshot.go) — invokes `tar` via `os/exec` with fixed paths (no shell).
- **Archive naming:** `snapshot-YYYY-MM-DD_HH-MM-SS.tar.gz` (Go reference time `2006-01-02_15-04-05`).
- **Sample text output:**

  ```
  status: ok
  archive: /opt/micro-services.d/snapshots/snapshot-2026-07-12_11-38-00.tar.gz
  source: /opt/micro-services.d/services
  excluded: image, vol
  created_at_utc: 2026-07-12T16:38:00Z
  ```

  > ⚠️ **Deployment dependency:** the `go-mcp` container must bind-mount the host paths (see [Section 8.2](#82-compose-stack)) and both directories must be writable by the container's `mcp` user.

### `snapshot_restore`

> Restores the VPS services codebase from a snapshot archive in `/opt/micro-services.d/snapshots/` into `/opt/micro-services.d/services`. **Requires the `filename` argument** (archive basename only).

- **Input schema:** `RestoreInput{ Filename string }` — required basename such as `snapshot-2026-07-12_12-00-00.tar.gz`.
- **Handler:** [`handleSnapshotRestore`](internal/skills/snapshot/restore.go) — validates archive, renames active `services` to `services.bak-<timestamp>`, extracts with `tar -xzf`, rolls back on failure.
- **Failsafe:** On extraction error, removes partial `services/` and renames `services.bak-*` back to `services`.
- **Sample text output:**

  ```
  status: ok
  restored_from: /opt/micro-services.d/snapshots/snapshot-2026-07-12_12-00-00.tar.gz
  services_dir: /opt/micro-services.d/services
  previous_services_backup: /opt/micro-services.d/services.bak-2026-07-12_12-05-00
  restored_at_utc: 2026-07-12T17:05:00Z
  ```

### `system_down`

> Gracefully stops the micro-services stack via **`docker compose down`** in `/opt/micro-services.d/services`. **Does not remove Docker volumes** — the MongoDB `quotes-api` volume and all persistent data are preserved. Takes no arguments.

- **Input schema:** `DownInput{}` (empty struct).
- **Command executed:** `docker compose down` — strictly **no** `-v` or `--volumes` flag (hardcoded safeguard in Go).
- **Warning:** Stops the `go-mcp` container; MCP becomes unreachable until `system_up` or manual host intervention.

### `system_up`

> Starts the stack via `docker compose up -d` in `/opt/micro-services.d/services`, loading environment variables from `.environs` (parsed in Go, not via shell `source`).

- **Input schema:** `UpInput{ Build bool }` — optional `build: true` adds `--build`.
- **Commands:** `docker compose up -d` or `docker compose up -d --build`.

### `system_logs`

> Returns a static log snapshot via `docker compose logs --tail N`. Never streams or follows (`-f` is forbidden).

- **Input schema:** `LogsInput{ Service string, Tail int }` — `service` optional (`reverse-proxy`, `web`, `api`, `dbs`, `go-mcp`); `tail` defaults to **100**.
- **Commands:** `docker compose logs --tail <N>` or `docker compose logs --tail <N> <service>`.

### `push_codebase`

> **Local-only:** Synchronizes the developer's local micro-services checkout to the production VPS via `rsync` over SSH. Must run on a **locally started** mcp-server (not production go-mcp). Pre-flight calls remote production `snapshot_create` over HTTPS+SSE before syncing. **Takes no arguments.**

- **Input schema:** `PushInput{}` (empty struct).
- **Handler:** [`handlePushCodebase`](internal/skills/deploy/deploy.go) — sequential lifecycle: remote `snapshot_create` → local `rsync -az --delete -i`.
- **Required environment (local MCP process):**

  | Variable | Purpose |
  | :--- | :--- |
  | `DEPLOY_SSH_TARGET` | SSH destination (`user@host`) — **required** |
  | `MCP_SECRET_TOKEN` | Bearer token for pre-flight production MCP — **required** |
  | `DEPLOY_LOCAL_ROOT` | Local repo root (default: auto-detect `docker-compose.yml` upward from cwd) |
  | `DEPLOY_REMOTE_PATH` | Remote sync root (default: `/opt/micro-services.d/services/`) |
  | `DEPLOY_MCP_URL` | Production MCP SSE URL (default: `https://api.thirdeye.live/sse`) |

- **Rsync flags:** `-a`, `-z`, `--delete`, `-i`, `--log-file=deploy_ledgers/deploy-YYYY-MM-DD_HH-MM-SS.log`, `-e "ssh -o BatchMode=yes"` (non-interactive SSH — fails fast instead of hanging on a password prompt); the whole sync is bounded by a 30-minute timeout
- **Exclusions:** `.git/`, `node_modules/`, `.venv/`, `__pycache__/`, `.env`, `.environs`, `image/`, `vol/`, `deploy_ledgers/`
- **Itemized ledger legend:** `>f+++++++++` new file; `>f..T......` updated file; `cd+++++++++` new directory; `*deleting` stale remote path removed
- **Sample local dev invocation:**

  ```bash
  export MCP_SECRET_TOKEN=your-production-token
  export DEPLOY_SSH_TARGET=deploy@your-vps
  export DEPLOY_LOCAL_ROOT=/path/to/services/
  go run ./cmd/server
  # Then call push_codebase via your MCP client (no arguments)
  ```

  > ⚠️ **Production go-mcp cannot run this tool** — it has no local checkout or rsync source. After a successful push, rebuild/restart on the VPS (`system_up` with `build=true` or `docker compose up -d --build`).

### Database skill (`db_*`, `user_*`, `quote_owner_lookup`)

> Native MongoDB tooling on the **official Go driver v2** — no shell, no `mongo --eval`, no JavaScript. Arguments are MongoDB **Extended JSON strings** (`{"$oid": "..."}` for ObjectIds). Full guardrail contract and the `mcp_agent` least-privilege runbook: [`../ARCHITECTURE.md`](../ARCHITECTURE.md) § "MCP database skill"; package charter: [`internal/skills/database/doc.go`](internal/skills/database/doc.go).

**Read tools (always registered):**

| Tool | Summary |
| :--- | :--- |
| `db_collections` | Allowlisted namespaces (`qdata`, `tdata`, `tokens`, `users`) + estimated counts + the skill's limits. No arguments. |
| `db_find` | Bounded find: `filter` (required), optional `projection`/`sort`/`limit`/`skip`/`include_secrets`. Hard cap **50** docs (default 20), **48 KiB** byte budget, skip ≤ 10 000; reports `has_more`/`next_skip` for pagination. |
| `db_count` | `countDocuments` for a filter; required first step before any `many=true` write. |
| `db_aggregate` | Read-only pipeline (`pipeline` as an Extended JSON array string). `$out`/`$merge`/`$where`/`$function`/`$accumulator` banned; `$lookup`/`$unionWith` targets allowlist + same-database checked; a terminal `$limit` is **always appended in Go**. |
| `user_list` | Bounded user listing sorted by email; `authorization` tokens **redacted** to `[REDACTED sha256:…]` unless `include_tokens=true`. Replaces `scripts/list_users.sh`. |
| `quote_owner_lookup` | Quote ObjectId (24 hex) → owner `uid` + attribution. Replaces `scripts/find_user_by_post_id.sh`. |

**Write tools (registered ONLY when `MCP_DB_ALLOW_WRITES=true` — fail-closed read-only default):**

| Tool | Summary |
| :--- | :--- |
| `db_insert` | Ordered insert of an Extended JSON array, max **25** docs/call; duplicate-key errors reported self-healingly. |
| `db_update` | **Non-empty** filter + `$`-operator update required (bare replacements rejected). Default updates ONE doc; `many=true` needs `expected_matches` from `db_count` (server re-counts, aborts on mismatch), ceiling **100** docs. `upsert` only with `many=false`. |
| `db_delete` | **Empty `{}` filters always rejected — no bypass flag** (collection wipes are host-only). Same single-doc default + count handshake + 100-doc ceiling as `db_update`. |
| `user_provision` | Validates email, generates crypto-random uid/token (legacy formats), inserts atomically; token returned **once**. Replaces `scripts/add_user.sh`. |
| `user_revoke` | Deletes exactly one user by email. Replaces `scripts/remove_user.sh`. |

> ⚠️ The connection is **lazy**: the server boots and serves all other skills even when MongoDB is down; database tools return self-healing errors (pointing at `system_logs service=dbs`) and retry on the next call. Connecting with root credentials (fallback) logs a warning — prefer the scoped `mcp_agent` user (§5.3).

---

## 4. Repository Anatomy

```
go-mcp-server/
├── cmd/
│   └── server/
│       ├── main.go            # Composition root: config, wiring, HTTP server, graceful shutdown
│       └── doc.go             # Package-level documentation
├── internal/
│   ├── auth/
│   │   ├── middleware.go      # Bearer-token gate (fail-closed, constant-time, header+query channels)
│   │   ├── middleware_test.go # Table-driven tests over the full auth decision matrix
│   │   └── doc.go
│   ├── mcpengine/
│   │   ├── server.go          # mcp.NewServer wrapper; server identity (name/version)
│   │   ├── transport.go       # SSETransport over mcp.NewSSEHandler; /sse + /message routes
│   │   └── doc.go
│   └── skills/
│       ├── doc.go             # Skills-layer overview
│       ├── quote/
│       │   └── random.go      # ThirdEye API client for quote_random
│       ├── snapshot/
│       │   ├── snapshot.go    # snapshot_create tool (VPS tar archive)
│       │   ├── restore.go     # snapshot_restore tool (failsafe extraction)
│       │   ├── snapshot_test.go
│       │   ├── restore_test.go
│       │   └── doc.go
│       ├── docker/
│       │   ├── lifecycle.go   # system_down, system_up, system_logs
│       │   ├── environs.go    # .environs parser for system_up
│       │   ├── compose.go     # runCompose helper
│       │   └── doc.go
│       ├── deploy/
│       │   ├── deploy.go      # push_codebase tool (local rsync → VPS)
│       │   ├── preflight.go   # remote production snapshot_create via SSE client
│       │   ├── rsync.go       # rsync builder/runner + deploy ledger
│       │   ├── config.go      # DEPLOY_* env resolution
│       │   ├── auth.go        # Bearer RoundTripper for pre-flight MCP
│       │   ├── deploy_test.go
│       │   └── doc.go
│       ├── database/
│       │   ├── doc.go         # Skill charter, threat model, guardrail contract
│       │   ├── database.go    # Register(): read tools always, write tools gated
│       │   ├── client.go      # Lazy mutex-guarded mongo.Client (retryable connect)
│       │   ├── config.go      # MCP_MONGO_* / root-fallback credential resolution
│       │   ├── namespace.go   # THE collection allowlist (qdata/users/tokens/tdata)
│       │   ├── extjson.go     # ExtJSON parsing, operator bans, bounding, redaction
│       │   ├── read.go        # db_collections, db_find, db_count, db_aggregate
│       │   ├── write.go       # db_insert, db_update, db_delete (+ count handshake)
│       │   ├── users.go       # user_provision/user_revoke/user_list, quote_owner_lookup
│       │   └── guardrails_test.go
│       └── system/
│           ├── system.go      # system_health, system_time, quote_random tools
│           ├── system_test.go # Handler tests: health status + system_time timezone matrix
│           └── doc.go
├── vps-docs/                  # REFERENCE ops stack (never modified) — source of the composites below
│   ├── docker-compose.yml     # Original quotes stack the composite was derived from
│   └── nginx.conf             # Original Nginx config the composite was derived from
├── Dockerfile                 # Multi-stage: golang:1.25-alpine builder → alpine:3.20 runtime (non-root)
├── client-connection-guide.md # End-user connection recipes (curl, Claude Desktop, Python)
├── mcp-dev-plan.md            # Phase-by-phase build plan / design record
├── go.mod                     # Module `mcp-server`; go 1.25.0; single direct dep: go-sdk v1.6.1
└── go.sum                     # Dependency checksums
```

> **Where the composite deployment descriptors live:** this module is a **nested git
> repository** inside the parent `services/` tree. The COMPOSITE stack definition
> (which adds the `go-mcp` service at 172.255.255.6) is at
> [`../docker-compose.yml`](../docker-compose.yml), and the SSE-tuned proxy config is at
> [`../prx/nginx.conf`](../prx/nginx.conf). System-wide documentation lives at
> [`../ARCHITECTURE.md`](../ARCHITECTURE.md). Neither composite file exists inside this
> module — only the untouched originals under `vps-docs/`.

### Role of each major piece

| Path | Role |
| :--- | :--- |
| [`cmd/server/main.go`](cmd/server/main.go) | The **only** entry point. Reads env, builds the engine, registers skills, wires transport + auth, runs the HTTP server with graceful shutdown. |
| [`internal/auth/middleware.go`](internal/auth/middleware.go) | The security perimeter. Single-wrap Bearer-token decorator; the one place that enforces authentication. |
| [`internal/auth/middleware_test.go`](internal/auth/middleware_test.go) | Table-driven coverage of every auth branch: unset secret (500), missing credential (401), invalid token (401), valid header (200), valid `?token=` fallback (200). |
| [`internal/skills/system/system_test.go`](internal/skills/system/system_test.go) | Handler-level coverage of `system_health` (`status: ok`) and `system_time` (UTC default, valid IANA zone, and the self-healing invalid-zone path). |
| [`internal/mcpengine/server.go`](internal/mcpengine/server.go) | SDK isolation layer — constructs the `*mcp.Server` and owns the advertised identity (`Custom-VPS-MCP-Engine` / `1.0.0`). |
| [`internal/mcpengine/transport.go`](internal/mcpengine/transport.go) | HTTP + SSE adapter. Binds `GET /sse` and `POST /message` to the SDK's `SSEHandler`. |
| [`internal/skills/system/system.go`](internal/skills/system/system.go) | The system domain layer — defines and registers `system_health`, `system_time`, and `quote_random`, plus the `textResult`/`errorResult` helpers. |
| [`internal/skills/snapshot/snapshot.go`](internal/skills/snapshot/snapshot.go) | The snapshot domain layer — registers `snapshot_create` and `snapshot_restore`; archives and restores `/opt/micro-services.d/services`. |
| [`internal/skills/snapshot/restore.go`](internal/skills/snapshot/restore.go) | Restore workflow: validate filename, rename active tree, extract archive, rollback on failure. |
| [`internal/skills/snapshot/snapshot_test.go`](internal/skills/snapshot/snapshot_test.go) | Coverage of tar archive creation (exclusions), missing-source errors, and handler self-healing contract. |
| [`internal/skills/docker/lifecycle.go`](internal/skills/docker/lifecycle.go) | Compose lifecycle tools; `system_down` runs `docker compose down` without volume removal. |
| [`internal/skills/deploy/deploy.go`](internal/skills/deploy/deploy.go) | Local-to-VPS `push_codebase` tool; pre-flight remote snapshot + rsync with deploy ledger. |
| [`vps-docs/`](vps-docs/docker-compose.yml) | Untouched reference ops stack. The composites generated from it live in the PARENT repo: [`../docker-compose.yml`](../docker-compose.yml) and [`../prx/nginx.conf`](../prx/nginx.conf). |
| [`Dockerfile`](Dockerfile) | Reproducible, hermetic, hardened multi-stage build. |

> **Convention:** `internal/` packages are import-private to this module — external repos cannot import them, keeping the public surface at zero.

---

## 5. Configuration Matrix

The engine's core (transport + auth) is configured by **exactly two** environment variables (§5.1). The database skill adds its own optional set (§5.3), and `push_codebase` its local-only set (§5.4). The composite `../docker-compose.yml` also references variables belonging to the *co-located reference stack* (web/api/dbs); those are listed separately (§5.5) so the boundary is unambiguous.

### 5.1 Engine variables (this project)

| Variable Name | Type / Format | Default Value | Description | Required |
| :--- | :--- | :--- | :--- | :--- |
| `MCP_SECRET_TOKEN` | string (shared secret) | *(none)* | The Bearer token every client must present. Injected at deploy time — **never** committed. If unset, the middleware **fails closed (HTTP 500)** and rejects all traffic. See [`middleware.go:20`](internal/auth/middleware.go:20). | **Yes** |
| `PORT` | integer (TCP port) | `8080` | Container-internal HTTP listen port. Not published to the host; Nginx proxies to it over the docker network. See [`main.go:29`](cmd/server/main.go:29). | No |

> Beyond §5.1, the only other variables read by application code are the database-skill set (§5.3) and the local-only deploy set (§5.4). `readHeaderTimeout` (10s) and `shutdownGracePeriod` (15s) are compile-time constants in [`main.go`](cmd/server/main.go:35), not env-configurable.

### 5.2 Client-side variable

| Variable Name | Type / Format | Default | Description | Required |
| :--- | :--- | :--- | :--- | :--- |
| `MCP_SECRET_TOKEN` | string | *(none)* | Same secret, referenced by the client smoke-test shell examples (`Authorization: Bearer $MCP_SECRET_TOKEN`). | Yes (client-side) |

### 5.3 Database skill variables

Consumed only by [`internal/skills/database/config.go`](internal/skills/database/config.go). Credential precedence: `MCP_MONGO_URI` → `MCP_MONGO_USERNAME`/`MCP_MONGO_PASSWORD` (preferred scoped `mcp_agent` user — creation runbook in [`../ARCHITECTURE.md`](../ARCHITECTURE.md)) → `MONGO_INITDB_ROOT_*` (fallback, logs a WARNING).

| Variable Name | Type / Format | Default Value | Description | Required |
| :--- | :--- | :--- | :--- | :--- |
| `MCP_MONGO_USERNAME` | string | *(none)* | Scoped MongoDB user (`mcp_agent`, readWrite on `qdb`+`tarotdb` only). | Recommended |
| `MCP_MONGO_PASSWORD` | string | *(none)* | Password for the scoped user. | Recommended |
| `MONGO_INITDB_ROOT_USERNAME` / `MONGO_INITDB_ROOT_PASSWORD` | string | *(none)* | Root fallback, mirroring the api service's connection logic. | Fallback |
| `MCP_MONGO_URI` | connection string | *(none)* | Full URI override (TLS/replica-set/local topologies); wins over everything. | No |
| `MCP_MONGO_HOST` | `host:port` | `172.255.255.2:27017` | MongoDB address on the internal `quotes` network. | No |
| `MONGO_DATABASE` | string | `qdb` | Application database for the `qdata`/`users`/`tokens` namespaces. | No |
| `MCP_DB_ALLOW_WRITES` | exactly `true` | *(unset = read-only)* | **Fail-closed write switch:** registers `db_insert`/`db_update`/`db_delete`/`user_provision`/`user_revoke`. Any other value leaves the skill read-only. | No |

> If none of the credential variables are set, the server still boots normally; every database tool returns a self-healing configuration error naming the variables to set.

### 5.4 Local deployment variables (`push_codebase` only)

These are read **only** when `push_codebase` runs on a locally started mcp-server. Production go-mcp does not use them.

| Variable Name | Type / Format | Default Value | Description | Required |
| :--- | :--- | :--- | :--- | :--- |
| `DEPLOY_SSH_TARGET` | string (`user@host`) | *(none)* | SSH destination for rsync. | **Yes** (for push) |
| `DEPLOY_LOCAL_ROOT` | filesystem path | auto-detect `docker-compose.yml` | Local repository root synced to the VPS. | No |
| `DEPLOY_REMOTE_PATH` | filesystem path | `/opt/micro-services.d/services/` | Remote sync destination on the VPS. | No |
| `DEPLOY_MCP_URL` | HTTPS URL | `https://api.thirdeye.live/sse` | Production MCP SSE endpoint for pre-flight `snapshot_create`. | No |

### 5.5 Co-located reference-stack variables (NOT this engine)

These appear in the **composite** [`../docker-compose.yml`](../docker-compose.yml) (parent repo root) because the engine is deployed alongside the pre-existing "quotes" stack. Document them for the operator's completeness; the MCP engine does not read them.

| Variable Name | Consumed by | Description | Required (for that service) |
| :--- | :--- | :--- | :--- |
| `MONGO_INITDB_ROOT_USERNAME` | `web`, `api`, `dbs` | MongoDB root username. | Yes |
| `MONGO_INITDB_ROOT_PASSWORD` | `web`, `api`, `dbs` | MongoDB root password. | Yes |
| `MONGO_DATABASE` | `api` | Target Mongo database name. | Yes |
| `ADMIN_ID` | `api` | Admin identifier for the quotes API. | Yes |
| `MAILSERVER` | `web` | SMTP host for the frontend mailer. | Yes |
| `MAILPASS` | `web` | SMTP password. | Yes |
| `AUTHORIZED` | `web` | Frontend authorization allow-list value. | Yes |

---

## 6. Local Development

### 6.1 Prerequisites (zero-state)

| Tool | Exact version | Check command |
| :--- | :--- | :--- |
| Go toolchain | **1.25.0+** (module requires `go 1.25.0`, see [`go.mod:21`](go.mod:21)) | `go version` → expect `go1.25.0` or newer |
| Git | any recent | `git --version` |
| Docker Engine | 24+ (only for container builds) | `docker --version` |
| Docker Compose | v2+ | `docker compose version` |
| curl | any (for smoke tests) | `curl --version` |

Verify Go first — a 1.24 toolchain will silently download 1.25 over the network, breaking hermetic builds:

```bash
go version
# go version go1.25.0 <os>/<arch>   ← 1.25.0 or newer required
```

### 6.2 Clone

```bash
git clone <your-repo-url> go-mcp-server
cd go-mcp-server
```

### 6.3 Resolve dependencies & build

```bash
# Sync go.mod / go.sum and download the module graph
go mod tidy

# Verify the module graph and vendored checksums
go mod verify

# Compile the composition-root binary
go build -o ./bin/mcp-server ./cmd/server
```

### 6.4 Run locally

The server binds `:8080` by default and **rejects all traffic until `MCP_SECRET_TOKEN` is set**. Export a secret first:

```bash
# Generate a strong 32-byte hex secret (one-time)
export MCP_SECRET_TOKEN="$(openssl rand -hex 32)"

# Optional: override the port
export PORT=8080

# Run the compiled binary...
./bin/mcp-server

# ...or run straight from source
go run ./cmd/server
```

Expected startup log (skill registration lines print first, then the listener):

```
skills/system: registered tools "system_health", "system_time", "quote_random"
skills/snapshot: registered tools "snapshot_create", "snapshot_restore"
skills/docker: registered tools "system_down", "system_up", "system_logs"
skills/deploy: registered tool "push_codebase"
skills/database: registered READ tools "db_collections", "db_find", "db_count", "db_aggregate", "user_list", "quote_owner_lookup" (read-only mode; set MCP_DB_ALLOW_WRITES=true to enable write tools)
Custom-VPS-MCP-Engine listening on :8080 (routes: /sse, /message; liveness: /healthz)
```

With `MCP_DB_ALLOW_WRITES=true` the database line instead lists the write tools as well (`db_insert`, `db_update`, `db_delete`, `user_provision`, `user_revoke`).

If you forget the secret you'll instead see:

```
WARNING: MCP_SECRET_TOKEN is not set; all requests will be rejected until it is configured
```

---

## 7. Operational Verification (Testing & Health Checks)

### 7.1 Static analysis, vet & format

```bash
# Compiler + type check (fast)
go build ./...

# Report suspicious constructs
go vet ./...

# Verify formatting (prints files that need formatting; empty output = clean)
gofmt -l .

# Auto-fix formatting in place
gofmt -w .
```

### 7.2 Unit / integration tests

```bash
# Run the full test suite with the race detector and verbose output
go test -race -v ./...

# Coverage report
go test -cover ./...

# Coverage profile + HTML view
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Run a single package's tests (e.g. the auth gate)
go test -race -v ./internal/auth/...

# Run one focused test by name
go test -race -run TestHandleTime ./internal/skills/system/...
```

### What is covered

| Test file | Test(s) | What it verifies |
| :--- | :--- | :--- |
| [`internal/auth/middleware_test.go`](internal/auth/middleware_test.go) | `TestTokenAuthMiddleware` | Table-driven walk of the full auth decision matrix: unset server secret → **500** (fail-closed, handler never reached); no credentials → **401**; invalid bearer token → **401**; valid `Authorization: Bearer` header → **200** (protected handler reached); valid `?token=` query fallback → **200**. Each row also asserts whether the wrapped handler was invoked. Runs sequentially because `t.Setenv` on `MCP_SECRET_TOKEN` is incompatible with parallel subtests. |
| [`internal/skills/system/system_test.go`](internal/skills/system/system_test.go) | `TestHandleHealth` | `system_health` returns a non-error result whose text contains `status: ok`, and never returns a Go error. |
| [`internal/skills/system/system_test.go`](internal/skills/system/system_test.go) | `TestHandleTime` | `system_time` across three contracts: omitted timezone defaults to **UTC**; a valid IANA zone (`America/New_York`) loads successfully; an invalid zone yields the **self-healing** result (`IsError: true` + guidance text, and crucially a **`nil` Go error** so the message reaches the LLM as tool output, not a protocol fault). |
| [`internal/skills/snapshot/snapshot_test.go`](internal/skills/snapshot/snapshot_test.go) | `TestCreateSnapshotAt` | Creates a temp-dir archive via `tar`, verifies `main.go` is included and `image/` + `vol/` are excluded. |
| [`internal/skills/snapshot/snapshot_test.go`](internal/skills/snapshot/snapshot_test.go) | `TestHandleSnapshotCreateMissingSource` | When the default VPS source path is absent, handler returns `IsError: true` with a **`nil` Go error**. |
| [`internal/skills/snapshot/restore_test.go`](internal/skills/snapshot/restore_test.go) | `TestRestoreSnapshotRoundTrip` | Creates archive, modifies tree, restores, verifies content and backup dir. |
| [`internal/skills/snapshot/restore_test.go`](internal/skills/snapshot/restore_test.go) | `TestRestoreSnapshotAtRollbackOnBadArchive` | Corrupt archive triggers rollback; original `services` content preserved. |
| [`internal/skills/docker/lifecycle_test.go`](internal/skills/docker/lifecycle_test.go) | `TestValidateComposeProjectDir`, `TestSystemUpAtRequiresEnvirons`, `TestTruncateOutput` | Compose project/compose-file validation, hard `.environs` requirement for `system_up`, and output truncation for the memory-limited container. |
| [`internal/skills/docker/environs_test.go`](internal/skills/docker/environs_test.go) | `TestParseEnvirons*`, `TestValidateComposeService*`, `TestHandleSystemLogsInvalidService`, `TestComposeDownArgsNeverRemoveVolumes` | `.environs` parsing (comments, quotes, malformed lines), service allowlist + **deterministic sorted error text**, self-healing invalid-service result, and the **hard guarantee that `system_down` can never carry `-v`/`--volumes`/`--rmi`** (asserted against the production `composeDownArgs` function). |
| [`internal/skills/deploy/deploy_test.go`](internal/skills/deploy/deploy_test.go) | `TestBuildRsyncArgs`, `TestLedgerPathFormat`, `TestResolveDeployConfig*`, `TestDetectLocalRoot`, `TestBearerRoundTripper`, `TestEnsureTrailingSlash` | rsync flag/exclusion construction (including the non-interactive `ssh -o BatchMode=yes` transport), ledger naming, required/default `DEPLOY_*` env resolution, repo-root auto-detection, and Bearer-token injection for the pre-flight SSE client. |
| [`internal/skills/database/guardrails_test.go`](internal/skills/database/guardrails_test.go) | `TestResolveNamespace*`, `TestClamp*`, `TestRenderDocumentsRespectsByteBudget`, `TestParseDocument*`, `TestFindForbiddenOperatorNested`, `TestValidateWriteFilterRejectsEmpty`, `TestValidateUpdateDocument`, `TestValidatePipelineGuards`, `TestRedactSecrets`, `TestValidateEmail`, `TestGenerateUserCredentials`, `TestResolveMongoConfig*`, `TestWritesEnabledIsStrict`, `TestHandlersReturnIsErrorWithNilGoError` | The database skill's ENTIRE guardrail contract, without a live MongoDB: namespace allowlist (admin unreachable, sorted deterministic errors), limit/skip clamps + 48 KiB byte budget, ExtJSON parsing, nested `$where` bans, the empty-filter wipe guard, operator-only updates, pipeline stage bans + cross-db `$lookup` rejection, token redaction, email/credential validation, credential precedence + URL-escaping, the strict `MCP_DB_ALLOW_WRITES` fail-closed switch, and the `IsError:true` + `nil`-error self-healing contract. |

> **Test conventions:** both suites are table-driven so each row is a self-documenting specification of one branch. The auth tests drive the middleware through `httptest.NewRecorder`; the skills tests call the handlers directly and assert on the first `*mcp.TextContent` block. As new skills are added under `internal/skills/...`, mirror this pattern with a co-located `_test.go` file.

### 7.3 Smoke test (local)

With the server running and `MCP_SECRET_TOKEN` exported, prove the transport, auth gate, and session handshake end-to-end.

**A. Auth gate — no token must be rejected (HTTP 401):**

```bash
curl -sS -o /dev/null -w "%{http_code}\n" http://localhost:8080/sse
# → 401
```

**B. Open an authenticated SSE stream — the first frame is the `endpoint` event:**

```bash
curl -N \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  http://localhost:8080/sse
# → stays open; first SSE frame is:
#   event: endpoint
#   data: /message?sessionid=<generated-id>
```

**C. Query-parameter fallback (for clients that cannot set headers):**

```bash
curl -N "http://localhost:8080/sse?token=$MCP_SECRET_TOKEN"
```

**D. Full JSON-RPC handshake + tool call.** Keep the `GET /sse` stream from step B open in one terminal, capture the `sessionid` it prints, then in a second terminal:

```bash
SESSION_ID="<paste-sessionid-from-the-endpoint-event>"

# 1) initialize
curl -sS -X POST \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  -H "Content-Type: application/json" \
  "http://localhost:8080/message?sessionid=${SESSION_ID}" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke-test","version":"0.0.0"}}}'

# 2) list tools (expect 15 in the SSE stream — 20 when MCP_DB_ALLOW_WRITES=true:
#    system_health, system_time, quote_random, snapshot_create, snapshot_restore,
#    system_down, system_up, system_logs, push_codebase, db_collections, db_find,
#    db_count, db_aggregate, user_list, quote_owner_lookup
#    [+ db_insert, db_update, db_delete, user_provision, user_revoke])
curl -sS -X POST \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  -H "Content-Type: application/json" \
  "http://localhost:8080/message?sessionid=${SESSION_ID}" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# 3) call system_health (result arrives on the open SSE stream)
curl -sS -X POST \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  -H "Content-Type: application/json" \
  "http://localhost:8080/message?sessionid=${SESSION_ID}" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"system_health","arguments":{}}}'
```

> **Health check for orchestrators:** the `system_health` tool is the canonical liveness probe. A raw TCP/HTTP `GET /sse` with a valid token returning a held-open `200` (and an `endpoint` frame) confirms the process is live and authenticating.

---

## 8. Deployment (Docker + Nginx Runbook)

### 8.1 Build the image

The [`Dockerfile`](Dockerfile) is a two-stage build:

- **Stage 1 — `builder` (`golang:1.25-alpine`):** caches `go mod download` on the manifests, then compiles a **fully static** binary:
  `CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp-server ./cmd/server`
- **Stage 2 — `runtime` (`alpine:3.20`):** installs only `ca-certificates` + `tzdata`, creates a non-root `mcp` user, copies **only** the binary, `EXPOSE 8080`, and runs it as PID 1 (exec form → receives `SIGTERM` for graceful shutdown).

Build standalone:

```bash
docker build -t go-mcp:latest .
```

Run standalone (host-mapped for local container testing):

```bash
docker run --rm -p 8080:8080 \
  -e MCP_SECRET_TOKEN="$(openssl rand -hex 32)" \
  -e PORT=8080 \
  go-mcp:latest
```

### 8.2 Compose stack

The composite [`../docker-compose.yml`](../docker-compose.yml) (parent repo root) attaches a new `go-mcp` service to the existing `quotes` bridge network at static IP **`172.255.255.6`**. Key decisions baked in:

- **No host port mapping** for `go-mcp` — it is reachable *only* via Nginx over the internal docker network (avoids colliding with the `api` service on host `8080`, and forces all access through TLS + the bearer gate).
- `MCP_SECRET_TOKEN` is passed **by reference** (`- MCP_SECRET_TOKEN`), so the value is injected from the host environment / `.env` and never lives in source control.
- `reverse-proxy.depends_on` includes `go-mcp` so Nginx starts after the engine.
- **Host bind mounts** for snapshot and lifecycle tools:
  - `/opt/micro-services.d/services:/opt/micro-services.d/services:rw`
  - `/opt/micro-services.d/snapshots:/opt/micro-services.d/snapshots:rw`
  - `/var/run/docker.sock:/var/run/docker.sock`
- **`group_add`:** set `DOCKER_GID` in `.env` to the host docker group GID (`getent group docker | cut -d: -f3`)

Before first use of snapshot/lifecycle tools on the VPS:

```bash
sudo mkdir -p /opt/micro-services.d/snapshots
# Copy or symlink .environs into /opt/micro-services.d/services/.environs
docker compose up -d go-mcp
MCP_UID=$(docker compose exec -T go-mcp id -u mcp)
MCP_GID=$(docker compose exec -T go-mcp id -g mcp)
sudo chown "${MCP_UID}:${MCP_GID}" /opt/micro-services.d/snapshots
sudo chown "${MCP_UID}:${MCP_GID}" /opt/micro-services.d/services
sudo chmod 750 /opt/micro-services.d/snapshots
export DOCKER_GID=$(getent group docker | cut -d: -f3)
docker compose up -d go-mcp
```

Provide the secret via a host `.env` next to the compose file:

```bash
# .env  (NOT committed)
MCP_SECRET_TOKEN=<your-strong-secret>
```

Deploy / update:

```bash
# Build + (re)create only the MCP engine, leaving the rest of the stack running
docker compose up -d --build go-mcp

# Reload Nginx after the composite nginx.conf changes (proxy is the `reverse-proxy` service)
docker compose restart reverse-proxy

# Full stack bring-up
docker compose up -d --build

# Tail engine logs
docker compose logs -f go-mcp

# Graceful stop (triggers signal.NotifyContext drain, 15s grace)
docker compose stop go-mcp
```

### 8.3 Nginx TLS + SSE proxy

The composite [`../prx/nginx.conf`](../prx/nginx.conf) (parent repo) redirects `:80 → :443`, terminates TLS, and adds two SSE-tuned location blocks that proxy to `172.255.255.6:8080`. The **critical, non-default** directives (do **not** reuse the generic proxy settings from the other services):

| Directive | Value | Why |
| :--- | :--- | :--- |
| `proxy_http_version` | `1.1` | SSE requires a persistent HTTP/1.1 connection. |
| `proxy_set_header Connection` | `""` | Prevents Nginx from sending `close` and killing the stream. |
| `proxy_buffering` / `proxy_cache` | `off` | Events must flow the instant the server emits them. |
| `proxy_read_timeout` / `proxy_send_timeout` | `24h` | Streams are long-lived; the 60s default would sever idle sessions. |
| `proxy_set_header Authorization` | `$http_authorization` | Forwards the Bearer token unchanged to the auth middleware. |
| `chunked_transfer_encoding` | `on` (on `/sse`) | Streams chunked events to the client. |

> **TLS note:** the reference config points at `/etc/ssl/api_thirdeye_live.pem` and mounts `/etc/ssl:ro`. Replace the cert/key paths and `server_name` to match your VPS hostname (see the DNS prerequisite in [`client-connection-guide.md`](client-connection-guide.md:5)).

### 8.4 Production deploy checklist

```bash
# On the VPS, from the deploy directory containing docker-compose.yml + .env:
git pull                                   # fetch latest engine source
docker compose build go-mcp                # rebuild the static binary image
docker compose up -d go-mcp                # rolling replace of the engine only
docker compose exec reverse-proxy nginx -t # validate proxy config syntax
docker compose restart reverse-proxy       # apply any nginx.conf changes
docker compose logs -f go-mcp              # confirm the "listening on :8080" line
```

---

## 9. Client Connection

Once live at, e.g., **`https://mcp.my-vps-domain.com`**, clients connect to the SSE endpoint and present the shared secret on **every** request. Full recipes live in [`client-connection-guide.md`](client-connection-guide.md).

| Component | Value |
| :--- | :--- |
| **Open a session (SSE stream)** | `GET https://<domain>/sse` |
| **Message channel** | `POST https://<domain>/message?sessionid=<id>` |
| **Auth (preferred)** | `Authorization: Bearer <MCP_SECRET_TOKEN>` |
| **Auth (fallback)** | append `?token=<MCP_SECRET_TOKEN>` to the URL |

> The `sessionid` is **not** invented by the client. After a successful `GET /sse`, the server's first SSE frame is an `endpoint` event carrying the exact `/message?sessionid=...` URL to POST to. The header channel always takes precedence over the `?token=` fallback ([`extractPresentedToken`](internal/auth/middleware.go:142)).

### 9.1 curl

```bash
curl -N \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  https://mcp.my-vps-domain.com/sse
# Missing/wrong token → 401. Healthy connection stays open and streams events.
```

### 9.2 Claude Desktop (via `mcp-remote`)

Claude Desktop launches MCP servers over stdio, so bridge to the remote SSE endpoint with `mcp-remote`. Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "custom-vps-mcp-engine": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "https://mcp.my-vps-domain.com/sse",
        "--header",
        "Authorization: Bearer YOUR_MCP_SECRET_TOKEN"
      ]
    }
  }
}
```

If a bridge cannot send headers on the handshake, use the query fallback:
`"https://mcp.my-vps-domain.com/sse?token=YOUR_MCP_SECRET_TOKEN"`.

### 9.3 Ollama / custom Python client

```python
from mcp.client.sse import sse_client
from mcp import ClientSession

async with sse_client(
    url="https://mcp.my-vps-domain.com/sse",
    headers={"Authorization": "Bearer YOUR_MCP_SECRET_TOKEN"},
) as (read, write):
    async with ClientSession(read, write) as session:
        await session.initialize()

        tools = await session.list_tools()          # → 15 tools (20 with MCP_DB_ALLOW_WRITES=true)

        result = await session.call_tool(
            "system_time", {"timezone": "America/New_York"}
        )
        print(result.content[0].text)
```

---

## 10. Troubleshooting & Common Failure Modes

| Symptom | Likely cause | Triage & fix |
| :--- | :--- | :--- |
| **Every request returns `500` ("Server authentication is not configured.")** | `MCP_SECRET_TOKEN` is unset in the engine's environment (fail-closed). | `docker compose exec go-mcp env \| grep MCP_SECRET_TOKEN`. If empty, set it in the host `.env` and `docker compose up -d go-mcp`. Startup log will also show the `WARNING: MCP_SECRET_TOKEN is not set` line. |
| **`401 Unauthorized` with a token attached** | Token mismatch, wrong scheme, or the proxy stripped the header. | Confirm the client sends `Authorization: Bearer <token>` (case-insensitive scheme, trailing space required). Verify Nginx forwards it: the `/sse` + `/message` blocks must include `proxy_set_header Authorization $http_authorization;`. As a diagnostic, try the `?token=` fallback. |
| **SSE stream connects then drops after ~60s** | Nginx default `proxy_read_timeout` (60s) or buffering is severing the long-lived stream. | Ensure the `/sse` block has `proxy_buffering off;`, `proxy_http_version 1.1;`, `proxy_set_header Connection "";`, and `proxy_read_timeout 24h;`. Run `docker compose exec reverse-proxy nginx -t` then `restart reverse-proxy`. |
| **Events never arrive / stream feels "stuck"** | A buffering proxy or cache is batching events. | Disable buffering/caching end-to-end (`proxy_buffering off; proxy_cache off;`). Do **not** reuse the generic proxy settings from the quotes services on `/sse`. |
| **`system_time` fails for any non-UTC zone** | `tzdata` missing from the runtime image, so `time.LoadLocation` cannot resolve IANA names. | Confirm `apk add --no-cache ca-certificates tzdata` is present in the runtime stage ([Dockerfile:77](Dockerfile:77)); rebuild the image. Distinguish this from a client typo — a genuine bad name returns the self-healing `IsError` message, not a crash. |
| **`POST /message` returns 404 / "unknown session"** | The `sessionid` is stale, invented, or from a stream that already closed. | Re-open `GET /sse`, read the fresh `endpoint` event, and POST to the exact URL it advertises. Sessions are per-stream and SDK-managed. |
| **`405 Method Not Allowed`** | Wrong HTTP method: `/sse` is `GET`-only, `/message` is `POST`-only. | The mux uses method-scoped patterns ([transport.go:86](internal/mcpengine/transport.go:86)). Use `GET` for the stream, `POST` for messages. |
| **Docker build downloads a Go toolchain over the network** | Builder base image older than the `go 1.25.0` module requirement. | Keep the builder pinned to `golang:1.25-alpine` and bump it together with the `go.mod` `go` directive. |
| **Container won't drain / hangs on `docker stop`** | A stubborn SSE stream exceeding the 15s grace window. | Expected behavior: `main()` logs `graceful shutdown incomplete` and exits after the grace period. Tune `shutdownGracePeriod` in [`main.go:35`](cmd/server/main.go:35) if longer draining is required. |
| **`go build` / `go mod tidy` fails on fresh clone** | Local Go older than 1.25.0. | `go version` must report `go1.25.0`+. Upgrade the toolchain, then re-run `go mod tidy && go build ./...`. |
| **Port conflict on the VPS** | Another service already binds host `8080` (the quotes `api`). | By design `go-mcp` has **no host port mapping** — it's reached only via Nginx over `172.255.255.6:8080`. Do not add a host port; access it through `https://<domain>/sse`. |
```
