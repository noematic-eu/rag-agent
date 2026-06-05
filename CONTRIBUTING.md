# Contributing

## Development model

Feature development happens in the private `noematic-eu/rag-agent` repository. This public repo (`ai-rag-agent`) receives release snapshots via `scripts/sync-public.sh` in that private tree.

## Prerequisites

- Go 1.24+ with CGO enabled
- Rust toolchain (for f4kvs FFI)
- f4kvs-v2 checkout (see below)
- Ollama or an OpenAI-compatible local LLM endpoint

## f4kvs-v2 dependency

Chunk persistence uses [f4kvs-v2](https://git.noematic.eu/f4kvs-org/f4kvs-v2.git), which is **not** bundled in this repository. Clone it separately and set `F4KVS_ROOT` in `.env`:

```bash
cp .env.example .env
# edit F4KVS_ROOT=/path/to/f4kvs-v2
```

Contact [contact@noematic.eu](mailto:contact@noematic.eu) if you need access to the f4kvs-v2 repository.

## Build and test

```bash
make f4kvs tantivy agent
make test
```

## Evaluation smoke test

```bash
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings &
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

See [`eval/README.md`](eval/README.md) for the full eval runbook.
