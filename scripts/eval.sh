#!/usr/bin/env bash
# Run retrieval eval against a running rag-agent.
# Usage: ./scripts/eval.sh [server_url] [gold_jsonl]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${1:-http://localhost:8080}"
GOLD="${2:-$ROOT/eval/gold/public.jsonl}"
TOP_K="${EVAL_TOP_K:-8}"
MIN_RECALL="${EVAL_MIN_RECALL:-0.65}"
OUT="${EVAL_OUT:-$ROOT/eval/out/retrieval_report.json}"

cd "$ROOT"
mkdir -p "$(dirname "$OUT")"

ARGS=(-mode eval-retrieval -server "$SERVER" -gold "$GOLD" -eval-top-k "$TOP_K" -eval-out "$OUT")
if [[ -n "$MIN_RECALL" && "$MIN_RECALL" != "0" ]]; then
  ARGS+=(-min-recall "$MIN_RECALL")
fi

go run ./client "${ARGS[@]}"
