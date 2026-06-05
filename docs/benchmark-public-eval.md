# Benchmark report: rag-agent retrieval on public eval set

## Why this benchmark exists
This report gives prospects a concrete quality signal before pilot kickoff. It measures retrieval quality on a fixed public corpus and gold questions.

## Test setup
- Engine: `bleve`
- Embeddings: disabled (`-disable-embeddings`) for a BM25-only baseline
- Server: `http://127.0.0.1:8080`
- Corpus: `eval/fixtures/docs`
- Gold set: `eval/gold/public.jsonl` (16 cases)

## Commands used

```bash
make f4kvs agent
./bin/agent -addr 127.0.0.1:8080 -data-dir /tmp/rag-eval -disable-embeddings
./scripts/eval_setup_public.sh http://127.0.0.1:8080
./scripts/eval.sh http://127.0.0.1:8080 eval/gold/public.jsonl
```

## Results summary
- Recall@8: **1.000**
- MRR: **0.875**
- Refusal accuracy: **1.000**

Interpretation:
- Every query with expected matches retrieved at least one relevant hit in top-8.
- The first relevant hit is frequently in top positions (strong MRR).
- No-result cases were correctly refused in this run.

## Case-level highlights
- Hits with reciprocal rank 1.0 include:
  - `pub-rag-pipeline`
  - `pub-hybrid-definition`
  - `pub-bm25-tfidf`
  - `pub-rrf-fusion`
  - `pub-fusion-weight`
- Correct no-result cases:
  - `pub-out-of-corpus`
  - `pub-nonsense-year`

## Sales-friendly takeaway
Even on BM25-only mode, rag-agent achieves strong retrieval reliability on the public gold set. This gives buyers a measurable baseline before enabling embeddings and domain-specific tuning in a pilot.

## Next benchmark steps
1. Re-run with embeddings enabled and compare Recall/MRR deltas.
2. Run the domain corpus benchmark (`eval/gold/domain.jsonl`).
3. Add generation scoring (RAGAS) for pilot-ready quality reporting.
