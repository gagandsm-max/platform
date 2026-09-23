#!/usr/bin/env python3
"""Mother Reference structural validator (A1).

Python standard library only. Validates the repository Mother Reference set
against constitutional invariants plus project_documentation/mother_reference_manifest.json.

This is structural protection, not semantic certification.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path
from typing import Any

SCHEMA_VERSION = "1.0.0"
ALLOWED_NORMALIZATION = {"NORMALIZED_V1", "TRANSITIONAL_LEGACY"}
MATURITY_STATES = (
    "SPECIFIED",
    "IMPLEMENTED",
    "TESTED",
    "BENCHMARKED",
    "CERTIFIED",
    "DEPLOYED",
)

# Constitutional invariant: AGENTS + Platform_00..Platform_23 (25 files).
CONSTITUTIONAL_PLATFORM_NUMBERS = tuple(f"{n:02d}" for n in range(0, 24))
CONSTITUTIONAL_AUTHORITATIVE_COUNT = 1 + len(CONSTITUTIONAL_PLATFORM_NUMBERS)  # 25

REQ_ID_RE = re.compile(r"\b(REQ-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
PRD_ID_RE = re.compile(r"\b(PRD-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
FORMAL_ID_RE = re.compile(
    r"\b((?:REQ|PRD)-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?|OSS-\d{3,})\b"
)
PLATFORM_NUM_RE = re.compile(r"^Platform_(\d{2})_")
DOC_ID_RE = re.compile(r"^\*\*Document ID:\*\*\s*(\S+)", re.M)
VERSION_RE = re.compile(r"^\*\*Version:\*\*\s*(\S+)", re.M)
TOP_SECTION_RE = re.compile(r"^##\s+(\d+)\.", re.M)

REQ_MARKER_START = "<!-- GLOBAL PRIMARY REQ REGISTRY -->"
REQ_MARKER_END = "<!-- END GLOBAL PRIMARY REQ REGISTRY -->"
PRD_MARKER_START = "<!-- GLOBAL PRIMARY PRD REGISTRY -->"
PRD_MARKER_END = "<!-- END GLOBAL PRIMARY PRD REGISTRY -->"

REQ_COLUMNS = 7
PRD_COLUMNS = 6
REQ_HEADERS = {
    "Requirement ID",
    "Summary",
    "Primary owner",
    "Source locator",
    "Traceability state",
    "Maturity",
    "Evidence / blocker pointer",
}
PRD_HEADERS = {
    "PRD ID",
    "Product requirement summary",
    "Owner document",
    "Source locator",
    "Related REQ family",
    "Maturity / implementation boundary",
}

# Product/specialized owners used for PRD discovery (authoritative prose homes).
PRD_SOURCE_FILES = {
    "Platform_02_PRD.md",
    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md",
    "Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md",
    "Platform_21_Enterprise_Gap_Closure_Master.md",
    "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md",
    "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md",
}


class ValidationError(Exception):
    pass


def strip_marker_block(text: str, start: str, end: str) -> str:
    if start not in text or end not in text:
        return text
    before, rest = text.split(start, 1)
    _, after = rest.split(end, 1)
    return before + after


def clean_cell(value: str) -> str:
    return value.strip().strip("`").strip()


class MotherReferenceValidator:
    def __init__(self, docs_root: Path, repo_root: Path | None = None):
        self.docs_root = docs_root.resolve()
        self.repo_root = (repo_root or docs_root.parent).resolve()
        self.manifest_path = self.docs_root / "mother_reference_manifest.json"
        self.errors: list[str] = []

    def fail(self, message: str) -> None:
        self.errors.append(message)

    def load_manifest(self) -> dict[str, Any]:
        if not self.manifest_path.is_file():
            raise ValidationError(f"missing manifest: {self.manifest_path}")
        try:
            data = json.loads(self.manifest_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            raise ValidationError(f"invalid JSON in manifest: {exc}") from exc
        if not isinstance(data, dict):
            raise ValidationError("manifest root must be an object")
        return data

    def validate_manifest_shape(self, manifest: dict[str, Any]) -> None:
        required = [
            "schemaVersion",
            "motherReferenceVersion",
            "globalRequirementIndex",
            "globalProductRequirementIndex",
            "documents",
            "legacyIdExceptions",
        ]
        for key in required:
            if key not in manifest:
                self.fail(f"manifest missing required field: {key}")
        if self.errors:
            return
        if manifest["schemaVersion"] != SCHEMA_VERSION:
            self.fail(
                f"wrong schemaVersion: {manifest['schemaVersion']!r} "
                f"(expected {SCHEMA_VERSION!r})"
            )
        if not isinstance(manifest["documents"], list) or not manifest["documents"]:
            self.fail("manifest.documents must be a non-empty list")
            return
        if not isinstance(manifest["legacyIdExceptions"], list):
            self.fail("manifest.legacyIdExceptions must be a list")
            return

        files: list[str] = []
        ordinals: list[Any] = []
        doc_ids: list[Any] = []
        for i, doc in enumerate(manifest["documents"]):
            if not isinstance(doc, dict):
                self.fail(f"documents[{i}] must be an object")
                continue
            for key in (
                "file",
                "ordinal",
                "kind",
                "governingRole",
                "normalizationState",
            ):
                if key not in doc:
                    self.fail(f"documents[{i}] missing required field: {key}")
            if "expectedDocumentId" not in doc:
                self.fail(f"documents[{i}] missing required field: expectedDocumentId")
            if "file" in doc:
                files.append(doc["file"])
            if "ordinal" in doc:
                ordinals.append(doc["ordinal"])
            if "expectedDocumentId" in doc and doc["expectedDocumentId"] is not None:
                doc_ids.append(doc["expectedDocumentId"])
            state = doc.get("normalizationState")
            if state not in ALLOWED_NORMALIZATION:
                self.fail(
                    f"documents[{i}] unknown normalizationState: {state!r} "
                    f"(allowed: {sorted(ALLOWED_NORMALIZATION)})"
                )

        for name, count in Counter(files).items():
            if count > 1:
                self.fail(f"duplicate manifest file: {name}")
        for ord_, count in Counter(ordinals).items():
            if count > 1:
                self.fail(f"duplicate manifest ordinal: {ord_}")
        for did, count in Counter(doc_ids).items():
            if count > 1:
                self.fail(f"duplicate expected Document ID: {did}")

        if len(files) != CONSTITUTIONAL_AUTHORITATIVE_COUNT:
            self.fail(
                f"manifest must contain exactly {CONSTITUTIONAL_AUTHORITATIVE_COUNT} "
                f"authoritative entries (got {len(files)})"
            )

    def validate_constitutional_set(self) -> dict[str, Path]:
        """Enforce AGENTS + exactly one Platform_00..23 independently of the manifest."""
        paths: dict[str, Path] = {}
        agents = self.docs_root / "AGENTS.md"
        if not agents.is_file():
            self.fail("missing authoritative file: AGENTS.md")
        else:
            paths["AGENTS.md"] = agents

        on_disk = sorted(self.docs_root.glob("Platform_*.md"))
        by_num: dict[str, list[str]] = {n: [] for n in CONSTITUTIONAL_PLATFORM_NUMBERS}
        extras: list[str] = []
        for path in on_disk:
            name = path.name
            m = PLATFORM_NUM_RE.match(name)
            if not m:
                self.fail(f"unexpected numbered document: {name}")
                continue
            num = m.group(1)
            if int(num) >= 24:
                self.fail(f"Platform_24+ without deliberate allocation: {name}")
                continue
            if num not in by_num:
                extras.append(name)
                continue
            by_num[num].append(name)

        for num in CONSTITUTIONAL_PLATFORM_NUMBERS:
            names = by_num[num]
            if not names:
                self.fail(
                    f"missing authoritative Platform_{num} Mother Reference "
                    f"(constitutional set requires Platform_00 through Platform_23)"
                )
            elif len(names) > 1:
                self.fail(
                    f"duplicate Platform number {num}: {' and '.join(names)}"
                )
            else:
                paths[names[0]] = self.docs_root / names[0]

        for name in extras:
            self.fail(f"unexpected numbered document: {name}")

        if len(paths) != CONSTITUTIONAL_AUTHORITATIVE_COUNT and "AGENTS.md" in paths:
            # Count only when AGENTS present; missing Platform numbers already reported.
            present_platforms = sum(
                1 for k in paths if k.startswith("Platform_")
            )
            if present_platforms != len(CONSTITUTIONAL_PLATFORM_NUMBERS):
                self.fail(
                    f"constitutional set requires exactly "
                    f"{len(CONSTITUTIONAL_PLATFORM_NUMBERS)} numbered Mother References "
                    f"Platform_00..23 (found {present_platforms})"
                )

        return paths

    def validate_manifest_matches_constitutional(
        self, manifest: dict[str, Any], constitutional: dict[str, Path]
    ) -> None:
        declared = [d.get("file") for d in manifest.get("documents", []) if isinstance(d, dict)]
        declared_set = {f for f in declared if isinstance(f, str)}
        constitutional_set = set(constitutional)

        if "AGENTS.md" not in declared_set:
            self.fail("manifest missing required authoritative entry: AGENTS.md")

        for num in CONSTITUTIONAL_PLATFORM_NUMBERS:
            matches = [f for f in declared_set if f.startswith(f"Platform_{num}_")]
            if not matches:
                self.fail(
                    f"manifest missing required Platform_{num} entry "
                    f"(constitutional set cannot be redefined by deleting manifest rows)"
                )
            elif len(matches) > 1:
                self.fail(f"manifest has duplicate Platform_{num} entries: {matches}")

        for name in sorted(declared_set - constitutional_set):
            # Manifest claims a file not in the constitutional on-disk set.
            self.fail(f"manifest entry not in constitutional set: {name}")

        for name in sorted(constitutional_set - declared_set):
            self.fail(f"constitutional file absent from manifest: {name}")

        if len(declared_set) != CONSTITUTIONAL_AUTHORITATIVE_COUNT:
            self.fail(
                f"manifest must contain exactly {CONSTITUTIONAL_AUTHORITATIVE_COUNT} "
                f"authoritative entries matching AGENTS + Platform_00..23"
            )

    def validate_document_set(
        self, manifest: dict[str, Any], constitutional: dict[str, Path]
    ) -> dict[str, Path]:
        paths = dict(constitutional)
        for doc in manifest.get("documents", []):
            if not isinstance(doc, dict) or "file" not in doc:
                continue
            rel = doc["file"]
            path = self.docs_root / rel
            if not path.is_file():
                self.fail(f"missing authoritative file: {rel}")
                continue
            paths[rel] = path
            m = PLATFORM_NUM_RE.match(rel)
            if m and int(m.group(1)) >= 24:
                self.fail(f"Platform_24+ without deliberate allocation: {rel}")
        return paths

    def validate_headers(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        version = manifest["motherReferenceVersion"]
        for doc in manifest["documents"]:
            rel = doc["file"]
            path = paths.get(rel)
            if path is None:
                continue
            text = path.read_text(encoding="utf-8")
            expected_id = doc.get("expectedDocumentId")
            if expected_id is None:
                continue
            m = DOC_ID_RE.search(text)
            if not m:
                self.fail(f"{rel}: missing Document ID header")
            elif m.group(1).rstrip("*") != expected_id:
                self.fail(
                    f"{rel}: wrong Document ID {m.group(1)!r} "
                    f"(expected {expected_id!r})"
                )
            vm = VERSION_RE.search(text)
            if not vm:
                self.fail(f"{rel}: missing Version header")
            elif vm.group(1).rstrip("*") != version:
                self.fail(
                    f"{rel}: version mismatch {vm.group(1)!r} "
                    f"(expected {version!r})"
                )

    def _parse_registry_rows(
        self, text: str, start: str, end: str, kind: str
    ) -> list[dict[str, str]]:
        if start not in text or end not in text:
            self.fail(f"Platform_08 missing {kind} registry markers")
            return []
        block = text.split(start, 1)[1].split(end, 1)[0]
        expected_cols = REQ_COLUMNS if kind == "REQ" else PRD_COLUMNS
        header_names = REQ_HEADERS if kind == "REQ" else PRD_HEADERS
        rows: list[dict[str, str]] = []
        for line in block.splitlines():
            if not line.strip().startswith("|"):
                continue
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            if not cells:
                continue
            first = clean_cell(cells[0])
            if first in header_names or first in {
                "Requirement ID",
                "PRD ID",
                "ID",
            }:
                continue
            if set(first) <= {"-", ":"}:
                continue
            if len(cells) != expected_cols:
                self.fail(
                    f"malformed {kind} registry row column count: "
                    f"expected {expected_cols}, got {len(cells)} ({first})"
                )
                continue
            cleaned = [clean_cell(c) for c in cells]
            if kind == "REQ":
                rid = cleaned[0]
                if not (REQ_ID_RE.fullmatch(rid) or re.fullmatch(r"OSS-\d{3,}", rid)):
                    continue
                rows.append(
                    {
                        "id": rid,
                        "summary": cleaned[1],
                        "owner": cleaned[2],
                        "locator": cleaned[3],
                        "trace": cleaned[4],
                        "maturity": cleaned[5],
                        "evidence": cleaned[6],
                    }
                )
            else:
                rid = cleaned[0]
                if not PRD_ID_RE.fullmatch(rid):
                    continue
                rows.append(
                    {
                        "id": rid,
                        "summary": cleaned[1],
                        "owner": cleaned[2],
                        "locator": cleaned[3],
                        "related": cleaned[4],
                        "maturity": cleaned[5],
                    }
                )
        return rows

    def _id_occurs_in_text(self, rid: str, text: str) -> bool:
        if rid == "OSS-008":
            return bool(re.search(r"\|\s*OSS-008\s*\|", text)) or bool(
                re.search(r"\bOSS-008\b", text)
            )
        if rid.startswith("REQ-"):
            return rid in REQ_ID_RE.findall(text)
        if rid.startswith("PRD-"):
            return rid in PRD_ID_RE.findall(text)
        return rid in text

    def _owner_text_without_primary_registry(
        self, owner: str, paths: dict[str, Path], kind: str
    ) -> str | None:
        path = paths.get(owner) or (self.docs_root / owner)
        if not path.is_file():
            return None
        text = path.read_text(encoding="utf-8")
        if kind == "REQ":
            text = strip_marker_block(text, REQ_MARKER_START, REQ_MARKER_END)
        else:
            text = strip_marker_block(text, PRD_MARKER_START, PRD_MARKER_END)
        return text

    def _validate_registry_owners(
        self,
        rows: list[dict[str, str]],
        kind: str,
        manifest: dict[str, Any],
        paths: dict[str, Path],
    ) -> None:
        manifest_files = {
            d["file"]
            for d in manifest.get("documents", [])
            if isinstance(d, dict) and "file" in d
        }
        for row in rows:
            rid = row["id"]
            owner = row["owner"]
            locator = row["locator"]
            if not locator:
                self.fail(f"{kind} {rid}: empty source locator")
            if not owner:
                self.fail(f"{kind} {rid}: empty owner")
                continue
            if owner not in manifest_files:
                self.fail(
                    f"{kind} {rid}: owner is not a valid Mother Reference filename: "
                    f"{owner}"
                )
                continue
            owner_path = paths.get(owner) or (self.docs_root / owner)
            if not owner_path.is_file():
                self.fail(f"{kind} {rid}: owner file does not exist: {owner}")
                continue
            owner_text = self._owner_text_without_primary_registry(owner, paths, kind)
            if owner_text is None or not self._id_occurs_in_text(rid, owner_text):
                self.fail(
                    f"{kind} {rid}: formal ID absent from declared owner "
                    f"outside primary registry: {owner}"
                )

    def validate_global_requirements(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        index_name = manifest["globalRequirementIndex"]
        index_path = paths.get(index_name) or (self.docs_root / index_name)
        if not index_path.is_file():
            self.fail(f"missing global RTM: {index_name}")
            return
        index_text = index_path.read_text(encoding="utf-8")
        rows = self._parse_registry_rows(
            index_text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
        )
        primary = [r["id"] for r in rows]
        for rid, count in Counter(primary).items():
            if count > 1:
                self.fail(f"duplicate primary REQ row: {rid}")

        legacy = {
            e["id"]
            for e in manifest.get("legacyIdExceptions", [])
            if isinstance(e, dict) and "id" in e
        }

        # Authoritative discovery EXCLUDES the primary REQ registry block so a
        # fabricated registry-only row cannot self-satisfy source presence.
        authoritative: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            if rel == index_name:
                text = strip_marker_block(text, REQ_MARKER_START, REQ_MARKER_END)
            for rid in REQ_ID_RE.findall(text):
                authoritative.add(rid)
            if rel == index_name and re.search(r"\|\s*OSS-008\s*\|", text):
                authoritative.add("OSS-008")

        primary_set = set(primary)
        for rid in sorted(authoritative):
            if rid not in primary_set:
                self.fail(
                    f"REQ in source but absent from global primary registry: {rid}"
                )
        for rid in sorted(primary_set - authoritative):
            if rid in legacy and rid in primary_set:
                self.fail(
                    f"legacy ID {rid} has no authoritative occurrence outside "
                    f"the primary REQ registry"
                )
                continue
            self.fail(
                f"unknown primary registry REQ with no authoritative source: {rid}"
            )

        for rid in primary:
            if REQ_ID_RE.fullmatch(rid):
                continue
            if rid in legacy:
                continue
            self.fail(f"invalid formal ID in primary REQ registry: {rid}")

        self._validate_registry_owners(rows, "REQ", manifest, paths)

    def validate_global_prds(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        index_name = manifest["globalProductRequirementIndex"]
        index_path = paths.get(index_name) or (self.docs_root / index_name)
        if not index_path.is_file():
            self.fail(f"missing global PRD index: {index_name}")
            return
        index_text = index_path.read_text(encoding="utf-8")
        rows = self._parse_registry_rows(
            index_text, PRD_MARKER_START, PRD_MARKER_END, "PRD"
        )
        primary = [r["id"] for r in rows]
        for rid, count in Counter(primary).items():
            if count > 1:
                self.fail(f"duplicate primary PRD row: {rid}")

        discovered: set[str] = set()
        for rel in PRD_SOURCE_FILES:
            path = paths.get(rel) or (self.docs_root / rel)
            if not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
            for rid in PRD_ID_RE.findall(text):
                discovered.add(rid)

        primary_set = set(primary)
        for rid in sorted(discovered):
            if rid not in primary_set:
                self.fail(
                    f"PRD in source but absent from global primary PRD registry: {rid}"
                )

        # Authoritative sources exclude the primary PRD registry block.
        authoritative: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            if rel == index_name:
                text = strip_marker_block(text, PRD_MARKER_START, PRD_MARKER_END)
            for rid in PRD_ID_RE.findall(text):
                authoritative.add(rid)

        for rid in sorted(primary_set):
            if rid not in authoritative:
                self.fail(
                    f"unknown primary registry PRD with no authoritative source / "
                    f"unknown owner/source file: {rid}"
                )

        self._validate_registry_owners(rows, "PRD", manifest, paths)

    def validate_legacy_and_malformed(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        legacy = {
            e["id"]
            for e in manifest.get("legacyIdExceptions", [])
            if isinstance(e, dict) and "id" in e
        }
        index_name = manifest["globalRequirementIndex"]
        index_path = paths.get(index_name)
        if index_path is None:
            return
        text = index_path.read_text(encoding="utf-8")
        rows = self._parse_registry_rows(
            text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
        )
        for row in rows:
            rid = row["id"]
            if REQ_ID_RE.fullmatch(rid):
                continue
            if rid in legacy:
                continue
            if FORMAL_ID_RE.fullmatch(rid) or re.fullmatch(r"OSS-\d{3,}", rid):
                self.fail(
                    f"unregistered malformed/legacy formal ID in primary registry: {rid}"
                )

    def validate_cross_references(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        on_disk = {p.name for p in self.docs_root.glob("Platform_*.md")}
        historical = set(manifest.get("historicalFilenameAllowlist") or [])

        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            cleaned_parts: list[str] = []
            in_fence = False
            for line in text.splitlines(keepends=True):
                if line.lstrip().startswith("```"):
                    in_fence = not in_fence
                    continue
                if not in_fence:
                    cleaned_parts.append(line)
            cleaned = "".join(cleaned_parts)
            refs = set(re.findall(r"`(Platform_\d{2}_[A-Za-z0-9_]+\.md)`", cleaned))
            refs |= set(
                re.findall(r"\]\((Platform_\d{2}_[A-Za-z0-9_]+\.md)\)", cleaned)
            )
            for ref in sorted(refs):
                # Exact filename must exist OR be an explicit historical allowlist entry.
                # Same Platform number with a different name is NOT sufficient.
                if ref in on_disk or ref in historical:
                    continue
                self.fail(f"{rel}: broken Platform_*.md reference: {ref}")

    def validate_normalized_structure(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        for doc in manifest["documents"]:
            if doc.get("normalizationState") != "NORMALIZED_V1":
                continue
            rel = doc["file"]
            path = paths.get(rel)
            if path is None:
                continue
            text = path.read_text(encoding="utf-8")
            # For Platform_00, ignore Appendix A transitional historical numbering.
            if rel.startswith("Platform_00"):
                appendix = text.find("## Appendix A")
                if appendix >= 0:
                    text = text[:appendix]
            nums = [int(m.group(1)) for m in TOP_SECTION_RE.finditer(text)]
            for n, count in Counter(nums).items():
                if count > 1:
                    self.fail(
                        f"{rel}: duplicate normalized top-level section number: {n}"
                    )

    def validate_maturity_language(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        banned = [
            re.compile(r"\bSPECIFIED\s*=\s*IMPLEMENTED\b"),
            re.compile(r"\bIMPLEMENTED\s*=\s*TESTED\b"),
            re.compile(r"\bTESTED\s*=\s*BENCHMARKED\b"),
            re.compile(r"\bBENCHMARKED\s*=\s*CERTIFIED\b"),
            re.compile(r"\bCERTIFIED\s*=\s*DEPLOYED\b"),
            re.compile(r"\bSPECIFIED\s*=>\s*IMPLEMENTED\b"),
        ]
        for doc in manifest["documents"]:
            if doc.get("normalizationState") != "NORMALIZED_V1":
                continue
            rel = doc["file"]
            path = paths.get(rel)
            if path is None:
                continue
            text = path.read_text(encoding="utf-8")
            if rel.startswith("Platform_00"):
                missing = [s for s in MATURITY_STATES if s not in text]
                if missing:
                    self.fail(
                        f"{rel}: missing canonical maturity vocabulary: {missing}"
                    )
                if not re.search(
                    r"SPECIFIED\s*!=\s*IMPLEMENTED\s*!=\s*TESTED\s*!=\s*"
                    r"BENCHMARKED\s*!=\s*CERTIFIED\s*!=\s*DEPLOYED",
                    text,
                ):
                    self.fail(
                        f"{rel}: missing canonical maturity inequality statement"
                    )
            for pat in banned:
                if pat.search(text):
                    self.fail(
                        f"{rel}: unsupported maturity equality spelling: {pat.pattern}"
                    )

    def validate(self) -> list[str]:
        self.errors = []
        constitutional = self.validate_constitutional_set()
        try:
            manifest = self.load_manifest()
        except ValidationError as exc:
            return self.errors + [str(exc)]
        self.validate_manifest_shape(manifest)
        if "documents" not in manifest or not isinstance(
            manifest.get("documents"), list
        ):
            return self.errors
        self.validate_manifest_matches_constitutional(manifest, constitutional)
        paths = self.validate_document_set(manifest, constitutional)
        self.validate_headers(manifest, paths)
        self.validate_global_requirements(manifest, paths)
        self.validate_global_prds(manifest, paths)
        self.validate_legacy_and_malformed(manifest, paths)
        self.validate_cross_references(manifest, paths)
        self.validate_normalized_structure(manifest, paths)
        self.validate_maturity_language(manifest, paths)
        return self.errors


def validate_docs_root(docs_root: Path) -> list[str]:
    return MotherReferenceValidator(docs_root).validate()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Validate Platform Mother Reference structural contracts"
    )
    parser.add_argument(
        "--docs-root",
        type=Path,
        default=None,
        help="Path to project_documentation (default: repository project_documentation)",
    )
    args = parser.parse_args(argv)
    if args.docs_root is None:
        docs_root = Path(__file__).resolve().parent.parent
    else:
        docs_root = args.docs_root
    errors = validate_docs_root(docs_root)
    if errors:
        print("MOTHER REFERENCE VALIDATION FAILED", file=sys.stderr)
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        return 1
    print("MOTHER REFERENCE VALIDATION PASSED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
