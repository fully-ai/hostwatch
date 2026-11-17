#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/hostwatch-server -addr "${HOSTWATCH_ADDR:-127.0.0.1:8096}"
