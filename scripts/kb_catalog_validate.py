#!/usr/bin/env python3
"""Validate KB catalog JSON for ingestion readiness."""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from kb_catalog_lib import print_validation_report, validate_catalog


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate KB catalog JSON.")
    parser.add_argument(
        "--catalog",
        default=os.path.expanduser("~/kb/catalog.json"),
        help="Catalog JSON path (default: ~/kb/catalog.json)",
    )
    args = parser.parse_args()

    catalog_path = Path(args.catalog).expanduser().resolve()
    if not catalog_path.is_file():
        print(f"error: catalog not found: {catalog_path}", file=sys.stderr)
        return 1

    try:
        catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"error: invalid JSON: {exc}", file=sys.stderr)
        return 1

    errors, warnings = validate_catalog(catalog)
    print_validation_report(catalog, errors, warnings)

    if errors:
        print("VALIDATION FAILED")
        return 1

    print("VALIDATION OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
