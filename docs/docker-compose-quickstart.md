# Docker Compose quickstart

This bundle starts `rag-agent` in Docker for local pilots. By default it uses **Ollama on the host** (`host.docker.internal:11434`), not a containerized Ollama service.

Pre-built images are published to **GHCR** on each GitHub release: `ghcr.io/noematic-eu/rag-agent` (`linux/amd64`, `linux/arm64`). Pin a version tag (e.g. `:v0.1.0`) or use `:latest` for the most recent release.

## 1) Pull and run

Ensure Ollama is running on the host, then start the agent:

```bash
docker compose pull
docker compose up -d
```

The compose file publishes the agent on **host port 8081** so it does not clash with a native `./bin/agent` on `127.0.0.1:8080`.

**Without Compose** (one-liner):

```bash
docker run -d -p 8080:8080 \
  -v ./rag-data:/data \
  -e RAG_LLM_BASE_URL=http://host.docker.internal:11434 \
  --add-host=host.docker.internal:host-gateway \
  --name rag-agent \
  ghcr.io/noematic-eu/rag-agent:latest
```

To run a fully self-contained stack (Ollama in Docker), uncomment the `ollama` service and `ollama-data` volume in [`docker-compose.yml`](../docker-compose.yml), then use `docker exec rag-agent-ollama ollama pull ...` instead of the host commands below.

## 2) Pull required models in Ollama (host)

```bash
ollama pull qwen2.5:7b-instruct
ollama pull nomic-embed-text
```

For query rewrite / HyDE experiments you may optionally pull a reasoning model (e.g. `qwq`), but use an instruct model for `/search` answers.

## 3) Smoke test API

```bash
curl http://127.0.0.1:8081/stats
```

## 4) Seed demo corpora

To populate the agent with the French Constitution (`legal-demo`) and the eval fixtures (`eval-public`):

```bash
# Constitution française (legal-demo)
./scripts/eval_setup_legal.sh http://127.0.0.1:8081

# Corpus eval-public (add without reset)
go run ./client -mode ingest-dir \
  -dir ./eval/fixtures/docs \
  -server http://127.0.0.1:8081 \
  -corpus eval-public \
  -finalize=true
```

For an **eval-public-only** index (wipes existing data), use [`scripts/eval_setup_public.sh`](../scripts/eval_setup_public.sh) instead — it calls `POST /reset` before ingesting.

## 5) Test retrieval and generation

```bash
curl --globoff 'http://127.0.0.1:8081/retrieve?corpus=eval-public&rq=hybrid+retrieval&top_k=6'
curl --globoff 'http://127.0.0.1:8081/search?corpus=eval-public&rq=hybrid+retrieval&q=Explain+hybrid+retrieval+in+3+points'
```

## Stop stack

```bash
docker compose down
```

To remove indexes too:

```bash
docker compose down -v
```

## Building from source (contributors)

`rag-agent` links against **f4kvs-ffi**. Clone or use your existing checkout:

```bash
git clone https://github.com/noematic-eu/f4kvs-ffi.git ~/dev/rust/f4kvs-ffi
```

Copy `.env.example` to `.env` and set `F4KVS_ROOT` to that path:

```bash
cp .env.example .env
# edit F4KVS_ROOT=/Users/you/dev/rust/f4kvs-ffi
```

Build locally instead of pulling from GHCR:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```
