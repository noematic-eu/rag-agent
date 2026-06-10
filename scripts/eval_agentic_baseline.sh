#!/usr/bin/env bash
# Measure single-pass retrieval baseline before comparing agentic modes.
set -euo pipefail

SERVER="${1:-http://127.0.0.1:8080}"
TOP_K="${EVAL_TOP_K:-8}"
OUT="${EVAL_OUT:-eval/out/agentic_baseline.json}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "Agentic baseline (single-pass retrieval)"
echo "  server=$SERVER top_k=$TOP_K"

go run ./client -mode eval-baseline \
  -server "$SERVER" \
  -eval-top-k "$TOP_K" \
  -eval-out "$OUT"

echo "Done. Compare agentic modes with:"
echo "  go run ./client -mode eval-generation -server $SERVER -gold eval/gold/legal.jsonl -search-mode crag"
