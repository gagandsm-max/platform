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

# --- A1 pinned constitutional invariants (deliberate change control required) ---
CANONICAL_MOTHER_REFERENCE_VERSION = "3.2.2"
CANONICAL_GLOBAL_REQUIREMENT_INDEX = "Platform_08_Requirements_Traceability.md"
CANONICAL_GLOBAL_PRODUCT_REQUIREMENT_INDEX = (
    "Platform_08_Requirements_Traceability.md"
)
CANONICAL_ALLOWED_LEGACY_IDS = frozenset()
# A2.3: former formal OSS-008 migrated to REQ-OSS-008; no current formal legacy IDs.

CANONICAL_FILES: tuple[str, ...] = (
    "AGENTS.md",
    "Platform_00_Document_Index.md",
    "Platform_01_BRD.md",
    "Platform_02_PRD.md",
    "Platform_03_Architecture.md",
    "Platform_04_HLD.md",
    "Platform_05_LLD.md",
    "Platform_06_Dataflow.md",
    "Platform_07_OSS_Catalog.md",
    "Platform_08_Requirements_Traceability.md",
    "Platform_09_UI_UX_Master.md",
    "Platform_10_Protocol_Security_Connector_Master.md",
    "Platform_11_Sizing_Capacity_Storage_Master.md",
    "Platform_12_Decision_Source_Register.md",
    "Platform_13_Engineering_Development_and_Docker_Standards.md",
    "Platform_14_Test_Performance_and_Release_Quality.md",
    "Platform_15_CI_CD_Packaging_and_Distribution.md",
    "Platform_16_Installation_Upgrade_Rollback_and_Operations.md",
    "Platform_17_API_Event_and_Data_Model_Contracts.md",
    "Platform_18_Security_Threat_Model.md",
    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md",
    "Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md",
    "Platform_21_Enterprise_Gap_Closure_Master.md",
    "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md",
    "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md",
)
CANONICAL_FILE_SET = frozenset(CANONICAL_FILES)
CONSTITUTIONAL_AUTHORITATIVE_COUNT = len(CANONICAL_FILES)
CONSTITUTIONAL_PLATFORM_NUMBERS = tuple(f"{n:02d}" for n in range(0, 24))

CANONICAL_DOCUMENT_IDS: dict[str, str | None] = {
    "AGENTS.md": None,
    "Platform_00_Document_Index.md": "PLATFORM-INDEX-001",
    "Platform_01_BRD.md": "PLATFORM-BRD-001",
    "Platform_02_PRD.md": "PLATFORM-PRD-001",
    "Platform_03_Architecture.md": "PLATFORM-ARC-001",
    "Platform_04_HLD.md": "PLATFORM-HLD-001",
    "Platform_05_LLD.md": "PLATFORM-LLD-001",
    "Platform_06_Dataflow.md": "PLATFORM-DF-001",
    "Platform_07_OSS_Catalog.md": "PLATFORM-OSS-001",
    "Platform_08_Requirements_Traceability.md": "PLATFORM-RTM-001",
    "Platform_09_UI_UX_Master.md": "PLATFORM-UIUX-001",
    "Platform_10_Protocol_Security_Connector_Master.md": "PLATFORM-PROTOCOL-001",
    "Platform_11_Sizing_Capacity_Storage_Master.md": "PLATFORM-SIZING-001",
    "Platform_12_Decision_Source_Register.md": "PLATFORM-DECISION-001",
    "Platform_13_Engineering_Development_and_Docker_Standards.md": "PLATFORM-ENGDEV-001",
    "Platform_14_Test_Performance_and_Release_Quality.md": "PLATFORM-QUALITY-001",
    "Platform_15_CI_CD_Packaging_and_Distribution.md": "PLATFORM-RELEASEENG-001",
    "Platform_16_Installation_Upgrade_Rollback_and_Operations.md": "PLATFORM-OPSINSTALL-001",
    "Platform_17_API_Event_and_Data_Model_Contracts.md": "PLATFORM-CONTRACTDATA-001",
    "Platform_18_Security_Threat_Model.md": "PLATFORM-THREAT-001",
    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md": "PLATFORM-INTELVALUE-001",
    "Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md": "PLATFORM-ONBOARDING-001",
    "Platform_21_Enterprise_Gap_Closure_Master.md": "PLATFORM-EGC-001",
    "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md": "PLATFORM-MIA-001",
    "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md": "PLATFORM-EMC-001",
}

CANONICAL_NORMALIZATION: dict[str, str] = {
    "AGENTS.md": "TRANSITIONAL_LEGACY",
    "Platform_00_Document_Index.md": "NORMALIZED_V1",
    "Platform_01_BRD.md": "NORMALIZED_V1",
    "Platform_02_PRD.md": "NORMALIZED_V1",
    "Platform_03_Architecture.md": "NORMALIZED_V1",
    "Platform_04_HLD.md": "NORMALIZED_V1",
    "Platform_05_LLD.md": "NORMALIZED_V1",
    "Platform_06_Dataflow.md": "NORMALIZED_V1",
    "Platform_07_OSS_Catalog.md": "NORMALIZED_V1",
    "Platform_08_Requirements_Traceability.md": "NORMALIZED_V1",
    "Platform_09_UI_UX_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_10_Protocol_Security_Connector_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_11_Sizing_Capacity_Storage_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_12_Decision_Source_Register.md": "TRANSITIONAL_LEGACY",
    "Platform_13_Engineering_Development_and_Docker_Standards.md": "TRANSITIONAL_LEGACY",
    "Platform_14_Test_Performance_and_Release_Quality.md": "TRANSITIONAL_LEGACY",
    "Platform_15_CI_CD_Packaging_and_Distribution.md": "TRANSITIONAL_LEGACY",
    "Platform_16_Installation_Upgrade_Rollback_and_Operations.md": "TRANSITIONAL_LEGACY",
    "Platform_17_API_Event_and_Data_Model_Contracts.md": "TRANSITIONAL_LEGACY",
    "Platform_18_Security_Threat_Model.md": "TRANSITIONAL_LEGACY",
    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md": "TRANSITIONAL_LEGACY",
    "Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_21_Enterprise_Gap_Closure_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md": "TRANSITIONAL_LEGACY",
    "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md": "TRANSITIONAL_LEGACY",
}

REQ_ID_RE = re.compile(r"\b(REQ-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
PRD_ID_RE = re.compile(r"\b(PRD-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)\b")
FORMAL_ID_RE = re.compile(
    r"\b((?:REQ|PRD)-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?|OSS-\d{3,})\b"
)
PLATFORM_NUM_RE = re.compile(r"^Platform_(\d{2})_")
DOC_ID_RE = re.compile(r"^\*\*Document ID:\*\*\s*(\S+)", re.M)
VERSION_RE = re.compile(r"^\*\*Version:\*\*\s*(\S+)", re.M)
TOP_SECTION_RE = re.compile(r"^##\s+(\d+)\.", re.M)
# Numeric ### / #### headings that claim hierarchy, e.g. ### 12.1 or #### 11.1.1
NUMERIC_SUBSECTION_RE = re.compile(
    r"^(#{3,6})\s+(\d+)((?:\.\d+)+)\s+\S", re.M
)

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
            "historicalFormalIdMigrations",
        ]
        for key in required:
            if key not in manifest:
                self.fail(f"manifest missing required field: {key}")
        if any("missing required field" in e for e in self.errors):
            return
        if manifest["schemaVersion"] != SCHEMA_VERSION:
            self.fail(
                f"wrong schemaVersion: {manifest['schemaVersion']!r} "
                f"(expected {SCHEMA_VERSION!r})"
            )
        if not isinstance(manifest["motherReferenceVersion"], str):
            self.fail("manifest.motherReferenceVersion must be a string")
        if not isinstance(manifest["globalRequirementIndex"], str):
            self.fail("manifest.globalRequirementIndex must be a string")
        if not isinstance(manifest["globalProductRequirementIndex"], str):
            self.fail("manifest.globalProductRequirementIndex must be a string")
        if not isinstance(manifest["documents"], list) or not manifest["documents"]:
            self.fail("manifest.documents must be a non-empty list")
            return
        if not isinstance(manifest["legacyIdExceptions"], list):
            self.fail("manifest.legacyIdExceptions must be a list")
            return
        if not isinstance(manifest["historicalFormalIdMigrations"], list):
            self.fail("historicalFormalIdMigrations must be a list")
            return
        if "historicalFilenameAllowlist" in manifest:
            hfa = manifest["historicalFilenameAllowlist"]
            if not isinstance(hfa, list) or not all(isinstance(x, str) for x in hfa):
                self.fail("historicalFilenameAllowlist must be a list of strings")

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
            if "file" in doc and not isinstance(doc["file"], str):
                self.fail(f"documents[{i}].file must be a string")
            if "kind" in doc and not isinstance(doc["kind"], str):
                self.fail(f"documents[{i}].kind must be a string")
            if "governingRole" in doc and not isinstance(doc["governingRole"], str):
                self.fail(f"documents[{i}].governingRole must be a string")
            if "normalizationState" in doc and not isinstance(
                doc["normalizationState"], str
            ):
                self.fail(f"documents[{i}].normalizationState must be a string")
            if "ordinal" in doc and not isinstance(doc["ordinal"], int):
                self.fail(f"documents[{i}].ordinal must be an integer")
            eid = doc.get("expectedDocumentId")
            if eid is not None and not isinstance(eid, str):
                self.fail(f"documents[{i}].expectedDocumentId must be string or null")
            if "file" in doc and isinstance(doc["file"], str):
                files.append(doc["file"])
            if "ordinal" in doc:
                ordinals.append(doc["ordinal"])
            if eid is not None:
                doc_ids.append(eid)
            state = doc.get("normalizationState")
            if isinstance(state, str) and state not in ALLOWED_NORMALIZATION:
                self.fail(
                    f"documents[{i}] unknown normalizationState: {state!r} "
                    f"(allowed: {sorted(ALLOWED_NORMALIZATION)})"
                )

        for j, exc in enumerate(manifest["legacyIdExceptions"]):
            if not isinstance(exc, dict):
                self.fail(f"legacyIdExceptions[{j}] must be an object")
                continue
            for key in ("id", "kind", "reason"):
                if key not in exc or not isinstance(exc[key], str):
                    self.fail(
                        f"legacyIdExceptions[{j}].{key} must be a present string"
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
        """Enforce exact A1 filenames independently of the editable manifest."""
        paths: dict[str, Path] = {}
        for name in CANONICAL_FILES:
            path = self.docs_root / name
            if not path.is_file():
                self.fail(f"missing authoritative file: {name}")
            else:
                paths[name] = path

        on_disk = {p.name for p in self.docs_root.glob("Platform_*.md")}
        on_disk |= (
            {"AGENTS.md"} if (self.docs_root / "AGENTS.md").is_file() else set()
        )
        for name in sorted(on_disk - CANONICAL_FILE_SET):
            m = PLATFORM_NUM_RE.match(name)
            if m and int(m.group(1)) >= 24:
                self.fail(f"Platform_24+ without deliberate allocation: {name}")
            else:
                self.fail(f"unexpected numbered document: {name}")

        # Duplicate Platform numbers among unexpected/extra files already covered;
        # also detect two files for same number if one is canonical and one not.
        by_num: dict[str, list[str]] = {}
        for name in on_disk:
            m = PLATFORM_NUM_RE.match(name)
            if m:
                by_num.setdefault(m.group(1), []).append(name)
        for num, names in by_num.items():
            if len(names) > 1:
                self.fail(f"duplicate Platform number {num}: {' and '.join(sorted(names))}")

        return paths

    def validate_manifest_matches_constitutional(
        self, manifest: dict[str, Any], constitutional: dict[str, Path]
    ) -> None:
        declared = [
            d.get("file")
            for d in manifest.get("documents", [])
            if isinstance(d, dict)
        ]
        declared_set = {f for f in declared if isinstance(f, str)}
        if declared_set != CANONICAL_FILE_SET:
            for name in sorted(CANONICAL_FILE_SET - declared_set):
                self.fail(f"manifest missing required canonical file: {name}")
            for name in sorted(declared_set - CANONICAL_FILE_SET):
                self.fail(f"manifest entry not in canonical Mother Reference set: {name}")
        if set(constitutional) != CANONICAL_FILE_SET:
            # constitutional set already reported missing files; keep alignment check.
            for name in sorted(CANONICAL_FILE_SET - set(constitutional)):
                if f"missing authoritative file: {name}" not in self.errors:
                    self.fail(f"constitutional file absent from on-disk set: {name}")

    def validate_pinned_invariants(self, manifest: dict[str, Any]) -> None:
        if manifest.get("motherReferenceVersion") != CANONICAL_MOTHER_REFERENCE_VERSION:
            self.fail(
                f"manifest motherReferenceVersion {manifest.get('motherReferenceVersion')!r} "
                f"does not match pinned constitutional version "
                f"{CANONICAL_MOTHER_REFERENCE_VERSION!r}"
            )
        if (
            manifest.get("globalRequirementIndex")
            != CANONICAL_GLOBAL_REQUIREMENT_INDEX
        ):
            self.fail(
                f"globalRequirementIndex redirect forbidden: "
                f"{manifest.get('globalRequirementIndex')!r} "
                f"(pinned to {CANONICAL_GLOBAL_REQUIREMENT_INDEX!r})"
            )
        if (
            manifest.get("globalProductRequirementIndex")
            != CANONICAL_GLOBAL_PRODUCT_REQUIREMENT_INDEX
        ):
            self.fail(
                f"globalProductRequirementIndex redirect forbidden: "
                f"{manifest.get('globalProductRequirementIndex')!r} "
                f"(pinned to {CANONICAL_GLOBAL_PRODUCT_REQUIREMENT_INDEX!r})"
            )

        by_file = {
            d["file"]: d
            for d in manifest.get("documents", [])
            if isinstance(d, dict) and isinstance(d.get("file"), str)
        }
        for fname, expected_id in CANONICAL_DOCUMENT_IDS.items():
            doc = by_file.get(fname)
            if doc is None:
                continue
            actual = doc.get("expectedDocumentId")
            if actual != expected_id:
                self.fail(
                    f"{fname}: manifest expectedDocumentId {actual!r} "
                    f"does not match pinned Document ID {expected_id!r}"
                )
            state = doc.get("normalizationState")
            pinned_state = CANONICAL_NORMALIZATION[fname]
            if state != pinned_state:
                self.fail(
                    f"{fname}: normalizationState {state!r} does not match "
                    f"pinned A1 baseline {pinned_state!r}"
                )

        legacy_ids = set()
        for exc in manifest.get("legacyIdExceptions", []):
            if isinstance(exc, dict) and isinstance(exc.get("id"), str):
                legacy_ids.add(exc["id"])
        if legacy_ids != CANONICAL_ALLOWED_LEGACY_IDS:
            for extra in sorted(legacy_ids - CANONICAL_ALLOWED_LEGACY_IDS):
                self.fail(
                    f"unapproved legacy formal-ID exception: {extra} "
                    f"(current canonical formal registry permits no legacy IDs; "
                    f"former formal OSS-008 migrated to REQ-OSS-008 in A2.3)"
                )
            for missing in sorted(CANONICAL_ALLOWED_LEGACY_IDS - legacy_ids):
                self.fail(f"missing required legacy formal-ID exception: {missing}")

        if "historicalFormalIdMigrations" not in manifest:
            self.fail(
                "manifest missing required field: historicalFormalIdMigrations "
                "(A2.3 requires permanent OSS-008 → REQ-OSS-008 provenance)"
            )
            return
        migrations = manifest["historicalFormalIdMigrations"]
        if not isinstance(migrations, list):
            self.fail("historicalFormalIdMigrations must be a list")
            return
        if not migrations:
            self.fail(
                "historicalFormalIdMigrations must not be empty "
                "(required OSS-008 → REQ-OSS-008 historical migration)"
            )
            return
        found = False
        for i, mig in enumerate(migrations):
            if not isinstance(mig, dict):
                self.fail(f"historicalFormalIdMigrations[{i}] must be an object")
                continue
            for key in ("oldId", "newId", "reason", "status"):
                if key not in mig or not isinstance(mig[key], str):
                    self.fail(
                        f"historicalFormalIdMigrations[{i}].{key} "
                        f"must be a present string"
                    )
            if mig.get("oldId") == "OSS-008" and mig.get("newId") == "REQ-OSS-008":
                found = True
                kind = str(mig.get("kind", ""))
                if kind and kind != "formal_requirement_id_migration":
                    self.fail(
                        "historicalFormalIdMigrations OSS-008→REQ-OSS-008 "
                        "kind must be formal_requirement_id_migration "
                        f"(got {kind!r})"
                    )
                reason = str(mig.get("reason", "")).strip()
                if not reason:
                    self.fail(
                        "historicalFormalIdMigrations OSS-008→REQ-OSS-008 "
                        "reason must be a non-empty string"
                    )
                elif not re.search(
                    r"(?i)namespace|collision|catalogue|catalog",
                    reason,
                ):
                    self.fail(
                        "historicalFormalIdMigrations OSS-008→REQ-OSS-008 "
                        "reason must identify the namespace migration context"
                    )
                status = str(mig.get("status", ""))
                if "migrat" not in status.lower() and "historical" not in status.lower():
                    self.fail(
                        "historicalFormalIdMigrations OSS-008→REQ-OSS-008 "
                        "status must indicate migrated/historical"
                    )
        if not found:
            self.fail(
                "historicalFormalIdMigrations must record OSS-008 → REQ-OSS-008"
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
        for fname in CANONICAL_FILES:
            if fname == "AGENTS.md":
                continue
            path = paths.get(fname)
            if path is None or not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
            pinned_id = CANONICAL_DOCUMENT_IDS[fname]
            m = DOC_ID_RE.search(text)
            if not m:
                self.fail(f"{fname}: missing Document ID header")
            else:
                header_id = m.group(1).rstrip("*")
                if header_id != pinned_id:
                    self.fail(
                        f"{fname}: wrong Document ID {header_id!r} "
                        f"(pinned expected {pinned_id!r})"
                    )
            vm = VERSION_RE.search(text)
            if not vm:
                self.fail(f"{fname}: missing Version header")
            elif vm.group(1).rstrip("*") != CANONICAL_MOTHER_REFERENCE_VERSION:
                self.fail(
                    f"{fname}: version mismatch {vm.group(1)!r} "
                    f"(pinned expected {CANONICAL_MOTHER_REFERENCE_VERSION!r})"
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
        paths: dict[str, Path],
    ) -> None:
        for row in rows:
            rid = row["id"]
            owner = row["owner"]
            locator = row["locator"]
            if not locator:
                self.fail(f"{kind} {rid}: empty source locator")
            if not owner:
                self.fail(f"{kind} {rid}: empty owner")
                continue
            if owner not in CANONICAL_FILE_SET:
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
        index_name = CANONICAL_GLOBAL_REQUIREMENT_INDEX
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

        legacy = set(CANONICAL_ALLOWED_LEGACY_IDS)

        authoritative: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            if rel == index_name:
                text = strip_marker_block(text, REQ_MARKER_START, REQ_MARKER_END)
            for rid in REQ_ID_RE.findall(text):
                authoritative.add(rid)

        primary_set = set(primary)
        if "OSS-008" in primary_set:
            self.fail(
                "Platform_08 primary REQ registry must not contain current canonical "
                "formal ID OSS-008 (migrated to REQ-OSS-008; catalogue OSS-008 remains "
                "ClickHouse in Platform_07)"
            )
        if "REQ-OSS-008" not in primary_set:
            self.fail(
                "Platform_08 primary REQ registry missing REQ-OSS-008 "
                "(air-gap / no runtime internet dependency install)"
            )
        for rid in sorted(authoritative):
            if rid not in primary_set:
                self.fail(
                    f"REQ in source but absent from global primary registry: {rid}"
                )
        for rid in sorted(primary_set - authoritative):
            if rid in legacy:
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

        self._validate_registry_owners(rows, "REQ", paths)

    def validate_global_prds(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        index_name = CANONICAL_GLOBAL_PRODUCT_REQUIREMENT_INDEX
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

        # Global PRD discovery across ALL numbered Mother References.
        # Primary PRD registry block cannot self-satisfy discovery.
        discovered: set[str] = set()
        for rel, path in paths.items():
            if not rel.startswith("Platform_"):
                continue
            text = path.read_text(encoding="utf-8")
            if rel == index_name:
                text = strip_marker_block(text, PRD_MARKER_START, PRD_MARKER_END)
            for rid in PRD_ID_RE.findall(text):
                discovered.add(rid)

        primary_set = set(primary)
        for rid in sorted(discovered):
            if rid not in primary_set:
                self.fail(
                    f"PRD in source but absent from global primary PRD registry: {rid}"
                )
        for rid in sorted(primary_set - discovered):
            self.fail(
                f"unknown primary registry PRD with no authoritative source / "
                f"unknown owner/source file: {rid}"
            )

        self._validate_registry_owners(rows, "PRD", paths)

    def validate_legacy_and_malformed(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        legacy = set(CANONICAL_ALLOWED_LEGACY_IDS)
        index_name = CANONICAL_GLOBAL_REQUIREMENT_INDEX
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
                if ref in on_disk or ref in historical:
                    continue
                self.fail(f"{rel}: broken Platform_*.md reference: {ref}")

    def validate_normalized_structure(
        self, paths: dict[str, Path]
    ) -> None:
        for rel, state in CANONICAL_NORMALIZATION.items():
            if state != "NORMALIZED_V1":
                continue
            path = paths.get(rel)
            if path is None:
                continue
            text = path.read_text(encoding="utf-8")
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
            self._validate_numeric_subsection_hierarchy(rel, text)

    def _validate_numeric_subsection_hierarchy(
        self, rel: str, text: str
    ) -> None:
        """For NORMALIZED_V1 docs, numeric ###/#### headings must belong to
        the current ## N parent. Descriptive unnumbered ### headings are OK.
        TRANSITIONAL_LEGACY documents are not checked here.

        Also rejects bare numeric-looking subsection tokens that lack a
        hierarchical suffix (e.g. ``### 44 Description``), and rejects
        temporary migration markers inside headings (e.g. ``__TMP29__``).
        """
        current_top: int | None = None
        for line in text.splitlines():
            # Any markdown heading (## through ######) for TMP-marker guard.
            heading = re.match(r"^(#{2,6})\s+(.+)$", line)
            if heading:
                title = heading.group(2)
                if re.search(r"(?:__TMP\d+__|\*\*TMP\d+\*\*)", title):
                    self.fail(
                        f"{rel}: temporary migration marker in heading: "
                        f"{line.strip()[:80]}"
                    )
            top_match = re.match(r"^##\s+(\d+)\.", line)
            if top_match:
                current_top = int(top_match.group(1))
                continue
            # Bare numeric parent token without hierarchical suffix:
            # ### 44 Description  /  #### 44 Nested detail
            bare = re.match(r"^(#{3,6})\s+(\d+)\s+\S", line)
            if bare and not re.match(r"^(#{3,6})\s+\d+(?:\.\d+)+\s+\S", line):
                self.fail(
                    f"{rel}: malformed bare-numeric subsection heading "
                    f"(missing hierarchical suffix): {line.strip()[:80]}"
                )
                continue
            sub = re.match(r"^(#{3,6})\s+(\d+)((?:\.\d+)+)\s+\S", line)
            if not sub:
                continue
            if current_top is None:
                self.fail(
                    f"{rel}: numeric subsection before any top-level section: "
                    f"{line.strip()[:80]}"
                )
                continue
            claimed_top = int(sub.group(2))
            if claimed_top != current_top:
                self.fail(
                    f"{rel}: numeric subsection {sub.group(2)}{sub.group(3)} "
                    f"under ## {current_top} (parent mismatch): "
                    f"{line.strip()[:80]}"
                )

    def validate_maturity_language(self, paths: dict[str, Path]) -> None:
        banned = [
            re.compile(r"\bSPECIFIED\s*=\s*IMPLEMENTED\b"),
            re.compile(r"\bIMPLEMENTED\s*=\s*TESTED\b"),
            re.compile(r"\bTESTED\s*=\s*BENCHMARKED\b"),
            re.compile(r"\bBENCHMARKED\s*=\s*CERTIFIED\b"),
            re.compile(r"\bCERTIFIED\s*=\s*DEPLOYED\b"),
            re.compile(r"\bSPECIFIED\s*=>\s*IMPLEMENTED\b"),
        ]
        for rel, state in CANONICAL_NORMALIZATION.items():
            if state != "NORMALIZED_V1":
                continue
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

    def validate_a23_oss_namespace_and_ui_truth(
        self, manifest: dict[str, Any], paths: dict[str, Path]
    ) -> None:
        """A2.3: catalogue/formal namespace separation and UI implementation truth."""
        p07 = paths.get("Platform_07_OSS_Catalog.md")
        if p07 is not None and p07.is_file():
            text = p07.read_text(encoding="utf-8")
            # Deterministic catalogue rows: first cell is OSS-NNN.
            catalogue_rows: list[tuple[str, str]] = []
            for line in text.splitlines():
                if not line.strip().startswith("|"):
                    continue
                cells = [c.strip() for c in line.strip().strip("|").split("|")]
                if not cells:
                    continue
                rid = cells[0]
                if not re.fullmatch(r"OSS-\d{3,}", rid):
                    continue
                catalogue_rows.append((rid, line))
            oss_ids = {rid for rid, _ in catalogue_rows}
            oss008_rows = [line for rid, line in catalogue_rows if rid == "OSS-008"]
            # Live-sized catalogue must include the OSS-008 row.
            if len(oss_ids) >= 50 and not oss008_rows:
                self.fail(
                    "Platform_07 live catalogue missing dependency OSS-008 (ClickHouse)"
                )
            # When an OSS-008 catalogue row exists, that row itself must name ClickHouse.
            # No whole-document or cross-row fallback.
            for line in oss008_rows:
                cells = [c.strip() for c in line.strip().strip("|").split("|")]
                row_body = " | ".join(cells[1:]) if len(cells) > 1 else ""
                if "ClickHouse" not in row_body:
                    self.fail(
                        "Platform_07 catalogue row OSS-008 must identify ClickHouse "
                        "in that same row (formal air-gap requirement is REQ-OSS-008); "
                        "unrelated prose or other rows cannot satisfy this invariant"
                    )

        p08 = paths.get("Platform_08_Requirements_Traceability.md")
        if p08 is not None and p08.is_file():
            text = p08.read_text(encoding="utf-8")
            rows = {
                r["id"]: r
                for r in self._parse_registry_rows(
                    text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
                )
            }
            row = rows.get("REQ-OSS-008")
            if row is not None:
                blob = " ".join(
                    [
                        row.get("summary", ""),
                        row.get("evidence", ""),
                        row.get("locator", ""),
                    ]
                ).lower()
                if "air-gap" not in blob and "air gap" not in blob:
                    self.fail(
                        "REQ-OSS-008 primary row must retain air-gap semantics"
                    )
                if "internet" not in blob:
                    self.fail(
                        "REQ-OSS-008 primary row must retain no-runtime-internet "
                        "dependency install semantics"
                    )

        p03 = paths.get("Platform_03_Architecture.md")
        if p03 is not None and p03.is_file():
            text = p03.read_text(encoding="utf-8")
            if "Tailwind" in text or "OSS-088" in text:
                for line in text.splitlines():
                    if "OSS-088" not in line and "Tailwind" not in line:
                        continue
                    # Stale current-USE claim without explicit NOT CURRENT marker.
                    if re.search(r"\|\s*USE\s*\|", line) and "NOT CURRENT" not in line:
                        self.fail(
                            "Platform_03_Architecture.md: Tailwind/OSS-088 must not be "
                            "presented as current USE UI implementation "
                            "(current stack is React + TypeScript + Vite + plain CSS)"
                        )
                # Reject affirmative "current ... Tailwind" claims; allow negation.
                for m in re.finditer(
                    r"(?is)current\s+(?:ui\s+)?(?:implementation|stack|foundation)"
                    r".{0,160}Tailwind",
                    text,
                ):
                    window = m.group(0)
                    if re.search(
                        r"(?i)(?:not|no)\s+(?:a\s+)?(?:current|required).{0,40}Tailwind"
                        r"|Tailwind.{0,40}(?:not|no)\s+(?:a\s+)?(?:current|required)"
                        r"|Tailwind/\w+\s+are\s+not\s+current"
                        r"|no\s+Tailwind",
                        window,
                    ):
                        continue
                    self.fail(
                        "Platform_03_Architecture.md: must not claim Tailwind as "
                        "current UI implementation/stack/foundation"
                    )

        # Current canonical formal IDs must not collide with catalogue OSS-NNN IDs.
        if p07 is not None and p07.is_file() and p08 is not None and p08.is_file():
            cat_ids = set(
                re.findall(
                    r"^\|\s*(OSS-\d{3,})\s*\|",
                    p07.read_text(encoding="utf-8"),
                    flags=re.M,
                )
            )
            primary = {
                r["id"]
                for r in self._parse_registry_rows(
                    p08.read_text(encoding="utf-8"),
                    REQ_MARKER_START,
                    REQ_MARKER_END,
                    "REQ",
                )
            }
            collisions = sorted(cat_ids & primary)
            for cid in collisions:
                self.fail(
                    f"formal/catalogue ID collision: {cid} appears in both "
                    f"Platform_07 catalogue and Platform_08 primary REQ registry"
                )

    def validate_normalized_rtm_licence_grace(
        self, paths: dict[str, Path]
    ) -> None:
        """NORMALIZED Platform_08 must not reintroduce fixed post-expiry grace.

        Guards REQ-LIC-004 / REQ-EGC-007 / PRD-EGC-007 primary-registry rows.
        Does not apply to TRANSITIONAL_LEGACY Platform_21. Does not reject
        REQ-LIC-003 (T-7 PRE-EXPIRY banner) or historical 7-vs-15 provenance
        when coupled to FOUNDER-LIC-GRACE UNRESOLVED.
        """
        rel = "Platform_08_Requirements_Traceability.md"
        if CANONICAL_NORMALIZATION.get(rel) != "NORMALIZED_V1":
            return
        path = paths.get(rel)
        if path is None or not path.is_file():
            return
        text = path.read_text(encoding="utf-8")

        stale_forms = (
            "7-day default grace after expiry",
            "governed seven-day grace",
            "existing seven-day grace requirement",
        )
        lowered = text.lower()
        for stale in stale_forms:
            if stale.lower() in lowered:
                self.fail(
                    f"{rel}: stale fixed post-expiry licence-grace wording "
                    f"must not appear in NORMALIZED_V1 RTM: {stale!r}"
                )

        req_rows = {
            r["id"]: r
            for r in self._parse_registry_rows(
                text, REQ_MARKER_START, REQ_MARKER_END, "REQ"
            )
        }
        prd_rows = {
            r["id"]: r
            for r in self._parse_registry_rows(
                text, PRD_MARKER_START, PRD_MARKER_END, "PRD"
            )
        }

        required: list[tuple[str, dict[str, str] | None, str]] = [
            ("REQ-LIC-004", req_rows.get("REQ-LIC-004"), "REQ"),
            ("REQ-EGC-007", req_rows.get("REQ-EGC-007"), "REQ"),
            ("PRD-EGC-007", prd_rows.get("PRD-EGC-007"), "PRD"),
        ]
        for rid, row, kind in required:
            if row is None:
                # Absence is covered by other registry completeness rules when
                # the ID exists in the live set; skip here if fixture omits it.
                continue
            if kind == "REQ":
                blob = " ".join(
                    [
                        row.get("summary", ""),
                        row.get("evidence", ""),
                        row.get("locator", ""),
                    ]
                )
            else:
                blob = row.get("summary", "")
            if "FOUNDER-LIC-GRACE" not in blob or "UNRESOLVED" not in blob:
                self.fail(
                    f"{rel}: {rid} primary registry row must state "
                    f"FOUNDER-LIC-GRACE and UNRESOLVED "
                    f"(controlling truth: Platform_02 §33)"
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
        self.validate_pinned_invariants(manifest)
        paths = self.validate_document_set(manifest, constitutional)
        self.validate_headers(manifest, paths)
        self.validate_global_requirements(manifest, paths)
        self.validate_global_prds(manifest, paths)
        self.validate_legacy_and_malformed(manifest, paths)
        self.validate_cross_references(manifest, paths)
        self.validate_normalized_structure(paths)
        self.validate_maturity_language(paths)
        self.validate_normalized_rtm_licence_grace(paths)
        self.validate_a23_oss_namespace_and_ui_truth(manifest, paths)
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
