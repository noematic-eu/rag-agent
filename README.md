# ai-rag-agent

A lightweight Retrieval-Augmented Generation (RAG) service in Go with:

- HTTP API (`/ingest`, `/finalize`, `/search`) and optional 9P file tree ([plan9port](https://github.com/9fans/plan9port))
- Markdown-first chunking pipeline
- HTML ingestion support via HTML -> Markdown normalization
- Hybrid retrieval (BM25 + vector embeddings)
- Chunk persistence in f4kvs (embedded via cgo) and pluggable lexical indexing (Bleve, Tantivy, or in-memory BM25 over chunks)

## Table of contents

- [Clone](#clone)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [f4kvs FFI](#f4kvs-ffi)
- [Build](#build)
- [Quick start for evaluators](#quick-start-for-evaluators)
- [Release artifacts](#release-artifacts)
- [Docker Compose](#docker-compose)
- [Run the API server](#run-the-api-server)
- [Ingest API Contract](#ingest-api-contract)
- [Directory Ingestion CLI](#directory-ingestion-cli)
- [Search](#search)
- [Development](#development)
- [Storage maintenance](#storage-maintenance)
- [Migrating from Badger](#migrating-from-badger)
- [Current Notes](#current-notes)

Further reading: [`CONTRIBUTING.md`](CONTRIBUTING.md) (contributor setup, CI) · [`docs/README.md`](docs/README.md) (guides index)

## Clone

```bash
git clone https://github.com/noematic-eu/ai-rag-agent.git
cd ai-rag-agent
```

## Architecture

- `agent/`
  - API server and ingestion/search pipeline
- `client/`
  - CLI helper to ingest directories and run searches
- `model/`
  - shared data models

Storage and retrieval flow:

1. Documents are sent to `/ingest`
2. If `content_type=html`, content is normalized to Markdown
3. Content is chunked and embedded
4. Chunks are indexed in the configured lexical engine and stored in f4kvs (`legal.f4kvs/`)
5. `/finalize` computes IDF statistics
6. `/search` runs hybrid retrieval and streams an answer

## Prerequisites

- Go `1.24+` with CGO enabled
- Rust toolchain (to build f4kvs FFI from `~/dev/rust/f4kvs-ffi`; remote: `https://github.com/noematic-eu/f4kvs-ffi.git`)
- An LLM backend:
  - **Ollama** (default): `http://localhost:11434`
  - **LM Studio** (OpenAI-compatible): enable the local server, typically `http://localhost:1234/v1`

## f4kvs FFI

Chunk storage requires the **f4kvs-ffi** Rust library, hosted at [github.com/noematic-eu/f4kvs-ffi](https://github.com/noematic-eu/f4kvs-ffi). Clone it locally, set `F4KVS_ROOT` in `.env`, then run `make f4kvs`:

```bash
git clone https://github.com/noematic-eu/f4kvs-ffi.git ~/dev/rust/f4kvs-ffi
cp .env.example .env
# edit F4KVS_ROOT=/path/to/f4kvs-ffi
make f4kvs
```

Once the f4kvs FFI library is built, Bleve and Tantivy lexical engines work as documented below.

### Without building f4kvs

You can work on much of the codebase without building the native library:

- **Build and test** (no CGO): `./client`, `./lexical`, `./agent/p9fs`, `./model`
- **Quality checks**: `make check` (format, vet, lint, and `test-lite` — no f4kvs required)

The API server and full test suite require f4kvs: run `make f4kvs` before `./bin/agent` or `make test`.

## Build

Build the f4kvs FFI library and Go binaries:

```bash
make f4kvs    # builds libf4kvs_ffi into ./lib
make tantivy  # builds libtantivy_go into ./lib (from github.com/anyproto/tantivy-go)
make agent    # builds ./bin/agent (CGO + -tags tantivy)
```

Override the f4kvs source tree if needed:

```bash
make f4kvs F4KVS_ROOT=/path/to/f4kvs-ffi
```

Build the CLI client only (no f4kvs, no CGO):

```bash
make client
# or: go build -o bin/client ./client
```

## Quick start for evaluators

This path is optimized for a first technical evaluation in less than 15 minutes.

```bash
make f4kvs tantivy agent
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings
```

In another terminal:

```bash
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

If you prefer the full runbook and grading rubric, see [`eval/README.md`](eval/README.md).

## Release artifacts

For reproducible macOS and Linux builds:

```bash
chmod +x ./scripts/release/build_binaries.sh
./scripts/release/build_binaries.sh v0.1.0
```

Release checklist and artifact list: [`docs/releases/v0.1.0-checklist.md`](docs/releases/v0.1.0-checklist.md).

## Docker (pre-built image)

Published multi-arch images: `ghcr.io/noematic-eu/rag-agent` (`linux/amd64`, `linux/arm64`). Images are pushed to GHCR when a [GitHub release](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository) is published (not on every push to `main`).

**One-liner** (Ollama on the host):

```bash
docker run -d -p 8080:8080 \
  -v ./rag-data:/data \
  -e RAG_LLM_BASE_URL=http://host.docker.internal:11434 \
  --add-host=host.docker.internal:host-gateway \
  --name rag-agent \
  ghcr.io/noematic-eu/rag-agent:latest
```

**Docker Compose** (pulls the image; no local Rust/Go build):

```bash
docker compose pull
docker compose up -d
```

Quickstart, seeding, and smoke-test commands: [`docs/docker-compose-quickstart.md`](docs/docker-compose-quickstart.md).

To build from source instead of pulling, use `docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build` (requires `F4KVS_ROOT` in `.env`).

To populate the agent with the Constitution (`legal-demo`) and eval fixtures (`eval-public`), follow §4 of the quickstart.

## Run the API server

Requires a built agent (see [Build](#build)). From repository root with Ollama defaults:

```bash
go run ./agent
```

With LM Studio:

```bash
go run ./agent \
  -llm-provider=openai \
  -llm-base-url=http://localhost:1234/v1 \
  -llm-model=your-loaded-chat-model \
  -embedding-model=your-loaded-embedding-model
```

Run without embeddings (keyword/BM25 only):

```bash
go run ./agent -disable-embeddings
```

Equivalent environment variables:

- `RAG_LLM_PROVIDER` — `ollama` (default) or `openai`
- `RAG_LLM_BASE_URL` — e.g. `http://localhost:1234/v1`
- `RAG_LLM_MODEL` — chat/completion model name
- `RAG_EMBEDDING_MODEL` — embedding model name
- `RAG_DISABLE_EMBEDDINGS` — `true|false` (disable embedding generation and vector search)
- `RAG_LISTEN` — HTTP listen address (default: `:8080`; set to `off` to disable HTTP)
- `RAG_9P_ADDR` — 9P listen address (e.g. `unix!/tmp/rag9p`, `tcp!127.0.0.1!5640`)
- `RAG_DATA_DIR` — directory for chunk store and lexical indexes (default: current working directory)
- `RAG_LEXICAL_ENGINE` — `bleve` (default), `tantivy`, or `f4kvs` (in-memory BM25 over `chunk:*`)

CLI flags (override env when set):

- `-addr` — same as `RAG_LISTEN`
- `-9p-addr` — same as `RAG_9P_ADDR`
- `-data-dir` — same as `RAG_DATA_DIR`
- `-lexical-engine` — same as `RAG_LEXICAL_ENGINE`

Server logs data directory, lexical engine, HTTP/9P addresses, and LLM configuration at startup.

### Plan 9 / plan9port (9P file tree)

The agent can expose the same RAG operations as a 9P2000 file tree for [plan9port](https://github.com/9fans/plan9port) userspace. The Go/CGO binary still runs on macOS/Linux; plan9port is the client (`9p`, `9pfuse`, rc).

#### Prerequisites

- [plan9port](https://github.com/9fans/plan9port) installed (`9`, `9p` in `PATH`)
- **Optional:** [macFUSE](https://macfuse.io/) only if you want `9pfuse` (directory mount). Without macFUSE, use the `9p` CLI — it works the same API.

#### Start the agent

9P only (no HTTP):

```bash
./bin/agent -9p-addr 'unix!/tmp/rag9p' -addr off -data-dir ~/rag-data
```

HTTP and 9P together:

```bash
./bin/agent -9p-addr 'unix!/tmp/rag9p'
```

TCP instead of a Unix socket:

```bash
./bin/agent -9p-addr 'tcp!127.0.0.1!5640' -addr off -data-dir ~/rag-data
# clients: 9p -a 'tcp!127.0.0.1!5640' read stats
```

**zsh:** always quote addresses containing `!` (history expansion), e.g. `'unix!/tmp/rag9p'`.

#### Two ways to access the file tree

| Tool | Address flag | When to use |
|------|--------------|-------------|
| `9p` | `-a 'unix!/tmp/rag9p'` | No FUSE; pipe stdin to `write` |
| `9pfuse` | positional 1st arg (not `-a`) | Mount `~/ragmnt` as a normal directory |

`9pfuse` on macOS:

```bash
9pfuse '/tmp/rag9p' ~/ragmnt
# or: 9pfuse 'unix!/tmp/rag9p' ~/ragmnt
```

If you see `mountfuse: cannot find load_fusefs`, install macFUSE or fall back to `9p` below.

`9p` CLI (no mount):

```bash
9p -a 'unix!/tmp/rag9p' read stats
9p -a 'unix!/tmp/rag9p' ls -l /
```

**Important:** `9p write` reads the payload from **stdin**, not from extra arguments:

```bash
echo 'What is article 1?' | 9p -a 'unix!/tmp/rag9p' write search/ctl
9p -a 'unix!/tmp/rag9p' read search/data
```

#### File tree

```
/
  README          usage (read)
  stats           JSON manifest + ingest counters (read)
  ctl             write: reset | finalize | status
  ingest          write JSON document; read last result
  search/
    ctl           write question (generation prompt)
    params        read/write retrieval options (key=value lines)
    data          read answer text (after ctl write)
    metadata      read JSON {prompt, model, provider, lang}
  retrieve/
    ctl           write retrieval query
    params        same option format as search/params
    data          read RetrieveResponse JSON
  documents/
    <doc_id>      create file, then remove to delete (needs mount or 9p remove)
```

Mounted path example: `~/ragmnt/search/ctl`. With `9p`, paths are relative to the server root: `search/ctl`.

#### Common operations

**Ingest** (same JSON as `POST /ingest`):

```bash
echo '{"id":"doc1","title":"T","content":"...","content_type":"markdown","corpus":"legal"}' \
  | 9p -a 'unix!/tmp/rag9p' write ingest
9p -a 'unix!/tmp/rag9p' read ingest    # optional: last result
```

**Search** (retrieve + LLM answer, buffered on 9P):

```bash
echo 'What is article 1?' | 9p -a 'unix!/tmp/rag9p' write search/ctl
9p -a 'unix!/tmp/rag9p' read search/data
9p -a 'unix!/tmp/rag9p' read search/metadata
```

**Retrieve only** (no LLM):

```bash
echo 'article 1 constitution' | 9p -a 'unix!/tmp/rag9p' write retrieve/ctl
9p -a 'unix!/tmp/rag9p' read retrieve/data
```

**Retrieval params** (`search/params` or `retrieve/params`), one `key=value` per line:

```
corpus=legal
top_k=8
bm25_k=20
vector_k=20
fusion=0.6
fusion=rrf
min_score=0.2
lang=fr
rq=shorter BM25 query
doc_id=doc-123
```

Example with corpus scope:

```bash
printf 'corpus=legal\ntop_k=6\n' | 9p -a 'unix!/tmp/rag9p' write search/params
echo 'What is article 1?' | 9p -a 'unix!/tmp/rag9p' write search/ctl
9p -a 'unix!/tmp/rag9p' read search/data
```

**Admin:**

```bash
echo reset | 9p -a 'unix!/tmp/rag9p' write ctl      # wipe indexes + chunk store
echo finalize | 9p -a 'unix!/tmp/rag9p' write ctl   # legacy IDF finalize
9p -a 'unix!/tmp/rag9p' read ctl                      # status line
```

**Delete document** (requires a mount — `9p` has no remove subcommand):

```bash
# with 9pfuse mount at ~/ragmnt:
touch ~/ragmnt/documents/doc1
rm ~/ragmnt/documents/doc1
```

#### Helper scripts

With plan9port in `PATH`:

```bash
export RAG_9P_ADDR='unix!/tmp/rag9p'
./scripts/plan9/rag-search 'What is article 1?'
./scripts/plan9/rag-retrieve 'article 1 constitution'
```

#### Multiple 9P instances

Same pattern as HTTP — one `-data-dir` and one socket per shelf:

```bash
./bin/agent -9p-addr 'unix!/tmp/rag-legal'  -addr off -data-dir ~/.rag-agents/legal
./bin/agent -9p-addr 'unix!/tmp/rag-domain' -addr off -data-dir ~/.rag-agents/domain
```

### Lexical engine

Keyword/BM25 retrieval uses one backend at a time. Chunks always live in f4kvs; the lexical engine only affects how text is indexed and searched.

| Engine | On-disk path | Notes |
|--------|--------------|-------|
| `bleve` (default) | `legal.bleve/` | Production default |
| `tantivy` | `legal.tantivy/` | Requires `make tantivy` before `make agent` |
| `f4kvs` | (none extra) | In-memory BM25; rebuilds from `chunk:*` on startup (~22k-chunk benchmark mode) |

Switching engines requires `POST /reset` (or a fresh `-data-dir`) and full re-ingest. Indexes are not portable across engines.

Compare engines on the public gold set:

```bash
make compare-lexical
# or: ./scripts/compare_lexical_engines.sh
```

`GET /stats` includes `manifest.lexical_engine` for reproducibility.

### Multiple instances (separate indexes)

Run one process per shelf with its own port and data directory:

```bash
./bin/agent -addr 127.0.0.1:8081 -data-dir ~/.rag-agents/summaries -disable-embeddings ...
./bin/agent -addr 127.0.0.1:8082 -data-dir ~/.rag-agents/encyclopedia -disable-embeddings ...
```

Point the client or router at the matching base URL, e.g. `http://127.0.0.1:8081`.

## Ingest API Contract

`POST /ingest` accepts:

```json
{
  "id": "doc-123",
  "title": "My document",
  "content": "<h1>Hello</h1>",
  "content_type": "html",
  "original_content": "<h1>Hello</h1>"
}
```

Fields:

- `id`: stable unique document id
- `title`: document title
- `content`: document payload
- `content_type`: `markdown` or `html` (defaults to `markdown`)
- `original_content`: optional raw payload for provenance

Notes:

- For `html`, the backend converts `content` into canonical Markdown before chunking.
- Original HTML is preserved in stored chunk payloads.

## Directory Ingestion CLI

The client now supports folder ingestion for `.md`, `.markdown`, `.html`, `.htm` files.

### Usage

```bash
go run ./client -mode ingest-dir -dir ./docs -server http://localhost:8080
```

Useful flags:

- `-mode ingest-dir|demo` (default: `ingest-dir`)
- `-dir <path>` source directory (required for `ingest-dir`)
- `-server <url>` API base URL (default: `http://localhost:8080`)
- `-finalize true|false` call `/finalize` after ingest (default: `true`)
- `-batch-size <n>` progress logging interval (default: `100`)
- `-reset-before-ingest` wipe Bleve + f4kvs via `POST /reset` before ingesting
- `-corpus <tag>` optional corpus label for scoped search (`/search?corpus=...`)

For a library of book summaries, ingest **one corpus per book** (or per folder) so search can scope results:

```bash
for d in ./docs/*/; do
  go run ./client -mode ingest-dir -dir "$d" -corpus "$(basename "$d")" -finalize
done
```

Behavior:

- Recursively scans the directory
- Auto-detects content type from extension
- Uses stable doc IDs from relative path hash
- Sends one `/ingest` request per file
- Calls `/finalize` once at the end (if enabled)

## Search

You can query the API directly:

```bash
curl "http://localhost:8080/search?q=your+question"
```

Separate **retrieval** from **generation** when `q` includes long instructions (otherwise BM25 matches instruction words like "ideas" or "excerpts"):

```bash
# Use --globoff: curl treats [n] in URLs as a range unless disabled
curl --globoff 'http://localhost:8080/search?corpus=mylibrary&rq=Marcus+Aurelius+virtue+nature&top_k=6&q=From+excerpts+only:+3+teachings+with+[n]+citations'
```

`retrieval_q` (alias `rq`) is used for Bleve/vector search; `q` is the full question sent to the LLM.

The endpoint supports optional parameters:

- `bm25_k` (default: 20)
- `vector_k` (default: 20)
- `top_k` (default: 8) — number of ranked chunks returned and sent to the LLM
- `max_per_doc` (default: 6 when `corpus` is set, else 1) — max chunks per parent document after fusion. On a single-document corpus (e.g. `legal-demo`), set **`max_per_doc` ≥ `top_k`** or some articles will be dropped
- `retrieval_q` / `rq` — BM25/vector query (optional; `q` used if omitted; long instructional `q` may be stripped heuristically for retrieval)
- `fusion` — weighted score in `0..1` (default: `0.6`) or `rrf` for reciprocal rank fusion
- `min_score` — minimum hybrid score before calling the LLM (default: `0.2`)
- `corpus` — filter results to a corpus tag set at ingest time
- `article` — filter hits to a legal article number (e.g. `article=7`)
- `legal_rerank` — `1`/`0` to enable/disable post-retrieval legal article reranking (auto-enabled for `legal-demo` and article-indexed chunks)
- `lang` — force answer language (`en` or `fr`)

Excerpts sent to the LLM use `book=<filename>` and `section=<heading>` labels when indexed after a re-ingest with the current agent.

### Re-index after search-quality updates

Required once after upgrading indexing (adds `doc_title` to Bleve and chunk metadata):

```bash
make agent
curl -X POST http://localhost:8080/reset
go run ./client -mode ingest-dir -dir ./docs -corpus mylibrary -finalize
```

Reset the index (also required after embedding model change or corrupted DB):

```bash
curl -X POST http://localhost:8080/reset
```

Benchmark search latency (10 fixed queries, no gold labels):

```bash
./scripts/bench_search.sh http://localhost:8080
```

### Retrieval-only API

`GET /retrieve` returns ranked chunks without calling the LLM (same params as `/search` for `rq`, `corpus`, `top_k`, `fusion`, etc.):

```bash
curl --globoff 'http://localhost:8080/retrieve?corpus=mylibrary&rq=Marcus+Aurelius&top_k=8'
```

### RAG quality evaluation

Graded retrieval eval uses gold Q&A in [`eval/gold/`](eval/gold/) and fixture corpora under [`eval/fixtures/`](eval/fixtures/). See [`eval/README.md`](eval/README.md) for the full runbook, grading rubric (A/B/C), and RAGAS generation scoring.

Quick start (BM25-only smoke test):

```bash
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

Or `make eval-public EVAL_SERVER=http://127.0.0.1:8080` after the agent is up.

Get ingestion metrics and compatibility manifest:

```bash
curl "http://localhost:8080/stats"
```

`/stats` returns:
- persisted manifest (`embedding_model`, `chunking_version`, `pipeline_version`)
- embedding mode state (`embeddings_enabled`)
- global ingestion counters (`documents_total`, `chunks_total`, `embedded_chunks`, `embedding_failures`)
- per-corpus counters (`by_corpus`)
- runtime compatibility warnings if current config differs from indexed manifest

## Development

Quick quality gate (no f4kvs required):

```bash
make check    # fmt-check + vet + lint + test-lite
make fmt      # format all Go packages
```

Full test suite (requires f4kvs in `./lib`):

```bash
make f4kvs && make test
```

Install [golangci-lint](https://golangci-lint.run/) v2+ locally for `make lint` / `make check`:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

Contributor setup, Makefile targets, and CI secrets: [`CONTRIBUTING.md`](CONTRIBUTING.md). Guides index: [`docs/README.md`](docs/README.md).

## Storage maintenance

- Under `-data-dir` (default `.`): `legal.bleve/` (BM25 index) and `legal.f4kvs/` (chunk payloads)
- Optional compaction (replaces Badger `Flatten`): `RAG_F4KVS_COMPACT=1` (legacy alias: `RAG_BADGER_REPAIR=1`)

## Migrating from Badger

The chunk store moved from Badger (`legal.db/`) to f4kvs (`legal.f4kvs/`). For a fresh deployment, call `POST /reset` and re-ingest. To preserve data from an existing Badger database, re-ingest from source documents (recommended) or copy keys manually with a one-off tool that reads Badger and writes to f4kvs using the same `chunk:{id}` key schema.

## Current Notes

- HTML is normalized at ingest boundary to ensure a single canonical chunking path.
- Existing Markdown ingestion remains unchanged.
- Chunk metadata retains source type (`markdown`/`html`) and preserved original payload for traceability.
