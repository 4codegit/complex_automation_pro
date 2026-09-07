#!/usr/bin/env bash
# AutoPro health check (Go server on :8000)
set -euo pipefail

echo "==> Server (http://127.0.0.1:8000)"
if curl -sf http://127.0.0.1:8000/api/v1/health > /dev/null 2>&1; then
  echo "    API: OK ($(curl -s http://127.0.0.1:8000/api/v1/health | grep -o '"status":"[^"]*"' | cut -d'"' -f4))"
else
  echo "    API: NOT RESPONDING"
fi

echo "==> Dashboard"
if curl -sf http://127.0.0.1:8000/ > /dev/null 2>&1; then
  echo "    Dashboard: OK"
else
  echo "    Dashboard: NOT RESPONDING"
fi

echo "==> Simulator"
if resp=$(curl -s http://127.0.0.1:8000/api/v1/simulator/status 2>/dev/null); then
  running=$(echo "$resp" | grep -o '"running":[a-z]*' | cut -d: -f2)
  count=$(echo "$resp" | grep -o '"telemetry_count":[0-9]*' | cut -d: -f2)
  echo "    running=$running, telemetry_count=$count"
else
  echo "    Simulator: NOT RESPONDING"
fi

echo
echo "Open http://127.0.0.1:8000"