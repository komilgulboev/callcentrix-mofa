#!/usr/bin/env bash
set -euo pipefail
sudo systemctl start callcentrix
systemctl status callcentrix --no-pager
