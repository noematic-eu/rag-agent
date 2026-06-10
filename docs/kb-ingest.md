# KB catalog ingest (graded)

Ingest a personal markdown library under `~/kb/` into a running RAG agent using
[`catalog.json`](file:///Users/baptisteboussemart/kb/catalog.json) for corpus assignment and
[`index-data.json`](file:///Users/baptisteboussemart/kb/index-data.json) for quality grades.

## Prerequisites

- RAG agent reachable (default `http://127.0.0.1:8081` — see [`docker-compose-quickstart.md`](docker-compose-quickstart.md))
- Ollama models: `qwen2.5:7b-instruct`, `nomic-embed-text`
- Catalog and index generated under `~/kb/`

## Ongoing maintenance loop

When new markdown appears (PDF conversion, manual adds):

```bash
# 1. Sort into topic folders (optional)
python3 ~/kb/kb_organize.py

# 2. Refresh catalog stats (preserves enabled/exclude edits)
python3 scripts/kb_catalog_scan.py --merge

# 3. Validate and review
python3 scripts/kb_catalog_validate.py
python3 scripts/kb_catalog_report.py

# 4. Regenerate grades and INDEX.md
python3 ~/kb/kb_index.py

# 5. Ingest new A/B docs only (skips already-completed doc ids)
python3 scripts/kb_catalog_ingest.py --grades A B --resume
```

Overlapping corpora (e.g. `kb-misc-root` with `*.md` plus `kb-business`) are OK when
assignment specificity differs; validation only errors on true ties. Prefer
`@root/*.md` instead of `*.md` for root-level files — bare `*.md` matches every
path's basename and triggers a validator warning.

Assign new documents to the right corpus by editing `~/kb/catalog.json`:
add or enable a corpus entry with matching `include` globs, then re-run scan and validate.

## Graded ingest (A → B, skip C/D/F)

Quality tiers come from `~/kb/kb_index.py` (grades A–F in `index-data.json`).

**Dry run:**

```bash
python3 scripts/kb_catalog_ingest.py --grades A --dry-run
```

**Wave 1 — grade A (~53 docs):**

```bash
python3 scripts/kb_catalog_ingest.py \
  --server http://127.0.0.1:8081 \
  --grades A \
  --resume
```

**Wave 2 — grade B (~112 docs):**

```bash
python3 scripts/kb_catalog_ingest.py \
  --server http://127.0.0.1:8081 \
  --grades B \
  --resume
```

Both waves together:

```bash
python3 scripts/kb_catalog_ingest.py --grades A B --resume
```

Progress is tracked in `~/kb/.ingest-progress.json` (stable doc ids). Reset it when
starting on a fresh RAG data volume:

```bash
mv ~/kb/.ingest-progress.json ~/kb/.ingest-progress.json.bak.$(date +%Y%m%d)
echo '{"completed_doc_ids":[]}' > ~/kb/.ingest-progress.json
```

## Long runs and stall recovery

Large encyclopedia volumes can take hours. Use the resume wrapper (restarts the
Docker container if ingest stalls):

```bash
GRADES="A" ./scripts/kb_ingest_resume.sh
GRADES="B" ./scripts/kb_ingest_resume.sh
```

Environment variables: `SERVER`, `CATALOG`, `PROGRESS`, `GRADES`, `INDEX_DATA`.

## Verify

```bash
curl -s http://127.0.0.1:8081/stats | python3 -m json.tool
curl -s "http://127.0.0.1:8081/search?q=Metasploit&corpus=kb-books-hacking"
```

After A + B waves, expect roughly **165** documents ingested.

## Encyclopedia articles (split from monoliths)

Monolith markdown lives under `encyclopedias/_sources/` (not ingested). Split
articles live under `encyclopedias/articles/` (see `~/kb/split_encyclopedia.py`).

Re-ingest after a split or re-split:

```bash
python3 scripts/kb_catalog_scan.py --merge
python3 scripts/kb_catalog_validate.py

# Drop legacy monolith doc IDs, then ingest article files only
python3 scripts/kb_catalog_ingest.py \
  --corpus kb-encyclopedias \
  --replace-corpus \
  --resume \
  --no-finalize
```

`--replace-corpus` deletes doc IDs for paths under `_sources/` and flat
`encyclopedias/*.md` before ingesting. `_toc.md` and `meta/` files are excluded.

## Re-ingest after copyright strip (kb-business / PDF books)

The agent strips PDF copyright boilerplate at ingest and derives section titles from
chapter headings in the body. Existing chunks keep polluted `section=` labels until
re-ingested.

Rebuild and restart the agent, then re-ingest the business corpus:

```bash
# Local build (or docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build)
go build -o ./bin/agent ./agent

# Re-ingest entire business shelf (omit --resume and --grades: most business/*.md are grade C)
python3 scripts/kb_catalog_ingest.py \
  --server http://127.0.0.1:8081 \
  --corpus kb-business
```

Graded waves only ingest docs listed as A/B in `~/kb/index-data.json` (often just a handful
in `business/`). Use `--grades A B C` if you want C-tier business books without ingesting D/F.

Smoke test (CRAG generation, French business query):

```bash
curl --globoff -N \
  'http://127.0.0.1:8081/search?corpus=kb-business&q=quel+est+le+meilleur+moyen+de+reussir+dans+le+developpement+logiciel+et+affaires+en+général&crag=1'
```

Expect `section=` labels with chapter titles (not copyright paragraphs) and a KB-specific
system prompt (no Constitution article 16/7 rules).

Eval fixture: `./scripts/eval_setup_business.sh http://127.0.0.1:8081` then
`go run ./client -mode eval-generation -server http://127.0.0.1:8081 -gold eval/gold/business.jsonl -search-mode crag`.

## Key files

| Path | Role |
|------|------|
| `~/kb/catalog.json` | Corpus definitions and glob assignments |
| `~/kb/index-data.json` | Per-doc grade (A–F) |
| `~/kb/INDEX.md` | Human-readable directory index |
| `~/kb/.ingest-progress.json` | Completed doc ids for `--resume` |
| `scripts/kb_catalog_ingest.py` | Main ingest CLI |
| `scripts/kb_ingest_resume.sh` | Stall recovery wrapper |
| `scripts/ingest_from_catalog.sh` | Validate + full-catalog ingest |
