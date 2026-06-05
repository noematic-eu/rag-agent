# Docker Compose quickstart

This bundle starts `rag-agent` and `ollama` together for local pilots.

## 0) f4kvs-v2 prerequisite

`rag-agent` links against **f4kvs-ffi**, which is **not** on public GitHub. Clone or use your existing checkout:

```bash
# if needed
git clone https://git.noematic.eu/f4kvs-org/f4kvs-v2.git ~/dev/rust/f4kvs-v2
```

Copy `.env.example` to `.env` and set `F4KVS_ROOT` to that path:

```bash
cp .env.example .env
# edit F4KVS_ROOT=/Users/you/dev/rust/f4kvs-v2
```

## 1) Build and run

```bash
docker compose up -d --build
```

## 2) Pull required models in Ollama

```bash
docker exec rag-agent-ollama ollama pull qwq
docker exec rag-agent-ollama ollama pull nomic-embed-text
```

## 3) Smoke test API

```bash
curl http://127.0.0.1:8081/stats
```

The compose file publishes the agent on **host port 8081** so it does not clash with a native `./bin/agent` on `127.0.0.1:8080` (which would shadow Docker on the same port).

## 4) Ingest sample docs

```bash
go run ./client -mode ingest-dir -dir ./eval/fixtures/docs -server http://127.0.0.1:8081 -corpus eval-public
```

## 5) Test retrieval and generation

```bash
curl --globoff 'http://127.0.0.1:8081/retrieve?corpus=eval-public&rq=hybrid+retrieval&top_k=6'
curl --globoff 'http://127.0.0.1:8081/search?corpus=eval-public&rq=hybrid+retrieval&q=Explain+hybrid+retrieval+in+3+points'
```


## Stop stack

```bash
docker compose down
```

To remove indexes and models too:

```bash
docker compose down -v
```
