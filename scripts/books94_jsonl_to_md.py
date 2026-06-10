#!/usr/bin/env python3
"""Convert books94 category JSONL files to structured Markdown for RAG ingestion."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

CATEGORIES = (
    "almanac",
    "atlas",
    "chronology",
    "dictionary",
    "encyclopedia",
    "quotations",
    "thesaurus",
)

MIN_CHUNK_CHARS = 80

# Slug-like titles: dots/underscores, digits, no spaces, or chr-prefixed almanac ids.
_SLUG_TITLE_RE = re.compile(r"^(?:chr\d+|[\w.]+\.[\w.]+)$", re.IGNORECASE)


def safe_filename(book: str, context_id: str) -> str:
    """Derive a stable filesystem name from context_id."""
    raw = context_id.strip()
    safe = re.sub(r"[|:/\\]+", "_", raw)
    safe = re.sub(r"\s+", "_", safe)
    safe = safe.strip("._")
    if not safe:
        safe = "unknown"
    return f"{book}_{safe}.md"


def is_technical_title(title: str) -> bool:
    title = title.strip()
    if not title:
        return True
    if " " in title:
        return False
    return bool(_SLUG_TITLE_RE.match(title))


def display_title(entry: dict) -> str:
    book = entry.get("book", "")
    title = str(entry.get("title", "")).strip()
    text = str(entry.get("text", "")).strip()

    if book == "thesaurus":
        headword = str(entry.get("headword", "")).strip()
        if headword:
            return headword
        return title or "Untitled"

    if is_technical_title(title) and text:
        first_line = text.split("\n", 1)[0].strip()
        if first_line:
            return first_line
    return title or "Untitled"


def format_body(entry: dict) -> str:
    text = str(entry.get("text", "")).strip()
    book = entry.get("book", "")
    parts: list[str] = []

    if book == "chronology":
        year = entry.get("year")
        if year is not None and str(year).strip():
            parts.append(f"**Période:** {year}")

    if book == "thesaurus":
        pos = str(entry.get("part_of_speech", "")).strip()
        if pos:
            parts.append(f"**Type:** {pos}")
        synonyms = entry.get("synonyms") or []
        if synonyms:
            parts.append("## Synonymes")
            for syn in synonyms:
                syn = str(syn).strip()
                if syn:
                    parts.append(f"- {syn}")

    if parts:
        parts.append("")
    parts.append(text)
    return "\n".join(parts).strip()


def render_markdown(entry: dict) -> str:
    title = display_title(entry)
    book = str(entry.get("book", "")).strip()
    book_title = str(entry.get("book_title", "")).strip()
    context_id = str(entry.get("context_id", "")).strip()
    body = format_body(entry)

    lines = [
        f"# {title}",
        "",
        f"**Livre:** {book_title}",
        f"**Catégorie:** {book}",
        f"**Référence:** `{context_id}`",
        "",
        "---",
        "",
        body,
        "",
    ]
    return "\n".join(lines)


def estimated_index_chars(markdown: str) -> int:
    """Rough plain-text length after metadata (conservative for minChunkChars)."""
    # Metadata block is always indexed; body adds on top.
    return len(markdown)


def convert_category(src_dir: Path, out_dir: Path, category: str) -> dict[str, int]:
    jsonl_path = src_dir / category / f"{category}.jsonl"
    if not jsonl_path.is_file():
        print(f"skip: missing {jsonl_path}", file=sys.stderr)
        return {"written": 0, "skipped_empty": 0, "skipped_short": 0, "errors": 0}

    cat_out = out_dir / category
    cat_out.mkdir(parents=True, exist_ok=True)

    stats = {"written": 0, "skipped_empty": 0, "skipped_short": 0, "errors": 0}
    seen_names: set[str] = set()

    with jsonl_path.open(encoding="utf-8") as fh:
        for line_no, line in enumerate(fh, start=1):
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except json.JSONDecodeError as exc:
                print(f"error: {jsonl_path}:{line_no}: {exc}", file=sys.stderr)
                stats["errors"] += 1
                continue

            text = str(entry.get("text", "")).strip()
            if not text:
                stats["skipped_empty"] += 1
                continue

            book = str(entry.get("book", category)).strip() or category
            context_id = str(entry.get("context_id", "")).strip()
            if not context_id:
                print(
                    f"warning: {jsonl_path}:{line_no}: missing context_id, skipping",
                    file=sys.stderr,
                )
                stats["errors"] += 1
                continue

            md = render_markdown(entry)
            if estimated_index_chars(md) < MIN_CHUNK_CHARS:
                print(
                    f"warning: {jsonl_path}:{line_no}: content < {MIN_CHUNK_CHARS} chars "
                    f"({context_id}), skipping",
                    file=sys.stderr,
                )
                stats["skipped_short"] += 1
                continue

            filename = safe_filename(book, context_id)
            if filename in seen_names:
                filename = safe_filename(book, f"{context_id}_{line_no}")
            seen_names.add(filename)

            out_path = cat_out / filename
            out_path.write_text(md, encoding="utf-8")
            stats["written"] += 1

    return stats


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Convert books94 JSONL category files to Markdown."
    )
    parser.add_argument(
        "--src",
        default=os.path.expanduser("~/extracted"),
        help="Source directory containing category subfolders (default: ~/extracted)",
    )
    parser.add_argument(
        "--out",
        default=os.path.expanduser("~/books94-md"),
        help="Output directory for Markdown files (default: ~/books94-md)",
    )
    parser.add_argument(
        "--category",
        action="append",
        dest="categories",
        help="Convert only this category (repeatable). Default: all categories.",
    )
    args = parser.parse_args()

    src_dir = Path(args.src).expanduser().resolve()
    out_dir = Path(args.out).expanduser().resolve()

    if not src_dir.is_dir():
        print(f"error: source directory not found: {src_dir}", file=sys.stderr)
        return 1

    categories = args.categories or list(CATEGORIES)
    unknown = [c for c in categories if c not in CATEGORIES]
    if unknown:
        print(f"error: unknown categories: {', '.join(unknown)}", file=sys.stderr)
        return 1

    totals = {"written": 0, "skipped_empty": 0, "skipped_short": 0, "errors": 0}
    for category in categories:
        stats = convert_category(src_dir, out_dir, category)
        print(
            f"{category}: written={stats['written']} "
            f"skipped_empty={stats['skipped_empty']} "
            f"skipped_short={stats['skipped_short']} "
            f"errors={stats['errors']}"
        )
        for key in totals:
            totals[key] += stats[key]

    print(
        f"total: written={totals['written']} "
        f"skipped_empty={totals['skipped_empty']} "
        f"skipped_short={totals['skipped_short']} "
        f"errors={totals['errors']} "
        f"-> {out_dir}"
    )
    return 0 if totals["errors"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
