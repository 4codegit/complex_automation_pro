#!/usr/bin/env bash
# CAP split-mode orchestrator: every service as its own process behind
# gateway-api. Usage: scripts/services.sh [start|stop|status|logs]
#
#   start  (default) build + launch live first (migrates/seeds the DB), then
#          the remaining services and gateway-api; waits for health
#   stop   terminate all service processes
#   status show running services and their ports
#   logs   tail service logs
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RUN="$ROOT/.run"
BIN="$RUN/bin"
PIDS="$RUN/services"
mkdir -p "$BIN" "$PIDS"

DB="${CAP_DB_URL:-sqlite://$RUN/cap.db}"

# name port extra-env...
SPECS=(
  "live 8001"
  "ingest 8002 EVENT_SINKS=http://127.0.0.1:8001/internal/events"
  "historian 8003"
  "alarms 8004"
  "profiles 8005"
  "registry 8006"
  "identity 8007"
  "gateway-api 8000"
)

start_one() {
  local name="$1" port="$2" extra="${3:-}"
  if [ -f "$PIDS/$name.pid" ] && kill -0 "$(cat "$PIDS/$name.pid")" 2>/dev/null; then
    echo "  $name already running (pid $(cat "$PIDS/$name.pid"))"
    return
  fi
  # shellcheck disable=SC2086 # extra env assignments are meant to word-split
  setsid nohup env HTTP_ADDR="127.0.0.1:$port" DB_URL="$DB" \
    SIMULATOR_ENABLED=true $extra "$BIN/$name" >> "$RUN/$name.log" 2>&1 &
  echo $! > "$PIDS/$name.pid"
  echo "  started $name on :$port (pid $!)"
}

wait_health() {
  local port="$1" i
  for i in $(seq 1 50); do
    if curl -fsS "http://127.0.0.1:$port/api/v1/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  echo "health check failed on :$port — see $RUN/*.log" >&2
  exit 1
}

cmd="${1:-start}"
case "$cmd" in
  start)
    cd "$ROOT"
    echo "==> building services"
    go build -o "$BIN" ./cmd/live ./cmd/ingest ./cmd/historian ./cmd/alarms \
      ./cmd/profiles ./cmd/registry ./cmd/identity ./cmd/gateway-api
    echo "==> starting (db=$DB)"
    for spec in "${SPECS[@]}"; do
      # live goes first: it migrates and seeds a fresh database.
      read -r name port extra <<< "$spec"
      start_one "$name" "$port" "$extra"
      if [ "$name" = "live" ]; then
        wait_health "$port"
      fi
    done
    wait_health 8000
    echo "==> gateway-api: http://127.0.0.1:8000  (UI + /api/v1 + WS)"
    ;;
  stop)
    for f in "$PIDS"/*.pid; do
      [ -e "$f" ] || break
      pid="$(cat "$f")"
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" && echo "  stopped $(basename "$f" .pid)"
      fi
      rm -f "$f"
    done
    ;;
  status)
    for f in "$PIDS"/*.pid; do
      [ -e "$f" ] || break
      pid="$(cat "$f")"
      if kill -0 "$pid" 2>/dev/null; then
        echo "  $(basename "$f" .pid): running (pid $pid)"
      else
        echo "  $(basename "$f" .pid): dead"
      fi
    done
    ;;
  logs)
    tail -n 50 -f "$RUN"/*.log
    ;;
  *)
    echo "usage: $0 [start|stop|status|logs]" >&2
    exit 2
    ;;
esac
