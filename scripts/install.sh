#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o bin/hostwatch-server ./cmd/hostwatch-server
go build -o bin/hostwatch-agent ./cmd/hostwatch-agent
