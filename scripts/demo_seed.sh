#!/usr/bin/env bash
# Seed the demo agent with legal-demo (Constitution) and eval-public corpora.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://127.0.0.1:8080}"
FORCE="${FORCE_DEMO_SEED:-0}"

wait_for_agent() {
  local i
  for i in $(seq 1 60); do
    if curl -sf "${SERVER}/stats" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "agent not reachable at $SERVER" >&2
  exit 1
}

document_count() {
  curl -sf "${SERVER}/stats" | python3 -c "import json,sys; print(json.load(sys.stdin).get('ingest',{}).get('documents_total',0))"
}

wait_for_agent

DOCS="$(document_count)"
if [[ "$DOCS" != "0" && "$FORCE" != "1" ]]; then
  echo "Demo corpora already seeded ($DOCS documents). Set FORCE_DEMO_SEED=1 to re-seed."
  exit 0
fi

cd "$ROOT"

if [[ "$FORCE" == "1" ]]; then
  echo "FORCE_DEMO_SEED=1: resetting index before seeding..."
  curl -sf -X POST "${SERVER}/reset" >/dev/null
fi

echo "Ingesting Constitution corpus (legal-demo)..."
go run ./client -mode ingest-dir \
  -dir "$ROOT/texts" \
  -server "$SERVER" \
  -corpus legal-demo \
  -finalize=false

echo "Ingesting eval-public fixtures..."
go run ./client -mode ingest-dir \
  -dir "$ROOT/eval/fixtures/docs" \
  -server "$SERVER" \
  -corpus eval-public \
  -finalize=true

echo "Demo seed complete."
curl -sf "${SERVER}/stats" | python3 -m json.tool
