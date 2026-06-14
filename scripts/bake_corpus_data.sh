#!/usr/bin/env bash
# Bake a corpus index offline into prebuilt/<name>/ for corpus Docker images.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAME=""
SRC=""
CORPUS_TAG=""
DISABLE_EMBEDDINGS=false
RESET=false
LEXICAL_ENGINE="f4kvs"

usage() {
  cat <<'EOF'
Usage: bake_corpus_data.sh --name NAME --src DIR [options]

Bake a RAG index into prebuilt/NAME/ (legal.f4kvs/ + manifest.json).

Options:
  --name NAME           Output directory name under prebuilt/ (required)
  --src DIR             Source markdown/html directory to ingest (required)
  --corpus TAG          Corpus tag stored in the index (default: --name)
  --disable-embeddings  Build lexical-only index (no embed:* keys)
  --reset               Call POST /reset before ingest (wipe temp index)
  --lexical-engine ENG  Lexical engine (default: f4kvs)

Requires: make agent client (binaries in ./bin/)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --name) NAME="$2"; shift 2 ;;
    --src) SRC="$2"; shift 2 ;;
    --corpus) CORPUS_TAG="$2"; shift 2 ;;
    --disable-embeddings) DISABLE_EMBEDDINGS=true; shift ;;
    --reset) RESET=true; shift ;;
    --lexical-engine) LEXICAL_ENGINE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [ -z "$NAME" ] || [ -z "$SRC" ]; then
  echo "error: --name and --src are required" >&2
  usage >&2
  exit 1
fi

if [ -z "$CORPUS_TAG" ]; then
  CORPUS_TAG="$NAME"
fi

if [ ! -d "$SRC" ]; then
  echo "error: source directory not found: $SRC" >&2
  exit 1
fi

case "$SRC" in
  /*) SRC_ABS="$SRC" ;;
  *) SRC_ABS="$ROOT/$SRC" ;;
esac

AGENT="$ROOT/bin/agent"
CLIENT="$ROOT/bin/client"
if [ ! -x "$AGENT" ]; then
  echo "error: $AGENT not found; run: make agent" >&2
  exit 1
fi
if [ ! -x "$CLIENT" ]; then
  echo "error: $CLIENT not found; run: make client" >&2
  exit 1
fi

TMPDATA="$(mktemp -d "${TMPDIR:-/tmp}/rag-bake.XXXXXX")"
OUT="$ROOT/prebuilt/$NAME"
AGENT_PID=""
PORT=""

cleanup() {
  if [ -n "$AGENT_PID" ]; then
    kill "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi
  rm -rf "$TMPDATA"
}
trap cleanup EXIT

PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1]); s.close()')"
SERVER="http://127.0.0.1:${PORT}"

AGENT_ARGS=(
  -addr "127.0.0.1:${PORT}"
  -data-dir "$TMPDATA"
  -lexical-engine "$LEXICAL_ENGINE"
)
if [ "$DISABLE_EMBEDDINGS" = true ]; then
  AGENT_ARGS+=(-disable-embeddings)
fi
if [ "$LEXICAL_ENGINE" = "f4kvs" ]; then
  export RAG_F4KVS_LEXICAL_MODE="${RAG_F4KVS_LEXICAL_MODE:-disk}"
fi

echo "Baking corpus '$CORPUS_TAG' from $SRC_ABS into prebuilt/$NAME ..."
"$AGENT" "${AGENT_ARGS[@]}" &
AGENT_PID=$!

ready=false
for _ in $(seq 1 60); do
  if curl -sf "${SERVER}/stats" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$AGENT_PID" 2>/dev/null; then
    echo "error: agent exited before becoming ready" >&2
    exit 1
  fi
  sleep 0.5
done
if [ "$ready" != true ]; then
  echo "error: agent not ready on ${SERVER}" >&2
  exit 1
fi

INGEST_ARGS=(
  -mode ingest-dir
  -dir "$SRC_ABS"
  -server "$SERVER"
  -corpus "$CORPUS_TAG"
  -finalize=false
)
if [ "$RESET" = true ]; then
  INGEST_ARGS+=(-reset-before-ingest)
fi
"$CLIENT" "${INGEST_ARGS[@]}"

STATS_JSON="$(curl -sf "${SERVER}/stats")"
kill "$AGENT_PID"
wait "$AGENT_PID" 2>/dev/null || true
AGENT_PID=""

if [ ! -d "$TMPDATA/legal.f4kvs" ]; then
  echo "error: ingest did not produce legal.f4kvs under $TMPDATA" >&2
  exit 1
fi

rm -rf "$OUT"
mkdir -p "$OUT"
cp -a "$TMPDATA/." "$OUT/"

GIT_SHA="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
BAKED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

python3 - "$OUT/manifest.json" "$NAME" "$CORPUS_TAG" "$SRC" "$GIT_SHA" "$BAKED_AT" "$DISABLE_EMBEDDINGS" "$LEXICAL_ENGINE" "$STATS_JSON" <<'PY'
import json, sys
out, name, tag, src, git_sha, baked_at, no_embed, lex_eng, stats_raw = sys.argv[1:10]
stats = json.loads(stats_raw)
manifest = {
    "corpus_name": name,
    "corpus_tag": tag,
    "source_dir": src,
    "git_sha": git_sha,
    "baked_at": baked_at,
    "disable_embeddings": no_embed == "true",
    "lexical_engine": lex_eng,
    "ingest": stats.get("ingest", {}),
    "index_manifest": stats.get("manifest", {}),
}
with open(out, "w", encoding="utf-8") as f:
    json.dump(manifest, f, indent=2)
    f.write("\n")
PY

echo "Baked index: $OUT"
echo "  corpus tag: $CORPUS_TAG"
echo "  chunks: $(python3 -c "import json; print(json.load(open('$OUT/manifest.json'))['ingest'].get('chunks_total', '?'))")"
