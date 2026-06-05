#!/usr/bin/env bash
# Ingest public eval fixtures into a running agent (corpus eval-public).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://localhost:8080}"

echo "WARNING: this calls POST /reset on $SERVER and replaces the index with eval fixtures only." >&2
echo "Use a dedicated agent (e.g. -data-dir /tmp/rag-eval on :8080), not your production encyclopedia." >&2

cd "$ROOT"
go run ./client -mode ingest-dir \
  -dir "$ROOT/eval/fixtures/docs" \
  -server "$SERVER" \
  -corpus eval-public \
  -reset-before-ingest \
  -finalize
