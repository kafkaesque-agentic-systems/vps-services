# Agent Onboarding — ThirdEye VPS

**Read this before touching anything.** It is the fastest path from cold start to
safe, productive work on this system, and it captures knowledge that is *not*
obvious from the codebase or from a standing agent profile.

## How to use this file

This is a **running log**, not a static spec. It has two jobs:

1. **Onboard** a new agent quickly.
2. **Accumulate** hard-won knowledge during a tenure.

**During your tenure:** whenever you discover something that surprised you, cost
you time, or would have changed your first move — append it to
[§9 Running Log](#9-running-log). Be specific. "Be careful with X" is useless;
"X does Y because Z, do W instead" is what the next agent needs.

**At the end of your tenure:** read §9 top to bottom, fold anything durable into
the numbered sections above it, and use the whole document as the basis for a
handover prompt.

> Prefer adding a hazard here over assuming the next agent will reason it out.
> Two production incidents in this system's history came from *plausible*
> reasoning about tooling that behaves unusually. See §5.

---

## 1. What this system is

A Docker-orchestrated micro-services stack on a production VPS serving
`https://api.thirdeye.live` — a quotes API, a tarot API, and a demo web UI.

| Service | Container | Stack | Static IP |
| :--- | :--- | :--- | :--- |
| reverse-proxy | `quotes-proxy` | NGINX (TLS, gateway) | 172.255.255.5 |
| web | `quotes-frontend` | Python 3.10 / Flask / Gunicorn | 172.255.255.4 |
| api | `quotes-server` | Go 1.19 / Gin | 172.255.255.3 |
| dbs | `quotes-database` | MongoDB 4.4.18 | 172.255.255.2 |
| go-mcp | `go-mcp` | Go 1.25 / MCP SDK — **your control plane** | 172.255.255.6 |

Inter-service traffic uses **hard-coded static IPs**, not Docker DNS. Do not
"fix" this without an explicit migration mandate — NGINX proxies to those exact
addresses.

---

## 2. The pipeline — where everything lives

```
   local edit  ->  git commit  ->  push_codebase (rsync)  ->  VPS
   ~/.local/dev/vps/services/        (local MCP instance)     /opt/micro-services.d/
```

**Critical path fact:** local `services/` maps to remote **`/opt/micro-services.d/`**
with **no `services/` level on the remote**. A 2026-07-14 layout migration removed
it. Any documentation (including a standing agent profile) that says the codebase
lives at `/opt/micro-services.d/services` is **stale**.

- **Local tree is the source of truth.** Edit locally, then push.
- **Never hand-edit the remote.** It creates drift that a later `--delete` sync
  silently reverts. The only justified exception is bootstrapping when the MCP
  control plane is down and cannot repair itself (see §5.2).
- **Git records what changed; `push_codebase` ships it.** They are independent —
  pushing does not commit, committing does not deploy.
- **`push_codebase` does not restart anything.** New code sits inert on disk until
  containers are rebuilt/recreated. See §5.1 for how to do that safely.

---

## 3. Tooling — two MCP servers, different powers

| Server | Runs on | Tools | Purpose |
| :--- | :--- | :--- | :--- |
| `my-remote-vps` | the VPS | all skills | operate production |
| `vps-deploy-local` | your machine | **`push_codebase` only** | deploy |

`push_codebase` is **local-only by design** — it needs the local checkout as an
rsync source, so production `go-mcp` cannot run it.

The local instance is restricted by `MCP_SKILLS=deploy` (set in
`services/mcp-server/run-local.sh`). It deliberately exposes *one* tool: a
credentialed background agent on a laptop must not be able to reach
`system_down`, `snapshot_restore`, or `db_delete`. Enforced at registration in
`cmd/server/main.go`, so no client config can re-expose them.

**The local instance runs as a macOS LaunchAgent** (`live.thirdeye.vps-mcp-deploy`):

```bash
services/mcp-server/launchd/install.sh status      # loaded? listening? recent stderr
services/mcp-server/launchd/install.sh uninstall   # stop it holding credentials
```

Logs: `services/mcp-server/logs/mcp-deploy.{out,err}.log` (gitignored).

**The database is READ-ONLY.** Write tools (`db_insert`/`db_update`/`db_delete`/
`user_provision`/`user_revoke`) are not registered because `MCP_DB_ALLOW_WRITES`
is unset. If you need a write, say so — do not look for a workaround.

---

## 4. Out-of-band access (SSH) — you need this

MCP is not your only channel, and **that matters**: when `go-mcp` is down, MCP
cannot fix it, but SSH can.

```bash
ssh thirdeye-vps          # alias in ~/.ssh/config
```

- **SSH is on port 43076, not 22.** UFW filters 22 to a stale source IP. A bare
  `ssh kafka@<ip>` hangs on connect. The `thirdeye-vps` alias supplies the port.
- **`sudo` requires a password.** You cannot automate privileged operations —
  ask the operator to run them.
- `DEPLOY_SSH_TARGET=thirdeye-vps` (the alias, not `user@host`) so the port lives
  in one place.

---

## 5. Hazards — read this section twice

### 5.1 `system_up` and `system_down` can self-terminate ⚠️

**The compose client runs *inside* the `go-mcp` container.** When compose
recreates or stops `go-mcp`, it kills the process issuing the command.

- `system_down` — documented hazard; teardown may complete only partially and
  the success report never arrives.
- **`system_up` has the same hazard and it is *not* obvious.** On 2026-07-19 a
  routine `system_up` (no `build`, believed idempotent) recreated `go-mcp`,
  killed the compose process mid-run, and left **`quotes-server` and
  `quotes-frontend` stopped but never restarted** — a ~4 minute production
  outage. `docker compose up -d` is idempotent *in general*; it is not safe
  *here*, because the client is one of the containers.

**Do this instead** — drive compose over SSH, where nothing you depend on dies:

```bash
ssh thirdeye-vps 'cd /opt/micro-services.d && set -a && . ./.environs && set +a && docker compose up -d'
```

Sourcing `.environs` is mandatory — the compose file uses required variables
(§6) that abort the run when unset.

### 5.2 Bootstrapping when the control plane is down

If `go-mcp` cannot start, MCP tools cannot repair it — chicken-and-egg. Recover
over SSH. When you must change a file to do so, **rsync it from the local tree**
rather than hand-editing the remote, so no drift is created:

```bash
rsync -az --no-perms -i -e "ssh" docker-compose.yml thirdeye-vps:/opt/micro-services.d/
```

### 5.3 File ownership is load-bearing — never `chown -R` the root

```
/opt/micro-services.d/      systemd-network:kafka  drwxrwxr-x   <- uid 100 = container's mcp user
  snapshots/                systemd-network:systemd-journal
  api/ dbs/ prx/ scripts/ web/   kafka:kafka
  mcp-server/ vol/          kafka:kafka
```

The root directory and `snapshots/` are owned by **uid 100 — the container's
`mcp` user** — so `snapshot_create` can write from *inside* the container. A
`chown -R kafka:kafka /opt/micro-services.d` would break snapshots, i.e. the
rollback safety net.

`snapshot_create` tars the whole tree as that unprivileged user, so it must be
able to **read every file**. Some files are deliberately group-only (`0640` DKIM
key, `mail.py`; `0750` scripts) — the container reads them via `CODEBASE_GID`.

### 5.4 Deploys must never rewrite production permissions

rsync `-a` implies `-p`, which makes your machine authoritative over server file
modes. Pushing local `0640`/`0750` modes onto files the container reads broke
`snapshot_create` on 2026-07-19 — **while every service still reported healthy**.
The deploy skill now passes `--no-perms` (host owns permissions) and
`--omit-dir-times` (the sync root cannot be restamped by a non-owner; that single
failure fails the whole push with exit 23). Do not remove either.

### 5.5 Silent misconfiguration is this system's dominant failure mode

Every significant incident here presented as something other than its cause:

| Symptom | Actual cause |
| :--- | :--- |
| All docker tools fail, permission denied | `${DOCKER_GID:-999}` silently defaulted to a wrong group |
| MCP unreachable, container "healthy" | `MCP_SECRET_TOKEN` unset; auth correctly refusing everything |
| Deploy hangs minutes, empty ledger | SSH connect unbounded; port filtered, error surfaced nowhere |
| Snapshots fail, services all healthy | pushed file modes made files unreadable to the container |

Hence the convention: **fail loud, never default silently.** Required config uses
`${VAR:?message}`; unknown `MCP_SKILLS` names are fatal at startup. Preserve this
when adding config.

---

## 6. Configuration — what lives where

**Never synced, never committed:** `.env`, `.environs`, `image/`, `vol/`,
`snapshots/`, `.git/`, `*.bak-*`, `deploy_ledgers/`, `/mcp-server/server`,
`/mcp-server/logs/`.

### Remote `/opt/micro-services.d/.environs` (host-specific, `export`-prefixed)

| Variable | Value | Purpose |
| :--- | :--- | :--- |
| `MCP_SECRET_TOKEN` | *(secret)* | Bearer token for all MCP clients |
| `DOCKER_GID` | `116` | host `docker` group — container needs the socket |
| `CODEBASE_GID` | `1000` | host `kafka` group — container must read the tree |

### Local `services/mcp-server/.env` (gitignored)

| Variable | Value |
| :--- | :--- |
| `MCP_SECRET_TOKEN` | *(same secret — also used for the production pre-flight call)* |
| `DEPLOY_SSH_TARGET` | `thirdeye-vps` |

> The token exists in **three** places: remote `.environs`, local `.env`, and the
> Claude Desktop config. Rotation must update all three.

---

## 7. Working conventions

- **Snapshot before any risky change.** `push_codebase` does this automatically
  as a pre-flight and aborts if it fails.
- **Evidence before intervention.** Quote the log line or query result that
  establishes the cause. Do not fix a symptom whose cause you have not proven.
- **Go:** `gofmt`, `go vet`, `go test ./...` all clean before committing.
  Exported identifiers get godoc comments.
- **Commits:** Conventional Commits. **Never add a `Co-Authored-By: Claude`
  trailer** — this repo must not show Claude as a contributor.
- **Tests guard invariants, not just behaviour.** A test that only checks "the
  list reaches the command line" will not catch an entry being *deleted* from the
  list. Assert contents directly. (This is not hypothetical — see §9.)

---

## 8. Cold-start verification

Run these to confirm the toolchain is healthy before real work. **Do not include
`system_up` — see §5.1.**

```
system_health          -> status ok
system_logs (api, 20)  -> log lines, no docker.sock permission error
db_collections         -> qdata / tdata / tokens / users with counts
quote_random           -> a quote
```

```bash
services/mcp-server/launchd/install.sh status     # local deploy agent
ssh thirdeye-vps 'docker ps --format "{{.Names}}: {{.Status}}"'   # expect 5 containers
curl -s -o /dev/null -w '%{http_code}\n' https://api.thirdeye.live/quote?json   # 200
```

A dry run shows exactly what a deploy would do, changing nothing:

```bash
cd services && rsync -az --delete -i --dry-run --no-perms --omit-dir-times \
  --exclude='.git/' --exclude='.env' --exclude='.environs' --exclude='vol/' \
  --exclude='image/' --exclude='snapshots/' --exclude='*.bak-*' \
  --exclude='deploy_ledgers/' --exclude='/mcp-server/server' --exclude='/mcp-server/logs/' \
  -e "ssh -o BatchMode=yes -o ConnectTimeout=10" ./ thirdeye-vps:/opt/micro-services.d/
```

### Local shell gotchas (this dev machine)

These wasted real time; they are environment quirks, not repo problems:

- `ls` is aliased to a modern replacement that rejects `-lt`. Use `/bin/ls`.
- `timeout` is not installed (BSD userland).
- `git log --format=… | grep` can hang even with `--no-pager`. Prefer
  `git log --oneline`.
- A heredoc into `git commit -F -` hangs. Write the message to a file and use
  `git commit -F <file>`.
- Restarting the local LaunchAgent drops the MCP session; the next tool call
  fails with *"invalid during session initialization"*. Wait for re-init, then
  retry — it is not a real error.

---

## 9. Running Log

Append discoveries here. Newest first. Date, what happened, what to do differently.

### 2026-07-19 / 20 — first end-to-end deploy, two self-inflicted incidents

- **`system_up` caused a ~4 min outage.** Believed idempotent; it recreated
  `go-mcp`, killing the compose client mid-run and leaving api + web stopped.
  → §5.1. Drive compose over SSH.
- **Pushed local file modes broke `snapshot_create`.** rsync `-a` propagated
  macOS `0640`/`0750` modes; the container could no longer read the tree to tar
  it. Every service stayed healthy, so nothing surfaced the loss of the rollback
  net. → fixed with `--no-perms`; §5.4.
- **An exclusion entry was silently dropped during an edit**, shipping
  `deploy_ledgers/` to production. The existing test only verified that entries
  *in* the list reached the command line, so deletion kept it green. → added a
  test asserting the list's contents; §7.
- **Port 22 is filtered; SSH is 43076.** Cost significant diagnosis time because
  `push_codebase` hung with no error. → §4, and `ConnectTimeout=10` now bounds it.
- **`${DOCKER_GID:-999}` silently defaulted**, disabling every docker tool with a
  permission error that only appeared at call time. → §5.5.
- **Claude Desktop does not inherit the shell environment.** GUI-launched macOS
  apps never read `~/.zshrc`, so `${MCP_SECRET_TOKEN}` indirection does not work
  there; the token must be written into its config. Claude Code (`.mcp.json`)
  *does* support the indirection.
- **Known unknown, unresolved:** users reported 502s on quote searches while the
  home page was fine. The Go API showed no 5xx and no search requests reached it;
  the leading hypothesis was Gunicorn `WORKER TIMEOUT` → SIGKILL in the `web`
  tier producing 502s at NGINX. Parked for lack of a reproduction window and the
  search endpoint. Revisit if reports resume.
