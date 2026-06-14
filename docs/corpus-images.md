# Corpus-bundled Docker images

Ship a **ready-to-run** RAG agent with a pre-built index baked into the image. No post-deploy seeding scripts and no required data volume for read-only deployments.

Base agent images (`ghcr.io/noematic-eu/rag-agent:latest`) still ship an empty `/data` tree. Corpus variants add a baked index at `/seed` inside the image.

## Published tags

On each GitHub release, CI publishes:

| Tag | Corpus tag | Source |
|-----|------------|--------|
| `corpus-eval-public` | `eval-public` | `eval/fixtures/docs/` |
| `corpus-legal-demo` | `legal-demo` | `texts/` |

Pin a release: `corpus-eval-public-v0.2.0` (semver suffix from the release tag).

Phase 1 images are **lexical-only** (`RAG_DISABLE_EMBEDDINGS=true`). Hybrid variants with baked embeddings can reuse the same `prebuilt/` workflow later.

## Zero-seed deploy

Ensure Ollama is running on the host, then:

```bash
docker compose -f docker-compose.corpus.yml pull
docker compose -f docker-compose.corpus.yml up -d
curl http://127.0.0.1:8081/stats
curl --globoff 'http://127.0.0.1:8081/retrieve?corpus=eval-public&rq=hybrid+retrieval&top_k=6'
```

No volume mount is required. Restarting the container keeps the baked corpus (immutable mode).

**Without Compose:**

```bash
docker run -d -p 8081:8080 \
  -e RAG_LLM_BASE_URL=http://host.docker.internal:11434 \
  -e RAG_DISABLE_EMBEDDINGS=true \
  --add-host=host.docker.internal:host-gateway \
  --name rag-agent-corpus \
  ghcr.io/noematic-eu/rag-agent:corpus-eval-public
```

## Writable overlay (optional)

To persist runtime ingests on top of the baked corpus, mount a volume at `/data`. The entrypoint copies `/seed` into an empty volume once on first boot:

```yaml
volumes:
  - rag-data:/data
```

See [`docker-compose.corpus.yml`](../docker-compose.corpus.yml) (commented example).

## Build paths

Two bake modes share one [`Dockerfile.corpus`](../Dockerfile.corpus):

| Mode | When | Input |
|------|------|-------|
| **ingest** | In-repo demos, CI | Source markdown tree + build-time `client ingest-dir` |
| **prebuilt** | Production / private KB | `prebuilt/<name>/` from offline bake |

### A. Build-time ingest (demos)

```bash
# Requires F4KVS_ROOT and F4KVS_LEXICAL_ROOT in .env (see .env.example)
docker compose -f docker-compose.corpus.yml -f docker-compose.corpus.build.yml up -d --build

# Legal demo variant
CORPUS_NAME=legal-demo CORPUS_SRC=texts docker compose \
  -f docker-compose.corpus.yml -f docker-compose.corpus.build.yml up -d --build
```

Or standalone:

```bash
docker build -f Dockerfile.corpus --build-context f4kvs=./f4kvs-ffi \
  --build-arg CORPUS_NAME=eval-public \
  --build-arg BAKE_MODE=ingest \
  --build-arg CORPUS_SRC=eval/fixtures/docs \
  --build-arg DISABLE_EMBEDDINGS=true \
  -t rag-agent:corpus-eval-public .
```

### B. Offline prebuilt data dir (production)

1. Bake the index locally (or on a build host with Ollama for hybrid):

```bash
make bake-eval-public
# → prebuilt/eval-public/legal.f4kvs/ + manifest.json
```

For a private markdown library, ingest with [`kb-ingest.md`](kb-ingest.md), then copy the resulting `legal.f4kvs/` tree into `prebuilt/<your-corpus-name>/`.

2. Build the image from the prebuilt tree:

```bash
docker build -f Dockerfile.corpus --build-context f4kvs=./f4kvs-ffi \
  --build-arg CORPUS_NAME=eval-public \
  --build-arg BAKE_MODE=prebuilt \
  -t rag-agent:corpus-eval-public .
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make bake-eval-public` | Offline bake `eval/fixtures/docs` → `prebuilt/eval-public/` |
| `make bake-legal-demo` | Offline bake `texts/` → `prebuilt/legal-demo/` |
| `make docker-corpus-build` | Local Docker build via compose overrides |

## Image layout

```
/app/bin/agent          # RAG HTTP server
/app/bin/entrypoint.sh  # Seeds /data from /seed when volume is empty
/seed/legal.f4kvs/      # Baked index (read-only image layer)
/data/                  # RAG_DATA_DIR when a volume is mounted
```

## Upgrading a corpus

Pin the image tag to a release (`corpus-eval-public-v0.2.0`). When source documents change, rebuild the image (ingest or prebuilt bake) and redeploy. The corpus version travels with the image tag — no separate volume backup required for read-only deployments.

## Related docs

- [`docker-compose-quickstart.md`](docker-compose-quickstart.md) — base agent + manual seeding
- [`kb-ingest.md`](kb-ingest.md) — graded ingest for private KB libraries
- [`disk-architecture.md`](disk-architecture.md) — on-disk key layout under `legal.f4kvs/`
