#!/usr/bin/env python3
"""Run RAGAS faithfulness and answer_relevancy on rag-agent /search answers."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path


def load_gold(path: Path) -> list[dict]:
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        rows.append(json.loads(line))
    return rows


def parse_sse(body: str) -> tuple[str, list[str]]:
    answer_parts: list[str] = []
    contexts: list[str] = []
    event = ""
    for line in body.splitlines():
        if line.startswith("event:"):
            event = line.split(":", 1)[1].strip()
            continue
        if not line.startswith("data:"):
            continue
        payload = line.split(":", 1)[1].strip()
        if event == "message":
            try:
                answer_parts.append(json.loads(payload))
            except json.JSONDecodeError:
                answer_parts.append(payload)
        elif event == "metadata":
            try:
                meta = json.loads(payload)
                prompt = meta.get("prompt", "")
                contexts = extract_contexts(prompt)
            except json.JSONDecodeError:
                pass
        elif event == "complete":
            try:
                data = json.loads(payload)
                if isinstance(data.get("response"), str):
                    answer_parts = [data["response"]]
            except json.JSONDecodeError:
                pass
    return "".join(answer_parts), contexts


def extract_contexts(prompt: str) -> list[str]:
    blocks = re.split(r"\n---+\n", prompt)
    out = []
    for block in blocks:
        block = block.strip()
        if block.startswith("[") and "book=" in block:
            out.append(block)
    return out


def search(server: str, question: str, corpus: str | None, retrieval_q: str | None) -> tuple[str, list[str], str]:
    params = {"q": question}
    if corpus:
        params["corpus"] = corpus
    if retrieval_q:
        params["rq"] = retrieval_q
    url = server.rstrip("/") + "/search?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=300) as resp:
        body = resp.read().decode("utf-8", errors="replace")
    if '"status":"no_results"' in body.replace(" ", ""):
        return "", [], "no_results"
    answer, contexts = parse_sse(body)
    return answer, contexts, "ok"


def main() -> int:
    parser = argparse.ArgumentParser(description="RAGAS generation eval for rag-agent")
    parser.add_argument("--server", default=os.environ.get("RAG_EVAL_SERVER", "http://localhost:8080"))
    parser.add_argument("--gold", type=Path, default=Path("eval/gold/public.jsonl"))
    parser.add_argument("--out", type=Path, default=Path("eval/out/generation_report.json"))
    parser.add_argument("--max-cases", type=int, default=0, help="Limit cases (0=all)")
    parser.add_argument("--skip-no-results", action="store_true", help="Skip expect_no_results rows")
    args = parser.parse_args()

    try:
        from datasets import Dataset
        from ragas import evaluate
        from ragas.metrics import answer_relevancy, faithfulness
    except ImportError:
        print("Install eval deps: pip install -r eval/requirements.txt", file=sys.stderr)
        return 1

    gold = load_gold(args.gold)
    if args.max_cases > 0:
        gold = gold[: args.max_cases]

    rows = []
    for item in gold:
        if item.get("expect_no_results"):
            if args.skip_no_results:
                continue
            continue
        question = item.get("generation_q") or item.get("retrieval_q")
        answer, contexts, status = search(
            args.server,
            question,
            item.get("corpus"),
            item.get("retrieval_q"),
        )
        rows.append(
            {
                "id": item["id"],
                "question": question,
                "answer": answer,
                "contexts": contexts if contexts else ["(no context retrieved)"],
                "ground_truth": item.get("reference_answer") or "",
                "status": status,
            }
        )

    if not rows:
        print("No generation cases to evaluate.", file=sys.stderr)
        return 1

    dataset = Dataset.from_list(rows)
    result = evaluate(
        dataset,
        metrics=[faithfulness, answer_relevancy],
    )

    scores = result.to_pandas()
    report = {
        "server": args.server,
        "gold": str(args.gold),
        "cases": len(rows),
        "faithfulness_mean": float(scores["faithfulness"].mean()),
        "answer_relevancy_mean": float(scores["answer_relevancy"].mean()),
        "per_case": scores.to_dict(orient="records"),
    }

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(
        f"faithfulness={report['faithfulness_mean']:.3f} "
        f"answer_relevancy={report['answer_relevancy_mean']:.3f} "
        f"-> {args.out}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
