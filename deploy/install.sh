#!/usr/bin/env bash
# AutoPro installer: builds binaries, installs systemd units, creates users/dirs
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="/opt/autopro/bin"
VAR_DIR="/var/lib/autopro"
SYSTEMD_DIR="/etc/systemd/system"

echo "==> Building binaries"
cd "$ROOT/backend"
go build -o "$BIN_DIR/autopro-server" ./cmd/server
go build -o "$BIN_DIR/autopro-gateway" ./cmd/gateway

echo "==> Creating user and directories"
id -u autopro &>/dev/null || useradd -r -s /usr/sbin/nologin -d /opt/autopro autopro
mkdir -p "$VAR_DIR"
chown autopro:autopro "$VAR_DIR"

echo "==> Installing systemd units"
install -m 644 "$ROOT/deploy/systemd/autopro-server.service" "$SYSTEMD_DIR/"
install -m 644 "$ROOT/deploy/systemd/autopro-gateway.service" "$SYSTEMD_DIR/"

echo "==> Reloading systemd"
systemctl daemon-reload

echo "==> Enabling services"
systemctl enable autopro-server autopro-gateway

echo "==> Done. Start with:"
echo "    systemctl start autopro-server"
echo "    systemctl start autopro-gateway"
echo "    journalctl -fu autopro-server -f autopro-gateway"