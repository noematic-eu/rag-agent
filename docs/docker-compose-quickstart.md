# Docker Compose quickstart

This bundle starts `rag-agent` in Docker for local pilots. By default it uses **Ollama on the host** (`host.docker.internal:11434`), not a containerized Ollama service.

## 0) f4kvs-ffi prerequisite

`rag-agent` links against **f4kvs-ffi**. Clone or use your existing checkout:

```bash
# if needed
git clone https://github.com/noematic-eu/f4kvs-ffi.git ~/dev/rust/f4kvs-ffi
```

Copy `.env.example` to `.env` and set `F4KVS_ROOT` to that path:

```bash
cp .env.example .env
# edit F4KVS_ROOT=/Users/you/dev/rust/f4kvs-ffi
```

## 1) Build and run

Ensure Ollama is running on the host, then start the agent:

```bash
docker compose up -d --build
```

The compose file publishes the agent on **host port 8081** so it does not clash with a native `./bin/agent` on `127.0.0.1:8080`.

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
