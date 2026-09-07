#!/usr/bin/env bash
# AutoPro quick start: build + run server (Go) + optional gateway.
# It serves dashboard, API and WebSocket on http://127.0.0.1:8000
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
mkdir -p "$ROOT/.run"
cd "$ROOT/backend"

echo "==> Building server"
go build -o /tmp/autopro-server ./cmd/server

echo "==> Starting AutoPro server on http://127.0.0.1:8000"
DB_URL="sqlite://$ROOT/.run/autopro.db" HTTP_ADDR="127.0.0.1:8000" \
  SIMULATOR_ENABLED=true STALENESS_SECONDS=60s \
  nohup /tmp/autopro-server > "$ROOT/.run/server.log" 2>&1 &

sleep 2

echo "==> Health check"
curl -sf http://127.0.0.1:8000/api/v1/health >/dev/null && echo "    server: OK"
curl -sf http://127.0.0.1:8000/ >/dev/null && echo "    dashboard: OK"

echo
echo "Open http://127.0.0.1:8000 in a browser."
echo "Stop the server with:  fuser -k 8000/tcp"