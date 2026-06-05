#!/usr/bin/env bash
# Compare retrieval eval scores across bleve, tantivy, and f4kvs lexical engines.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -x ./bin/agent ]]; then
  echo "Building agent (requires: make f4kvs tantivy && make agent)" >&2
  make f4kvs tantivy agent
fi

mkdir -p eval/out
ENGINES=(bleve tantivy f4kvs)
BASE_PORT=18080

cleanup() {
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

PIDS=()
for i in "${!ENGINES[@]}"; do
  engine="${ENGINES[$i]}"
  port=$((BASE_PORT + i))
  data_dir="/tmp/rag-eval-${engine}"
  rm -rf "$data_dir"
  mkdir -p "$data_dir"

  ./bin/agent -addr "127.0.0.1:${port}" -data-dir "$data_dir" -lexical-engine="$engine" -disable-embeddings &
  PIDS+=($!)
  for _ in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${port}/stats" >/dev/null; then
      break
    fi
    sleep 1
  done

  ./scripts/eval_setup_public.sh "http://127.0.0.1:${port}"
  EVAL_MIN_RECALL=0 EVAL_OUT="eval/out/retrieval_${engine}.json" \
    ./scripts/eval.sh "http://127.0.0.1:${port}" eval/gold/public.jsonl

  kill "${PIDS[$i]}" 2>/dev/null || true
  wait "${PIDS[$i]}" 2>/dev/null || true

  if [[ "$engine" == "bleve" ]]; then
    du -sh "$data_dir/legal.bleve" 2>/dev/null || true
  elif [[ "$engine" == "tantivy" ]]; then
    du -sh "$data_dir/legal.tantivy" 2>/dev/null || true
  else
    du -sh "$data_dir/legal.f4kvs" 2>/dev/null || true
  fi
done

echo ""
echo "=== Lexical engine comparison (public gold) ==="
for engine in "${ENGINES[@]}"; do
  report="eval/out/retrieval_${engine}.json"
  if [[ -f "$report" ]]; then
    python3 - <<PY "$report" "$engine"
import json, sys
path, engine = sys.argv[1], sys.argv[2]
with open(path) as f:
    r = json.load(f)
print(f"{engine}: recall@{r['top_k']}={r['recall_at_k']:.3f} mrr={r['mrr']:.3f} cases={r['cases']}")
PY
  fi
done
