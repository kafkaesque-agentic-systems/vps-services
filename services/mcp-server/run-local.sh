#!/usr/bin/env bash
#
# run-local.sh - start the local Custom-VPS-MCP-Engine instance.
#
# This local instance exists for ONE purpose: to serve push_codebase, which
# rsyncs the local checkout to the production VPS over SSH. Production go-mcp
# cannot run that tool (it has no local checkout to sync from).
#
# Reads configuration from ./.env (gitignored - holds MCP_SECRET_TOKEN and
# DEPLOY_SSH_TARGET). Fails fast on missing config rather than starting a
# server that is silently misconfigured.
#
# Usage:
#   ./run-local.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ ! -f .env ]]; then
  echo "error: .env not found in ${SCRIPT_DIR}" >&2
  echo "       it must define MCP_SECRET_TOKEN and DEPLOY_SSH_TARGET" >&2
  exit 1
fi

# Export every variable defined by .env so the Go process inherits them,
# regardless of whether the file uses 'export' prefixes.
set -a
# shellcheck disable=SC1091
. ./.env
set +a

# Fail fast on missing required config. An unset value aborts here with a clear
# message instead of producing a server that only fails at tool-call time.
: "${MCP_SECRET_TOKEN:?must be set in .env (bearer token for this server AND for the production pre-flight snapshot)}"
: "${DEPLOY_SSH_TARGET:?must be set in .env (SSH destination, e.g. deploy@your-vps)}"

# Default the rsync source to the services/ tree one level up. Trailing slash is
# significant to rsync: it syncs the CONTENTS of the directory, not the
# directory itself.
export DEPLOY_LOCAL_ROOT="${DEPLOY_LOCAL_ROOT:-$(cd .. && pwd)/}"
export PORT="${PORT:-8080}"

# Least privilege. This instance exists ONLY to serve push_codebase. Registering
# the docker or database skills here would expose system_down, snapshot_restore
# and db_delete -- against production credentials -- from a background process
# running on a laptop. Enforced at registration in cmd/server/main.go, so no
# client-side configuration can re-expose them.
export MCP_SKILLS="${MCP_SKILLS:-deploy}"

echo "local mcp-server starting"
echo "  port:          ${PORT}"
echo "  skills:        ${MCP_SKILLS}  (restricted; production runs unset = all)"
echo "  deploy target: ${DEPLOY_SSH_TARGET}"
echo "  local root:    ${DEPLOY_LOCAL_ROOT}"
echo "  remote path:   ${DEPLOY_REMOTE_PATH:-/opt/micro-services.d/ (default)}"
echo

exec go run ./cmd/server
