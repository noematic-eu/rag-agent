#!/usr/bin/env bash
set -euo pipefail

SERVER="${1:-http://localhost:8080}"
cd "$(dirname "$0")/.."
go run ./client -mode bench -server "$SERVER"
