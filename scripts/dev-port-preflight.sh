#!/usr/bin/env bash
set -euo pipefail

PORT="${1:-8317}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v lsof >/dev/null 2>&1; then
  echo "dev preflight: lsof not found, skipping port ${PORT} check"
  exit 0
fi

PIDS=()
while IFS= read -r pid; do
  [ -n "${pid}" ] && PIDS+=("${pid}")
done < <(lsof -tiTCP:"${PORT}" -sTCP:LISTEN 2>/dev/null | sort -u)
if [ "${#PIDS[@]}" -eq 0 ]; then
  exit 0
fi

for pid in "${PIDS[@]}"; do
  [ -n "${pid}" ] || continue
  command_line="$(ps -p "${pid}" -o command= 2>/dev/null || true)"
  [ -n "${command_line}" ] || continue
  cwd_path="$(lsof -a -p "${pid}" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)"

  case "${command_line}" in
    /opt/homebrew/opt/cliproxyapi/bin/cliproxyapi*|/usr/local/opt/cliproxyapi/bin/cliproxyapi*)
      echo "dev preflight: stopping Homebrew cliproxyapi service on port ${PORT} (pid ${pid})"
      if command -v brew >/dev/null 2>&1; then
        brew services stop cliproxyapi >/dev/null 2>&1 || kill "${pid}" >/dev/null 2>&1 || true
      else
        kill "${pid}" >/dev/null 2>&1 || true
      fi
      ;;
    *"/go-build/"*"cmd/server"*|*"/go-build/"*"/server"*)
      echo "dev preflight: stopping stale Go dev server on port ${PORT} (pid ${pid})"
      kill "${pid}" >/dev/null 2>&1 || true
      ;;
    */go-build*/exe/server)
      if [ "${cwd_path}" = "${REPO_ROOT}" ]; then
        echo "dev preflight: stopping stale Go dev server on port ${PORT} (pid ${pid})"
        kill "${pid}" >/dev/null 2>&1 || true
      else
        echo "dev preflight: port ${PORT} is already used by an unknown Go server:" >&2
        echo "  pid ${pid}: ${command_line}" >&2
        echo "  cwd: ${cwd_path:-unknown}" >&2
        echo "Stop it manually or run make dev with DEV_PORT set to a free port." >&2
        exit 1
      fi
      ;;
    "${REPO_ROOT}"/*)
      echo "dev preflight: stopping stale repository dev server on port ${PORT} (pid ${pid})"
      kill "${pid}" >/dev/null 2>&1 || true
      ;;
    *)
      echo "dev preflight: port ${PORT} is already used by an unknown process:" >&2
      echo "  pid ${pid}: ${command_line}" >&2
      echo "Stop it manually or run make dev with DEV_PORT set to a free port." >&2
      exit 1
      ;;
  esac
done

sleep 0.5
if lsof -tiTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "dev preflight: port ${PORT} is still occupied after cleanup:" >&2
  lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN >&2 || true
  exit 1
fi
