# Disk-first f4kvs architecture (Option 4)

All durable RAG state lives in a single `legal.f4kvs/` LSM directory. RAM holds only query-time buffers, top-k heaps, and optional bounded caches.

## Key layout

| Prefix | Content | Purpose |
|--------|---------|---------|
| `chunk:{id}` | JSON: text + metadata | Chunk text for generation and rerank |
| `embed:{id}` | JSON: vector + corpus/doc_id | Vector search (brute-force scan) |
| `embed:meta` | JSON: count, dim, model | Embed store metadata |
| `lex:meta` | JSON: N, avg_dl, chunk_count | BM25 global stats |
| `lex:df:{term}` | uint32 BE | Document frequency |
| `lex:post:{term}` | posting list binary | Inverted index |
| `lex:chunk:{id}` | JSON: field lengths | Scoring metadata |
| `lex:terms:{id}` | JSON: term array | Delete support |
| `doc:{id}` | document metadata | Existing doc registry |

## Configuration

| Env | Default | Description |
|-----|---------|-------------|
| `RAG_LEXICAL_ENGINE` | `bleve` | Set to `f4kvs` for single-store mode |
| `RAG_F4KVS_LEXICAL_MODE` | `disk` | `disk` (persistent `lex:*`) or `ram` (legacy in-process BM25) |

## Startup behavior

1. **Lexical (disk mode):** If `lex:meta` is missing, rebuild once from `chunk:*` (logged, no re-ingest required).
2. **Embeddings:** If `embed:meta` is missing but chunks contain inline `embedding` fields, migrate to `embed:*` automatically.

## Admin

- `POST /finalize` — rebuild `lex:*` from all chunks (repair or after bulk ingest).
- `DELETE /documents/:doc_id` — removes `chunk:`, `embed:`, and lexical entries for the document.

## RAM usage

- Lexical search reads posting lists for query terms only (not full corpus scan).
- Vector search scans compact `embed:*` records (~4 KB per 768-dim vector), not full chunk text.
- Chunk text is loaded from `chunk:*` only for fused top-k results (~8–40 Get calls per query).

## Future (deferred)

- Bounded LRU caches (`RAG_CACHE_MB`)
- ANN index under `vec:*` when brute-force embed scan exceeds latency budget

See also [F4KVS_LEXICAL_DISK_PLAN.md](F4KVS_LEXICAL_DISK_PLAN.md) for lexical index design details.
