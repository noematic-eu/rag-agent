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

1. **Lexical (disk mode):** No blocking rebuild on restart. If `lex:meta` is missing or partial, logs a hint; use `retrieval_lex=auto` (default) for scan fallback until the index catches up.
2. **Embeddings:** If `embed:meta` is missing but chunks contain inline `embedding` fields, migrate to `embed:*` automatically.

## Ingest and lexical index

Disk mode indexes `lex:*` **during each `/ingest`** (incremental per chunk). You do not need a long finalize pass after bulk ingest for search to work.

## Admin

- `POST /finalize` — incremental catch-up: index chunks missing from `lex:*` (fast, default).
- `POST /finalize?full=1` — wipe all `lex:*` and rebuild from scratch (slow on large corpora; repair only).
- `DELETE /documents/:doc_id` — removes `chunk:`, `embed:`, and lexical entries for the document.

## Degraded lexical search

When `lex:*` is missing or rebuilding, set `retrieval_lex=auto` (default) on `/search` or `/retrieve`: the agent falls back to an in-memory BM25 scan over `chunk:*` for the requested corpus. This is slower (O(n)) but does not require a finalized index. Nominal path remains `POST /finalize` then `retrieval_lex=index`.

## RAM usage

- Lexical search reads posting lists for query terms only (not full corpus scan).
- Vector search scans compact `embed:*` records (~4 KB per 768-dim vector), not full chunk text.
- Chunk text is loaded from `chunk:*` only for fused top-k results (~8–40 Get calls per query).

## Future (deferred)

- Bounded LRU caches (`RAG_CACHE_MB`)
- ANN index under `vec:*` when brute-force embed scan exceeds latency budget

See also [F4KVS_LEXICAL_DISK_PLAN.md](F4KVS_LEXICAL_DISK_PLAN.md) for lexical index design details.
