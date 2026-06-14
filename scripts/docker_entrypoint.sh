#!/bin/sh
# Seed RAG_DATA_DIR from baked /seed when a mounted volume is empty.
set -e

DATA_DIR="${RAG_DATA_DIR:-/data}"
SEED_DIR="${RAG_SEED_DIR:-/seed}"

has_index() {
  [ -d "$1/legal.f4kvs" ] && [ -n "$(ls -A "$1/legal.f4kvs" 2>/dev/null)" ]
}

data_is_mount() {
  grep -qs "[[:space:]]${DATA_DIR}[[:space:]]" /proc/mounts 2>/dev/null
}

if has_index "$SEED_DIR"; then
  if ! has_index "$DATA_DIR"; then
    if [ "${RAG_FORCE_SEED_COPY:-}" = "1" ] || data_is_mount; then
      mkdir -p "$DATA_DIR"
      echo "Seeding ${DATA_DIR} from baked corpus at ${SEED_DIR}..." >&2
      cp -a "${SEED_DIR}/." "${DATA_DIR}/"
    fi
  fi
fi

if has_index "$DATA_DIR"; then
  export RAG_DATA_DIR="$DATA_DIR"
elif has_index "$SEED_DIR"; then
  export RAG_DATA_DIR="$SEED_DIR"
fi

exec "$@"
