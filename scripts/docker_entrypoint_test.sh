#!/usr/bin/env bash
# Smoke-test scripts/docker_entrypoint.sh seed/copy logic without Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENTRYPOINT="$ROOT/scripts/docker_entrypoint.sh"

if [ ! -x "$ENTRYPOINT" ]; then
  echo "error: missing executable $ENTRYPOINT" >&2
  exit 1
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/rag-entrypoint-test.XXXXXX")"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

make_index() {
  mkdir -p "$1/legal.f4kvs"
  touch "$1/legal.f4kvs/.keep"
}

run_entrypoint() {
  local seed="$1"
  local data="$2"
  shift 2
  env RAG_SEED_DIR="$seed" RAG_DATA_DIR="$data" "$@" "$ENTRYPOINT" /bin/sh -c 'echo "RAG_DATA_DIR=${RAG_DATA_DIR:-unset}"'
}

assert_eq() {
  local got="$1" want="$2" msg="$3"
  if [ "$got" != "$want" ]; then
    echo "FAIL: $msg (got=$got want=$want)" >&2
    exit 1
  fi
  echo "ok: $msg"
}

# Case 1: seed only, no volume → use read-only seed (no copy)
seed="$tmpdir/seed1"
data="$tmpdir/data1"
make_index "$seed"
got="$(run_entrypoint "$seed" "$data")"
assert_eq "$got" "RAG_DATA_DIR=$seed" "seed-only uses /seed without copy"
test ! -d "$data/legal.f4kvs" || { echo "FAIL: should not copy without volume" >&2; exit 1; }
echo "ok: no copy without volume mount"

# Case 2: seed + empty data dir with forced copy → use /data
seed="$tmpdir/seed2"
data="$tmpdir/data2"
make_index "$seed"
got="$(run_entrypoint "$seed" "$data" env RAG_FORCE_SEED_COPY=1)"
assert_eq "$got" "RAG_DATA_DIR=$data" "empty /data seeded from /seed"
test -f "$data/legal.f4kvs/.keep" || { echo "FAIL: copy missing index" >&2; exit 1; }
echo "ok: index copied to /data"

# Case 3: seed + existing /data index → no overwrite
seed="$tmpdir/seed3"
data="$tmpdir/data3"
make_index "$seed"
make_index "$data"
echo "existing" > "$data/legal.f4kvs/marker"
got="$(run_entrypoint "$seed" "$data" env RAG_FORCE_SEED_COPY=1)"
assert_eq "$got" "RAG_DATA_DIR=$data" "existing /data kept"
grep -q existing "$data/legal.f4kvs/marker" || { echo "FAIL: /data was overwritten" >&2; exit 1; }
echo "ok: existing /data not overwritten"

# Case 4: no seed → fall through to data dir
seed="$tmpdir/empty-seed"
data="$tmpdir/data4"
mkdir -p "$seed" "$data"
got="$(run_entrypoint "$seed" "$data")"
assert_eq "$got" "RAG_DATA_DIR=$data" "no seed uses /data default"

echo "All docker_entrypoint tests passed."
