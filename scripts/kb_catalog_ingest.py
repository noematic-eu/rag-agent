#!/usr/bin/env python3
"""Ingest KB markdown files into RAG using catalog.json corpus assignments."""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from kb_catalog_lib import (
    assign_files_to_enabled_corpora,
    rel_posix,
    stale_doc_ids_for_corpus,
    stable_doc_id,
    title_from_path,
    validate_catalog,
)


def load_progress(path: Path) -> set[str]:
    if not path.is_file():
        return set()
    data = json.loads(path.read_text(encoding="utf-8"))
    return set(data.get("completed_doc_ids", []))


def save_progress(path: Path, completed: set[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps({"completed_doc_ids": sorted(completed)}, indent=2) + "\n",
        encoding="utf-8",
    )


def delete_json(url: str, *, timeout: int = 120) -> tuple[int, str]:
    req = urllib.request.Request(url, method="DELETE")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", errors="replace")
    except (TimeoutError, urllib.error.URLError) as exc:
        return 0, str(exc)


def post_json(
    url: str,
    payload: dict | None = None,
    *,
    timeout: int = 1800,
    retries: int = 3,
) -> tuple[int, str]:
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    last_error = ""
    for attempt in range(1, retries + 1):
        req = urllib.request.Request(
            url,
            data=data,
            headers={"Content-Type": "application/json"} if data else {},
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                body = resp.read().decode("utf-8", errors="replace")
                return resp.status, body
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            return exc.code, body
        except (TimeoutError, urllib.error.URLError, OSError) as exc:
            last_error = str(exc)
            if attempt < retries:
                print(f"  retry {attempt}/{retries - 1}: {last_error}", file=sys.stderr)
                time.sleep(min(2**attempt, 30))
                continue
            return 0, last_error
    return 0, last_error


def post_finalize_stream(server: str, *, timeout: int = 7200) -> tuple[bool, str]:
    """POST /finalize?stream=1 and print SSE progress events."""
    url = f"{server.rstrip('/')}/finalize?stream=1"
    req = urllib.request.Request(
        url,
        data=b"",
        headers={"Accept": "text/event-stream"},
        method="POST",
    )
    last_line = ""
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            event = ""
            for raw in resp:
                line = raw.decode("utf-8", errors="replace").rstrip("\r\n")
                last_line = line
                if line.startswith("event:"):
                    event = line.split(":", 1)[1].strip()
                elif line.startswith("data:") and event == "progress":
                    try:
                        payload = json.loads(line.split(":", 1)[1].strip())
                        indexed = payload.get("chunks_indexed", 0)
                        total = payload.get("chunks_total", 0)
                        pct = payload.get("percent")
                        if total:
                            pct_s = f"{pct:.1f}%" if pct is not None else "?"
                            print(f"  finalize: {indexed}/{total} ({pct_s})")
                        else:
                            print(f"  finalize: {indexed} chunks indexed")
                    except json.JSONDecodeError:
                        pass
                elif line.startswith("data:") and event == "complete":
                    return True, line.split(":", 1)[1].strip()
                elif line.startswith("data:") and event == "error":
                    return False, line.split(":", 1)[1].strip()
    except (urllib.error.HTTPError, urllib.error.URLError, OSError, TimeoutError) as exc:
        return False, str(exc)
    return False, last_line or "finalize stream ended without complete event"


def wait_for_server(
    server: str,
    *,
    timeout: int = 180,
    poll_interval: float = 2.0,
) -> bool:
    """Poll GET /stats until the agent HTTP listener is ready."""
    deadline = time.monotonic() + timeout
    url = f"{server.rstrip('/')}/stats"
    while time.monotonic() < deadline:
        try:
            req = urllib.request.Request(url, method="GET")
            with urllib.request.urlopen(req, timeout=5) as resp:
                if resp.status == 200:
                    return True
        except (urllib.error.URLError, OSError, TimeoutError):
            pass
        time.sleep(poll_interval)
    return False


def purge_documents(
    server: str,
    doc_ids: set[str],
    *,
    dry_run: bool = False,
    timeout: int = 120,
) -> tuple[int, int]:
    ok = 0
    fail = 0
    for doc_id in sorted(doc_ids):
        if dry_run:
            print(f"  purge dry-run {doc_id}")
            ok += 1
            continue
        status, body = delete_json(
            f"{server.rstrip('/')}/documents/{doc_id}", timeout=timeout
        )
        if status in (200, 204, 404):
            ok += 1
        else:
            fail += 1
            print(f"  purge FAIL {doc_id}: HTTP {status} {body[:120]}", file=sys.stderr)
    return ok, fail


def load_grades_map(index_data_path: Path) -> dict[str, str] | None:
    if not index_data_path.is_file():
        return None
    data = json.loads(index_data_path.read_text(encoding="utf-8"))
    return {e["path"]: e["grade"] for e in data.get("entries", []) if "path" in e}


def ingest_file(
    server: str,
    kb_root: Path,
    path: Path,
    corpus_id: str,
    content_type: str = "markdown",
    *,
    dry_run: bool = False,
    timeout: int = 1800,
) -> tuple[bool, str]:
    rel = rel_posix(path, kb_root)
    try:
        content = path.read_text(encoding="utf-8")
    except OSError as exc:
        return False, f"read error: {exc}"

    doc = {
        "id": stable_doc_id(rel),
        "title": title_from_path(rel),
        "content": content,
        "content_type": content_type,
        "corpus": corpus_id,
    }

    if dry_run:
        return True, f"dry-run {rel} -> {corpus_id} ({doc['id']})"

    status, body = post_json(f"{server.rstrip('/')}/ingest", doc, timeout=timeout)
    if status != 200:
        return False, f"HTTP {status}: {body[:200]}"
    return True, rel


def main() -> int:
    parser = argparse.ArgumentParser(description="Ingest KB catalog into RAG.")
    parser.add_argument(
        "--catalog",
        default=os.path.expanduser("~/kb/catalog.json"),
        help="Catalog JSON path (default: ~/kb/catalog.json)",
    )
    parser.add_argument(
        "--server",
        default="http://127.0.0.1:8081",
        help="RAG API base URL (default: http://127.0.0.1:8081)",
    )
    parser.add_argument(
        "--corpus",
        action="append",
        dest="corpora",
        help="Ingest only this corpus id (repeatable)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List files that would be ingested without calling the API",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=50,
        help="Log progress every N files (default: 50)",
    )
    parser.add_argument(
        "--finalize",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Call POST /finalize after ingestion (default: true). Required for RAG_F4KVS_LEXICAL_MODE=disk to build lex:* index.",
    )
    parser.add_argument(
        "--fail-fast",
        action="store_true",
        help="Stop on first ingestion error",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=1800,
        help="HTTP timeout per file in seconds (default: 1800)",
    )
    parser.add_argument(
        "--skip-corpus",
        action="append",
        dest="skip_corpora",
        help="Skip this corpus id (repeatable)",
    )
    parser.add_argument(
        "--progress-file",
        default=os.path.expanduser("~/kb/.ingest-progress.json"),
        help="Track completed doc ids for resume (default: ~/kb/.ingest-progress.json)",
    )
    parser.add_argument(
        "--resume",
        action="store_true",
        help="Skip doc ids already listed in --progress-file",
    )
    parser.add_argument(
        "--resume-after",
        default="",
        help="Skip files until after this kb_root-relative path (e.g. books/it/High_Performance_Spark.pdf.md)",
    )
    parser.add_argument(
        "--skip-validate",
        action="store_true",
        help="Skip catalog validation (faster startup when resuming)",
    )
    parser.add_argument(
        "--index-data",
        default=os.path.expanduser("~/kb/index-data.json"),
        help="Index JSON with per-doc grades (default: ~/kb/index-data.json)",
    )
    parser.add_argument(
        "--grades",
        nargs="+",
        default=[],
        help="Ingest only docs with these grades (e.g. --grades A or --grades A B)",
    )
    parser.add_argument(
        "--replace-corpus",
        action="store_true",
        help="Delete legacy monolith doc IDs for --corpus before ingesting (requires --corpus)",
    )
    parser.add_argument(
        "--wait-ready",
        type=int,
        default=180,
        metavar="SECS",
        help="Wait up to SECS for GET /stats before ingesting (default: 180; 0=skip)",
    )
    args = parser.parse_args()

    if args.replace_corpus and not args.corpora:
        print("error: --replace-corpus requires --corpus", file=sys.stderr)
        return 1
    if args.replace_corpus and len(args.corpora) != 1:
        print("error: --replace-corpus supports a single --corpus only", file=sys.stderr)
        return 1

    if not args.dry_run and args.wait_ready > 0:
        print(f"Waiting for {args.server} (up to {args.wait_ready}s)...")
        if not wait_for_server(args.server, timeout=args.wait_ready):
            print(
                f"error: server not ready after {args.wait_ready}s "
                f"(check docker logs for 'listening on')",
                file=sys.stderr,
            )
            return 1
        print("Server ready.")

    catalog_path = Path(args.catalog).expanduser().resolve()
    if not catalog_path.is_file():
        print(f"error: catalog not found: {catalog_path}", file=sys.stderr)
        return 1

    catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    if not args.skip_validate:
        errors, warnings = validate_catalog(catalog)
        for msg in warnings:
            print(f"warning: {msg}", file=sys.stderr)
        if errors:
            for msg in errors:
                print(f"error: {msg}", file=sys.stderr)
            print("aborting: catalog validation failed", file=sys.stderr)
            return 1

    kb_root = Path(catalog["kb_root"]).resolve()
    assigned, _, _ = assign_files_to_enabled_corpora(kb_root, catalog)

    grade_filter: set[str] | None = None
    grades_map: dict[str, str] = {}
    if args.grades:
        grade_filter = set(args.grades)
        index_data_path = Path(args.index_data).expanduser().resolve()
        grades_map = load_grades_map(index_data_path)
        if grades_map is None:
            print(f"error: index-data not found: {index_data_path}", file=sys.stderr)
            return 1
        print(f"Grade filter: {sorted(grade_filter)}")

    progress_path = Path(args.progress_file).expanduser()
    completed = load_progress(progress_path) if args.resume else set()

    if args.replace_corpus:
        corpus_id = args.corpora[0]
        stale_ids = stale_doc_ids_for_corpus(kb_root, corpus_id)
        if stale_ids:
            print(
                f"Purging {len(stale_ids)} legacy document(s) for {corpus_id}"
                + (" (dry-run)" if args.dry_run else "")
            )
            purge_ok, purge_fail = purge_documents(
                args.server,
                stale_ids,
                dry_run=args.dry_run,
                timeout=min(args.timeout, 120),
            )
            print(f"  purge: ok={purge_ok} fail={purge_fail}")
            if purge_fail:
                return 1
            completed -= stale_ids
            if not args.dry_run:
                save_progress(progress_path, completed)
        else:
            print(f"No legacy doc IDs to purge for {corpus_id}")
    resume_after = args.resume_after.strip()
    skipping_until_after = bool(resume_after)

    corpus_filter = set(args.corpora) if args.corpora else None
    skip_corpora = set(args.skip_corpora or [])
    total_ok = 0
    total_fail = 0
    total_skipped = 0
    total_files = 0

    for corpus_id in sorted(assigned.keys()):
        if corpus_filter and corpus_id not in corpus_filter:
            continue
        if corpus_id in skip_corpora:
            print(f"Skipping {corpus_id}")
            continue
        paths = assigned[corpus_id]
        if not paths:
            continue

        entry = next(
            (
                e
                for e in catalog.get("corpora", []) + catalog.get("suggestions", [])
                if e.get("id") == corpus_id
            ),
            {},
        )
        content_type = entry.get("ingest", {}).get("content_type", "markdown")

        print(f"Ingesting {corpus_id} ({len(paths)} files)...")
        corpus_ok = 0
        corpus_fail = 0

        for i, path in enumerate(paths, start=1):
            rel = rel_posix(path, kb_root)
            doc_id = stable_doc_id(rel)

            if skipping_until_after:
                if rel == resume_after:
                    skipping_until_after = False
                total_skipped += 1
                continue

            if args.resume and doc_id in completed:
                total_skipped += 1
                continue

            if grade_filter is not None and grades_map.get(rel) not in grade_filter:
                total_skipped += 1
                continue

            total_files += 1
            ok, msg = ingest_file(
                args.server,
                kb_root,
                path,
                corpus_id,
                content_type,
                dry_run=args.dry_run,
                timeout=args.timeout,
            )
            if ok:
                corpus_ok += 1
                total_ok += 1
                if not args.dry_run:
                    print(f"  ok {rel}")
                    completed.add(doc_id)
                    save_progress(progress_path, completed)
            else:
                corpus_fail += 1
                total_fail += 1
                print(f"  FAIL {msg}", file=sys.stderr)
                if args.fail_fast:
                    print("aborting (--fail-fast)", file=sys.stderr)
                    return 1

            if i % args.batch_size == 0:
                print(f"  {corpus_id}: {i}/{len(paths)} processed...")

        print(f"  {corpus_id}: ok={corpus_ok} fail={corpus_fail}")

    print(
        f"Done. total={total_files} ok={total_ok} fail={total_fail} skipped={total_skipped}"
        + (" (dry-run)" if args.dry_run else "")
    )

    if total_fail:
        return 1

    if args.finalize and not args.dry_run:
        print(f"Finalizing index on {args.server}")
        ok, body = post_finalize_stream(args.server, timeout=max(args.timeout, 7200))
        if not ok:
            print(f"finalize failed: {body}", file=sys.stderr)
            return 1
        print(body.strip())

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
