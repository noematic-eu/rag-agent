# Contributing

## Development model

Feature development happens in the private `noematic-eu/rag-agent` repository. This public repo (`ai-rag-agent`) receives release snapshots via `scripts/sync-public.sh` in that private tree.

## Prerequisites

- Go 1.24+ with CGO enabled (for agent build and full tests)
- Rust toolchain (for f4kvs FFI)
- [golangci-lint](https://golangci-lint.run/) v2+ (for `make lint` / `make check`)
- f4kvs-v2 checkout (for agent and full test suite — see below)
- Ollama or an OpenAI-compatible local LLM endpoint (for runtime eval)

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

## f4kvs-v2 dependency

Chunk persistence uses [f4kvs-v2](https://git.noematic.eu/f4kvs-org/f4kvs-v2.git), which is **not** bundled in this repository. Clone it separately and set `F4KVS_ROOT` in `.env`:

```bash
cp .env.example .env
# edit F4KVS_ROOT=/path/to/f4kvs-v2
```

Contact [contact@noematic.eu](mailto:contact@noematic.eu) if you need access to the f4kvs-v2 repository.

Without f4kvs, you can still run `make check` and work on `./client`, `./lexical`, `./agent/p9fs`, and `./model`. See the README section [Without f4kvs access](../README.md#without-f4kvs-access).

## Makefile targets

| Target | Description |
|--------|-------------|
| `make fmt` | Format Go sources (`gofmt -w`) |
| `make fmt-check` | Fail if any Go file is not formatted |
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

## CI secrets

Public CI runs format checks, lint, and tests that do not require f4kvs. To enable the full build, test suite, and retrieval eval workflow on GitHub Actions, add a repository secret:

- `F4KVS_REPO_TOKEN` — read-only personal access token for `git.noematic.eu` with access to `f4kvs-org/f4kvs-v2`

Without this secret, forks and external contributors still get a green lint/test CI; the f4kvs-gated jobs are skipped.

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
