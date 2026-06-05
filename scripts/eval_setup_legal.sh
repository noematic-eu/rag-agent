#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://127.0.0.1:8080}"

echo "Ingesting legal-demo corpus into ${SERVER}..."
go run "${ROOT}/client" -mode ingest-dir \
  -dir "${ROOT}/texts" \
  -corpus legal-demo \
  -finalize=true \
  -server "${SERVER}"

echo "Legal demo corpus ready."
