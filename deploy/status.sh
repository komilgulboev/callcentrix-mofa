#!/usr/bin/env bash
set -euo pipefail
systemctl status callcentrix --no-pager
echo
echo "--- last 50 log lines (journalctl -u callcentrix -f to follow) ---"
journalctl -u callcentrix -n 50 --no-pager
