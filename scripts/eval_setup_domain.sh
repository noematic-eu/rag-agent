#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://localhost:8080}"

cd "$ROOT"
go run ./client -mode ingest-dir \
  -dir "$ROOT/eval/fixtures/domain" \
  -server "$SERVER" \
  -corpus eval-domain \
  -reset-before-ingest \
  -finalize
