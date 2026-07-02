#!/usr/bin/env bash
set -euo pipefail

PORT="${DEV_PORT:-8317}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_PROXY="${GO_PROXY:-https://goproxy.cn,direct}"

children_of() {
  local pid="$1"
  pgrep -P "${pid}" 2>/dev/null || true
}

terminate_tree() {
  local pid="$1"
  local signal="${2:-TERM}"
  local child
  [ -n "${pid}" ] || return 0
  for child in $(children_of "${pid}"); do
    terminate_tree "${child}" "${signal}"
  done
  kill "-${signal}" "${pid}" >/dev/null 2>&1 || true
}

wait_for_exit() {
  local pid="$1"
  local attempts="$2"
  local i
  for ((i = 0; i < attempts; i++)); do
    if ! kill -0 "${pid}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

cleanup() {
  local status="${1:-130}"
  trap - INT TERM EXIT
  if [ -n "${DEV_CHILD_PID:-}" ] && kill -0 "${DEV_CHILD_PID}" >/dev/null 2>&1; then
    echo "dev: stopping local server..."
    terminate_tree "${DEV_CHILD_PID}" INT
    if ! wait_for_exit "${DEV_CHILD_PID}" 15; then
      terminate_tree "${DEV_CHILD_PID}" TERM
    fi
    if ! wait_for_exit "${DEV_CHILD_PID}" 15; then
      terminate_tree "${DEV_CHILD_PID}" KILL
    fi
    wait "${DEV_CHILD_PID}" >/dev/null 2>&1 || true
  fi
  "${REPO_ROOT}/scripts/dev-port-preflight.sh" "${PORT}" >/dev/null 2>&1 || true
  exit "${status}"
}

trap 'cleanup 130' INT
trap 'cleanup 143' TERM

cd "${REPO_ROOT}"
mkdir -p .tmp/dev
env GOPROXY="${GO_PROXY}" go build -o .tmp/dev/cliproxyapi-dev ./cmd/server
.tmp/dev/cliproxyapi-dev &
DEV_CHILD_PID="$!"

set +e
wait "${DEV_CHILD_PID}"
status="$?"
set -e
trap - INT TERM EXIT
exit "${status}"
