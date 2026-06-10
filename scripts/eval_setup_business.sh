#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://localhost:8080}"

cd "$ROOT"
go run ./client -mode ingest-dir \
  -dir "$ROOT/eval/fixtures/business" \
  -server "$SERVER" \
  -corpus eval-business \
  -reset-before-ingest \
  -finalize
