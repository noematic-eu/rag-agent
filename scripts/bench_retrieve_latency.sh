#!/usr/bin/env bash
# Measure p50/p99 /retrieve latency at a target chunk count (f4kvs disk, retrieval_lex=index).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TARGET_CHUNKS="${1:-5000}"
PORT="${2:-18100}"
SERVER="http://127.0.0.1:${PORT}"
DATA_DIR="/tmp/rag-bench-${TARGET_CHUNKS}"
CORPUS="bench-${TARGET_CHUNKS}"
QUERIES=50
WARMUP=5

if [[ ! -x ./bin/agent ]]; then
  make f4kvs agent
fi

rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR/corpus"

echo "Generating ${TARGET_CHUNKS} synthetic markdown docs..."
python3 - <<PY "$TARGET_CHUNKS" "$DATA_DIR/corpus"
import sys, pathlib
n = int(sys.argv[1])
out = pathlib.Path(sys.argv[2])
for i in range(n):
    (out / f"doc-{i:06d}.md").write_text(
        f"# Bench document {i}\n\n"
        f"Unique token bench{i} lexical retrieval latency measurement corpus filler text. "
        f"Topic rotation alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi chi psi omega.\n",
        encoding="utf-8",
    )
PY

./bin/agent -addr "127.0.0.1:${PORT}" -data-dir "$DATA_DIR" \
  -lexical-engine=f4kvs -disable-embeddings &
AGENT_PID=$!
trap 'kill $AGENT_PID 2>/dev/null || true' EXIT

for _ in $(seq 1 60); do
  if curl -sf "${SERVER}/stats" >/dev/null; then break; fi
  sleep 1
done

echo "Ingesting corpus (corpus=${CORPUS})..."
go run ./client -mode ingest-dir \
  -dir "$DATA_DIR/corpus" \
  -server "$SERVER" \
  -corpus "$CORPUS" \
  -reset-before-ingest \
  -finalize

CHUNKS=$(curl -sf "${SERVER}/stats" | python3 -c "import json,sys; s=json.load(sys.stdin); print(s.get('ingest',{}).get('chunks_total',0))")
echo "Indexed chunks: ${CHUNKS}"

bench_query() {
  local q="$1"
  curl -sf -o /dev/null -w '%{time_total}\n' \
    "${SERVER}/retrieve?corpus=${CORPUS}&retrieval_lex=index&rq=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$q")&top_k=8"
}

echo "Warmup (${WARMUP} queries)..."
for i in $(seq 1 "$WARMUP"); do
  bench_query "bench topic alpha document ${i}" >/dev/null
done

echo "Benchmark (${QUERIES} queries)..."
TIMES_FILE=$(mktemp)
for i in $(seq 1 "$QUERIES"); do
  bench_query "bench${i} lexical retrieval latency unique token" >> "$TIMES_FILE"
done

python3 - <<PY "$TIMES_FILE" "$TARGET_CHUNKS" "$CHUNKS" "$CORPUS"
import sys, statistics
path, target, chunks, corpus = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
times = sorted(float(line.strip()) * 1000 for line in open(path) if line.strip())
if not times:
    raise SystemExit("no timings")
p50 = statistics.median(times)
p99 = times[int(len(times) * 0.99) - 1]
print(f"corpus={corpus} target_chunks={target} actual_chunks={chunks} queries={len(times)}")
print(f"p50_ms={p50:.2f} p99_ms={p99:.2f} min_ms={times[0]:.2f} max_ms={times[-1]:.2f}")
PY
rm -f "$TIMES_FILE"

kill "$AGENT_PID" 2>/dev/null || true
wait "$AGENT_PID" 2>/dev/null || true
trap - EXIT
