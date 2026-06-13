#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found; install Docker Desktop (https://www.docker.com/products/docker-desktop/)" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "error: Docker daemon is not running" >&2
  echo "  Start Docker Desktop, then retry: make unikraft-build" >&2
  exit 1
fi

if ! command -v kraft >/dev/null 2>&1; then
  echo "error: kraft not found; install Kraftkit (https://unikraft.org/docs/cli)" >&2
  exit 1
fi
