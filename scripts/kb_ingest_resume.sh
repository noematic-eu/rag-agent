#!/usr/bin/env bash
# Resume KB catalog ingestion; restarts until the full catalog is ingested.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CATALOG="${CATALOG:-$HOME/kb/catalog.json}"
SERVER="${SERVER:-http://127.0.0.1:8081}"
PROGRESS="${PROGRESS:-$HOME/kb/.ingest-progress.json}"
LOG="${LOG:-/tmp/kb-ingest-resume.log}"
RESUME_AFTER="${RESUME_AFTER:-}"
GRADES="${GRADES:-}"
INDEX_DATA="${INDEX_DATA:-$HOME/kb/index-data.json}"
REPLACE_CORPUS="${REPLACE_CORPUS:-}"
CORPUS="${CORPUS:-}"

cd "$ROOT"
export PYTHONUNBUFFERED=1

args=(
  --catalog "$CATALOG"
  --server "$SERVER"
  --skip-validate
  --resume
  --progress-file "$PROGRESS"
  --batch-size 10
  --timeout 7200
  --no-finalize
)
if [[ -n "$RESUME_AFTER" ]]; then
  args+=(--resume-after "$RESUME_AFTER")
fi
if [[ -n "$GRADES" ]]; then
  # shellcheck disable=SC2206
  args+=(--grades $GRADES)
  args+=(--index-data "$INDEX_DATA")
fi
if [[ -n "$REPLACE_CORPUS" ]]; then
  args+=(--replace-corpus)
fi
if [[ -n "$CORPUS" ]]; then
  args+=(--corpus "$CORPUS")
fi

STALL_SECONDS="${STALL_SECONDS:-300}"
CONTAINER="${CONTAINER:-rag-agent}"

rag_healthy() {
  curl -sf --max-time 5 "$SERVER/stats" >/dev/null
}

restart_rag_agent() {
  echo "=== $(date -Iseconds) restarting $CONTAINER (ingest mutex likely stuck) ===" | tee -a "$LOG"
  docker compose -f "$ROOT/docker-compose.yml" restart "$CONTAINER" >>"$LOG" 2>&1
  sleep 8
}

chunk_log_count() {
  docker logs "$CONTAINER" 2>&1 | grep -c 'Chunk .* indexé' || true
}

# Returns 0 if rag-agent chunk logs advanced since $1 (baseline count).
ingest_is_progressing() {
  local baseline="$1"
  local current
  current="$(chunk_log_count)"
  [[ "$current" -gt "$baseline" ]]
}

while true; do
  echo "=== $(date -Iseconds) kb ingest resume ===" | tee -a "$LOG"
  if ! rag_healthy; then
    restart_rag_agent
  fi
  stalled=0
  python3 "$ROOT/scripts/kb_catalog_ingest.py" "${args[@]}" >>"$LOG" 2>&1 &
  ingest_pid=$!
  baseline_chunks="$(chunk_log_count)"
  stall_checks=0
  max_stall_checks=$(( STALL_SECONDS / 60 ))
  [[ "$max_stall_checks" -lt 1 ]] && max_stall_checks=1
  while kill -0 "$ingest_pid" 2>/dev/null; do
    sleep 60
    if ingest_is_progressing "$baseline_chunks"; then
      baseline_chunks="$(chunk_log_count)"
      stall_checks=0
      continue
    fi
    stall_checks=$(( stall_checks + 1 ))
    if [[ "$stall_checks" -ge "$max_stall_checks" ]]; then
      echo "=== $(date -Iseconds) no chunk progress for ${STALL_SECONDS}s, restarting agent ===" | tee -a "$LOG"
      restart_rag_agent
      kill "$ingest_pid" 2>/dev/null || true
      stalled=1
      break
    fi
  done
  if wait "$ingest_pid"; then
    echo "=== $(date -Iseconds) ingest complete ===" | tee -a "$LOG"
    curl -sf -X POST "$SERVER/finalize" >/dev/null && \
      echo "=== $(date -Iseconds) finalize ok ===" | tee -a "$LOG"
    exit 0
  fi
  code=$?
  if [[ "$stalled" -eq 1 ]]; then
    echo "=== $(date -Iseconds) ingest stalled, retrying ===" | tee -a "$LOG"
  else
    echo "=== $(date -Iseconds) ingest exited $code, restarting rag-agent ===" | tee -a "$LOG"
    restart_rag_agent
  fi
  sleep 10
done
