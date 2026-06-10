#!/usr/bin/env python3
"""Scan ~/kb/markdowns and generate or refresh ~/kb/catalog.json."""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from kb_catalog_lib import (
    attach_stats,
    default_catalog,
    merge_catalog,
)


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate KB markdown catalog JSON.")
    parser.add_argument(
        "--kb",
        default=os.path.expanduser("~/kb/markdowns"),
        help="KB markdown root (default: ~/kb/markdowns)",
    )
    parser.add_argument(
        "--out",
        default=os.path.expanduser("~/kb/catalog.json"),
        help="Output catalog path (default: ~/kb/catalog.json)",
    )
    parser.add_argument(
        "--merge",
        action="store_true",
        help="Merge with existing catalog (preserve enabled, exclude, descriptions)",
    )
    args = parser.parse_args()

    kb_root = Path(args.kb).expanduser().resolve()
    out_path = Path(args.out).expanduser().resolve()

    if not kb_root.is_dir():
        print(f"error: KB root not found: {kb_root}", file=sys.stderr)
        return 1

    catalog = default_catalog(kb_root)

    if args.merge and out_path.is_file():
        try:
            existing = json.loads(out_path.read_text(encoding="utf-8"))
            catalog = merge_catalog(existing, catalog)
            print(f"merged with existing catalog: {out_path}")
        except json.JSONDecodeError as exc:
            print(f"error: invalid existing catalog: {exc}", file=sys.stderr)
            return 1

    catalog["kb_root"] = str(kb_root)
    catalog["updated_at"] = datetime.now(timezone.utc).isoformat()
    catalog = attach_stats(catalog)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(
        json.dumps(catalog, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )

    summary = catalog.get("stats_summary", {})
    print(f"catalog written: {out_path}")
    print(
        f"markdown={summary.get('total_markdown_files', 0)} "
        f"enabled_assigned={summary.get('enabled_assigned', 0)} "
        f"unassigned={summary.get('unassigned', 0)} "
        f"excluded={summary.get('globally_excluded', 0)}"
    )
    print(f"corpora={len(catalog.get('corpora', []))} "
          f"suggestions={len(catalog.get('suggestions', []))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
