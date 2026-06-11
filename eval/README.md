# RAG quality evaluation

Reproducible retrieval and generation grading for rag-agent.

## Layout

| Path | Purpose |
|------|---------|
| `fixtures/docs/` | Public sanity corpus (5 markdown files, corpus `eval-public`) |
| `fixtures/domain/` | Domain-style mini corpus (8 files, corpus `eval-domain`) |
| `fixtures/business/` | Business PDF-style fixture with copyright boilerplate (corpus `eval-business`) |
| `gold/public.jsonl` | 16 labeled retrieval cases (CI gate) |
| `gold/domain.jsonl` | 32 labeled retrieval cases |
| `gold/legal.jsonl` | 5 labeled Constitution retrieval cases (corpus `legal-demo`) |
| `gold/multihop.jsonl` | Multi-doc / multi-section retrieval cases for agentic baseline |
| `gold/business.jsonl` | Generation eval for KB business excerpts (copyright noise regression) |
| `out/` | Generated reports (gitignored except `.gitkeep`) |
| `run_ragas.py` | Generation metrics via [RAGAS](https://docs.ragas.io/) |

## Gold JSONL format

One JSON object per line:

```json
{
  "id": "unique-id",
  "corpus": "eval-public",
  "retrieval_q": "keywords for BM25/vector",
  "generation_q": "optional full question for /search",
  "expected_doc_ids": ["doc-..."],
  "expected_chunk_ids": ["doc-...-chunk-0"],
  "expected_sections": ["ARTICLE 16"],
  "match_all_sections": false,
  "excerpt_terms_by_section": {"ARTICLE 16": ["pouvoirs exceptionnels"]},
  "reference_answer": "optional for RAGAS ground_truth",
  "expect_no_results": false
}
```

Document ids match directory ingest: `doc-` + SHA1(relative path). Compute with:

```bash
go run ./client -mode eval-retrieval -gold /dev/null 2>/dev/null || \
  python3 -c "import hashlib; p='rag-overview.md'; print('doc-'+hashlib.sha1(p.encode()).hexdigest())"
```

Or use `StableDocID` from the client package when writing new gold rows.

## Retrieval eval (fast)

Prerequisites: agent running (BM25-only is enough for public set):

```bash
make agent
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings
```

Ingest and score:

```bash
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

Domain corpus:

```bash
./scripts/eval_setup_domain.sh http://127.0.0.1:8080
EVAL_MIN_RECALL=0 ./scripts/eval.sh http://127.0.0.1:8080 eval/gold/domain.jsonl
```

French Constitution (`legal-demo`):

```bash
./scripts/eval_setup_legal.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/legal.jsonl
```

Direct client:

```bash
go run ./client -mode eval-retrieval \
  -server http://127.0.0.1:8080 \
  -gold eval/gold/public.jsonl \
  -eval-top-k 8 \
  -min-recall 0.65 \
  -eval-out eval/out/retrieval_report.json
```

### Metrics

- **Recall@k**: fraction of cases where any expected doc/chunk appears in top-k (`/retrieve`).
- **MRR**: mean reciprocal rank of the first expected hit.
- **Refusal accuracy**: `expect_no_results` cases that return no hits.

### Grading rubric

| Grade | Public Recall@8 | Domain Recall@8 |
|-------|-----------------|-------------------|
| A | ≥ 0.85 | ≥ 0.75 |
| B | ≥ 0.70 | ≥ 0.60 |
| C | below B | below B |

Default CI threshold for public set: **0.65** (`EVAL_MIN_RECALL` in `scripts/eval.sh`) on the **bleve** engine only.

## Excerpt eval (generation context)

Checks that `/retrieve?include_text=1` excerpts contain substantive body text, not heading-only snippets. Gold rows opt in with `excerpt_terms_by_section`:

```bash
go run ./client -mode eval-excerpt \
  -server http://127.0.0.1:8080 \
  -gold eval/gold/legal.jsonl \
  -eval-top-k 8 \
  -min-excerpt-pass 1.0
```

Cases without `excerpt_terms_by_section` are skipped. The `legal-pleins-pouvoirs-election` case asserts Article 16 and Article 7 excerpts include key constitutional phrases (including `reporter` on Art. 7).

## Generation checklist eval

Lightweight phrase checks on live `/search` answers (requires a running agent + Ollama):

```bash
go run ./client -mode eval-generation \
  -server http://127.0.0.1:8080 \
  -gold eval/gold/legal.jsonl \
  -min-generation-pass 1.0
```

Gold rows opt in with `generation_phrases` and `generation_q`.

Optional `-search-mode crag`, `-search-mode agent`, or `-search-mode auto` on `/search` (see [`docs/agentic-rag.md`](../docs/agentic-rag.md)).

Auto-escalation routing report (no pass/fail threshold):

```bash
go run ./client -mode eval-escalation \
  -server http://127.0.0.1:8080 \
  -gold eval/gold/multihop.jsonl
```

## Agentic baseline (pre-CRAG)

Single-pass retrieval on legal + multi-hop gold:

```bash
./scripts/eval_agentic_baseline.sh http://127.0.0.1:8080
```

Requires public + legal corpora ingested (`eval_setup_public.sh`, `eval_setup_legal.sh`).

## Lexical engine matrix

CI runs public retrieval eval for `bleve`, `tantivy`, and `f4kvs` (see `.github/workflows/eval.yml`). Locally:

```bash
make f4kvs tantivy agent
./scripts/compare_lexical_engines.sh
```

Or one engine at a time (separate data dir per run):

```bash
./bin/agent -data-dir /tmp/rag-eval-tantivy -lexical-engine=tantivy -disable-embeddings
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

After each run, confirm `GET /stats` → `manifest.lexical_engine` matches the configured engine. Use `POST /reset` before switching engines on the same data directory.

## Generation eval (RAGAS)

Requires a chat model (OpenAI-compatible or Ollama) and API keys as required by RAGAS/LangChain.

```bash
pip install -r eval/requirements.txt
export OPENAI_API_KEY=...   # or configure RAGAS LLM per their docs
python3 eval/run_ragas.py --server http://127.0.0.1:8080 --gold eval/gold/public.jsonl
```

Release gate suggestion: **faithfulness ≥ 0.8** on domain gold after human review of answers.

## Comparing other OSS RAG tools

1. Ingest the same `eval/fixtures/` trees into the other system.
2. Map their chunk or document ids into `expected_doc_ids` in a copy of the gold file.
3. Run the same RAGAS script against their chat API, or add a thin adapter that returns the same JSON shape as `/search`.

Hold corpus, embedding model, and chat model constant when comparing scores.

## API: `/retrieve`

Retrieval only (no LLM):

```bash
curl --globoff 'http://localhost:8080/retrieve?corpus=eval-public&rq=BM25+hybrid+search&top_k=8'
```

Same tuning parameters as `/search`: `bm25_k`, `vector_k`, `top_k`, `fusion`, `min_score`, `corpus`, `rq` / `retrieval_q`, `q`.
