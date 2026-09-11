#!/usr/bin/env bash
# Sends a scenario command to the process stand through the platform proxy.
# Usage: scenario.sh <code> [value]   — codes are documented in TZ §7.3.
set -euo pipefail
CODE="${1:?usage: scenario.sh <code> [value]}"
VALUE="${2:-0}"

COOKIE=$(mktemp)
trap 'rm -f "$COOKIE"' EXIT

curl -sf -c "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"username":"operator","password":"operator"}' \
  http://127.0.0.1:8000/api/v1/access/login > /dev/null

curl -sf -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"code\":${CODE},\"value\":${VALUE}}" \
  http://127.0.0.1:8000/api/v1/scenario
echo
