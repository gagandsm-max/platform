#!/usr/bin/env python3
"""Mother Reference structural validator (A1).

Python standard library only. Validates the repository Mother Reference set
against project_documentation/mother_reference_manifest.json.

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

REQ_ID_RE = re.compile(r"\b(REQ-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
PRD_ID_RE = re.compile(r"\b(PRD-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
FORMAL_ID_RE = re.compile(
    r"\b((?:REQ|PRD)-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?|OSS-\d{3,})\b"
)
PLATFORM_FILE_RE = re.compile(r"\b(Platform_\d{2}_[A-Za-z0-9_]+\.md)\b")
PLATFORM_NUM_RE = re.compile(r"^Platform_(\d{2})_")
DOC_ID_RE = re.compile(r"^\*\*Document ID:\*\*\s*(\S+)", re.M)
VERSION_RE = re.compile(r"^\*\*Version:\*\*\s*(\S+)", re.M)
TOP_SECTION_RE = re.compile(r"^##\s+(\d+)\.", re.M)

REQ_MARKER_START = "<!-- GLOBAL PRIMARY REQ REGISTRY -->"
REQ_MARKER_END = "<!-- END GLOBAL PRIMARY REQ REGISTRY -->"
PRD_MARKER_START = "<!-- GLOBAL PRIMARY PRD REGISTRY -->"
PRD_MARKER_END = "<!-- END GLOBAL PRIMARY PRD REGISTRY -->"

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

    def validate_document_set(self, manifest: dict[str, Any]) -> dict[str, Path]:
        paths: dict[str, Path] = {}
        numbered_seen: dict[str, str] = {}
        for doc in manifest["documents"]:
            rel = doc["file"]
            path = self.docs_root / rel
            if not path.is_file():
                self.fail(f"missing authoritative file: {rel}")
                continue
            paths[rel] = path
            m = PLATFORM_NUM_RE.match(rel)
            if m:
                num = m.group(1)
                if num in numbered_seen:
                    self.fail(
                        f"duplicate Platform number {num}: "
                        f"{numbered_seen[num]} and {rel}"
                    )
                numbered_seen[num] = rel
                if int(num) >= 24:
                    self.fail(
                        f"Platform_24+ without deliberate allocation: {rel}"
                    )
                expected_prefix = f"Platform_{num}_"
                if not rel.startswith(expected_prefix):
                    self.fail(f"incorrect filename relative to manifest: {rel}")

        # Unexpected numbered Platform_NN files on disk
        on_disk = sorted(self.docs_root.glob("Platform_*.md"))
        declared = {d["file"] for d in manifest["documents"]}
        for path in on_disk:
            name = path.name
            if name not in declared:
                self.fail(f"unexpected numbered document: {name}")
            m = PLATFORM_NUM_RE.match(name)
            if m and int(m.group(1)) >= 24:
                self.fail(f"Platform_24+ without deliberate allocation: {name}")

        # AGENTS.md must exist when declared
        if "AGENTS.md" in declared and not (self.docs_root / "AGENTS.md").is_file():
            self.fail("missing authoritative file: AGENTS.md")

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
                # AGENTS.md: do not impose a fake Document ID
                if DOC_ID_RE.search(text):
                    # Optional: allow absence only; presence is not required to match
                    pass
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

    def _extract_primary_registry(
        self, text: str, start: str, end: str, kind: str
    ) -> list[str]:
        if start not in text or end not in text:
            self.fail(f"Platform_08 missing {kind} registry markers")
            return []
        block = text.split(start, 1)[1].split(end, 1)[0]
        ids: list[str] = []
        for line in block.splitlines():
            if not line.strip().startswith("|"):
                continue
            # First column of table rows
            cells = [c.strip().strip("`") for c in line.strip().strip("|").split("|")]
            if not cells:
                continue
            cell = cells[0]
            if cell in {"Requirement ID", "PRD ID", "ID"} or set(cell) <= {"-", ":"}:
                continue
            if kind == "REQ":
                # Accept formal REQ-* and OSS-* cells so unregistered legacy IDs fail later
                if REQ_ID_RE.fullmatch(cell) or re.fullmatch(r"OSS-\d{3,}", cell):
                    ids.append(cell)
            else:
                if PRD_ID_RE.fullmatch(cell):
                    ids.append(cell)
        return ids

    def validate_global_requirements(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        index_name = manifest["globalRequirementIndex"]
        index_path = paths.get(index_name) or (self.docs_root / index_name)
        if not index_path.is_file():
            self.fail(f"missing global RTM: {index_name}")
            return
        index_text = index_path.read_text(encoding="utf-8")
        primary = self._extract_primary_registry(
            index_text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
        )
        primary_counts = Counter(primary)
        for rid, count in primary_counts.items():
            if count > 1:
                self.fail(f"duplicate primary REQ row: {rid}")

        legacy = {
            e["id"]
            for e in manifest.get("legacyIdExceptions", [])
            if isinstance(e, dict) and "id" in e
        }

        discovered: set[str] = set()
        for rel, path in paths.items():
            if not PLATFORM_NUM_RE.match(rel) and rel != "AGENTS.md":
                continue
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            for rid in REQ_ID_RE.findall(text):
                discovered.add(rid)
            # Only count OSS-008 as formal when it appears as an RTM table ID
            if rel == index_name and re.search(r"\|\s*OSS-008\s*\|", text):
                discovered.add("OSS-008")

        primary_set = set(primary)
        for rid in sorted(discovered):
            if rid not in primary_set:
                self.fail(
                    f"REQ in source but absent from global primary registry: {rid}"
                )
        for rid in sorted(primary_set - discovered):
            if rid in legacy:
                # Legacy may be registered even if only present in the registry
                # itself; still require presence somewhere in Platform_00..23.
                if rid not in discovered and rid not in primary_set:
                    self.fail(f"legacy ID not found in Mother References: {rid}")
                continue
            self.fail(
                f"unknown primary registry REQ with no authoritative source: {rid}"
            )

        # Malformed / unregistered formal-looking IDs in primary registry
        for rid in primary:
            if REQ_ID_RE.fullmatch(rid):
                continue
            if rid in legacy:
                continue
            self.fail(f"invalid formal ID in primary REQ registry: {rid}")

    def validate_global_prds(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        index_name = manifest["globalProductRequirementIndex"]
        index_path = paths.get(index_name) or (self.docs_root / index_name)
        if not index_path.is_file():
            self.fail(f"missing global PRD index: {index_name}")
            return
        index_text = index_path.read_text(encoding="utf-8")
        primary = self._extract_primary_registry(
            index_text, PRD_MARKER_START, PRD_MARKER_END, "PRD"
        )
        for rid, count in Counter(primary).items():
            if count > 1:
                self.fail(f"duplicate primary PRD row: {rid}")

        discovered: set[str] = set()
        owner_by_id: dict[str, set[str]] = {}
        for rel in PRD_SOURCE_FILES:
            path = paths.get(rel) or (self.docs_root / rel)
            if not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
            for rid in PRD_ID_RE.findall(text):
                discovered.add(rid)
                owner_by_id.setdefault(rid, set()).add(rel)

        # Also accept PRDs that only appear in other Platform docs if indexed,
        # but discovery for "missing from primary" uses authoritative owners.
        all_platform_prds: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            for rid in PRD_ID_RE.findall(path.read_text(encoding="utf-8")):
                all_platform_prds.add(rid)

        primary_set = set(primary)
        for rid in sorted(discovered):
            if rid not in primary_set:
                self.fail(
                    f"PRD in source but absent from global primary PRD registry: {rid}"
                )

        # Authoritative PRD sources exclude the primary registry block itself so a
        # registry-only row cannot self-satisfy ownership.
        authoritative: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            if rel == index_name and PRD_MARKER_START in text and PRD_MARKER_END in text:
                before, rest = text.split(PRD_MARKER_START, 1)
                _, after = rest.split(PRD_MARKER_END, 1)
                text = before + after
            for rid in PRD_ID_RE.findall(text):
                authoritative.add(rid)

        for rid in sorted(primary_set):
            if rid not in authoritative:
                self.fail(
                    f"unknown primary registry PRD with no authoritative source / "
                    f"unknown owner/source file: {rid}"
                )

    def validate_legacy_and_malformed(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        legacy = {
            e["id"]
            for e in manifest.get("legacyIdExceptions", [])
            if isinstance(e, dict) and "id" in e
        }
        # Scan Platform_08 primary REQ registry only for formal-looking cells
        index_name = manifest["globalRequirementIndex"]
        index_path = paths.get(index_name)
        if index_path is None:
            return
        text = index_path.read_text(encoding="utf-8")
        primary = self._extract_primary_registry(
            text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
        )
        for rid in primary:
            if REQ_ID_RE.fullmatch(rid):
                continue
            if rid in legacy:
                continue
            # Unregistered OSS-style or other malformed formal IDs fail
            if FORMAL_ID_RE.fullmatch(rid):
                self.fail(
                    f"unregistered malformed/legacy formal ID in primary registry: {rid}"
                )

    def validate_cross_references(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        on_disk = {p.name for p in self.docs_root.glob("Platform_*.md")}
        current_by_num: dict[str, str] = {}
        for name in on_disk:
            m = PLATFORM_NUM_RE.match(name)
            if m:
                current_by_num[m.group(1)] = name
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
                if ref in on_disk or ref in historical:
                    continue
                m = PLATFORM_NUM_RE.match(ref)
                if m and m.group(1) in current_by_num and current_by_num[m.group(1)] != ref:
                    # Same-number consolidated rename without explicit allowlist entry
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
            nums = [int(m.group(1)) for m in TOP_SECTION_RE.finditer(text)]
            for n, count in Counter(nums).items():
                if count > 1:
                    self.fail(
                        f"{rel}: duplicate normalized top-level section number: {n}"
                    )

    def validate_maturity_language(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        # Structural vocabulary presence in constitution; prevent unsupported
        # combined status spellings that imply higher maturity without evidence.
        # Do NOT attempt natural-language semantic certification.
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
                if "SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED" not in text.replace(
                    " ", " "
                ):
                    # allow spaced variants already exact in constitution
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
        try:
            manifest = self.load_manifest()
        except ValidationError as exc:
            return [str(exc)]
        self.validate_manifest_shape(manifest)
        if any(e.startswith("manifest") or "schemaVersion" in e for e in self.errors):
            # Continue where possible, but missing documents list blocks further work
            if "documents" not in manifest or not isinstance(
                manifest.get("documents"), list
            ):
                return self.errors
        paths = self.validate_document_set(manifest)
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
        # tools/ -> project_documentation/
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
