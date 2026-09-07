#!/usr/bin/env bash
# CAP installer: builds binaries, installs systemd units, creates users/dirs
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="/opt/cap/bin"
VAR_DIR="/var/lib/cap"
SYSTEMD_DIR="/etc/systemd/system"

echo "==> Building binaries"
cd "$ROOT/backend"
go build -o "$BIN_DIR/cap-server" ./cmd/server
go build -o "$BIN_DIR/cap-gateway" ./cmd/gateway

echo "==> Creating user and directories"
id -u cap &>/dev/null || useradd -r -s /usr/sbin/nologin -d /opt/cap cap
mkdir -p "$VAR_DIR"
chown cap:cap "$VAR_DIR"

echo "==> Installing systemd units"
install -m 644 "$ROOT/deploy/systemd/cap-server.service" "$SYSTEMD_DIR/"
install -m 644 "$ROOT/deploy/systemd/cap-gateway.service" "$SYSTEMD_DIR/"

echo "==> Reloading systemd"
systemctl daemon-reload

echo "==> Enabling services"
systemctl enable cap-server cap-gateway

echo "==> Done. Start with:"
echo "    systemctl start cap-server"
echo "    systemctl start cap-gateway"
echo "    journalctl -fu cap-server -f cap-gateway"