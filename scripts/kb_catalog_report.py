#!/usr/bin/env python3
"""Print a human-readable report from KB catalog JSON."""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from kb_catalog_lib import assign_files_to_enabled_corpora


def fmt_bytes(n: int) -> str:
    if n >= 1_000_000:
        return f"{n / 1_000_000:.1f} MB"
    if n >= 1_000:
        return f"{n / 1_000:.1f} KB"
    return f"{n} B"


def main() -> int:
    parser = argparse.ArgumentParser(description="Report on KB catalog JSON.")
    parser.add_argument(
        "--catalog",
        default=os.path.expanduser("~/kb/catalog.json"),
        help="Catalog JSON path (default: ~/kb/catalog.json)",
    )
    parser.add_argument(
        "--format",
        choices=("text", "markdown"),
        default="text",
        help="Output format (default: text)",
    )
    args = parser.parse_args()

    catalog_path = Path(args.catalog).expanduser().resolve()
    if not catalog_path.is_file():
        print(f"error: catalog not found: {catalog_path}", file=sys.stderr)
        return 1

    catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    kb_root = Path(catalog["kb_root"]).resolve()
    assigned, unassigned, excluded = assign_files_to_enabled_corpora(
        kb_root, catalog
    )
    md = args.format == "markdown"
    summary = catalog.get("stats_summary", {})

    def heading(text: str) -> None:
        print(f"## {text}" if md else f"=== {text} ===")

    def line(text: str) -> None:
        print(text)

    heading("KB Catalog Report")
    line(f"Root: {catalog['kb_root']}")
    line(f"Updated: {catalog.get('updated_at', 'unknown')}")
    line("")

    heading("Corpora")
    for item in catalog.get("corpora", []):
        status = "enabled" if item.get("enabled") else "disabled"
        stats = item.get("stats", {})
        count = stats.get("file_count", 0)
        size = fmt_bytes(stats.get("total_bytes", 0))
        line(f"{'- ' if md else ''}{item['id']} ({count} files, {size}) [{status}]")
        if item.get("description"):
            line(f"  {item['description']}")
        largest = stats.get("largest_file")
        if largest:
            line(
                f"  largest: {largest['path']} "
                f"({fmt_bytes(largest['bytes'])})"
            )
        enabled_count = stats.get("enabled_file_count")
        if enabled_count is not None and item.get("enabled"):
            line(f"  assigned when enabled: {enabled_count}")
    line("")

    suggestions = catalog.get("suggestions", [])
    if suggestions:
        heading("Suggestions (books sub-corpus)")
        for item in suggestions:
            status = "enabled" if item.get("enabled") else "disabled"
            stats = item.get("stats", {})
            count = stats.get("file_count", 0)
            parent = item.get("parent", "")
            parent_note = f", parent={parent}" if parent else ""
            line(
                f"{'- ' if md else ''}{item['id']} ({count} files) "
                f"[{status}{parent_note}]"
            )
        line("")

    heading("Global exclusions")
    for pattern in catalog.get("global_exclude", []):
        line(f"{'- ' if md else '  '}{pattern}")
    line("")

    heading("Summary")
    line(f"orphans (unassigned): {summary.get('unassigned', len(unassigned))}")
    line(f"globally excluded: {summary.get('globally_excluded', len(excluded))}")
    line(
        f"enabled assigned: {summary.get('enabled_assigned', sum(len(v) for v in assigned.values()))}"
    )
    line(f"total markdown on disk: {summary.get('total_markdown_files', '?')}")

    unassigned_list = catalog.get("unassigned", [])
    if unassigned_list:
        line("")
        heading("Unassigned files")
        for rel in unassigned_list[:20]:
            line(f"{'- ' if md else '  '}{rel}")
        if len(unassigned_list) > 20:
            line(f"  ... and {len(unassigned_list) - 20} more")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
