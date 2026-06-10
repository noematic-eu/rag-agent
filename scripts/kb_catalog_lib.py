"""Shared helpers for KB markdown catalog scripts."""

from __future__ import annotations

import fnmatch
import hashlib
import os
import re
from collections import defaultdict
from pathlib import Path
from typing import Iterable

MARKDOWN_SUFFIXES = {".md", ".markdown"}
CORPUS_ID_RE = re.compile(r"^kb-[a-z0-9]+(?:-[a-z0-9]+)*$")

DEFAULT_GLOBAL_EXCLUDE = [
    ".DS_Store",
    "**/.DS_Store",
    "cv.md",
    "placements.md",
    "boussemart_baptiste_MSc_2007.md",
    "encyclopedias/_sources/**",
    "encyclopedias/articles/**/meta/**",
]

ENCYCLOPEDIA_ARTICLE_EXCLUDE = [
    "encyclopedias/articles/**/_toc.md",
    "encyclopedias/articles/**/meta/**",
]

TOP_LEVEL_CORPORA = [
    {
        "id": "kb-encyclopedias",
        "name": "Encyclopédies",
        "description": "Encyclopedia articles split from monolith sources",
        "include": [
            "encyclopedias/articles/**/*.md",
            "encyclopedias/articles/**/*.markdown",
        ],
        "exclude": list(ENCYCLOPEDIA_ARTICLE_EXCLUDE),
    },
    {
        "id": "kb-books",
        "name": "Books",
        "description": "Livres et guides techniques",
        "include": ["books/**/*.md", "books/**/*.markdown"],
        "exclude": ["books/rust/**"],
    },
    {
        "id": "kb-rust",
        "name": "Rust",
        "description": "Livres et notes Rust (langage, tokio, adrust, etc.)",
        "include": ["books/rust/**/*.md", "books/rust/**/*.markdown"],
    },
    {
        "id": "kb-history-chats",
        "name": "History chats",
        "description": "Exports de conversations par projet",
        "include": ["history-chats/**/*.md", "history-chats/**/*.markdown"],
    },
    {
        "id": "kb-genai-guide",
        "name": "Generative AI guide",
        "description": "Awesome generative AI guide (markdown)",
        "include": [
            "awesome-generative-ai-guide/**/*.md",
            "awesome-generative-ai-guide/**/*.markdown",
        ],
    },
    {
        "id": "kb-noematic",
        "name": "Noematic",
        "description": "Documentation projet Noematic",
        "include": ["noematic/**/*.md", "noematic/**/*.markdown"],
    },
    {
        "id": "kb-memoirs",
        "name": "Living memoirs notes",
        "description": "Notes personnelles living memoirs",
        "include": [
            "living-memoirs-notes/**/*.md",
            "living-memoirs-notes/**/*.markdown",
        ],
    },
    {
        "id": "kb-business",
        "name": "Business",
        "description": "Livres et guides business / vente",
        "include": ["business/**/*.md", "business/**/*.markdown"],
    },
    {
        "id": "kb-finances",
        "name": "Finances",
        "description": "Finances personnelles et investissement",
        "include": ["finances/**/*.md", "finances/**/*.markdown"],
    },
    {
        "id": "kb-misc",
        "name": "Miscellaneous",
        "description": "Fichiers racine et dossier misc",
        "include": [
            "misc/**/*.md",
            "misc/**/*.markdown",
            "@root/*.md",
            "@root/*.markdown",
        ],
    },
]

BOOKS_SUGGESTION_MIN_FILES = 5
BOOKS_SUBDIR_SKIP = {"to-read", "rust"}

# Large or out-of-catalog trees under kb_root — skipped during directory walks.
DEFAULT_PRUNE_DIRS = frozenset(
    {
        "grokipedia",
        "input",
        "tosort",
        "done",
        ".git",
    }
)


def is_markdown(path: Path) -> bool:
    return path.suffix.lower() in MARKDOWN_SUFFIXES


def prune_dir_names(global_exclude: list[str] | None = None) -> set[str]:
    names = set(DEFAULT_PRUNE_DIRS)
    for pattern in global_exclude or []:
        if not pattern.endswith("/**"):
            continue
        # Only single-segment trees (e.g. grokipedia/**) — not encyclopedias/_sources/**.
        prefix = pattern[: -len("/**")]
        if prefix and "/" not in prefix and prefix != "**":
            names.add(prefix)
    return names


def collect_markdown_files(
    kb_root: Path, *, global_exclude: list[str] | None = None
) -> list[Path]:
    kb_root = kb_root.resolve()
    prune = prune_dir_names(global_exclude)
    files: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(kb_root, topdown=True):
        dirnames[:] = sorted(
            d for d in dirnames if d not in prune and not d.startswith(".")
        )
        for name in sorted(filenames):
            if name.startswith("."):
                continue
            path = Path(dirpath) / name
            if is_markdown(path):
                files.append(path)
    return files


def rel_posix(path: Path, kb_root: Path) -> str:
    return path.relative_to(kb_root).as_posix()


def _match_segment(pattern: str, text: str) -> bool:
    if fnmatch.fnmatchcase(text, pattern):
        return True
    # Case-insensitive match for markdown extension globs (*.md, *.markdown).
    lower = pattern.lower()
    if lower.endswith((".md", ".markdown")):
        return fnmatch.fnmatchcase(text.lower(), lower)
    return False


def match_glob(rel_path: str, pattern: str) -> bool:
    """Match a posix relative path against a glob (supports ** and @root/)."""
    if pattern.startswith("@root/"):
        if "/" in rel_path:
            return False
        return fnmatch.fnmatchcase(Path(rel_path).name, pattern[len("@root/") :])

    path_parts = rel_path.split("/")
    pattern_parts = pattern.split("/")

    # Single-segment patterns apply to the basename only (never across /).
    if len(pattern_parts) == 1:
        return fnmatch.fnmatchcase(path_parts[-1], pattern_parts[0])

    def match_parts(pi: int, si: int) -> bool:
        if pi == len(pattern_parts):
            return si == len(path_parts)
        part = pattern_parts[pi]
        if part == "**":
            if pi == len(pattern_parts) - 1:
                return True
            for end in range(si, len(path_parts) + 1):
                if match_parts(pi + 1, end):
                    return True
            return False
        if si >= len(path_parts):
            return False
        if not _match_segment(part, path_parts[si]):
            return False
        return match_parts(pi + 1, si + 1)

    return match_parts(0, 0)


def matches_any(rel_path: str, patterns: Iterable[str]) -> bool:
    return any(match_glob(rel_path, pattern) for pattern in patterns)


def slugify_segment(name: str) -> str:
    slug = name.lower().strip()
    slug = re.sub(r"[^a-z0-9]+", "-", slug)
    return slug.strip("-")


def corpus_entry(
    entry_id: str,
    name: str,
    description: str,
    include: list[str],
    *,
    enabled: bool = True,
    exclude: list[str] | None = None,
    parent: str | None = None,
    extra: dict | None = None,
) -> dict:
    item = {
        "id": entry_id,
        "name": name,
        "description": description,
        "enabled": enabled,
        "include": include,
        "exclude": exclude or [],
        "ingest": {"content_type": "markdown", "finalize_group": "kb"},
    }
    if parent:
        item["parent"] = parent
    if extra:
        item.update(extra)
    return item


def default_catalog(kb_root: Path) -> dict:
    kb_root = kb_root.resolve()
    corpora = []
    for spec in TOP_LEVEL_CORPORA:
        extra = {k: v for k, v in spec.items() if k not in ("id", "name", "description", "include")}
        corpora.append(
            corpus_entry(
                spec["id"],
                spec["name"],
                spec["description"],
                spec["include"],
                enabled=True,
                exclude=extra.pop("exclude", None),
                extra=extra or None,
            )
        )
    suggestions = build_books_suggestions(kb_root)
    return {
        "schema_version": 1,
        "kb_root": str(kb_root),
        "updated_at": None,
        "corpora": corpora,
        "suggestions": suggestions,
        "global_exclude": list(DEFAULT_GLOBAL_EXCLUDE),
        "unassigned": [],
    }


def build_books_suggestions(kb_root: Path) -> list[dict]:
    books_dir = kb_root / "books"
    if not books_dir.is_dir():
        return []

    suggestions: list[dict] = []
    for subdir in sorted(books_dir.iterdir()):
        if not subdir.is_dir() or subdir.name in BOOKS_SUBDIR_SKIP:
            continue
        md_files = [p for p in subdir.rglob("*") if p.is_file() and is_markdown(p)]
        if len(md_files) < BOOKS_SUGGESTION_MIN_FILES:
            continue
        rel = subdir.relative_to(kb_root).as_posix()
        slug = slugify_segment(subdir.name)
        suggestions.append(
            corpus_entry(
                f"kb-books-{slug}",
                f"Books — {subdir.name}",
                f"Sous-ensemble books/{subdir.name}",
                [f"{rel}/**/*.md", f"{rel}/**/*.markdown"],
                enabled=False,
                parent="kb-books",
            )
        )
    return suggestions


def entry_maps(catalog: dict) -> tuple[dict[str, dict], dict[str, dict]]:
    corpora = {item["id"]: item for item in catalog.get("corpora", [])}
    suggestions = {item["id"]: item for item in catalog.get("suggestions", [])}
    return corpora, suggestions


def all_entries(catalog: dict, *, enabled_only: bool = False) -> list[dict]:
    items = list(catalog.get("corpora", [])) + list(catalog.get("suggestions", []))
    if enabled_only:
        items = [item for item in items if item.get("enabled", False)]
    return items


def is_globally_excluded(rel_path: str, global_exclude: list[str]) -> bool:
    return matches_any(rel_path, global_exclude)


def corpus_specificity(entry: dict) -> int:
    """Higher = more specific (longer include prefix without wildcards)."""
    best = 0
    for pattern in entry.get("include", []):
        prefix = pattern.split("*", 1)[0].rstrip("/")
        best = max(best, len(prefix))
    return best


def matching_specificity(entry: dict, rel_path: str) -> int:
    """Specificity of the best-matching include pattern for rel_path (0 if none)."""
    best = 0
    for pattern in entry.get("include", []):
        if not match_glob(rel_path, pattern):
            continue
        prefix = pattern.split("*", 1)[0].rstrip("/")
        best = max(best, len(prefix))
    return best


def is_basename_only_glob(pattern: str) -> bool:
    """True for *.md-style patterns that match any path's basename (not @root/)."""
    if pattern.startswith("@root/"):
        return False
    return "/" not in pattern and "*" in pattern


def files_for_entry(
    kb_root: Path,
    entry: dict,
    global_exclude: list[str],
    all_files: list[Path] | None = None,
) -> list[Path]:
    if all_files is None:
        all_files = collect_markdown_files(kb_root, global_exclude=global_exclude)

    matched: list[Path] = []
    include = entry.get("include", [])
    exclude = entry.get("exclude", [])

    for path in all_files:
        rel = rel_posix(path, kb_root)
        if is_globally_excluded(rel, global_exclude):
            continue
        if not matches_any(rel, include):
            continue
        if exclude and matches_any(rel, exclude):
            continue
        matched.append(path)
    return matched


def assign_files_to_enabled_corpora(
    kb_root: Path,
    catalog: dict,
    *,
    all_files: list[Path] | None = None,
) -> tuple[dict[str, list[Path]], list[Path], list[Path]]:
    """Return mapping corpus_id -> files, unassigned files, globally excluded files."""
    kb_root = Path(catalog["kb_root"]).resolve()
    global_exclude = catalog.get("global_exclude", [])
    if all_files is None:
        all_files = collect_markdown_files(kb_root, global_exclude=global_exclude)

    excluded: list[Path] = []
    eligible: list[Path] = []
    for path in all_files:
        rel = rel_posix(path, kb_root)
        if is_globally_excluded(rel, global_exclude):
            excluded.append(path)
        else:
            eligible.append(path)

    enabled_entries = sorted(
        [e for e in all_entries(catalog, enabled_only=True)],
        key=corpus_specificity,
        reverse=True,
    )

    assigned: dict[str, list[Path]] = {e["id"]: [] for e in enabled_entries}
    file_owner: dict[str, str] = {}

    for entry in enabled_entries:
        for path in files_for_entry(kb_root, entry, global_exclude, eligible):
            rel = rel_posix(path, kb_root)
            if rel in file_owner:
                continue
            file_owner[rel] = entry["id"]
            assigned[entry["id"]].append(path)

    unassigned = [p for p in eligible if rel_posix(p, kb_root) not in file_owner]
    return assigned, unassigned, excluded


def compute_stats(files: list[Path]) -> dict:
    total_bytes = sum(p.stat().st_size for p in files)
    samples = [p.name for p in files[:3]]
    largest = None
    if files:
        largest_path = max(files, key=lambda p: p.stat().st_size)
        largest = {
            "path": largest_path.name,
            "bytes": largest_path.stat().st_size,
        }
    return {
        "file_count": len(files),
        "total_bytes": total_bytes,
        "sample_files": samples,
        "largest_file": largest,
    }


def merge_catalog(existing: dict, fresh: dict) -> dict:
    """Preserve user edits (enabled, exclude, description, name) while refreshing stats."""
    old_corpora, old_suggestions = entry_maps(existing)

    def merge_entries(
        fresh_items: list[dict],
        old_map: dict[str, dict],
        *,
        keep_stale: bool = True,
    ) -> list[dict]:
        merged: list[dict] = []
        fresh_ids = {item["id"] for item in fresh_items}
        for item in fresh_items:
            old = old_map.get(item["id"], {})
            merged_item = dict(item)
            for key in ("enabled", "description", "name", "ingest"):
                if key in old:
                    merged_item[key] = old[key]
            if old.get("exclude"):
                merged_item["exclude"] = old["exclude"]
            merged.append(merged_item)
        if keep_stale:
            for entry_id, old in old_map.items():
                if entry_id not in fresh_ids:
                    merged.append(old)
        return merged

    merged = dict(fresh)
    merged["corpora"] = merge_entries(fresh["corpora"], old_corpora)
    # Suggestions are regenerated from disk; drop removed sub-corpus (e.g. to-read).
    merged["suggestions"] = merge_entries(
        fresh["suggestions"], old_suggestions, keep_stale=False
    )
    if "global_exclude" in existing:
        merged["global_exclude"] = existing["global_exclude"]
    return merged


def attach_stats(catalog: dict) -> dict:
    kb_root = Path(catalog["kb_root"]).resolve()
    global_exclude = catalog.get("global_exclude", [])
    all_files = collect_markdown_files(kb_root, global_exclude=global_exclude)
    assigned, unassigned, excluded = assign_files_to_enabled_corpora(
        kb_root, catalog, all_files=all_files
    )

    for section in ("corpora", "suggestions"):
        for item in catalog.get(section, []):
            files = files_for_entry(
                kb_root, item, global_exclude, all_files=all_files
            )
            item["stats"] = compute_stats(files)
            if item.get("enabled"):
                item["stats"]["enabled_file_count"] = len(assigned.get(item["id"], []))

    catalog["unassigned"] = sorted(rel_posix(p, kb_root) for p in unassigned)
    catalog["stats_summary"] = {
        "total_markdown_files": len(all_files),
        "globally_excluded": len(excluded),
        "enabled_assigned": sum(len(v) for v in assigned.values()),
        "unassigned": len(unassigned),
    }
    return catalog


def stable_doc_id(rel_path: str) -> str:
    """Match client/docid.go StableDocID (SHA1 of relative path)."""
    digest = hashlib.sha1(rel_path.encode("utf-8")).hexdigest()
    return f"doc-{digest}"


def stale_encyclopedia_doc_ids(kb_root: Path) -> set[str]:
    """Doc IDs for monolith sources superseded by encyclopedias/articles/."""
    kb_root = kb_root.resolve()
    ids: set[str] = set()
    sources = kb_root / "encyclopedias" / "_sources"
    if sources.is_dir():
        for path in sources.rglob("*"):
            if path.is_file() and is_markdown(path):
                ids.add(stable_doc_id(rel_posix(path, kb_root)))
    enc = kb_root / "encyclopedias"
    for pattern in ("*.md", "*.markdown"):
        for path in enc.glob(pattern):
            ids.add(stable_doc_id(rel_posix(path, kb_root)))
    return ids


def stale_doc_ids_for_corpus(kb_root: Path, corpus_id: str) -> set[str]:
    if corpus_id == "kb-encyclopedias":
        return stale_encyclopedia_doc_ids(kb_root)
    return set()


def title_from_path(rel_path: str) -> str:
    return Path(rel_path).stem


def validate_catalog(catalog: dict) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warnings: list[str] = []

    kb_root = Path(catalog.get("kb_root", "")).expanduser()
    if not kb_root.is_dir():
        errors.append(f"kb_root does not exist: {kb_root}")
        return errors, warnings

    kb_root = kb_root.resolve()
    global_exclude = catalog.get("global_exclude", [])
    all_files = collect_markdown_files(kb_root, global_exclude=global_exclude)

    seen_ids: set[str] = set()
    for section in ("corpora", "suggestions"):
        for item in catalog.get(section, []):
            entry_id = item.get("id", "")
            if not entry_id:
                errors.append(f"{section}: missing id")
                continue
            if entry_id in seen_ids:
                errors.append(f"duplicate id: {entry_id}")
            seen_ids.add(entry_id)
            if not CORPUS_ID_RE.match(entry_id):
                errors.append(f"invalid id format (expected kb-*): {entry_id}")
            if not item.get("include"):
                errors.append(f"{entry_id}: empty include list")

            files = files_for_entry(
                kb_root, item, global_exclude, all_files=all_files
            )
            if not files:
                warnings.append(f"{entry_id}: include patterns match 0 files")

    enabled = [e for e in all_entries(catalog, enabled_only=True)]
    if not enabled:
        warnings.append("no enabled corpus")

    entry_by_id = {entry["id"]: entry for entry in all_entries(catalog)}

    file_hits: dict[str, list[str]] = defaultdict(list)
    for entry in enabled:
        for path in files_for_entry(
            kb_root, entry, global_exclude, all_files=all_files
        ):
            rel = rel_posix(path, kb_root)
            file_hits[rel].append(entry["id"])

    for rel, owners in sorted(file_hits.items()):
        if len(owners) <= 1:
            continue
        specs = {
            owner: matching_specificity(entry_by_id[owner], rel) for owner in owners
        }
        max_spec = max(specs.values())
        tied = [owner for owner, spec in specs.items() if spec == max_spec]
        if len(tied) > 1:
            errors.append(
                f"conflict: {rel} matched by enabled corpora {tied} "
                f"(equal specificity {max_spec})"
            )

    for entry in enabled:
        for pattern in entry.get("include", []):
            if is_basename_only_glob(pattern):
                warnings.append(
                    f"{entry['id']}: include {pattern!r} matches every markdown "
                    f"basename; use @root/{pattern} for kb-root files only"
                )
                break

    assigned, unassigned, excluded = assign_files_to_enabled_corpora(
        kb_root, catalog, all_files=all_files
    )
    if unassigned:
        warnings.append(
            f"{len(unassigned)} unassigned markdown file(s) "
            f"(first: {rel_posix(unassigned[0], kb_root)})"
        )

    for entry_id, paths in assigned.items():
        for path in paths:
            if not path.is_file():
                errors.append(f"stale file for {entry_id}: {path}")

    for pattern in global_exclude:
        if not pattern:
            warnings.append("empty global_exclude pattern")

    enabled_ids = {e["id"] for e in enabled}
    for item in catalog.get("suggestions", []):
        if not item.get("enabled"):
            continue
        parent = item.get("parent")
        if parent and parent in enabled_ids:
            parent_entry = next(
                (c for c in catalog.get("corpora", []) if c["id"] == parent), None
            )
            if parent_entry and parent_entry.get("enabled"):
                warnings.append(
                    f"{item['id']} enabled while parent {parent} is also enabled "
                    f"(assignment uses specificity; validate conflicts above)"
                )

    all_catalogued = set()
    for entry in all_entries(catalog):
        for path in files_for_entry(
            kb_root, entry, global_exclude, all_files=all_files
        ):
            all_catalogued.add(rel_posix(path, kb_root))

    on_disk = {rel_posix(p, kb_root) for p in all_files}
    orphans = on_disk - all_catalogued - {
        rel for rel in on_disk if is_globally_excluded(rel, global_exclude)
    }
    if orphans:
        warnings.append(
            f"{len(orphans)} markdown file(s) not covered by any corpus include "
            f"(first: {sorted(orphans)[0]})"
        )

    return errors, warnings


def print_validation_report(
    catalog: dict, errors: list[str], warnings: list[str]
) -> None:
    kb_root = Path(catalog["kb_root"]).resolve()
    assigned, unassigned, excluded = assign_files_to_enabled_corpora(
        kb_root, catalog
    )

    print(f"Catalog: {catalog.get('kb_root')}")
    print(f"Updated: {catalog.get('updated_at', 'unknown')}")
    print()

    for section, label in (("corpora", "Corpora"), ("suggestions", "Suggestions")):
        items = catalog.get(section, [])
        if not items:
            continue
        print(f"=== {label} ===")
        for item in items:
            status = "enabled" if item.get("enabled") else "disabled"
            stats = item.get("stats", {})
            count = stats.get("file_count", 0)
            enabled_count = stats.get("enabled_file_count", "")
            extra = f", assigned={enabled_count}" if enabled_count != "" else ""
            print(f"  {item['id']}: {count} files{extra} [{status}]")
        print()

    summary = catalog.get("stats_summary", {})
    print("=== Summary ===")
    print(f"  total markdown: {summary.get('total_markdown_files', '?')}")
    print(f"  enabled assigned: {summary.get('enabled_assigned', '?')}")
    print(f"  unassigned: {summary.get('unassigned', '?')}")
    print(f"  globally excluded: {summary.get('globally_excluded', '?')}")
    print()

    if warnings:
        print("=== Warnings ===")
        for msg in warnings:
            print(f"  - {msg}")
        print()

    if errors:
        print("=== Errors ===")
        for msg in errors:
            print(f"  - {msg}")
        print()
