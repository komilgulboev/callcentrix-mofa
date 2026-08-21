#!/usr/bin/env bash
set -euo pipefail
sudo systemctl restart callcentrix
systemctl status callcentrix --no-pager
