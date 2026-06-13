#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

F4KVS_LINK="$ROOT/f4kvs-ffi"
F4KVS_REPO="${F4KVS_REPO:-https://github.com/noematic-eu/f4kvs-ffi.git}"

if [ -d "$F4KVS_LINK" ] || [ -L "$F4KVS_LINK" ]; then
  echo "f4kvs-ffi ready: $F4KVS_LINK"
  exit 0
fi

if [ -f "$ROOT/.env" ]; then
  # shellcheck disable=SC1091
  set -a
  source "$ROOT/.env"
  set +a
fi

if [ -n "${F4KVS_ROOT:-}" ] && [ -d "$F4KVS_ROOT" ]; then
  echo "Linking f4kvs-ffi -> $F4KVS_ROOT"
  ln -s "$F4KVS_ROOT" "$F4KVS_LINK"
  exit 0
fi

echo "Cloning f4kvs-ffi into $F4KVS_LINK"
git clone --depth 1 "$F4KVS_REPO" "$F4KVS_LINK"
