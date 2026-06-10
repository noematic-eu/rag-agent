#!/usr/bin/env bash
# Convert books94 JSONL categories to Markdown and ingest into a running RAG agent.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${1:-$HOME/extracted}"
OUT="${2:-$HOME/books94-md}"
SERVER="${3:-http://127.0.0.1:8081}"
CATEGORY="${4:-}"

cd "$ROOT"

echo "Converting JSONL from $SRC -> $OUT"
if [[ -n "$CATEGORY" ]]; then
  python3 "$ROOT/scripts/books94_jsonl_to_md.py" --src "$SRC" --out "$OUT" --category "$CATEGORY"
  CATEGORIES=("$CATEGORY")
else
  python3 "$ROOT/scripts/books94_jsonl_to_md.py" --src "$SRC" --out "$OUT"
  CATEGORIES=(almanac atlas chronology dictionary encyclopedia quotations thesaurus)
fi

for cat in "${CATEGORIES[@]}"; do
  if [[ ! -d "$OUT/$cat" ]]; then
    echo "skip ingest: no output directory $OUT/$cat" >&2
    continue
  fi
  echo "Ingesting $OUT/$cat (corpus=books94-$cat) -> $SERVER"
  go run ./client -mode ingest-dir \
    -dir "$OUT/$cat" \
    -server "$SERVER" \
    -corpus "books94-$cat" \
    -finalize=false \
    -batch-size 500
done

echo "Finalizing index on $SERVER"
curl -sf -X POST "$SERVER/finalize"
echo "Done."
