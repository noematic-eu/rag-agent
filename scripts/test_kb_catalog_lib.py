#!/usr/bin/env python3
"""Tests for kb_catalog_lib assignment and validation."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from kb_catalog_lib import (
    assign_files_to_enabled_corpora,
    corpus_entry,
    default_catalog,
    match_glob,
    matching_specificity,
    validate_catalog,
)


class CatalogAssignmentTests(unittest.TestCase):
    def test_basename_glob_matches_nested_files(self) -> None:
        entry = corpus_entry("kb-misc-root", "root", "root", ["*.md"])
        self.assertTrue(match_glob("business/book.md", "*.md"))
        self.assertEqual(matching_specificity(entry, "business/book.md"), 0)
        self.assertEqual(matching_specificity(entry, "book.md"), 0)

    def test_specificity_resolves_business_over_misc_root(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            kb = Path(tmp).resolve()
            (kb / "business").mkdir()
            (kb / "business" / "guide.md").write_text("# guide\n", encoding="utf-8")
            (kb / "notes.md").write_text("# notes\n", encoding="utf-8")

            catalog = default_catalog(kb)
            catalog["corpora"].append(
                corpus_entry(
                    "kb-misc-root",
                    "Root catch-all",
                    "Legacy root pattern",
                    ["*.md"],
                )
            )
            for item in catalog["corpora"]:
                item["enabled"] = item["id"] in ("kb-business", "kb-misc-root")

            assigned, unassigned, _ = assign_files_to_enabled_corpora(kb, catalog)
            business_rels = {
                p.relative_to(kb).as_posix() for p in assigned.get("kb-business", [])
            }
            root_rels = {
                p.relative_to(kb).as_posix() for p in assigned.get("kb-misc-root", [])
            }
            self.assertIn("business/guide.md", business_rels)
            self.assertIn("notes.md", root_rels)
            self.assertNotIn("business/guide.md", root_rels)

            errors, _warnings = validate_catalog(catalog)
            self.assertEqual(errors, [], errors)


if __name__ == "__main__":
    unittest.main()
