#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v0.1.0}"
OUT_DIR="${2:-dist/${VERSION}}"

mkdir -p "${OUT_DIR}"

echo "[1/3] building native libraries"
make f4kvs tantivy

echo "[2/3] building macOS and Linux binaries"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -tags tantivy -o "${OUT_DIR}/rag-agent_darwin_arm64" ./agent
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -tags tantivy -o "${OUT_DIR}/rag-agent_darwin_amd64" ./agent
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags tantivy -o "${OUT_DIR}/rag-agent_linux_amd64" ./agent
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags tantivy -o "${OUT_DIR}/rag-agent_linux_arm64" ./agent

echo "[3/3] checksums"
(
  cd "${OUT_DIR}"
  shasum -a 256 rag-agent_* > SHA256SUMS.txt
)

echo "Artifacts ready in ${OUT_DIR}"
