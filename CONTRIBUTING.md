# Contributing

## Development model

Feature development happens in the private `noematic-eu/rag-agent` repository. This public repo (`ai-rag-agent`) receives release snapshots via `scripts/sync-public.sh` in that private tree.

## Prerequisites

- Go 1.24+ with CGO enabled (for agent build and full tests)
- Rust toolchain (for f4kvs FFI)
- [golangci-lint](https://golangci-lint.run/) v2+ (for `make lint` / `make check`)
- f4kvs-ffi checkout (for agent and full test suite — see below)
- Ollama or an OpenAI-compatible local LLM endpoint (for runtime eval)

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

## f4kvs-ffi dependency

Chunk persistence uses [f4kvs-ffi](https://github.com/noematic-eu/f4kvs-ffi), which is **not** bundled in this repository. Clone it separately and set `F4KVS_ROOT` in `.env`:

```bash
git clone https://github.com/noematic-eu/f4kvs-ffi.git ~/dev/rust/f4kvs-ffi
cp .env.example .env
# edit F4KVS_ROOT=/path/to/f4kvs-ffi
```

Without building f4kvs, you can still run `make check` and work on `./client`, `./lexical`, `./agent/p9fs`, and `./model`. See the README section [Without building f4kvs](../README.md#without-building-f4kvs).

## Git hooks

Install the repository pre-commit hook once per clone:

```bash
make install-hooks
```

The hook blocks commits that include unformatted Go files under `agent/`, `client/`, `lexical/`, `model/`, or `internal/`, and runs `golangci-lint run ./...` (same as CI). Run `make fmt` to fix formatting and `make lint` to reproduce lint failures locally.

## Makefile targets

| Target | Description |
|--------|-------------|
| `make fmt` | Format Go sources (`gofmt -w`) |
| `make fmt-check` | Fail if any Go file is not formatted |
| `make install-hooks` | Enable `.githooks/pre-commit` for this clone |
| `make vet` | `go vet` on packages that build without f4kvs |
| `make lint` | Run golangci-lint on `./...` |
| `make test-lite` | Test `./client`, `./lexical`, `./agent/p9fs`, `./model` |
| `make check` | `fmt-check` + `vet` + `lint` + `test-lite` |
| `make f4kvs` | Build f4kvs FFI into `./lib` |
| `make tantivy` | Build Tantivy native lib into `./lib` |
| `make build` | Full build (`f4kvs` + all packages with `-tags tantivy`) |
| `make test` | Full test suite (requires f4kvs) |
| `make agent` | Build `./bin/agent` |
| `make client` | Build `./bin/client` (no CGO) |
| `make bake-eval-public` | Offline bake `eval/fixtures/docs` → `prebuilt/eval-public/` |
| `make bake-legal-demo` | Offline bake `texts/` → `prebuilt/legal-demo/` |
| `make docker-corpus-build` | Build corpus Docker image via compose overrides |
| `make test-docker-entrypoint` | Smoke-test corpus entrypoint seed/copy logic |

## CI

GitHub Actions runs format checks, lint, lightweight tests, full build/test (with f4kvs-ffi cloned from public GitHub), and retrieval eval on every push and pull request. Docker images are published to GHCR only when a GitHub release is published: base agent (`docker-publish`) and corpus-bundled variants (`docker-corpus-publish`). No repository secrets are required for f4kvs.

## Build and test

```bash
make check              # no f4kvs required
make f4kvs tantivy agent
make test               # full suite
```

API reference, runtime configuration, and environment variables: [`README.md`](../README.md).

## Evaluation smoke test

Minimal retrieval eval (requires built agent):

```bash
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings &
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

Step-by-step evaluator path: [README — Quick start for evaluators](../README.md#quick-start-for-evaluators). Full runbook and grading rubric: [`eval/README.md`](eval/README.md).

More guides: [`docs/README.md`](docs/README.md).
