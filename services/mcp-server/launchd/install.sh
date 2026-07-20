#!/usr/bin/env bash
#
# install.sh - manage the LaunchAgent for the LOCAL mcp-server (push_codebase).
#
#   ./install.sh install     generate + load the agent (starts at login)
#   ./install.sh uninstall   unload + remove the agent
#   ./install.sh status      report whether it is loaded and listening
#
# WHAT THIS RUNS IN THE BACKGROUND
# --------------------------------
# A local MCP server holding production deploy credentials (MCP_SECRET_TOKEN,
# DEPLOY_SSH_TARGET, read from ../.env). It is restricted to MCP_SKILLS=deploy,
# so push_codebase is the only tool it exposes - not system_down, not db_delete.
# Remove it with './install.sh uninstall' when you no longer want a credentialed
# agent running at login.
#
set -euo pipefail

LABEL="live.thirdeye.vps-mcp-deploy"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MCP_SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEMPLATE="${SCRIPT_DIR}/${LABEL}.plist.template"
TARGET="${HOME}/Library/LaunchAgents/${LABEL}.plist"
LOG_DIR="${MCP_SERVER_DIR}/logs"
PORT="${PORT:-8080}"

usage() {
  sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit 1
}

do_install() {
  [[ -f "${TEMPLATE}" ]] || { echo "error: template not found: ${TEMPLATE}" >&2; exit 1; }
  [[ -x "${MCP_SERVER_DIR}/run-local.sh" ]] || {
    echo "error: ${MCP_SERVER_DIR}/run-local.sh missing or not executable" >&2
    echo "       fix with: chmod +x ${MCP_SERVER_DIR}/run-local.sh" >&2
    exit 1
  }
  if [[ ! -f "${MCP_SERVER_DIR}/.env" ]]; then
    echo "error: ${MCP_SERVER_DIR}/.env not found" >&2
    echo "       it must define MCP_SECRET_TOKEN and DEPLOY_SSH_TARGET" >&2
    exit 1
  fi

  # launchd does not create log destinations; a missing directory makes the
  # agent fail to spawn with no diagnostic anywhere.
  mkdir -p "${LOG_DIR}"
  mkdir -p "${HOME}/Library/LaunchAgents"

  # Unload any previous copy so re-running install is idempotent.
  do_uninstall_quiet

  sed -e "s#__MCP_SERVER_DIR__#${MCP_SERVER_DIR}#g" \
      -e "s#__HOME__#${HOME}#g" \
      "${TEMPLATE}" > "${TARGET}"

  # plutil validates the generated XML before launchd ever sees it.
  if ! plutil -lint "${TARGET}" >/dev/null; then
    echo "error: generated plist failed validation: ${TARGET}" >&2
    exit 1
  fi

  if launchctl bootstrap "gui/$(id -u)" "${TARGET}" 2>/dev/null; then
    :
  else
    # Fallback for older macOS releases.
    launchctl load -w "${TARGET}"
  fi

  echo
  echo "==============================================================="
  echo " INSTALLED background LaunchAgent: ${LABEL}"
  echo "==============================================================="
  echo " It starts at login and holds production deploy credentials."
  echo
  echo "   plist:   ${TARGET}"
  echo "   source:  ${TEMPLATE}"
  echo "   logs:    ${LOG_DIR}/mcp-deploy.{out,err}.log"
  echo "   skills:  deploy only (push_codebase; no docker/database tools)"
  echo
  echo "   status:  ${SCRIPT_DIR}/install.sh status"
  echo "   remove:  ${SCRIPT_DIR}/install.sh uninstall"
  echo "==============================================================="
  echo
  do_status || true
}

do_uninstall_quiet() {
  launchctl bootout "gui/$(id -u)/${LABEL}" 2>/dev/null || true
  launchctl unload "${TARGET}" 2>/dev/null || true
}

do_uninstall() {
  do_uninstall_quiet
  rm -f "${TARGET}"
  echo "removed LaunchAgent ${LABEL} (plist deleted, agent unloaded)"
  echo "logs left in place: ${LOG_DIR}"
}

do_status() {
  echo "--- ${LABEL} ---"
  if launchctl list | grep -q "${LABEL}"; then
    echo "launchd:   LOADED"
    launchctl list | grep "${LABEL}" | awk '{print "  pid=" $1 "  last_exit=" $2}'
  else
    echo "launchd:   NOT LOADED"
  fi

  if lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port ${PORT}:  LISTENING"
  else
    echo "port ${PORT}:  not listening"
  fi

  if [[ -s "${LOG_DIR}/mcp-deploy.err.log" ]]; then
    echo "stderr:    last 3 lines of ${LOG_DIR}/mcp-deploy.err.log"
    tail -n 3 "${LOG_DIR}/mcp-deploy.err.log" | sed 's/^/  | /'
  fi
}

case "${1:-}" in
  install)   do_install ;;
  uninstall) do_uninstall ;;
  status)    do_status ;;
  *)         usage ;;
esac
