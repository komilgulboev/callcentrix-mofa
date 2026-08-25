#!/usr/bin/env bash
# One-time setup: installs the callcentrix systemd service.
# Run from the deploy/ directory on the server: sudo bash install.sh
set -euo pipefail

APP_DIR=/home/komil/cc_hosting_version
SERVICE_NAME=callcentrix
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root (sudo bash install.sh)" >&2
    exit 1
fi

# scp from Windows often drops the exec bit — restore it on everything that needs it.
chmod +x "$APP_DIR/callcentrix-linux" "$SCRIPT_DIR"/*.sh

cp "$SCRIPT_DIR/callcentrix.service" "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "Installed and started. Check status with: systemctl status $SERVICE_NAME"
