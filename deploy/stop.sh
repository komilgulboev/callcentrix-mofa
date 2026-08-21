#!/usr/bin/env bash
set -euo pipefail
sudo systemctl stop callcentrix
systemctl status callcentrix --no-pager || true
