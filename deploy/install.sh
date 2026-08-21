#!/usr/bin/env bash
# One-time setup: installs the callcentrix systemd service.
# Run from the deploy/ directory on the server: sudo ./install.sh
set -euo pipefail

APP_DIR=/home/komil/cc_hosting_version
SERVICE_NAME=callcentrix

if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root (sudo ./install.sh)" >&2
    exit 1
fi

chmod +x "$APP_DIR/callcentrix-linux"

cp "$(dirname "$0")/callcentrix.service" "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "Installed and started. Check status with: systemctl status $SERVICE_NAME"
