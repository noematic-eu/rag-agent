#!/usr/bin/env bash
# Ingest KB markdown library from catalog.json into a running RAG agent.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG="${1:-$HOME/kb/catalog.json}"
SERVER="${2:-http://127.0.0.1:8081}"

cd "$ROOT"

echo "Validating catalog: $CATALOG"
python3 "$ROOT/scripts/kb_catalog_validate.py" --catalog "$CATALOG"

echo "Ingesting catalog -> $SERVER"
python3 "$ROOT/scripts/kb_catalog_ingest.py" \
  --catalog "$CATALOG" \
  --server "$SERVER"

echo "Done."
