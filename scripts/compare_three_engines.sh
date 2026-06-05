#!/usr/bin/env bash
# Compare retrieval + optional /search across bleve:tantivy:f4kvs agents.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="${1:-eval/out/engine_compare_$(date +%Y%m%d_%H%M%S).json}"
mkdir -p "$(dirname "$OUT")"
RUN_SEARCH="${RUN_SEARCH:-1}"
python3 - "$OUT" "$RUN_SEARCH" <<'PY'
import json, sys, urllib.parse, urllib.request, re

OUT, RUN_SEARCH = sys.argv[1], sys.argv[2] == "1"
PORTS = [(8080, "bleve"), (8081, "tantivy"), (8082, "f4kvs")]
CORPUS = "mylibrary"

RETRIEVAL_QUERIES = [
    # encyclopedia / philosophy (8080-heavy)
    ("Marcus Aurelius virtue nature", "encyclopedia"),
    ("Stoicism meditations emperor", "encyclopedia"),
    ("Plato republic justice", "encyclopedia"),
    ("Aristotle ethics politics", "encyclopedia"),
    ("Kant categorical imperative", "encyclopedia"),
    ("French Revolution causes", "encyclopedia"),
    ("supply and demand economics", "encyclopedia"),
    ("DNA replication genetics", "encyclopedia"),
    # business shelf (8082 ingest)
    ("Epictetus dichotomy of control", "business"),
    ("good to great hedgehog", "business"),
    ("thinking fast slow system 1", "business"),
    ("100 million offers hormozi", "business"),
    ("pyramid principle minto", "business"),
    ("building second brain PARA", "business"),
    ("start with why golden circle", "business"),
    ("critical thinking fallacy logic", "business"),
    # ambiguous / cross
    ("stoicism practice virtue", "both"),
    ("leadership trust culture", "both"),
    ("artificial intelligence future", "both"),
]

SEARCH_PROMPTS = [
    ("From excerpts only: 3 Stoic teachings with citations", "Marcus Aurelius virtue", "encyclopedia"),
    ("What is the hedgehog concept? Cite sources.", "good to great hedgehog", "business"),
    ("Summarize Epictetus on control vs indifference.", "Epictetus dichotomy control", "business"),
    ("Explain thinking fast vs slow in one paragraph.", "thinking fast slow", "business"),
]

def get(url, timeout=20):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return json.load(r), None
    except Exception as e:
        return None, str(e)

def retrieve(port, rq, top_k=5):
    q = urllib.parse.urlencode({"corpus": CORPUS, "rq": rq, "top_k": top_k, "bm25_k": 30})
    return get(f"http://127.0.0.1:{port}/retrieve?{q}")

def search(port, q, rq, timeout=90):
    params = {"corpus": CORPUS, "q": q, "rq": rq, "top_k": "6", "bm25_k": "25"}
    url = f"http://127.0.0.1:{port}/search?" + urllib.parse.urlencode(params)
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            body = r.read().decode("utf-8", errors="replace")
            # SSE or JSON
            if body.strip().startswith("{"):
                return json.loads(body), None
            # extract last data line or full text
            lines = [ln for ln in body.splitlines() if ln.startswith("data:")]
            text = ""
            if lines:
                last = lines[-1][5:].strip()
                try:
                    obj = json.loads(last)
                    text = obj.get("content") or obj.get("answer") or str(obj)[:800]
                except json.JSONDecodeError:
                    text = last[:800]
            else:
                text = body[:800]
            return {"status": "ok", "answer_preview": text}, None
    except Exception as e:
        return None, str(e)

report = {"ports": {}, "retrieval": [], "search": [], "warnings": []}

for port, engine in PORTS:
    data, err = get(f"http://127.0.0.1:{port}/stats", timeout=5)
    if err:
        report["ports"][engine] = {"port": port, "error": err}
        report["warnings"].append(f":{port} {engine} unreachable")
        continue
    m, i = data.get("manifest", {}), data.get("ingest", {})
    report["ports"][engine] = {
        "port": port,
        "lexical_engine": m.get("lexical_engine") or ("bleve" if engine == "bleve" else engine),
        "documents_total": i.get("documents_total", 0),
        "chunks_total": i.get("chunks_total", 0),
    }

if report["ports"].get("bleve", {}).get("chunks_total") != report["ports"].get("f4kvs", {}).get("chunks_total"):
    report["warnings"].append(
        "8080 (bleve) and 8082 (f4kvs) have different chunk counts — not the same ingest; engine comparison is indicative only."
    )
if report["ports"].get("tantivy", {}).get("chunks_total", 0) == 0:
    report["warnings"].append(
        ":8081 tantivy index is empty — ingest into encyclopedia.tantivy (e.g. same dir as 8082) before tantivy comparisons."
    )

for rq, tag in RETRIEVAL_QUERIES:
    row = {"rq": rq, "tag": tag, "engines": {}}
    for port, engine in PORTS:
        data, err = retrieve(port, rq)
        if err:
            row["engines"][engine] = {"status": "error", "error": err}
            continue
        hits = data.get("hits") or []
        row["engines"][engine] = {
            "status": data.get("status", "ok"),
            "n": len(hits),
            "top": [
                {
                    "doc_title": h.get("doc_title"),
                    "section": (h.get("section") or "")[:60],
                    "score": round(float(h.get("score", 0)), 4),
                }
                for h in hits[:3]
            ],
        }
    report["retrieval"].append(row)

if RUN_SEARCH:
    for q, rq, tag in SEARCH_PROMPTS:
        row = {"q": q, "rq": rq, "tag": tag, "engines": {}}
        for port, engine in PORTS:
            data, err = search(port, q, rq)
            if err:
                row["engines"][engine] = {"status": "error", "error": err[:200]}
            else:
                prev = (data.get("answer_preview") or data.get("message") or str(data))[:500]
                row["engines"][engine] = {"status": data.get("status", "ok"), "preview": prev}
        report["search"].append(row)

with open(OUT, "w") as f:
    json.dump(report, f, indent=2, ensure_ascii=False)
print(OUT)
PY
echo "Wrote $OUT"
