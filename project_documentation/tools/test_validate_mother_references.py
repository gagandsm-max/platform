#!/usr/bin/env python3
"""Unit tests for Mother Reference structural validator (stdlib only)."""

from __future__ import annotations

import json
import re
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

_TOOLS_DIR = Path(__file__).resolve().parent
if str(_TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(_TOOLS_DIR))

from validate_mother_references import (  # noqa: E402
    CANONICAL_MOTHER_REFERENCE_VERSION,
    CONSTITUTIONAL_AUTHORITATIVE_COUNT,
    SCHEMA_VERSION,
    MotherReferenceValidator,
    validate_docs_root,
)

MINIMAL_DOCS = [
    ("AGENTS.md", None, "TRANSITIONAL_LEGACY", "navigation_engineering_governance"),
    ("Platform_00_Document_Index.md", "PLATFORM-INDEX-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_01_BRD.md", "PLATFORM-BRD-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_02_PRD.md", "PLATFORM-PRD-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_03_Architecture.md", "PLATFORM-ARC-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_04_HLD.md", "PLATFORM-HLD-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_05_LLD.md", "PLATFORM-LLD-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_06_Dataflow.md", "PLATFORM-DF-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_07_OSS_Catalog.md", "PLATFORM-OSS-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_08_Requirements_Traceability.md", "PLATFORM-RTM-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_09_UI_UX_Master.md", "PLATFORM-UIUX-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_10_Protocol_Security_Connector_Master.md", "PLATFORM-PROTOCOL-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_11_Sizing_Capacity_Storage_Master.md", "PLATFORM-SIZING-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_12_Decision_Source_Register.md", "PLATFORM-DECISION-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_13_Engineering_Development_and_Docker_Standards.md", "PLATFORM-ENGDEV-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_14_Test_Performance_and_Release_Quality.md", "PLATFORM-QUALITY-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_15_CI_CD_Packaging_and_Distribution.md", "PLATFORM-RELEASEENG-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_16_Installation_Upgrade_Rollback_and_Operations.md", "PLATFORM-OPSINSTALL-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_17_API_Event_and_Data_Model_Contracts.md", "PLATFORM-CONTRACTDATA-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_18_Security_Threat_Model.md", "PLATFORM-THREAT-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md", "PLATFORM-INTELVALUE-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md", "PLATFORM-ONBOARDING-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_21_Enterprise_Gap_Closure_Master.md", "PLATFORM-EGC-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md", "PLATFORM-MIA-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md", "PLATFORM-EMC-001", "TRANSITIONAL_LEGACY", "mother_reference"),
]


def _header(doc_id: str | None, version: str = "3.3.0") -> str:
    if doc_id is None:
        return f"# AGENTS\n**Version:** {version}\n\n"
    return (
        f"# Doc\n"
        f"**Document ID:** {doc_id}\n"
        f"**Version:** {version}\n\n"
    )


def _req_registry(rows: list[tuple[str, str, str]]) -> str:
    """rows: (id, summary, owner). Locator defaults to the fixture section that holds the ID."""
    lines = [
        "<!-- GLOBAL PRIMARY REQ REGISTRY -->",
        "### GLOBAL PRIMARY REQ REGISTRY",
        "",
        "| Requirement ID | Summary | Primary owner | Source locator | Traceability state | Maturity | Evidence / blocker pointer |",
        "|---|---|---|---|---|---|---|",
    ]
    for rid, summary, owner in rows:
        # Platform_08-owned fixture IDs live under historical §6; other owners use §1.
        locator = (
            "§6 Historical requirement catalogue"
            if owner.endswith("Platform_08_Requirements_Traceability.md")
            else "§1 Section"
        )
        lines.append(
            f"| {rid} | {summary} | `{owner}` | {locator} | BASE_RTM_INDEXED | SPECIFIED | n/a |"
        )
    lines.append("<!-- END GLOBAL PRIMARY REQ REGISTRY -->")
    return "\n".join(lines)


def _prd_registry(rows: list[tuple[str, str, str]]) -> str:
    """rows: (id, summary, owner)."""
    lines = [
        "<!-- GLOBAL PRIMARY PRD REGISTRY -->",
        "### GLOBAL PRIMARY PRD REGISTRY",
        "",
        "| PRD ID | Product requirement summary | Owner document | Source locator | Related REQ family | Maturity / implementation boundary |",
        "|---|---|---|---|---|---|",
    ]
    for rid, summary, owner in rows:
        lines.append(
            f"| {rid} | {summary} | `{owner}` | §1 Section | — | SPECIFIED |"
        )
    lines.append("<!-- END GLOBAL PRIMARY PRD REGISTRY -->")
    return "\n".join(lines)


class MotherReferenceValidatorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp(prefix="mr-val-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)

    def _write_minimal(
        self,
        *,
        req_rows: list[tuple[str, str, str]] | None = None,
        prd_rows: list[tuple[str, str, str]] | None = None,
        legacy: list[dict] | None = None,
        extra_files: dict[str, str] | None = None,
        omit_files: set[str] | None = None,
        version: str = "3.3.0",
        p00_sections: str | None = None,
        p08_extra: str = "",
        p08_historical: str | None = None,
        p02_body: str | None = None,
        normalize_overrides: dict[str, str] | None = None,
        doc_id_overrides: dict[str, str] | None = None,
        include_platform_24: bool = False,
        duplicate_platform_file: str | None = None,
        broken_ref: str | None = None,
        skip_manifest_legacy: bool = False,
        historical_allowlist: list[str] | None = None,
        drop_manifest_files: set[str] | None = None,
        req_row_overrides: dict[str, str] | None = None,
        historical_migrations: list[dict] | None = None,
        omit_historical_migrations: bool = False,
    ) -> Path:
        omit_files = omit_files or set()
        drop_manifest_files = drop_manifest_files or set()
        req_rows = req_rows or [
            ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
        ]
        # A2.3: default fixtures carry REQ-OSS-008 (not formal OSS-008).
        req_ids = {r[0] for r in req_rows}
        if "REQ-OSS-008" not in req_ids and "OSS-008" not in req_ids:
            req_rows = list(req_rows) + [
                (
                    "REQ-OSS-008",
                    "Air-gap bundles contain all pinned/scanned artifacts; "
                    "no runtime internet dependency install",
                    "Platform_08_Requirements_Traceability.md",
                )
            ]
        prd_rows = prd_rows or [("PRD-HA-001", "HA pair", "Platform_02_PRD.md")]
        if skip_manifest_legacy:
            legacy = []
        elif legacy is None:
            legacy = []

        default_historical_migrations = [
            {
                "oldId": "OSS-008",
                "newId": "REQ-OSS-008",
                "kind": "formal_requirement_id_migration",
                "reason": (
                    "Namespace collision with Platform_07 dependency catalogue "
                    "OSS-008 (ClickHouse); air-gap formal requirement migrated "
                    "to REQ-OSS-008"
                ),
                "status": "migrated_historical_only",
            }
        ]
        if omit_historical_migrations:
            migrations_value = None
        elif historical_migrations is not None:
            migrations_value = historical_migrations
        else:
            migrations_value = default_historical_migrations

        docs_meta = []
        ordinal = 0
        for fname, doc_id, state, kind in MINIMAL_DOCS:
            if fname in omit_files or fname in drop_manifest_files:
                continue
            ordinal += 1
            state = (normalize_overrides or {}).get(fname, state)
            docs_meta.append(
                {
                    "file": fname,
                    "ordinal": ordinal,
                    "kind": kind,
                    "expectedDocumentId": doc_id,
                    "governingRole": f"role for {fname}",
                    "normalizationState": state,
                }
            )

        if include_platform_24:
            docs_meta.append(
                {
                    "file": "Platform_24_Fake.md",
                    "ordinal": 99,
                    "kind": "mother_reference",
                    "expectedDocumentId": "PLATFORM-FAKE-001",
                    "governingRole": "fake",
                    "normalizationState": "TRANSITIONAL_LEGACY",
                }
            )

        manifest = {
            "schemaVersion": SCHEMA_VERSION,
            "motherReferenceVersion": version,
            "globalRequirementIndex": "Platform_08_Requirements_Traceability.md",
            "globalProductRequirementIndex": "Platform_08_Requirements_Traceability.md",
            "documents": docs_meta,
            "legacyIdExceptions": [] if skip_manifest_legacy else legacy,
            "historicalFilenameAllowlist": historical_allowlist or [],
        }
        if migrations_value is not None:
            manifest["historicalFormalIdMigrations"] = migrations_value
        (self.tmp / "mother_reference_manifest.json").write_text(
            json.dumps(manifest, indent=2), encoding="utf-8"
        )

        maturity = (
            "SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED\n"
            "States: SPECIFIED IMPLEMENTED TESTED BENCHMARKED CERTIFIED DEPLOYED\n"
        )
        default_p00 = (
            "## 1. Authority\n\n"
            "## 2. No-assumption\n\n"
            "## 3. Maturity\n\n"
            f"{maturity}\n"
            "## 4. Logical\n\n"
            "## 5. Set\n\n"
            "### 5.1 Allocation\n\n"
            "## 6. Precedence\n\n"
        )

        # Historical base RTM outside primary registry for Platform_08-owned REQs.
        hist_lines = ["### Historical requirement catalogue (pre-A1 detail)", ""]
        for rid, summary, owner in req_rows:
            if owner.endswith("Platform_08_Requirements_Traceability.md") or owner == "Platform_08_Requirements_Traceability.md":
                hist_lines.append(f"| {rid} | {summary} | ✓ |")
        hist_default = "\n".join(hist_lines) + "\n"

        for fname, doc_id, state, kind in MINIMAL_DOCS:
            if fname in omit_files:
                continue
            if fname == "AGENTS.md":
                body = _header(None, version) + "Navigation rules.\n"
            elif fname == "Platform_00_Document_Index.md":
                body = _header(doc_id, version) + (p00_sections or default_p00)
            elif fname == "Platform_08_Requirements_Traceability.md":
                body = (
                    _header(doc_id, version)
                    + "## 1. Purpose\n\n"
                    + "## 2. Global primary requirement registry\n\n"
                    + _req_registry(req_rows)
                    + "\n\n## 3. Global primary product-requirement registry\n\n"
                    + _prd_registry(prd_rows)
                    + "\n\n## 4. Completeness gate\n\n"
                    + "## 5. Supporting master coverage\n\n"
                    + "## 6. Historical base RTM detail\n\n"
                    + (p08_historical if p08_historical is not None else hist_default)
                    + p08_extra
                    + "\n"
                )
                if broken_ref:
                    body += f"\nSee `{broken_ref}` for details.\n"
                if req_row_overrides:
                    for old, new in req_row_overrides.items():
                        body = body.replace(old, new, 1)
            elif fname == "Platform_02_PRD.md":
                if p02_body is None:
                    p02_content = (
                        "## 1. Section\n\n"
                        + "\n".join(
                            f"| `{rid}` | {summary} |" for rid, summary, _owner in prd_rows
                        )
                        + "\n"
                    )
                elif re.search(r"^##\s+1\b", p02_body, re.M):
                    p02_content = p02_body
                else:
                    # Preserve custom bodies while satisfying default §1 PRD locator.
                    p02_content = "## 1. Section\n\n" + p02_body
                body = _header(doc_id, version) + p02_content
            else:
                body = _header(
                    (doc_id_overrides or {}).get(fname, doc_id), version
                ) + "## 1. Section\n\nContent.\n"
                # Place specialized REQs in their owner files when declared.
                for rid, summary, owner in req_rows:
                    if owner == fname:
                        body += f"\n| `{rid}` | {summary} |\n"
                for rid, summary, owner in prd_rows:
                    if owner == fname and fname != "Platform_02_PRD.md":
                        body += f"\n| `{rid}` | {summary} |\n"
            if (
                doc_id_overrides
                and fname in doc_id_overrides
                and fname
                not in {
                    "Platform_00_Document_Index.md",
                    "Platform_08_Requirements_Traceability.md",
                    "Platform_02_PRD.md",
                    "AGENTS.md",
                }
            ):
                body = _header(doc_id_overrides[fname], version) + "## 1. Section\n\nContent.\n"
                for rid, summary, owner in req_rows:
                    if owner == fname:
                        body += f"\n| `{rid}` | {summary} |\n"
            (self.tmp / fname).write_text(body, encoding="utf-8")

        if include_platform_24:
            (self.tmp / "Platform_24_Fake.md").write_text(
                _header("PLATFORM-FAKE-001", version) + "## 1. X\n", encoding="utf-8"
            )
        if duplicate_platform_file:
            (self.tmp / duplicate_platform_file).write_text(
                _header("PLATFORM-DUP-001", version) + "## 1. X\n", encoding="utf-8"
            )
        if extra_files:
            for name, content in extra_files.items():
                (self.tmp / name).write_text(content, encoding="utf-8")
        return self.tmp

    def test_01_valid_minimal_fixture_passes(self) -> None:
        root = self._write_minimal(
            legacy=[],
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-OSS-008",
                    "Air-gap bundles contain all pinned/scanned artifacts; "
                    "no runtime internet dependency install",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_02_missing_mother_reference_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").unlink()
        errors = validate_docs_root(root)
        self.assertTrue(
            any("missing authoritative" in e and "Platform_03" in e for e in errors),
            errors,
        )

    def test_03_unexpected_numbered_document_fails(self) -> None:
        root = self._write_minimal(
            extra_files={
                "Platform_99_Unexpected.md": _header("PLATFORM-X-001") + "## 1. X\n"
            }
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "unexpected numbered document" in e or "Platform_24+" in e
                for e in errors
            ),
            errors,
        )

    def test_04_platform_24_fails(self) -> None:
        root = self._write_minimal(include_platform_24=True)
        errors = validate_docs_root(root)
        self.assertTrue(any("Platform_24+" in e for e in errors), errors)

    def test_05_duplicate_platform_number_fails(self) -> None:
        root = self._write_minimal(duplicate_platform_file="Platform_03_Duplicate.md")
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "unexpected numbered document" in e or "duplicate Platform number" in e
                for e in errors
            ),
            errors,
        )

    def test_06_wrong_document_id_fails(self) -> None:
        root = self._write_minimal(
            doc_id_overrides={"Platform_01_BRD.md": "PLATFORM-WRONG-001"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("wrong Document ID" in e for e in errors), errors)

    def test_07_version_mismatch_fails(self) -> None:
        root = self._write_minimal(version="9.9.9")
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        man["motherReferenceVersion"] = "3.3.0"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("version mismatch" in e for e in errors), errors)

    def test_08_req_absent_from_global_registry_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ]
        )
        p01 = (root / "Platform_01_BRD.md").read_text(encoding="utf-8")
        (root / "Platform_01_BRD.md").write_text(
            p01 + "\nREQ-NET-001 must be indexed.\n", encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ in source but absent from global primary registry: REQ-NET-001" in e
                for e in errors
            ),
            errors,
        )

    def test_09_duplicate_primary_req_row_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                ("REQ-P-001", "Dup", "Platform_08_Requirements_Traceability.md"),
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("duplicate primary REQ row: REQ-P-001" in e for e in errors), errors
        )

    def test_10_prd_absent_from_global_registry_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[("PRD-HA-001", "HA", "Platform_02_PRD.md")]
        )
        p02 = (root / "Platform_02_PRD.md").read_text(encoding="utf-8")
        (root / "Platform_02_PRD.md").write_text(
            p02 + "\nPRD-SNMP-001 appears here.\n", encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "PRD in source but absent from global primary PRD registry: PRD-SNMP-001"
                in e
                for e in errors
            ),
            errors,
        )

    def test_11_duplicate_primary_prd_row_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[
                ("PRD-HA-001", "HA", "Platform_02_PRD.md"),
                ("PRD-HA-001", "Dup", "Platform_02_PRD.md"),
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("duplicate primary PRD row: PRD-HA-001" in e for e in errors), errors
        )

    def test_12_unknown_owner_source_file_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[("PRD-HA-001", "HA", "Platform_02_PRD.md")]
        )
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001") + "## 1. Empty of this PRD\n", encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("no authoritative source" in e or "absent from declared owner" in e for e in errors),
            errors,
        )

    def test_13_broken_platform_md_reference_fails(self) -> None:
        root = self._write_minimal(broken_ref="Platform_99_Does_Not_Exist.md")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("broken Platform_*.md reference" in e for e in errors), errors
        )

    def test_14_invalid_formal_id_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                ("OSS-999", "Bad legacy", "Platform_08_Requirements_Traceability.md"),
            ],
            skip_manifest_legacy=True,
            p08_historical=(
                "### Historical\n| REQ-P-001 | Quality | ✓ |\n| OSS-999 | Bad | ✓ |\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "invalid formal ID" in e
                or "unregistered malformed" in e
                or "unapproved legacy" in e
                or "missing required legacy" in e
                for e in errors
            ),
            errors,
        )

    def test_15_req_oss008_canonical_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-OSS-008",
                    "Air-gap bundles contain all pinned/scanned artifacts; "
                    "no runtime internet dependency install",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            legacy=[],
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_16_current_canonical_formal_oss008_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                ("OSS-008", "Air-gap", "Platform_08_Requirements_Traceability.md"),
            ],
            skip_manifest_legacy=True,
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "OSS-008" in e
                and (
                    "must not contain current canonical" in e
                    or "invalid" in e
                    or "unregistered" in e
                    or "unapproved legacy" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_17_duplicate_normalized_top_level_section_fails(self) -> None:
        root = self._write_minimal(
            p00_sections=(
                "## 1. Authority\n\n"
                "SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED\n"
                "SPECIFIED IMPLEMENTED TESTED BENCHMARKED CERTIFIED DEPLOYED\n"
                "## 1. Duplicate\n\n"
                "## 2. Next\n\n"
            )
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "duplicate normalized top-level section number: 1" in e for e in errors
            ),
            errors,
        )

    def test_18_transitional_legacy_duplicate_sections_are_blocked(self) -> None:
        # All Mother References reject duplicate numeric section identifiers,
        # including TRANSITIONAL_LEGACY documents.
        root = self._write_minimal()
        (root / "Platform_09_UI_UX_Master.md").write_text(
            _header("PLATFORM-UIUX-001")
            + "## 1. One\n\n## 1. Two\n\n## 18.1 Nested historical\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_09_UI_UX_Master.md" in e
                and "duplicate numbered section identifier" in e
                for e in errors
            ),
            errors,
        )

    def test_18b_normalized_platform_03_duplicate_sections_blocked(self) -> None:
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 1. One\n\n## 1. Two\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_03_Architecture.md: duplicate normalized" in e for e in errors
            ),
            errors,
        )

    def test_18c_normalized_platform_02_stale_subsection_parent_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 12. Infrastructure\n\n"
            + "### 67.1 Windows/Linux infrastructure monitoring\n\n"
            + "| `PRD-HA-001` | HA |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_02_PRD.md" in e
                and "parent mismatch" in e
                and "67.1" in e
                for e in errors
            ),
            errors,
        )

    def test_18d_normalized_platform_03_stale_subsection_parent_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 9. Sensor architecture\n\n"
            + "### 8.1 Sensor runtime\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_03_Architecture.md" in e
                and "parent mismatch" in e
                and "8.1" in e
                for e in errors
            ),
            errors,
        )

    def test_18e_normalized_matching_subsection_parent_passes(self) -> None:
        root = self._write_minimal()
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 12. Infrastructure\n\n"
            + "### 12.1 Server/OS\n\n"
            + "| `PRD-HA-001` | HA |\n",
            encoding="utf-8",
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 9. Sensor architecture\n\n"
            + "### 9.1 Sensor runtime\n\n"
            + "#### 9.1.1 Nested detail\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("parent mismatch" in e for e in errors),
            errors,
        )

    def test_18f_normalized_descriptive_unnumbered_subsection_passes(self) -> None:
        root = self._write_minimal()
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 12. Infrastructure\n\n"
            + "### Windows/Linux infrastructure monitoring modes\n\n"
            + "| `PRD-HA-001` | HA |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("parent mismatch" in e for e in errors),
            errors,
        )

    def test_18f2_normalized_proper_dotted_subsection_passes(self) -> None:
        root = self._write_minimal()
        (root / "Platform_04_HLD.md").write_text(
            _header("PLATFORM-HLD-001")
            + "## 44. Engineering-runtime\n\n"
            + "### 44.1 Development deployment units\n\n"
            + "### Core node\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("malformed bare-numeric" in e for e in errors),
            errors,
        )
        self.assertFalse(
            any("parent mismatch" in e for e in errors),
            errors,
        )

    def test_18f3_normalized_bare_numeric_subsection_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_04_HLD.md").write_text(
            _header("PLATFORM-HLD-001")
            + "## 44. Engineering-runtime\n\n"
            + "### 44 Development deployment units\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_04_HLD.md" in e and "malformed bare-numeric" in e
                for e in errors
            ),
            errors,
        )

    def test_18f4_normalized_bare_numeric_deep_heading_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_04_HLD.md").write_text(
            _header("PLATFORM-HLD-001")
            + "## 44. Engineering-runtime\n\n"
            + "#### 44 Nested detail\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_04_HLD.md" in e and "malformed bare-numeric" in e
                for e in errors
            ),
            errors,
        )

    def test_18f5_normalized_tmp_marker_in_heading_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_06_Dataflow.md").write_text(
            _header("PLATFORM-DF-001")
            + "## 29. __TMP29__. Flow DF-027A — Example\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_06_Dataflow.md" in e
                and "temporary migration marker" in e
                for e in errors
            ),
            errors,
        )

    def test_18f6_transitional_platform_09_bare_numeric_not_blocked(self) -> None:
        # Platform_09+ remain TRANSITIONAL_LEGACY; bare-numeric / TMP guards
        # apply only to NORMALIZED_V1 documents.
        root = self._write_minimal()
        (root / "Platform_09_UI_UX_Master.md").write_text(
            _header("PLATFORM-UIUX-001")
            + "## 9. Placement\n\n"
            + "### 9 Historical UI bare token retained until later tranche\n\n"
            + "## 10. __TMP10__. Historical marker\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("Platform_09_UI_UX_Master.md" in e and "malformed bare-numeric" in e for e in errors),
            errors,
        )
        self.assertFalse(
            any(
                "Platform_09_UI_UX_Master.md" in e and "temporary migration marker" in e
                for e in errors
            ),
            errors,
        )

    def test_18g_transitional_platform_09_may_retain_historical_numbering(self) -> None:
        root = self._write_minimal()
        (root / "Platform_09_UI_UX_Master.md").write_text(
            _header("PLATFORM-UIUX-001")
            + "## 9. Placement\n\n"
            + "### 8.1 Historical UI numbering retained until later tranche\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any(
                "Platform_09_UI_UX_Master.md" in e and "parent mismatch" in e for e in errors
            ),
            errors,
        )

    def test_18h_normalized_platform_04_duplicate_sections_blocked(self) -> None:
        root = self._write_minimal()
        (root / "Platform_04_HLD.md").write_text(
            _header("PLATFORM-HLD-001")
            + "## 1. One\n\n## 1. Two\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_04_HLD.md: duplicate normalized" in e for e in errors
            ),
            errors,
        )

    # --- Bypass-closure tests (control-plane corrective patch) ---

    def test_19_physical_file_only_deletion_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_12_Decision_Source_Register.md").unlink()
        errors = validate_docs_root(root)
        self.assertTrue(
            any("Platform_12" in e and "missing" in e for e in errors), errors
        )

    def test_20_physical_and_manifest_deletion_still_fails(self) -> None:
        root = self._write_minimal(
            drop_manifest_files={"Platform_12_Decision_Source_Register.md"}
        )
        (root / "Platform_12_Decision_Source_Register.md").unlink()
        errors = validate_docs_root(root)
        self.assertTrue(
            any("Platform_12" in e for e in errors),
            errors,
        )
        self.assertTrue(
            any(
                "manifest missing required Platform_12" in e
                or "constitutional set requires" in e
                or "missing authoritative" in e
                for e in errors
            ),
            errors,
        )

    def test_21_agents_and_manifest_deletion_still_fails(self) -> None:
        root = self._write_minimal(drop_manifest_files={"AGENTS.md"})
        (root / "AGENTS.md").unlink()
        errors = validate_docs_root(root)
        self.assertTrue(any("AGENTS.md" in e for e in errors), errors)

    def test_22_missing_platform_23_and_manifest_entry_fails(self) -> None:
        root = self._write_minimal(
            drop_manifest_files={
                "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md"
            }
        )
        (
            root
            / "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md"
        ).unlink()
        errors = validate_docs_root(root)
        self.assertTrue(any("Platform_23" in e for e in errors), errors)

    def test_23_exactly_25_authoritative_entries_required(self) -> None:
        root = self._write_minimal()
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        self.assertEqual(len(man["documents"]), CONSTITUTIONAL_AUTHORITATIVE_COUNT)
        # Drop one manifest entry without deleting file → still fail (count + mismatch)
        man["documents"] = [d for d in man["documents"] if d["file"] != "AGENTS.md"]
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "exactly 25" in e or "AGENTS.md" in e or "authoritative entries" in e
                for e in errors
            ),
            errors,
        )

    def test_24_same_number_bogus_filename_reference_fails(self) -> None:
        root = self._write_minimal(broken_ref="Platform_17_Bogus.md")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("Platform_17_Bogus.md" in e and "broken" in e for e in errors),
            errors,
        )

    def test_25_same_number_platform_03_bogus_fails(self) -> None:
        root = self._write_minimal(broken_ref="Platform_03_Bogus.md")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("Platform_03_Bogus.md" in e and "broken" in e for e in errors),
            errors,
        )

    def test_26_exact_live_filename_reference_passes(self) -> None:
        root = self._write_minimal(
            broken_ref="Platform_17_API_Event_and_Data_Model_Contracts.md"
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("Platform_17_API_Event_and_Data_Model_Contracts.md" in e and "broken" in e for e in errors),
            errors,
        )

    def test_27_allowlisted_historical_filename_passes(self) -> None:
        hist = "Platform_24_Release_Acceptance_Criteria.md"
        root = self._write_minimal(
            broken_ref=hist,
            historical_allowlist=[hist],
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any(hist in e and "broken" in e for e in errors),
            errors,
        )

    def test_28_fabricated_registry_only_req_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                ("REQ-FAKE-001", "Fabricated", "Platform_08_Requirements_Traceability.md"),
            ],
            # Historical only keeps REQ-P-001
            p08_historical="### Historical\n| REQ-P-001 | Quality | ✓ |\n",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-FAKE-001" in e
                and (
                    "no authoritative source" in e
                    or "absent from declared owner" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_29_platform_08_historical_base_req_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ]
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_30_specialized_req_correct_owner_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-OIV-001",
                    "SaaS monitoring",
                    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md",
                )
            ]
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_31_specialized_req_wrong_owner_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-OIV-001",
                    "SaaS monitoring",
                    "Platform_08_Requirements_Traceability.md",
                )
            ],
            # Put ID only in Platform_19, not in Platform_08 historical
            p08_historical="### Historical\n(no OIV rows)\n",
            extra_files={},
        )
        # Ensure specialized master has the ID (fixture writer places by owner;
        # wrong owner means P19 won't get it from req_rows owner match — add manually)
        p19 = root / "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md"
        p19.write_text(
            p19.read_text(encoding="utf-8") + "\n| `REQ-OIV-001` | SaaS monitoring |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-OIV-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_32_nonexistent_req_owner_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ],
            req_row_overrides={
                "`Platform_08_Requirements_Traceability.md`": "`Platform_99_Does_Not_Exist.md`",
            },
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-P-001" in e
                and (
                    "not a valid Mother Reference filename" in e
                    or "does not exist" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_33_prd_wrong_owner_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[
                (
                    "PRD-HA-001",
                    "HA pair",
                    "Platform_03_Architecture.md",
                )
            ],
            p02_body="| `PRD-HA-001` | HA pair |\n",
        )
        # Ensure declared owner Platform_03 does NOT contain the PRD (only Platform_02 does).
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001") + "## 1. Section\n\nNo PRD here.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "PRD-HA-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_34_empty_source_locator_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ]
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §6 Historical requirement catalogue |",
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` |  |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("empty source locator" in e for e in errors),
            errors,
        )

    def test_35_malformed_registry_column_count_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ]
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §6 Historical requirement catalogue | BASE_RTM_INDEXED | SPECIFIED | n/a |",
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §6 Historical requirement catalogue |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("malformed REQ registry row column count" in e for e in errors),
            errors,
        )

    # --- Final constitutional hardening (pinned invariants) ---

    def test_36_coordinated_filename_and_manifest_rename_fails(self) -> None:
        root = self._write_minimal()
        old = root / "Platform_03_Architecture.md"
        new = root / "Platform_03_Other.md"
        old.rename(new)
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        for doc in man["documents"]:
            if doc["file"] == "Platform_03_Architecture.md":
                doc["file"] = "Platform_03_Other.md"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_03_Architecture.md" in e
                or "Platform_03_Other.md" in e
                or "canonical" in e
                for e in errors
            ),
            errors,
        )

    def test_37_coordinated_document_id_and_header_change_fails(self) -> None:
        root = self._write_minimal()
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        for doc in man["documents"]:
            if doc["file"] == "Platform_03_Architecture.md":
                doc["expectedDocumentId"] = "PLATFORM-REDEFINED-001"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-REDEFINED-001") + "## 1. Section\n\nContent.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "PLATFORM-ARC-001" in e
                or "pinned Document ID" in e
                or "wrong Document ID" in e
                for e in errors
            ),
            errors,
        )

    def test_38_manifest_and_document_version_bump_still_fails(self) -> None:
        root = self._write_minimal(version="9.9.9")
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        man["motherReferenceVersion"] = "9.9.9"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                CANONICAL_MOTHER_REFERENCE_VERSION in e
                or "pinned constitutional version" in e
                or "version mismatch" in e
                for e in errors
            ),
            errors,
        )

    def test_39_global_req_index_redirect_fails(self) -> None:
        root = self._write_minimal()
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        man["globalRequirementIndex"] = "Platform_02_PRD.md"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("globalRequirementIndex redirect forbidden" in e for e in errors),
            errors,
        )

    def test_40_global_prd_index_redirect_fails(self) -> None:
        root = self._write_minimal()
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        man["globalProductRequirementIndex"] = "Platform_02_PRD.md"
        (root / "mother_reference_manifest.json").write_text(
            json.dumps(man), encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "globalProductRequirementIndex redirect forbidden" in e for e in errors
            ),
            errors,
        )

    def test_41_platform_00_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_00_Document_Index.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_00_Document_Index.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_42_platform_08_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={
                "Platform_08_Requirements_Traceability.md": "TRANSITIONAL_LEGACY"
            }
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_08_Requirements_Traceability.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43_premature_transitional_to_normalized_fails(self) -> None:
        # Platform_09+ remain TRANSITIONAL_LEGACY; premature upgrade must fail.
        root = self._write_minimal(
            normalize_overrides={"Platform_09_UI_UX_Master.md": "NORMALIZED_V1"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_09_UI_UX_Master.md" in e
                and "normalizationState" in e
                and "TRANSITIONAL_LEGACY" in e
                for e in errors
            ),
            errors,
        )

    def test_43h_platform_07_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_07_OSS_Catalog.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_07_OSS_Catalog.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43e_platform_04_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_04_HLD.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_04_HLD.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43f_platform_05_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_05_LLD.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_05_LLD.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43g_platform_06_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_06_Dataflow.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_06_Dataflow.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43b_platform_01_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_01_BRD.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_01_BRD.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43c_platform_02_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_02_PRD.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_02_PRD.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_43d_platform_03_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_03_Architecture.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_03_Architecture.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_44_extra_legacy_exception_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-OSS-008",
                    "Air-gap bundles contain all pinned/scanned artifacts; "
                    "no runtime internet dependency install",
                    "Platform_08_Requirements_Traceability.md",
                ),
                ("OSS-999", "Extra", "Platform_08_Requirements_Traceability.md"),
            ],
            legacy=[
                {
                    "id": "OSS-999",
                    "kind": "formal_requirement_legacy",
                    "reason": "unapproved expansion",
                },
            ],
            p08_historical=(
                "### Historical\n"
                "| REQ-P-001 | Quality | ✓ |\n"
                "| REQ-OSS-008 | Air-gap | ✓ |\n"
                "| OSS-999 | Extra | ✓ |\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "OSS-999" in e
                and (
                    "unapproved legacy" in e
                    or "unregistered malformed" in e
                    or "invalid formal ID" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_45_prd_in_platform_09_missing_registry_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[("PRD-HA-001", "HA", "Platform_02_PRD.md")]
        )
        p09 = root / "Platform_09_UI_UX_Master.md"
        p09.write_text(
            p09.read_text(encoding="utf-8") + "\n| `PRD-UI-999` | UI requirement |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "PRD-UI-999" in e
                and "absent from global primary PRD registry" in e
                for e in errors
            ),
            errors,
        )

    def test_46_prd_in_platform_09_with_correct_primary_row_passes(self) -> None:
        root = self._write_minimal(
            prd_rows=[
                ("PRD-HA-001", "HA", "Platform_02_PRD.md"),
                ("PRD-UI-999", "UI requirement", "Platform_09_UI_UX_Master.md"),
            ]
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_47_fabricated_registry_only_prd_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[
                ("PRD-HA-001", "HA", "Platform_02_PRD.md"),
                ("PRD-FAKE-001", "Fabricated", "Platform_02_PRD.md"),
            ],
            p02_body="| `PRD-HA-001` | HA |\n",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "PRD-FAKE-001" in e
                and (
                    "no authoritative source" in e
                    or "absent from declared owner" in e
                )
                for e in errors
            ),
            errors,
        )

    # --- A2.2 normalized RTM licence-grace guard (15-day Founder decision 2026-09-30) ---

    def test_48_req_lic_004_fifteen_day_grace_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D); "
                    "T-7 banner distinct; supersedes historical FOUNDER-LIC-GRACE UNRESOLVED",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("licence-grace" in e or "FOUNDER-LIC-GRACE" in e or "15-day" in e for e in errors),
            errors,
        )

    def test_49_req_egc_007_fifteen_day_grace_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-P-001",
                    "Quality",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-EGC-007",
                    "Licensing tests preserve Founder-approved 15-day post-expiry grace",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("REQ-EGC-007" in e for e in errors),
            errors,
        )
        self.assertFalse(
            any("stale fixed post-expiry" in e for e in errors),
            errors,
        )

    def test_50_prd_egc_007_fifteen_day_grace_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
            ],
            prd_rows=[
                ("PRD-HA-001", "HA pair", "Platform_02_PRD.md"),
                (
                    "PRD-EGC-007",
                    "Fail-safe licence enforcement; post-expiry grace is 15 days "
                    "(EXPIRED_GRACE_15D); T-7 banner distinct",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("PRD-EGC-007" in e for e in errors),
            errors,
        )

    def test_51_req_lic_004_stale_seven_day_default_grace_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence enforcement\n\n"
                "Normative policy still states 7-day default grace after expiry.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "stale fixed post-expiry licence-grace" in e
                and "7-day default grace after expiry" in e
                for e in errors
            ),
            errors,
        )

    def test_52_req_egc_007_stale_governed_seven_day_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-EGC-007",
                    "Licensing tests preserve Founder-approved 15-day post-expiry grace; "
                    "also preserve the governed seven-day grace",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "stale fixed post-expiry licence-grace" in e
                and "governed seven-day grace" in e
                for e in errors
            ),
            errors,
        )

    def test_53_prd_egc_007_stale_existing_seven_day_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
            ],
            prd_rows=[
                ("PRD-HA-001", "HA pair", "Platform_02_PRD.md"),
                (
                    "PRD-EGC-007",
                    "Fail-safe licence enforcement; post-expiry grace is 15 days "
                    "(EXPIRED_GRACE_15D); preserve the existing seven-day grace "
                    "requirement where already governed",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "stale fixed post-expiry licence-grace" in e
                and "existing seven-day grace requirement" in e
                for e in errors
            ),
            errors,
        )

    def test_54_req_lic_003_pre_expiry_banner_still_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-003",
                    "7-day dashboard banner before expiry",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_54b_req_lic_003_dashboard_banner_alone_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-003",
                    "dashboard banner",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-LIC-003" in e and "T-7 / 7-day pre-expiry banner" in e
                for e in errors
            ),
            errors,
        )

    def test_54c_req_lic_003_expiry_warning_banner_without_t7_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-003",
                    "expiry warning banner",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-LIC-003" in e and "T-7 / 7-day pre-expiry banner" in e
                for e in errors
            ),
            errors,
        )

    def test_55_historical_unresolved_provenance_allowed_when_labelled(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D). "
                    "Historical/superseded provenance only: FOUNDER-LIC-GRACE UNRESOLVED "
                    "and 7-day vs 15-day conflict are closed.",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_56_normative_founder_lic_grace_unresolved_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D). "
                    "Posture also uses FOUNDER-LIC-GRACE UNRESOLVED; "
                    "7-day vs 15-day; neither normative",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "must not state normative FOUNDER-LIC-GRACE UNRESOLVED" in e
                or "normative FOUNDER-LIC-GRACE UNRESOLVED is obsolete" in e
                for e in errors
            ),
            errors,
        )

    def test_56b_mixed_historical_and_normative_unresolved_fails(self) -> None:
        """P1-GOV-01/04: nearby historical statement must not mask normative occurrence."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence grace notes\n\n"
                "Historical note: the old entitlement implementation was superseded.\n\n"
                "FOUNDER-LIC-GRACE UNRESOLVED remains the active licence rule.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "FOUNDER-LIC-GRACE UNRESOLVED" in e
                and (
                    "normative" in e.lower()
                    or "obsolete" in e.lower()
                )
                for e in errors
            ),
            errors,
        )

    def test_56b2_nearby_historical_under_160_chars_still_fails(self) -> None:
        """P1-GOV-04-A: separate nearby statements fail even with <160 char gap."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence grace notes\n\n"
                "Historical note: the old entitlement implementation was superseded.\n"
                "FOUNDER-LIC-GRACE UNRESOLVED remains the active licence rule.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("FOUNDER-LIC-GRACE UNRESOLVED" in e for e in errors),
            errors,
        )

    def test_56b3_same_statement_historical_unresolved_passes(self) -> None:
        """P1-GOV-04-B: same-statement historical unresolved provenance passes."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence grace provenance\n\n"
                "Historical: FOUNDER-LIC-GRACE UNRESOLVED was previously recorded.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_56b4_same_statement_is_superseded_passes(self) -> None:
        """P1-GOV-04-C: same-statement '... is superseded' provenance passes."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence grace provenance\n\n"
                "The former FOUNDER-LIC-GRACE UNRESOLVED posture is superseded.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_56c_semantic_post_expiry_grace_is_7_days_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence enforcement\n\n"
                "Normative policy: post-expiry grace is 7 days.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("stale fixed post-expiry licence-grace" in e for e in errors),
            errors,
        )

    def test_56d_semantic_seven_day_grace_after_expiry_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence enforcement\n\n"
                "Normative policy: seven-day grace after expiry applies.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("stale fixed post-expiry licence-grace" in e for e in errors),
            errors,
        )

    def test_56e_t7_pre_expiry_banner_not_flagged_as_post_expiry(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-003",
                    "7-day dashboard banner before expiry",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Pre-expiry notifications\n\n"
                "T-7: 7-day dashboard banner before expiry remains required.\n"
                "7-day pre-expiry warning remains governed.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_56e2_nearby_t7_does_not_excuse_post_expiry_seven_day(self) -> None:
        """P1-GOV-05: nearby T-7 wording must not suppress explicit post-expiry match."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence enforcement\n\n"
                "T-7 dashboard banner remains required. Post-expiry grace is 7 days.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("stale fixed post-expiry licence-grace" in e for e in errors),
            errors,
        )

    def test_56f_historical_seven_day_post_expiry_provenance_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            p08_extra=(
                "\n### Licence grace provenance\n\n"
                "Historical/superseded provenance only: seven-day grace after expiry "
                "was replaced by EXPIRED_GRACE_15D.\n"
                "Historical: seven-day grace after expiry was previously used.\n"
                "The former seven-day grace after expiry is superseded.\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_56g_canonical_fifteen_day_post_expiry_rows_pass(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D); "
                    "T-7 banner distinct",
                    "Platform_08_Requirements_Traceability.md",
                ),
                (
                    "REQ-EGC-007",
                    "Licensing tests preserve Founder-approved 15-day post-expiry grace",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
            prd_rows=[
                ("PRD-HA-001", "HA pair", "Platform_02_PRD.md"),
                (
                    "PRD-EGC-007",
                    "Fail-safe licence enforcement; post-expiry grace is 15 days "
                    "(EXPIRED_GRACE_15D); T-7 banner distinct",
                    "Platform_21_Enterprise_Gap_Closure_Master.md",
                ),
            ],
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any(
                "licence-grace" in e
                or "FOUNDER-LIC-GRACE" in e
                or "15-day post-expiry grace" in e
                for e in errors
            ),
            errors,
        )

    def test_56h_broad_semantic_seven_day_variants_fail(self) -> None:
        """P1-GOV-06: bounded semantic post-expiry variants must fail."""
        variants = (
            "post-expiry grace: 7 days",
            "post-expiry grace = 7 days",
            "post-expiry grace is seven days",
            "post-expiry grace duration is 7 days",
            "post expiry grace period is seven days",
            "7-day post-expiry grace",
            "seven-day post expiry grace",
            "7 days grace after expiry",
            "seven days grace after expiry",
        )
        for variant in variants:
            with self.subTest(variant=variant):
                root = self._write_minimal(
                    req_rows=[
                        (
                            "REQ-LIC-004",
                            "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                            "Platform_08_Requirements_Traceability.md",
                        ),
                    ],
                    prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
                    p08_extra=f"\n### Licence enforcement\n\nNormative: {variant}.\n",
                )
                errors = validate_docs_root(root)
                self.assertTrue(
                    any("stale fixed post-expiry licence-grace" in e for e in errors),
                    errors,
                )

    def test_56i_transitional_platform_21_normative_unresolved_fails(self) -> None:
        """P1-GOV-07: TRANSITIONAL Platform_21 is guarded for normative UNRESOLVED."""
        self.assertEqual(
            next(s for f, _d, s, _k in MINIMAL_DOCS if f == "Platform_21_Enterprise_Gap_Closure_Master.md"),
            "TRANSITIONAL_LEGACY",
        )
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            extra_files={
                "Platform_21_Enterprise_Gap_Closure_Master.md": (
                    _header("PLATFORM-EGC-001")
                    + "## 1. Section\n\n"
                    "FOUNDER-LIC-GRACE UNRESOLVED remains the active licence rule.\n"
                ),
            },
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_21_Enterprise_Gap_Closure_Master.md" in e
                and "FOUNDER-LIC-GRACE UNRESOLVED" in e
                for e in errors
            ),
            errors,
        )

    def test_56j_transitional_platform_12_normative_seven_day_fails(self) -> None:
        """P1-GOV-07: TRANSITIONAL Platform_12 is guarded for seven-day post-expiry."""
        self.assertEqual(
            next(s for f, _d, s, _k in MINIMAL_DOCS if f == "Platform_12_Decision_Source_Register.md"),
            "TRANSITIONAL_LEGACY",
        )
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            extra_files={
                "Platform_12_Decision_Source_Register.md": (
                    _header("PLATFORM-DECISION-001")
                    + "## 1. Section\n\n"
                    "Normative policy: post-expiry grace is 7 days.\n"
                ),
            },
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_12_Decision_Source_Register.md" in e
                and "stale fixed post-expiry licence-grace" in e
                for e in errors
            ),
            errors,
        )

    def test_56k_transitional_agents_normative_unresolved_fails(self) -> None:
        """P1-GOV-07: TRANSITIONAL AGENTS.md is guarded for normative UNRESOLVED."""
        self.assertEqual(
            next(s for f, _d, s, _k in MINIMAL_DOCS if f == "AGENTS.md"),
            "TRANSITIONAL_LEGACY",
        )
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            extra_files={
                "AGENTS.md": (
                    _header(None)
                    + "Navigation rules.\n\n"
                    "FOUNDER-LIC-GRACE UNRESOLVED remains the active licence rule.\n"
                ),
            },
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "AGENTS.md" in e and "FOUNDER-LIC-GRACE UNRESOLVED" in e for e in errors
            ),
            errors,
        )

    def test_56l_transitional_historical_provenance_still_passes(self) -> None:
        """P1-GOV-07: same-statement historical provenance in transitional docs passes."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-LIC-004",
                    "Post-expiry grace is 15 days (EXPIRED_GRACE_15D)",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ],
            prd_rows=[("PRD-HA-001", "HA pair", "Platform_02_PRD.md")],
            extra_files={
                "Platform_21_Enterprise_Gap_Closure_Master.md": (
                    _header("PLATFORM-EGC-001")
                    + "## 1. Section\n\n"
                    "Historical/superseded provenance only: FOUNDER-LIC-GRACE UNRESOLVED "
                    "and seven-day grace after expiry are closed.\n"
                ),
                "Platform_12_Decision_Source_Register.md": (
                    _header("PLATFORM-DECISION-001")
                    + "## 1. Section\n\n"
                    "The former seven-day grace after expiry is superseded.\n"
                ),
                "AGENTS.md": (
                    _header(None)
                    + "Navigation rules.\n\n"
                    "Historical: FOUNDER-LIC-GRACE UNRESOLVED was previously recorded.\n"
                ),
            },
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    # --- A2.3 OSS catalogue + REQ-OSS-008 migration (Issue #108) ---

    def test_57_platform_07_normalized_v1_passes(self) -> None:
        root = self._write_minimal()
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)
        man = json.loads(
            (root / "mother_reference_manifest.json").read_text(encoding="utf-8")
        )
        by_file = {d["file"]: d for d in man["documents"]}
        self.assertEqual(
            by_file["Platform_07_OSS_Catalog.md"]["normalizationState"],
            "NORMALIZED_V1",
        )

    def test_58_platform_07_normalization_downgrade_fails(self) -> None:
        root = self._write_minimal(
            normalize_overrides={"Platform_07_OSS_Catalog.md": "TRANSITIONAL_LEGACY"}
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_07_OSS_Catalog.md" in e
                and "normalizationState" in e
                and "NORMALIZED_V1" in e
                for e in errors
            ),
            errors,
        )

    def test_59_req_oss008_airgap_semantics_required(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-OSS-008",
                    "Something unrelated without the required keywords",
                    "Platform_08_Requirements_Traceability.md",
                ),
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("REQ-OSS-008" in e and ("air-gap" in e or "internet" in e) for e in errors),
            errors,
        )

    def test_60_historical_migration_alias_permitted(self) -> None:
        # TEST E — valid OSS-008 → REQ-OSS-008 migration passes.
        root = self._write_minimal(
            historical_migrations=[
                {
                    "oldId": "OSS-008",
                    "newId": "REQ-OSS-008",
                    "kind": "formal_requirement_id_migration",
                    "reason": "Namespace collision with Platform_07 dependency OSS-008",
                    "status": "migrated_historical_only",
                }
            ]
        )
        p08 = root / "Platform_08_Requirements_Traceability.md"
        p08.write_text(
            p08.read_text(encoding="utf-8")
            + "\n**Historical formal-ID migration:** former formal requirement ID "
            "`OSS-008` migrated to `REQ-OSS-008`.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_61_catalogue_oss008_clickhouse_permitted(self) -> None:
        root = self._write_minimal()
        (root / "Platform_07_OSS_Catalog.md").write_text(
            _header("PLATFORM-OSS-001")
            + "## 1. Catalogue\n\n"
            + "| ID | Capability | OSS | Decision | Truth state |\n"
            + "|---|---|---|---|---|\n"
            + "| OSS-008 | Telemetry | ClickHouse | USE | CURRENT_RUNTIME |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_62_legacy_oss008_exception_no_longer_allowed(self) -> None:
        root = self._write_minimal(
            legacy=[
                {
                    "id": "OSS-008",
                    "kind": "formal_requirement_legacy",
                    "reason": "stale A1 exception",
                }
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "OSS-008" in e and "unapproved legacy formal-ID exception" in e
                for e in errors
            ),
            errors,
        )

    def test_63_platform_03_tailwind_current_use_fails(self) -> None:
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 1. Dependencies\n\n"
            + "| ID | Domain | Capability | OSS | License | Decision | Runtime | Notes |\n"
            + "|---|---|---|---|---|---|---|---|\n"
            + "| OSS-088 | UI | Styling | Tailwind CSS | MIT | USE | Browser | "
            + "Current Browser dependency |\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_03_Architecture.md" in e and "Tailwind" in e for e in errors
            ),
            errors,
        )

    def test_64_platform_09_plus_remain_transitional(self) -> None:
        root = self._write_minimal(
            normalize_overrides={
                "Platform_09_UI_UX_Master.md": "NORMALIZED_V1",
                "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md": "NORMALIZED_V1",
            }
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("Platform_09_UI_UX_Master.md" in e and "TRANSITIONAL_LEGACY" in e for e in errors),
            errors,
        )
        self.assertTrue(
            any(
                "Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md" in e
                and "TRANSITIONAL_LEGACY" in e
                for e in errors
            ),
            errors,
        )

    def test_65_wrong_oss008_row_not_rescued_by_prose_clickhouse(self) -> None:
        # TEST A — OSS-008 row maps to non-ClickHouse; prose/other row cannot rescue.
        root = self._write_minimal()
        (root / "Platform_07_OSS_Catalog.md").write_text(
            _header("PLATFORM-OSS-001")
            + "## 1. Catalogue\n\n"
            + "| ID | Capability | OSS | Decision | Truth state |\n"
            + "|---|---|---|---|---|\n"
            + "| OSS-008 | Telemetry | PostgreSQL | USE | CURRENT_RUNTIME |\n"
            + "| OSS-007 | Control DB | PostgreSQL | USE | CURRENT_RUNTIME |\n\n"
            + "Historical prose mentions ClickHouse and OSS-008 elsewhere "
            + "but must not satisfy the catalogue-row invariant.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "OSS-008" in e
                and "ClickHouse" in e
                and ("same row" in e or "catalogue row" in e)
                for e in errors
            ),
            errors,
        )

    def test_66_missing_historical_migration_key_fails(self) -> None:
        # TEST B — omitting historicalFormalIdMigrations must fail.
        root = self._write_minimal(omit_historical_migrations=True)
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "historicalFormalIdMigrations" in e and "missing" in e for e in errors
            ),
            errors,
        )

    def test_67_empty_historical_migration_list_fails(self) -> None:
        # TEST C — empty migration list must fail.
        root = self._write_minimal(historical_migrations=[])
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "historicalFormalIdMigrations" in e and "empty" in e for e in errors
            ),
            errors,
        )

    def test_68_wrong_historical_migration_mapping_fails(self) -> None:
        # TEST D — migration that is not OSS-008 → REQ-OSS-008 must fail.
        root = self._write_minimal(
            historical_migrations=[
                {
                    "oldId": "OSS-999",
                    "newId": "REQ-OSS-999",
                    "kind": "formal_requirement_id_migration",
                    "reason": "Namespace collision unrelated placeholder",
                    "status": "migrated_historical_only",
                }
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "historicalFormalIdMigrations" in e
                and "OSS-008" in e
                and "REQ-OSS-008" in e
                for e in errors
            ),
            errors,
        )

    def test_69_normative_owner_table_with_requirement_prose_passes(self) -> None:
        """Legitimate normative owner anchors (ID + requirement prose) must pass."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-INT-001",
                    "ITSM connectors",
                    "Platform_02_PRD.md",
                )
            ],
            p02_body=(
                "## 1. Section\n\n"
                "| `PRD-HA-001` | HA pair |\n\n"
                "## 28. ITSM and notification integrations\n\n"
                "Platform SHALL provide ServiceNow/BMC/Jira SM ITSM connectors with "
                "binding, certification and durable outbox semantics (`REQ-INT-001`).\n"
            ),
        )
        # Override default §1 locator to the substantive §28 owner section.
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-INT-001 | ITSM connectors | `Platform_02_PRD.md` | §1 Section |",
            "| REQ-INT-001 | ITSM connectors | `Platform_02_PRD.md` | §28 ITSM and notification integrations |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_70_identifier_only_ownership_index_footer_fails(self) -> None:
        """Trailing RTM formal-ID ownership index placeholders must not prove ownership."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-INT-001",
                    "ITSM connectors",
                    "Platform_02_PRD.md",
                )
            ],
            p02_body=(
                "| `PRD-HA-001` | HA pair |\n\n"
                "## 28. ITSM\n\nProduct prose without the formal ID.\n\n"
                "<!-- RTM FORMAL-ID OWNERSHIP INDEX -->\n"
                "## RTM formal-ID ownership index\n\n"
                "This section is a structural ownership index.\n\n"
                "- `REQ-INT-001`\n"
                "<!-- END RTM FORMAL-ID OWNERSHIP INDEX -->\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-INT-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_71_bare_id_bullet_list_without_prose_fails(self) -> None:
        """Bare identifier bullets (no requirement prose) must not prove ownership."""
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-NET-008",
                    "Protocol depth",
                    "Platform_02_PRD.md",
                )
            ],
            p02_body=(
                "| `PRD-HA-001` | HA pair |\n\n"
                "## 11. Network monitoring\n\n"
                "Some unrelated prose.\n\n"
                "- `REQ-NET-008`\n"
            ),
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-NET-008" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_72_missing_owner_id_still_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                (
                    "REQ-INT-001",
                    "ITSM connectors",
                    "Platform_02_PRD.md",
                )
            ],
            p02_body="| `PRD-HA-001` | HA pair |\n\n## 28. ITSM\n\nNo formal ID here.\n",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-INT-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_73_invalid_owner_document_still_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md")
            ],
            req_row_overrides={
                "`Platform_08_Requirements_Traceability.md`": "`Platform_99_Does_Not_Exist.md`",
            },
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-P-001" in e
                and (
                    "not a valid Mother Reference filename" in e
                    or "does not exist" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_74_duplicate_primary_ids_still_fail(self) -> None:
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                ("REQ-P-001", "Duplicate", "Platform_08_Requirements_Traceability.md"),
            ]
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("duplicate primary REQ row: REQ-P-001" in e for e in errors),
            errors,
        )

    def test_75_existing_source_ids_with_normative_prose_remain_accepted(self) -> None:
        """Valid existing source IDs in owner prose/tables remain accepted."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-P-001", "Quality", "Platform_08_Requirements_Traceability.md"),
                (
                    "REQ-OIV-001",
                    "SaaS monitoring",
                    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md",
                ),
            ]
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_76_rtm_section_formal_requirements_marker_alone_fails(self) -> None:
        """<!-- RTM SECTION FORMAL REQUIREMENTS --> catalogues must not prove ownership."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-OSS-001", "Clean-room", "Platform_07_OSS_Catalog.md"),
            ],
        )
        (root / "Platform_07_OSS_Catalog.md").write_text(
            _header("PLATFORM-OSS-001")
            + "## 1. Purpose\n\nCatalogue prose without the formal ID.\n\n"
            + "<!-- RTM SECTION FORMAL REQUIREMENTS: REQ-OSS -->\n"
            + "#### Formal requirements — REQ-OSS\n\n"
            + "| Requirement ID | Normative requirement |\n"
            + "|---|---|\n"
            + "| `REQ-OSS-001` | Clean-room Platform |\n"
            + "<!-- END RTM SECTION FORMAL REQUIREMENTS: REQ-OSS -->\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-OSS-001 | Clean-room | `Platform_07_OSS_Catalog.md` | §1 Section |",
            "| REQ-OSS-001 | Clean-room | `Platform_07_OSS_Catalog.md` | §1 Purpose |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-OSS-001" in e
                and (
                    "absent from declared owner" in e
                    or "does not bind ID" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_77_synthetic_two_column_summary_under_marker_fails(self) -> None:
        """Synthetic two-column summary tables under markers cannot own a requirement."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-DATA-001", "Retention", "Platform_11_Sizing_Capacity_Storage_Master.md"),
            ],
        )
        (root / "Platform_11_Sizing_Capacity_Storage_Master.md").write_text(
            _header("PLATFORM-SIZING-001")
            + "## 10. Retention\n\nNo substantive ID outside the marker.\n\n"
            + "<!-- RTM SECTION FORMAL REQUIREMENTS: REQ-DATA -->\n"
            + "| Requirement ID | Normative requirement |\n"
            + "|---|---|\n"
            + "| `REQ-DATA-001` | Customer-configurable raw retention |\n"
            + "<!-- END RTM SECTION FORMAL REQUIREMENTS: REQ-DATA -->\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-DATA-001 | Retention | `Platform_11_Sizing_Capacity_Storage_Master.md` | §1 Section |",
            "| REQ-DATA-001 | Retention | `Platform_11_Sizing_Capacity_Storage_Master.md` | §10 Retention |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("REQ-DATA-001" in e for e in errors),
            errors,
        )

    def test_78_wrong_domain_owner_fails(self) -> None:
        """ID present only in a non-owner document does not satisfy ownership."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-UI-001", "Dashboard Studio", "Platform_09_UI_UX_Master.md"),
            ],
        )
        (root / "Platform_09_UI_UX_Master.md").write_text(
            _header("PLATFORM-UIUX-001") + "## 1. Section\n\nNo UI requirement ID here.\n",
            encoding="utf-8",
        )
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 1. Section\n\n| `PRD-HA-001` | HA |\n\n"
            + "## 9. UI\n\n`REQ-UI-001` lives only in the wrong owner.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-UI-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_79_missing_and_stale_section_locator_fail(self) -> None:
        """Missing section and stale locator that does not contain the ID both fail."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-A-001", "Two-Core HA", "Platform_03_Architecture.md"),
            ],
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 1. Mandate\n\nUnrelated.\n\n"
            + "## 7. Exactly-two-Core HA\n\n"
            + "Platform SHALL use exactly two Core nodes (`REQ-A-001`).\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        # Missing section
        p08_missing = p08.replace(
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §1 Section |",
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §99 Missing |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08_missing, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-A-001" in e and "locator section not found" in e for e in errors
            ),
            errors,
        )
        # Stale section: exists but does not contain the ID
        p08_stale = p08.replace(
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §1 Section |",
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §1 Mandate |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08_stale, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-A-001" in e and "absent from locator section" in e for e in errors
            ),
            errors,
        )

    def test_80_duplicate_conflicting_owner_still_fails_on_declared_owner(self) -> None:
        """Declared owner without the ID fails even if another doc also mentions it."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-NET-001", "SNMP", "Platform_02_PRD.md"),
            ],
            p02_body="## 1. Section\n\n| `PRD-HA-001` | HA |\n\nNo NET ID here.\n",
        )
        (root / "Platform_10_Protocol_Security_Connector_Master.md").write_text(
            _header("PLATFORM-PROTOCOL-001")
            + "## 1. Section\n\nAlso mentions `REQ-NET-001` but is not the declared owner.\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-NET-001" in e and "absent from declared owner" in e for e in errors
            ),
            errors,
        )

    def test_81_valid_substantive_and_specialized_master_owners_pass(self) -> None:
        """Exact locator + ID inside substantive owner section must pass."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-A-001", "Two-Core HA", "Platform_03_Architecture.md"),
                (
                    "REQ-OIV-001",
                    "SaaS monitoring",
                    "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md",
                ),
            ],
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 7. Exactly-two-Core HA\n\n"
            + "Platform SHALL deploy exactly two Core nodes for HA (`REQ-A-001`).\n",
            encoding="utf-8",
        )
        (root / "Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md").write_text(
            _header("PLATFORM-INTELVALUE-001")
            + "## 1. Section\n\n"
            + "Platform SHALL provide SaaS monitoring evidence (`REQ-OIV-001`).\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §1 Section |",
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §7 Exactly-two-Core HA |",
            1,
        )
        p08 = p08.replace(
            "| REQ-OIV-001 | SaaS monitoring | `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` | §1 Section |",
            "| REQ-OIV-001 | SaaS monitoring | `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` | §1 Section |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_81b_synthetic_dot_zero_family_locator_fails(self) -> None:
        """Platform_08 may not point primary ownership at `.0 Normative requirements`."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-A-001", "Two-Core HA", "Platform_03_Architecture.md"),
            ],
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 7. Exactly-two-Core HA\n\n"
            + "### 7.0 Normative requirements — REQ-A family\n\n"
            + "Platform SHALL deploy exactly two Core nodes for HA (`REQ-A-001`).\n\n"
            + "### 7.1 Substantive HA contract\n\n"
            + "Exactly two Core nodes.\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §1 Section |",
            "| REQ-A-001 | Two-Core HA | `Platform_03_Architecture.md` | §7.0 Normative requirements — REQ-A family |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-A-001" in e
                and "synthetic .0 Normative requirements" in e
                for e in errors
            ),
            errors,
        )

    def test_82_unmarked_two_column_summary_table_alone_fails(self) -> None:
        """Two-column Requirement ID | Normative requirement tables alone cannot own."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-OSS-002", "Approved OSS only", "Platform_07_OSS_Catalog.md"),
            ],
        )
        (root / "Platform_07_OSS_Catalog.md").write_text(
            _header("PLATFORM-OSS-001")
            + "## 1. Purpose\n\nCatalogue intro without the formal ID in prose.\n\n"
            + "| Requirement ID | Normative requirement |\n"
            + "|---|---|\n"
            + "| `REQ-OSS-002` | Approved OSS only |\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-OSS-002 | Approved OSS only | `Platform_07_OSS_Catalog.md` | §1 Section |",
            "| REQ-OSS-002 | Approved OSS only | `Platform_07_OSS_Catalog.md` | §1 Purpose |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "REQ-OSS-002" in e
                and (
                    "absent from declared owner" in e
                    or "does not bind ID" in e
                    or "absent from locator section" in e
                )
                for e in errors
            ),
            errors,
        )

    def test_83_four_column_synthetic_family_table_alone_fails(self) -> None:
        """Four-column family boilerplate tables cannot satisfy primary ownership."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-OSS-003", "Licence review", "Platform_07_OSS_Catalog.md"),
            ],
        )
        (root / "Platform_07_OSS_Catalog.md").write_text(
            _header("PLATFORM-OSS-001")
            + "## 1. Purpose\n\n"
            + "### 1.0 Normative requirements — REQ-OSS family\n\n"
            + "**Shared normative envelope:** inherits parent OSS admission contract.\n\n"
            + "| Requirement ID | Normative SHALL contract | Parent-section envelope | Boundary / non-goal |\n"
            + "|---|---|---|---|\n"
            + "| `REQ-OSS-003` | Platform SHALL require licence/CVE/SBOM review before merge. "
            + "Distinctive obligations bind with the shared envelope. | Inherits shared envelope | "
            + "Do not invent unlisted OSS. |\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-OSS-003 | Licence review | `Platform_07_OSS_Catalog.md` | §1 Section |",
            "| REQ-OSS-003 | Licence review | `Platform_07_OSS_Catalog.md` | §1.0 Normative requirements — REQ-OSS family |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("REQ-OSS-003" in e for e in errors),
            errors,
        )

    def test_84_differently_named_family_catalogue_fails(self) -> None:
        """Named family-catalogue headings cannot own a requirement."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-DATA-001", "Retention", "Platform_11_Sizing_Capacity_Storage_Master.md"),
            ],
        )
        (root / "Platform_11_Sizing_Capacity_Storage_Master.md").write_text(
            _header("PLATFORM-SIZING-001")
            + "## 10. Retention\n\nIntro only.\n\n"
            + "### REQ-DATA family catalogue\n\n"
            + "`REQ-DATA-001` Customer-configurable raw retention.\n",
            encoding="utf-8",
        )
        p08 = (root / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        p08 = p08.replace(
            "| REQ-DATA-001 | Retention | `Platform_11_Sizing_Capacity_Storage_Master.md` | §1 Section |",
            "| REQ-DATA-001 | Retention | `Platform_11_Sizing_Capacity_Storage_Master.md` | §10 Retention |",
            1,
        )
        (root / "Platform_08_Requirements_Traceability.md").write_text(
            p08, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("REQ-DATA-001" in e for e in errors), errors)

    def test_85_duplicate_conflicting_primary_owner_rows_fail(self) -> None:
        """Duplicate primary ownership rows for the same ID fail."""
        root = self._write_minimal(
            req_rows=[
                ("REQ-A-001", "Two-Core HA", "Platform_03_Architecture.md"),
                ("REQ-A-001", "Conflicting HA", "Platform_02_PRD.md"),
            ],
        )
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 7. Exactly-two-Core HA\n\nPlatform SHALL use two Cores (`REQ-A-001`).\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("duplicate primary REQ row: REQ-A-001" in e for e in errors),
            errors,
        )

    def test_86_duplicate_subsection_11_1_fails(self) -> None:
        """Duplicate ### 11.1 headings fail closed."""
        root = self._write_minimal()
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 11. Network\n\n"
            + "### 11.1 SNMP\n\nFirst body.\n\n"
            + "### 11.1 SNMP\n\nDuplicate body.\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_02_PRD.md" in e
                and "duplicate numbered section identifier" in e
                and "11.1" in e
                for e in errors
            ),
            errors,
        )

    def test_87_duplicate_nested_11_1_1_fails(self) -> None:
        """Duplicate #### 11.1.1 headings fail closed."""
        root = self._write_minimal()
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001")
            + "## 11. Network\n\n"
            + "### 11.1 SNMP\n\n"
            + "#### 11.1.1 Enterprise SNMP\n\nOne.\n\n"
            + "#### 11.1.1 Enterprise SNMP\n\nTwo.\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "Platform_02_PRD.md" in e
                and "duplicate numbered section identifier" in e
                and "11.1.1" in e
                for e in errors
            ),
            errors,
        )

    def test_88_duplicate_section_with_req_id_is_ambiguous(self) -> None:
        """Locator into a duplicated section fails closed even if one copy has the REQ."""
        from validate_mother_references import locator_binds_id

        owner = (
            "## 4. Alerts\n\n"
            "### 4.2 Ownership\n\n`REQ-ALT-011` ownership contract.\n\n"
            "### 4.2 Ownership\n\nDuplicate without id.\n"
        )
        ok, reason = locator_binds_id(
            "§4.2 Alert ownership", "REQ-ALT-011", owner
        )
        self.assertFalse(ok)
        self.assertIn("ambiguous", reason)

    def test_89_synthetic_subtree_strips_through_nested_children(self) -> None:
        """Non-normative stripping continues past nested child headings."""
        from validate_mother_references import strip_ownership_index_placeholders

        text = (
            "## 1. Purpose\n\nKeep.\n\n"
            "### Non-normative ownership index\n\n"
            "#### Nested child still non-normative\n\n"
            "`REQ-OSS-001` bare.\n\n"
            "### 1.1 Real subsection\n\n"
            "Platform SHALL enforce OSS admission (`REQ-OSS-001`).\n"
        )
        cleaned = strip_ownership_index_placeholders(text)
        self.assertNotIn("Nested child still non-normative", cleaned)
        self.assertIn("### 1.1 Real subsection", cleaned)
        self.assertIn("REQ-OSS-001", cleaned)

    def test_90_valid_unique_hierarchy_passes_section_map(self) -> None:
        """Unique numbered hierarchy maps without collision."""
        from validate_mother_references import section_body_map

        text = (
            "## 11. Network\n\n"
            "### 11.1 SNMP\n\nSNMP body.\n\n"
            "#### 11.1.1 Enterprise\n\nNested.\n\n"
            "### 11.2 Path\n\nPath body.\n"
        )
        bodies = section_body_map(text)
        self.assertIn("11", bodies)
        self.assertIn("11.1", bodies)
        self.assertIn("11.1.1", bodies)
        self.assertIn("11.2", bodies)
        self.assertIn("SNMP body", bodies["11.1"])
        self.assertIn("Path body", bodies["11.2"])

    def test_91_req_alt_002_binds_to_substantive_threshold_contract(self) -> None:
        """REQ-ALT-002 must bind to §6.1 threshold contract, not a one-line §6 header."""
        from validate_mother_references import locator_binds_id, section_body_map

        docs = Path(__file__).resolve().parents[1]
        owner_path = (
            docs / "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md"
        )
        owner = owner_path.read_text(encoding="utf-8")
        bodies = section_body_map(owner)
        self.assertIn("6.1", bodies)
        body_61 = bodies["6.1"]
        self.assertIn("REQ-ALT-002", body_61)
        # Substantive envelope + typed families must be present (not binding-only).
        for needle in (
            "Common threshold evaluation envelope",
            "Static threshold",
            "Duration threshold",
            "Consecutive threshold",
            "Rate threshold",
            "Baseline threshold",
            "Missing threshold",
            "Composite threshold",
            "Dependency threshold",
            "Business-time threshold",
            "GROUP / cross-entity threshold",
            "TRUE | FALSE | UNKNOWN",
            "Float64",
            "IANA",
            "AlertPolicyDefinitionV1",
        ):
            self.assertIn(needle, body_61, needle)
        self.assertIn("6.1.11", bodies)
        body_group = bodies["6.1.11"]
        for needle in (
            "canonical inventory/group/service selector",
            "ANY | ALL | COUNT | PERCENT",
            "Minimum qualified member count",
            "UNKNOWN",
            "No Data ≠ Device Down",
            "as_of",
            "membership snapshot",
        ):
            self.assertIn(needle, body_group, needle)
        ok, reason = locator_binds_id(
            "§6.1 Alert threshold evaluation contract", "REQ-ALT-002", owner
        )
        self.assertTrue(ok, reason)
        # Coarse §6-only binding header must not be treated as the RTM locator target
        # once §6.1 owns the contract; §6 still contains the ID via children, but the
        # primary Platform_08 locator is required to name §6.1.
        rtm = (docs / "Platform_08_Requirements_Traceability.md").read_text(
            encoding="utf-8"
        )
        self.assertRegex(
            rtm,
            r"REQ-ALT-002.*§6\.1 Alert threshold evaluation contract",
        )
        self.assertNotRegex(
            rtm,
            r"REQ-ALT-002.*§6 Alert-policy lifecycle and false-alert quality gates",
        )

    def test_92_varchar_enum_width_too_narrow_fails(self) -> None:
        """VARCHAR width must fit every declared enum literal."""
        root = self._write_minimal()
        p17 = (root / "Platform_17_API_Event_and_Data_Model_Contracts.md").read_text(
            encoding="utf-8"
        )
        if "outcome VARCHAR(" not in p17:
            p17 += (
                "\n| `rca_revision` | outcome VARCHAR(14) NN "
                "(CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE) |\n"
            )
        else:
            p17 = p17.replace(
                "outcome VARCHAR(16) NN (CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE)",
                "outcome VARCHAR(14) NN (CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE)",
            )
            if "PARTIAL_EVIDENCE" not in p17:
                p17 += (
                    "\n| `rca_revision` | outcome VARCHAR(14) NN "
                    "(CONCLUDED/INCONCLUSIVE/PARTIAL_EVIDENCE) |\n"
                )
        (root / "Platform_17_API_Event_and_Data_Model_Contracts.md").write_text(
            p17, encoding="utf-8"
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any(
                "VARCHAR(14) too narrow" in e and "PARTIAL_EVIDENCE" in e
                for e in errors
            ),
            errors,
        )

    def test_93_alert_policy_definition_v1_contract(self) -> None:
        """Platform_17 freezes AlertPolicyDefinitionV1 + authoring ops."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        for needle in (
            "AlertPolicyDefinitionV1",
            'schema_version: "1.0"',
            "subject_selector",
            "evidence_class",
            "root_condition: ThresholdExprV1",
            "recovery_condition",
            "severity_mapping",
            "evaluation_cadence",
            "evidence_contract_ref",
            "ThresholdExprV1",
            "STATIC | DURATION | CONSECUTIVE | RATE | BASELINE | MISSING",
            "COMPOSITE | GROUP | DEPENDENCY | BUSINESS_TIME",
            "SubjectSelectorV1",
            "SeverityMappingV1",
            "EvaluationCadenceV1",
            "CalendarRefV1",
            "expr_id",
            "OP-POLICY-CREATE",
            "POST /alert-policies",
            "POST /alert-policies/{policyId}/versions",
            ":validate",
            "version_no",
            "typed field/path",
        ):
            self.assertIn(needle, p17, needle)
        # Root business_calendar_ref is forbidden in schema V1 (single BUSINESS_TIME path).
        root_block = p17[
            p17.find("AlertPolicyDefinitionV1 {") : p17.find(
                "AlertPolicyDefinitionV1 {"
            )
            + 700
        ]
        self.assertNotIn("business_calendar_ref", root_block)
        self.assertIn(
            "forbids `business_calendar_ref` on `AlertPolicyDefinitionV1`",
            p17,
        )

    def test_93b_alert_policy_nested_schemas_and_single_validate_route(self) -> None:
        """Nested AlertPolicy types are fully defined; one canonical validate route."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        # Nested type field freezes
        for needle in (
            "scope_kind: TENANT | REGION | SITE | GROUP | SERVICE | ENTITY",
            "scope_id: UUID | null",
            "metric_ref: string | null",
            "monitor_ref: string | null",
            "include_descendants: boolean",
            "on_true: INFO | WARNING | MINOR | MAJOR | CRITICAL",
            "on_false: NO_FIRE | CLEAR",
            "on_unknown: NO_FIRE | PRESERVE_ACTIVE",
            "mode: ON_EVIDENCE | FIXED_INTERVAL",
            "interval_seconds: uint32 | null",
            "max_staleness_seconds: uint32",
            "calendar_id: UUID",
            "version_no: uint32",
            "kind: STATIC",
            "ComparisonV1",
            "threshold_high",
            "lower_inclusive",
            "upper_inclusive",
            "fire: ComparisonV1",
            "clear: ComparisonV1 | null",
            "kind: DURATION",
            "duration_seconds",
            "unknown_behavior: PAUSE | RESET",
            "unknown_grace_seconds",
            "kind: CONSECUTIVE",
            "required_true_count",
            "unknown_behavior: RESET | SKIP",
            "kind: RATE",
            "comparison: ComparisonV1",
            "minimum_interval_seconds",
            "kind: BASELINE",
            "BaselineRefV1",
            "baseline_ref: BaselineRefV1",
            "comparison: ABOVE | BELOW | OUTSIDE_BAND",
            "kind: MISSING",
            "expected_cadence_seconds",
            "freshness_deadline_seconds",
            "kind: COMPOSITE",
            "operator: AND | OR | NOT",
            "kind: GROUP",
            "member_selector: SubjectSelectorV1",
            "aggregation: ANY | ALL | COUNT | PERCENT",
            "min_qualified_members",
            "count_threshold",
            "percent_threshold",
            "evaluation_window_seconds",
            "alignment_tolerance_seconds",
            "NESTED_GROUP_NOT_SUPPORTED_V1",
            "kind: DEPENDENCY",
            "DependencyRefV1",
            "dependency_ref: DependencyRefV1",
            "DependencyActivationV1",
            "activation: DependencyActivationV1",
            "action: SUPPRESS | DEFER | ANNOTATE",
            "kind: BUSINESS_TIME",
            "blackout_calendar_ref: CalendarRefV1 | null",
            "applicability: IN_WINDOW | OUTSIDE_WINDOW",
            "gap_behavior: OUTSIDE_WINDOW",
            "fold_behavior: EARLIER_OFFSET | LATER_OFFSET",
            "Float64 before delta",
            "**never** silently normalizes",
        ):
            self.assertIn(needle, p17, needle)
        # Retired ambiguous RATE/STATIC fields must not remain as the V1 contract
        self.assertNotIn("unit_per_second: string", p17)
        self.assertNotIn("hysteresis: number | null", p17)
        self.assertNotIn("sensitivity: number", p17)
        # Single canonical validate route — no alternate public body-validate API
        self.assertIn(
            "POST /alert-policies/{policyId}/versions/{versionNo}:validate",
            p17,
        )
        self.assertIn("ONLY** standalone validation operation", p17)
        self.assertNotIn("validate body `{definition}` before persist", p17)
        self.assertNotIn(
            "**or** validate body",
            p17,
        )
        self.assertIn("ValidationResult{ok, errors[{path, code, message_key}]}", p17)
        self.assertIn("pre-persist validation boundary", p17)

    def test_93c_alert_policy_threshold_persistence_closes_p22_61(self) -> None:
        """Platform_17 persists every configurable Platform_22 §6.1 choice (docs only)."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        # 1. STATIC inclusive/exclusive bounds
        for needle in (
            "lower_inclusive: boolean | null",
            "upper_inclusive: boolean | null",
            "INSIDE` / `OUTSIDE` require non-null `threshold_high`",
            "§6.1.2 range semantics",
        ):
            self.assertIn(needle, p17, needle)
        # 2. explicit clear/hysteresis representation
        for needle in (
            "StaticThresholdV1",
            "clear: ComparisonV1 | null",
            "clear = null` means ordinary FALSE evaluation of `fire`",
            "no hidden hysteresis number",
        ):
            self.assertIn(needle, p17, needle)
        # 3. DURATION PAUSE/RESET + grace
        for needle in (
            "DurationThresholdV1",
            "unknown_behavior: PAUSE | RESET",
            "unknown_grace_seconds: uint32",
            "RESET` requires `unknown_grace_seconds = 0`",
            "PAUSE` preserves elapsed TRUE duration",
        ):
            self.assertIn(needle, p17, needle)
        # 4. CONSECUTIVE RESET/SKIP
        for needle in (
            "ConsecutiveThresholdV1",
            "unknown_behavior: RESET | SKIP",
            "SKIP` — UNKNOWN/stale does not increment",
        ):
            self.assertIn(needle, p17, needle)
        # 5–9. BASELINE ID+version, reference window, sample sufficiency,
        # deviation mode/units, stale behavior/fallback immutable policy version
        for needle in (
            "BaselineThresholdV1",
            "BaselineRefV1",
            "baseline_id: string",
            "version_no: uint32",
            "reference_window_seconds: uint32",
            "minimum_qualified_samples: uint32",
            "deviation_mode: ABSOLUTE | RELATIVE_PERCENT | SIGMA_MULTIPLE",
            "deviation_unit: string | null",
            "stale_behavior: UNKNOWN | USE_FALLBACK_POLICY",
            "fallback_policy_ref: PolicyVersionRefV1 | null",
            "PolicyVersionRefV1",
            "baseline fallback-cycle rejection",
        ):
            self.assertIn(needle, p17, needle)
        # 10. recursive nested GROUP rejection
        for needle in (
            "NESTED_GROUP_NOT_SUPPORTED_V1",
            "recursively walk the complete `child` expression tree",
            "No descendant at any depth may have `kind=GROUP`",
            "GROUP → COMPOSITE → GROUP",
            "exact offending JSON path",
        ):
            self.assertIn(needle, p17, needle)
        # 11. BUSINESS_TIME fold/gap behavior + blackout identity
        for needle in (
            "BusinessTimeThresholdV1",
            "gap_behavior: OUTSIDE_WINDOW",
            "fold_behavior: EARLIER_OFFSET | LATER_OFFSET",
            "MUST NEVER depend on an unwritten implementation default",
            "blackout_calendar_ref: CalendarRefV1 | null",
            "HOLD_STATE",
        ):
            self.assertIn(needle, p17, needle)
        # 12. common-envelope persistence coverage matrix
        for needle in (
            "common-envelope persistence coverage matrix",
            "FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1",
            "Clear / reset / hysteresis",
            "Missing / stale behavior",
            "Dependency behavior",
            "DependencyThresholdV1.action` + `DependencyThresholdV1.activation",
            "Business-time applicability",
            "BusinessTimeThresholdV1.calendar_ref` / `applicability` / `gap_behavior` / `fold_behavior` ONLY",
            "Blackout / freeze identity",
            "BusinessTimeThresholdV1.blackout_calendar_ref",
            "Version authority",
        ):
            self.assertIn(needle, p17, needle)
        # Root dual-representation must not remain in the coverage matrix row.
        matrix = p17[
            p17.find("common-envelope persistence coverage matrix") : p17.find(
                "This matrix exists to prevent persistence-versus-evaluation divergence."
            )
        ]
        self.assertNotIn("`business_calendar_ref`; `BusinessTimeThresholdV1", matrix)
        # 13. single canonical validation route remains unchanged
        self.assertIn(
            "POST /alert-policies/{policyId}/versions/{versionNo}:validate",
            p17,
        )
        self.assertIn("ONLY** standalone validation operation", p17)
        self.assertIn(
            "OP-POLICY-CREATE**, **OP-POLICY-VERSION-CREATE** and **OP-POLICY-VERSION-VALIDATE**",
            p17,
        )
        self.assertIn("recursive schema and semantic validation", p17)
        self.assertIn("unknown fields in schema V1 MUST fail validation", p17)
        self.assertIn(
            "root `business_calendar_ref` is not a valid V1 field",
            p17,
        )
        self.assertIn(
            "`DEPENDENCY` requires `dependency_ref`, `activation`",
            p17,
        )
        # Documentation proof only — not a runtime implementation claim
        self.assertIn(
            "this schema owns the persisted shape only",
            p17,
        )

    def test_93d_dependency_action_and_business_time_single_path(self) -> None:
        """Docs prove dependency action persistence + single business-time path."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p22 = (
            docs
            / "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md"
        ).read_text(encoding="utf-8")

        # --- DependencyThresholdV1 action + activation persistence ---
        dep17 = p17[
            p17.find("**DEPENDENCY — `DependencyThresholdV1`**") : p17.find(
                "**BUSINESS_TIME — `BusinessTimeThresholdV1`**"
            )
        ]
        for needle in (
            "activation: DependencyActivationV1",
            "action: SUPPRESS | DEFER | ANNOTATE",
            "immutable within a policy version",
            "no dependency action",
            "Dependency disappearance/revocation removes the dependency effect reversibly",
        ):
            self.assertIn(needle, dep17, needle)
        act = p17[
            p17.find("###### `DependencyActivationV1`") : p17.find(
                "###### `ThresholdExprV1` (discriminated union)"
            )
        ]
        for needle in (
            "semantic_states: SemanticStateV1[]",
            "maintenance_active: boolean",
            "combine: ANY | ALL",
            "At least one criterion MUST be enabled",
            "combine=ANY",
            "combine=ALL",
            "No hidden/default combination behavior is permitted",
            "ALL` with only one enabled criterion is invalid",
            "Finite activation fixtures",
            "stale/unqualified dependency evidence ⇒ `UNKNOWN`",
        ):
            self.assertIn(needle, act, needle)
        self.assertIn("SemanticStateV1", p17)
        self.assertIn(
            "MUST NOT invent a second state model",
            p17,
        )

        # Platform_22 owns exact SUPPRESS / DEFER / ANNOTATE semantics
        body_dep = p22[p22.find("#### 6.1.9 Dependency threshold") : p22.find("#### 6.1.10")]
        for needle in (
            "##### SUPPRESS",
            "child expression is still evaluated and evidence retained",
            "MUST NOT rewrite child semantic state to `UP`",
            "MUST NOT clear an already-active alert solely because the dependency became active",
            "suppression is removed and current child evidence is immediately re-evaluated",
            "no historical child evidence is deleted",
            "##### DEFER",
            "does not emit a new firing transition yet",
            "evidence is retained as dependency-deferred",
            "no false healthy state is emitted",
            "no delayed alert is emitted",
            "re-evaluate CURRENT qualified child evidence",
            "do not backdate a new alert to the original deferred time",
            "An already-active alert is not silently cleared by `DEFER`",
            "##### ANNOTATE",
            "does not alter child fire/clear evaluation",
            "cannot independently suppress or clear the child alert",
            "No Data / stale graph evidence cannot activate `SUPPRESS`/`DEFER`",
            "current `as_of` graph authority",
            "combine: ANY | ALL",
            "combine=ANY",
            "combine=ALL",
            "no hidden/default combination behavior is permitted",
            "cannot activate `SUPPRESS`/`DEFER`",
        ):
            self.assertIn(needle, body_dep, needle)

        # --- Business-time single representation + blackout identity ---
        root_block = p17[
            p17.find("AlertPolicyDefinitionV1 {") : p17.find(
                "###### `SubjectSelectorV1`"
            )
        ]
        self.assertNotIn("business_calendar_ref:", root_block)
        self.assertIn(
            "forbids `business_calendar_ref` on `AlertPolicyDefinitionV1`",
            root_block,
        )
        bt17 = p17[
            p17.find("**BUSINESS_TIME — `BusinessTimeThresholdV1`**") : p17.find(
                "###### `ThresholdExprV1` ↔ `Platform_22` §6.1.1 common-envelope persistence coverage matrix"
            )
        ]
        for needle in (
            "calendar_ref: CalendarRefV1",
            "blackout_calendar_ref: CalendarRefV1 | null",
            "exactly one** business-time representation",
            "HOLD_STATE",
            "FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1",
            "Missing/invalid configured `blackout_calendar_ref` ⇒ `UNKNOWN`",
            "not silently “no blackout”",
            "never fall back to always-on evaluation",
        ):
            self.assertIn(needle, bt17, needle)
        body_bt = p22[p22.find("#### 6.1.10 Business-time threshold") : p22.find("#### 6.1.11")]
        for needle in (
            "exactly one** representation",
            "Root `AlertPolicyDefinitionV1.business_calendar_ref` is forbidden",
            "blackout_calendar_ref",
            "HOLD_STATE",
            "FIXED BY EVALUATION CONTRACT — NOT CONFIGURABLE IN V1",
            "do not clear an already-active alert solely because blackout started",
            "BLACKOUT_HOLD",
            "do not backdate a new firing transition",
            "not silently “no blackout”",
            "Do not fall back to always-on evaluation",
        ):
            self.assertIn(needle, body_bt, needle)

        # Coverage matrix mapping + recursive validator proofs
        matrix = p17[
            p17.find("common-envelope persistence coverage matrix") : p17.find(
                "This matrix exists to prevent persistence-versus-evaluation divergence."
            )
        ]
        self.assertIn(
            "DependencyThresholdV1.action` + `DependencyThresholdV1.activation",
            matrix,
        )
        self.assertIn(
            "BusinessTimeThresholdV1.calendar_ref` / `applicability` / `gap_behavior` / `fold_behavior` ONLY",
            matrix,
        )
        self.assertIn("BusinessTimeThresholdV1.blackout_calendar_ref", matrix)
        self.assertNotIn("`business_calendar_ref`; `BusinessTimeThresholdV1", matrix)
        for needle in (
            "root `business_calendar_ref` is not a valid V1 field",
            "no alternate calendar representation may exist elsewhere on `AlertPolicyDefinitionV1` outside `BusinessTimeThresholdV1`",
            "`BUSINESS_TIME` requires `calendar_ref`",
            "when `blackout_calendar_ref` is present it MUST resolve same-tenant",
            "`DEPENDENCY` requires `dependency_ref`, `activation`",
            "action ∈ {SUPPRESS, DEFER, ANNOTATE}",
        ):
            self.assertIn(needle, p17, needle)

    def test_94_async_state_mapping_g05_cc_async(self) -> None:
        """G-05 / CC-ASYNC freeze admission vs persisted async lifecycle."""
        docs = Path(__file__).resolve().parents[1]
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        for needle in (
            "ACCEPTED_FOR_ASYNC_EXECUTION",
            "state=QUEUED",
            "QUEUED → RUNNING → SUCCEEDED",
            "cancel_requested=true",
            "COMPLETED` is **not** a second persisted state",
        ):
            self.assertIn(needle, p06, needle)
        for needle in (
            "CC-ASYNC",
            "ACCEPTED_FOR_ASYNC_EXECUTION",
            "SUCCEEDED",
            "cancel_requested",
            "COMPLETED` is not a second persisted",
        ):
            self.assertIn(needle, p17, needle)
        # Stale G-05 vocabulary must not remain as the sole mapping.
        g05 = p06[p06.find("| G-05 |") : p06.find("| G-05 |") + 900]
        self.assertNotIn("`ACCEPTED`, `RUNNING`, `COMPLETED`", g05)

    def test_95_query_cursor_envelope_cc_cursor(self) -> None:
        """Platform_17 CC-CURSOR envelope + BUILD-067 fixture bindings."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        for needle in (
            "CC-CURSOR",
            "cursor_schema_version",
            "query_operation_id",
            "filter_fingerprint",
            "resource_scope_fingerprint",
            "sort_definition",
            "last_sort_tuple",
            "tie_break_resource_id",
            "CURSOR_INVALID",
            "CURSOR_EXPIRED",
            "CURSOR_STALE",
            "CURSOR_SCOPE_CHANGED",
            "never grants authorization",
            "LIVE_KEYSET",
            "SNAPSHOT",
            "consistency_mode",
        ):
            self.assertIn(needle, p17, needle)
        for needle in (
            "CC-CURSOR",
            "duplicate sort values",
            "CURSOR_INVALID",
            "CURSOR_SCOPE_CHANGED",
            "LIVE_KEYSET",
            "SNAPSHOT",
        ):
            self.assertIn(needle, register, needle)

    def test_95b_cc_cursor_live_keyset_and_snapshot_semantics(self) -> None:
        """LIVE_KEYSET mutation semantics vs SNAPSHOT guarantees are distinct."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        for needle in (
            "consistency_mode` | literal `LIVE_KEYSET`",
            "MAY be omitted or reappear",
            "LIVE/evolving-list behavior",
            "not** snapshot consistency",
            "MUST NOT be used as an implicit closed",
            "Do **not** add a growing `seen_ids`",
            "consistency_mode` | literal `SNAPSHOT`",
            "snapshot_ref",
            "Stable membership",
            "MUST NOT advertise SNAPSHOT",
            "mutation-proof no-duplicate",
        ):
            self.assertIn(needle, p17, needle)
        # Impossible mutation-proof guarantee removed from LIVE_KEYSET
        self.assertNotIn(
            "already-returned rows are never duplicated solely because of mutation",
            p17,
        )
        for needle in (
            "FX-LIVE_KEYSET",
            "FX-SNAPSHOT",
            "POS-LIVE_KEYSET",
            "POS-SNAPSHOT",
            "concurrent sort-key mutation follows documented LIVE semantics",
            "insert/update/delete after snapshot does not alter the paged snapshot",
            "no growing `seen_ids`",
            "Not every query must support SNAPSHOT",
        ):
            self.assertIn(needle, register, needle)

    def test_96_flow_observed_estimated_separation(self) -> None:
        """Flow raw/DAF-FLW/BUILD-093 keep observed and estimated separate."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        for needle in (
            "observed_bytes",
            "observed_packets",
            "estimated_bytes",
            "estimated_packets",
            "estimate_method",
            "sampling_breakdown",
            "sampling_cohort_id",
            "estimate_coverage",
            "UNSAMPLED | SAMPLED_KNOWN | SAMPLED_UNKNOWN",
            "INGRESS | EGRESS | UNKNOWN",
            "never one unlabeled total",
        ):
            self.assertIn(needle, p17, needle)
        self.assertIn("observed vs estimated", register.lower())
        self.assertIn("SAMPLED_UNKNOWN", register)

    def test_96b_flow_mixed_sampling_cohort_and_partial_coverage(self) -> None:
        """Mixed sampling cohorts + PARTIAL estimate coverage are frozen."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        p09 = (docs / "Platform_09_UI_UX_Master.md").read_text(encoding="utf-8")
        for needle in (
            "sampling_cohort_id",
            "FULL | PARTIAL | NONE",
            "sampling_breakdown[]",
            "populated ONLY for `FULL`",
            "For `PARTIAL`, top-level estimate is null",
            "Never treat `SAMPLED_UNKNOWN` as `UNSAMPLED`",
            "never silently average different sampling rates/models",
            "flow_rollup_1m/1h",
            "Estimated values may be computed only per certified `SAMPLED_KNOWN`",
        ):
            self.assertIn(needle, p17, needle)
        for needle in (
            "UNSAMPLED + SAMPLED_KNOWN",
            "two SAMPLED_KNOWN cohorts with different rates",
            "SAMPLED_UNKNOWN mixed with known",
            "validity-window transition",
            "replay of one cohort",
            "estimate_coverage=FULL",
            "misleading unlabeled whole-traffic total",
        ):
            self.assertIn(needle, register, needle)
        self.assertIn("estimate_coverage", p09)
        self.assertIn("sampling_breakdown", p09)
        # Ambiguous singular sampling_metadata must not remain as the DAF-FLW projection
        self.assertNotIn(
            "estimate_method?, sampling_metadata, quality",
            p17,
        )

    def test_97_profile_freeze_gates_block_dispatch(self) -> None:
        """BUILD-031/032/045 remain NOT DISPATCHABLE until named gates PASS."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        p12 = (docs / "Platform_12_Decision_Source_Register.md").read_text(
            encoding="utf-8"
        )
        for gate in (
            "WIN_AGENTLESS_P1_PROFILE_FREEZE_GATE",
            "REDFISH_P1_PROFILE_FREEZE_GATE",
            "DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE",
        ):
            self.assertIn(gate, p12, gate)
            self.assertIn(gate, register, gate)
            self.assertIn("NOT DISPATCHABLE", register)

    def test_98_network_flow_source_reality_status(self) -> None:
        """BUILD-043/044/045/051 must not overstate main 92f0fb37 source reality."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        register_path = root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        register = register_path.read_text(encoding="utf-8")

        def summary_maturity(build_id: str) -> str:
            prefix = f"| {build_id} |"
            for line in register.splitlines():
                if line.startswith(prefix):
                    cells = [c.strip() for c in line.strip().strip("|").split("|")]
                    return cells[4]
            self.fail(f"missing summary row {build_id}")
            return ""

        def ledger_maturity(num: str) -> str:
            section = register[register.find("### BUILD-040..054") :]
            nxt = section.find("\n### BUILD-", 1)
            if nxt != -1:
                section = section[:nxt]
            prefix = f"| {num} |"
            for line in section.splitlines():
                if line.startswith(prefix):
                    cells = [c.strip() for c in line.strip().strip("|").split("|")]
                    return cells[-1]
            self.fail(f"missing ledger row {num}")
            return ""

        for build_id, num, required in (
            ("BUILD-043", "043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-044", "044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            (
                "BUILD-045",
                "045",
                (
                    "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE "
                    "PASS; NOT_IMPLEMENTED"
                ),
            ),
            ("BUILD-051", "051", "NOT_IMPLEMENTED"),
        ):
            self.assertEqual(summary_maturity(build_id), required, build_id)
            self.assertEqual(ledger_maturity(num), required, f"ledger {num}")

        for banned in (
            "Mostly not implemented",
            "Not production",
            "Partial/spec",
        ):
            # Banned tokens must not remain as maturity claims for the four builds.
            for build_id in ("BUILD-043", "BUILD-044", "BUILD-045", "BUILD-051"):
                row = next(
                    line
                    for line in register.splitlines()
                    if line.startswith(f"| {build_id} |")
                )
                cells = [c.strip() for c in row.strip().strip("|").split("|")]
                self.assertNotIn(banned, cells[4], f"{build_id}:{banned}")

        # Neighbor genuine partials must not be downgraded by this correction.
        for build_id, needle in (
            ("BUILD-040", "Partial"),
            ("BUILD-041", "Partial"),
            (
                "BUILD-042",
                "PARTIAL — exact raw Counter64 + counter derivation leaf "
                "implemented; integration/benchmark/certification pending",
            ),
            ("BUILD-046", "Partial"),
            ("BUILD-047", "Partial code"),
            ("BUILD-050", "Partial"),
        ):
            self.assertIn(needle, summary_maturity(build_id), build_id)
        self.assertNotEqual(
            summary_maturity("BUILD-042"),
            "Raw Counter64 exists",
            "BUILD-042 must not regress to obsolete sole raw-path maturity",
        )

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_network_flow_source_reality_status()
        self.assertEqual(validator.errors, [])

    def test_98b_network_flow_source_reality_regression_fails(self) -> None:
        """Overstated BUILD-043/051 maturity must fail the source-reality pin."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        with tempfile.TemporaryDirectory() as tmp:
            tmp_root = Path(tmp)
            bad = (
                "# Register\n\n"
                "| ID | Build unit | Primary scope | Depends on | Starting maturity | Exit gate |\n"
                "|---|---|---|---|---|---|\n"
                "| BUILD-043 | Interface/entity reconciliation | ifIndex | 041/042 | Partial | exit |\n"
                "| BUILD-044 | SNMP traps/informs | traps | 041 | SPECIFIED_ONLY / NOT_IMPLEMENTED | exit |\n"
                "| BUILD-045 | gNMI/NETCONF/REST | connector | DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE | "
                "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE PASS; NOT_IMPLEMENTED | exit |\n"
                "| BUILD-051 | Flow receiver | NetFlow | 020 | Not production | exit |\n"
                "\n"
                "### BUILD-040..054 — Network, Flow, Syslog\n\n"
                "| BUILD | ① | ② | ③ delta | ④ D / I / C | ⑤ exit & acceptance | ⑥ rollout / rollback | ⑦ evidence / gate |\n"
                "|---|---|---|---|---|---|---|---|\n"
                "| 043 | Interface/entity reconciliation | Core | delta | D | exit | n/a | Partial |\n"
                "| 044 | SNMP traps/informs | Sensor | delta | D | exit | n/a | SPECIFIED_ONLY / NOT_IMPLEMENTED |\n"
                "| 045 | gNMI; **NOT DISPATCHABLE until `DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE` PASS** | TARGET | Gate | D | exit | rb | "
                "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE PASS; NOT_IMPLEMENTED |\n"
                "| 051 | Flow receiver | Sensor | delta | D | exit | allowlist | Not production |\n"
            )
            (tmp_root / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                bad, encoding="utf-8"
            )
            validator = MotherReferenceValidator(docs, repo_root=tmp_root)
            validator.errors = []
            validator.validate_network_flow_source_reality_status()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected source-reality failures")
            self.assertIn("BUILD-043", joined)
            self.assertIn("BUILD-051", joined)
            self.assertIn("Partial", joined)
            self.assertIn("Not production", joined)

    def test_99_agent_ingress_pending_reservation_invariant(self) -> None:
        """Platform_17 §7.1 pending-reservation safety must remain frozen."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p05 = (docs / "Platform_05_LLD.md").read_text(encoding="utf-8")
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        p16 = (
            docs / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        frozen = (
            "A deterministic pre-publication non-terminal rejection MUST NOT "
            "leave an unpublished durable pending reservation blocking continuity."
        )
        self.assertIn(frozen, p17)
        self.assertIn("### 7.1 Agent-ingress pending-reservation", p17)
        for needle in (
            "TERMINALLY_REJECTED",
            "ERR_AGENT_IDENTITY_CONFLICT",
            "ERR_AGENT_ASSIGNMENT_UNKNOWN",
            "pending_first_sequence IS NULL",
            "Authenticate Sensor",
            "exact durable prior-disposition",
            "mutable assignment / entity / binding validation",
            "conditional release",
            "publisher-owned",
            "Sensor retains custody",
            "same unsettled sequence",
            "### 7.1.8 Mandatory acceptance tests / fixtures",
            "### 7.1.9 Schema migration / release-guard upgrade precondition",
            "Historical pending publication state is **not inferable**",
            "SUPERSEDED contract — zero-pending as mandatory migration blocker",
            "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            "LEGACY_UNKNOWN",
            "AUTHORITY_VALIDATED",
            "Crash after mutable validation failure, before pending release",
            "Manual pending deletion/reset is **not** accepted remediation",
        ):
            self.assertIn(needle, p17, needle)

        self.assertIn(frozen, p05)
        self.assertIn("`Platform_17` §7.1", p05)
        self.assertIn("AUTHORITY_CHECKPOINT_RECOVERY", p05)
        self.assertIn(frozen, p06)
        self.assertIn("`Platform_17` §7.1", p06)
        self.assertIn("`Platform_17` §7.1", register)
        self.assertIn("pending-reservation", register)
        self.assertIn("does **not** reopen this certified record", register)

        # Authority-checkpoint upgrade gate + ownership split + race-safe cutover.
        self.assertIn(
            "SUPERSEDED: zero-pending as mandatory migration blocker",
            p16,
        )
        self.assertIn(
            "active Agent-ingress publisher/writer ownership = non-zero",
            p16,
        )
        self.assertIn("MUST NOT delete pending Agent-ingress rows manually", p16)
        self.assertIn("STAGE 1 — DRAIN MODE", p16)
        self.assertIn("STAGE 2 — HARD CUTOVER FENCE", p16)
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", p16)
        self.assertIn("check-then-unlock-then-migrate gap is forbidden", p16)
        self.assertIn(
            "active Agent-ingress publisher/writer ownership = non-zero",
            p06,
        )
        self.assertIn("LEGACY_UNKNOWN", p06)
        self.assertIn("STAGE 2 hard cutover fence", p06)
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", p06)
        self.assertIn("No Core partial-schema state is valid", p06)
        for race in (
            "CUTOVER-RACE-1",
            "CUTOVER-RACE-2",
            "CUTOVER-RACE-3",
            "CUTOVER-RACE-4",
        ):
            self.assertIn(race, p17, race)
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", p17)
        self.assertIn("STAGE 1 — DRAIN MODE", p17)
        self.assertIn("STAGE 2 — HARD CUTOVER FENCE", p17)
        row_062 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-062 |")
        )
        self.assertIn("runtime", row_062.lower())
        self.assertIn(
            "crash-after-mutable-validation-before-pending-release",
            row_062,
        )
        self.assertIn("AUTHORITY_CHECKPOINT_RECOVERY", row_062)
        row_127 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-127 |")
        )
        self.assertIn("LEGACY_UNKNOWN", row_127)
        self.assertIn(
            "zero-pending-as-mandatory-migration-blocker",
            row_127,
        )
        self.assertIn("BUILD-127", register)
        ledger_127 = next(
            line
            for line in register.splitlines()
            if line.startswith("| 127 |")
        )
        self.assertIn("U1 clean migration", ledger_127)
        self.assertIn("U2 dormant legacy pending", ledger_127)
        self.assertIn("U4 active publisher", ledger_127)
        self.assertIn(
            "manual pending deletion/reset is **not** accepted remediation",
            ledger_127,
        )

        # Just-corrected Network/Flow status truth must remain intact.
        for build_id, required in (
            ("BUILD-043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-051", "NOT_IMPLEMENTED"),
        ):
            row = next(
                line
                for line in register.splitlines()
                if line.startswith(f"| {build_id} |")
            )
            self.assertIn(required, row, build_id)
        row_045 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-045 |")
        )
        self.assertIn("NOT DISPATCHABLE", row_045)
        self.assertIn("NOT_IMPLEMENTED", row_045)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

    def test_99b_agent_ingress_pending_reservation_regression_fails(self) -> None:
        """Removing the frozen pending-reservation invariant must fail closed."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            # Minimal Platform_17 without the frozen invariant / §7.1 owner.
            (tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md").write_text(
                "# Platform 17\n\n## 7. Telemetry durability\n\n"
                "reserve then publish then finalize.\n",
                encoding="utf-8",
            )
            (tmp_docs / "Platform_05_LLD.md").write_text(
                "# LLD\n\nNo pending-reservation binding.\n",
                encoding="utf-8",
            )
            (tmp_docs / "Platform_06_Dataflow.md").write_text(
                "# Dataflow\n\nNo pending-reservation binding.\n",
                encoding="utf-8",
            )
            (Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                "# Register\n\n| BUILD-003 | transport | x | y | CERTIFIED | z |\n",
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected pending-reservation failures")
            self.assertIn("pending-reservation", joined.lower())
            self.assertIn("MUST NOT", joined)

    def test_99c_agent_ingress_zero_pending_upgrade_gate(self) -> None:
        """Authority-checkpoint upgrade gate + crash recovery + BUILD-127 ownership."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p16 = (
            docs / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        ).read_text(encoding="utf-8")
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        # Migration abort on active ownership; dormant legacy may preserve.
        for needle in (
            "If the active-writer / active-publisher-lease drain precondition is violated, migration MUST abort",
            "Migration apply MUST be transactional",
            "U1 clean migration",
            "U2 dormant legacy pending migration",
            "U4 active publisher blocks migration",
            "Dormant legacy pending alone MUST NOT abort migration",
            "Failure is observable/audited",
            "CUTOVER-RACE-1",
            "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
            "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            "LEGACY_UNKNOWN",
            "J7",
        ):
            self.assertIn(needle, p17, needle)

        # Crash/restart exact retry recovery fixture under BUILD-062.
        self.assertIn(
            "Crash after mutable validation failure, before pending release",
            p17,
        )
        self.assertIn("Fixture 6 is a required BUILD-062 failure fixture", p17)
        ledger_062 = next(
            line
            for line in register.splitlines()
            if line.startswith("| 062 |")
        )
        self.assertIn(
            "crash after mutable-validation failure before pending release",
            ledger_062,
        )
        self.assertIn("exact retry reconciles/releases unpublished pending", ledger_062)

        # BUILD-127 owns the upgrade/cutover gate; BUILD-062 owns runtime fence.
        self.assertIn("LEGACY_UNKNOWN", register)
        row_127 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-127 |")
        )
        self.assertIn("LEGACY_UNKNOWN", row_127)
        self.assertIn("hard cutover fence", row_127)
        self.assertIn(
            "zero-pending-as-mandatory-migration-blocker",
            row_127,
        )
        self.assertIn("CUTOVER-RACE-1", register)
        row_062 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-062 |")
        )
        self.assertIn(
            "Active-writer-drain **upgrade/migration cutover certification**",
            row_062,
        )
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", row_062)
        self.assertIn("BUILD-127", row_062)
        self.assertIn("AUTHORITY_CHECKPOINT_RECOVERY", row_062)

        # Platform_16 / §119.21 preflight surface + hard cutover fence.
        self.assertIn("Enter STAGE 1 drain mode", p16)
        self.assertIn("Enter STAGE 2 hard cutover fence", p16)
        self.assertIn("upgrade aborts at preflight/migration gate", p16)
        self.assertIn("STAGE 1 drain mode", p06)
        self.assertIn("STAGE 2 hard cutover fence", p06)
        self.assertIn("publisher leases=0", p06)
        self.assertIn("CUTOVER-RACE-1", register)
        self.assertIn("hard cutover fence", row_127)
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", row_062)

        # No regression of sFlow + status truth.
        self.assertIn("sFlow v5", register)
        self.assertIn("S5 sFlow v5", register)
        for build_id, required in (
            ("BUILD-043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-051", "NOT_IMPLEMENTED"),
        ):
            row = next(
                line
                for line in register.splitlines()
                if line.startswith(f"| {build_id} |")
            )
            self.assertIn(required, row, build_id)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

    def test_99d_agent_ingress_zero_pending_upgrade_gate_regression_fails(
        self,
    ) -> None:
        """Dropping authority-checkpoint upgrade gate / BUILD-127 ownership fails closed."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17_full = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        # Keep §7.1 runtime contract but strip the upgrade-gate subsection.
        stripped = p17_full.replace(
            "#### 7.1.9 Schema migration / release-guard upgrade precondition",
            "#### 7.1.9 REMOVED",
        )
        self.assertNotIn(
            "#### 7.1.9 Schema migration / release-guard upgrade precondition",
            stripped,
        )
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            (
                tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
            ).write_text(stripped, encoding="utf-8")
            for name in (
                "Platform_05_LLD.md",
                "Platform_06_Dataflow.md",
                "Platform_16_Installation_Upgrade_Rollback_and_Operations.md",
            ):
                (tmp_docs / name).write_text(
                    (docs / name).read_text(encoding="utf-8"),
                    encoding="utf-8",
                )
            # Register without BUILD-127 LEGACY_UNKNOWN / supersession ownership.
            register_bad = (
                "# Register\n\n"
                "| BUILD-003 | transport | x | y | CERTIFIED — does **not** "
                "reopen this certified record | z |\n"
                "| BUILD-024 | agent gateway | pending-reservation | "
                "`Platform_17` §7.1 | Partial | z |\n"
                "| BUILD-062 | ingestion | runtime pending-reservation | "
                "`Platform_17` §7.1 | Partial | z |\n"
                "| BUILD-127 | upgrade | matrix only | 126 | Incomplete | "
                "Upgrade matrix |\n"
                "| BUILD-043 | net | x | y | SPECIFIED_ONLY / NOT_IMPLEMENTED | z |\n"
                "| BUILD-044 | net | x | y | SPECIFIED_ONLY / NOT_IMPLEMENTED | z |\n"
                "| BUILD-051 | flow | x | y | NOT_IMPLEMENTED | z |\n"
            )
            (
                Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md"
            ).write_text(register_bad, encoding="utf-8")
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(
                validator.errors, "expected authority-checkpoint gate failures"
            )
            self.assertTrue(
                "7.1.9" in joined
                or "LEGACY_UNKNOWN" in joined
                or "AUTHORITY_CHECKPOINT" in joined
                or "BUILD-127" in joined
                or "zero-pending" in joined.lower(),
                joined,
            )

    def test_99e_agent_ingress_hard_cutover_fence_and_races(self) -> None:
        """Hard cutover fence, fenced zero-check, and CUTOVER-RACE-1..4."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p16 = (
            docs / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        ).read_text(encoding="utf-8")
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        for needle in (
            "STAGE 1 — DRAIN MODE",
            "STAGE 2 — HARD CUTOVER FENCE",
            "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
            "check-then-unlock-then-migrate gap",
            "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1",
            "SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW",
            "per-Sensor lock alone does **not** fence",
            "CUTOVER-RACE-1",
            "CUTOVER-RACE-2",
            "CUTOVER-RACE-3",
            "CUTOVER-RACE-4",
            "CUTOVER-LOCK-1",
            "CUTOVER-LOCK-10",
            "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER",
            "final active-ownership check and migration MUST share one uninterrupted",
            "create no reservation",
            "no terminal disposition",
            "no deletion authorization",
            "no watermark change",
        ):
            self.assertIn(needle, p17, needle)

        # CUTOVER-RACE-1: either abort on visible active ownership OR fence reject.
        self.assertIn(
            "retry enters before hard fence and becomes visible active pending/writer work",
            p17,
        )
        self.assertIn(
            "hard fence wins first and retry receives retryable",
            p17,
        )
        self.assertIn(
            "Forbidden: migration commits while the retry creates or owns",
            p17,
        )
        # CUTOVER-RACE-2 in-flight writer drain.
        self.assertIn(
            "cutover waits for it to terminate/commit/rollback",
            p17,
        )
        # CUTOVER-RACE-3 publisher lease block.
        self.assertIn("Publisher lease exists at cutover", p17)
        self.assertIn("Migration must not proceed", p17)
        # CUTOVER-RACE-4 post-commit resume.
        self.assertIn(
            "exact retained Sensor request retries normally under the upgraded runtime",
            p17,
        )
        self.assertIn(
            "no duplicate deletion-authorizing ACK or false sequence gap",
            p17,
        )

        for needle in (
            "STAGE 1 — DRAIN MODE",
            "STAGE 2 — HARD CUTOVER FENCE",
            "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
            "check-then-unlock-then-migrate gap is forbidden",
            "while fenced",
            "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1",
            "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER",
            "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW",
            "Do not introduce a third witness/coordinator",
        ):
            self.assertIn(needle, p16, needle)

        self.assertIn("STAGE 2 hard cutover fence", p06)
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", p06)
        self.assertIn("CUTOVER-RACE abort", p06)
        self.assertIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", p06)
        self.assertIn("UNSUPPORTED_DIRECT_SCHEMA_CUTOVER", p06)

        self.assertIn("CUTOVER-RACE-1", register)
        self.assertIn("CUTOVER-LOCK-1", register)
        self.assertIn("hard cutover fence", register)
        row_127 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-127 |")
        )
        self.assertIn("hard cutover fence", row_127)
        self.assertIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", row_127)
        self.assertIn("CUTOVER-LOCK-1", row_127)
        row_062 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-062 |")
        )
        self.assertIn("ERR_PLATFORM_UPGRADE_IN_PROGRESS", row_062)
        self.assertIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", row_062)

        # Prior P0 closures must remain intact.
        self.assertIn("sFlow v5", register)
        for build_id, required in (
            ("BUILD-043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-051", "NOT_IMPLEMENTED"),
        ):
            row = next(
                line
                for line in register.splitlines()
                if line.startswith(f"| {build_id} |")
            )
            self.assertIn(required, row, build_id)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

    def test_99f_agent_ingress_hard_cutover_regression_fails(self) -> None:
        """Dropping hard-cutover fence / race fixtures must fail closed."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17_full = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        stripped = (
            p17_full.replace("STAGE 2 — HARD CUTOVER FENCE", "STAGE 2 REMOVED")
            .replace("CUTOVER-RACE-1", "RACE-REMOVED-1")
            .replace("ERR_PLATFORM_UPGRADE_IN_PROGRESS", "ERR_REMOVED")
        )
        self.assertNotIn("STAGE 2 — HARD CUTOVER FENCE", stripped)
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            (
                tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
            ).write_text(stripped, encoding="utf-8")
            for name in (
                "Platform_05_LLD.md",
                "Platform_06_Dataflow.md",
                "Platform_16_Installation_Upgrade_Rollback_and_Operations.md",
            ):
                (tmp_docs / name).write_text(
                    (docs / name).read_text(encoding="utf-8"),
                    encoding="utf-8",
                )
            (
                Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md"
            ).write_text(
                (
                    root / "Platform_Master_Build_Sequence_Register_v1.0.md"
                ).read_text(encoding="utf-8"),
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected hard-cutover failures")
            self.assertTrue(
                "HARD CUTOVER" in joined
                or "CUTOVER-RACE" in joined
                or "ERR_PLATFORM_UPGRADE_IN_PROGRESS" in joined,
                joined,
            )

    def test_99g_agent_ingress_global_cutover_lock_protocol(self) -> None:
        """Global AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 protocol must stay frozen."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p16 = (
            docs / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        ).read_text(encoding="utf-8")
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        for needle in (
            "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1",
            "SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW",
            "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER",
            "both Cores must honor the same PostgreSQL global cutover lock",
            "CUTOVER-LOCK-1",
            "CUTOVER-LOCK-2",
            "CUTOVER-LOCK-3",
            "CUTOVER-LOCK-4",
            "CUTOVER-LOCK-5",
            "CUTOVER-LOCK-6",
            "CUTOVER-LOCK-7",
            "CUTOVER-LOCK-8",
            "CUTOVER-LOCK-9",
            "CUTOVER-LOCK-10",
            "ordinary reservation holds SHARED",
            "old-version binary without global-lock support",
            "two-Core profile proves neither Core can mutate",
            "migration failure rolls back schema and releases EXCLUSIVE",
        ):
            self.assertIn(needle, p17, needle)

        for needle in (
            "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1",
            "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER",
            "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW",
            "Do not introduce a third witness/coordinator",
            "SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
        ):
            self.assertIn(needle, p16, needle)

        self.assertIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", p06)
        self.assertIn("UNSUPPORTED_DIRECT_SCHEMA_CUTOVER", p06)
        self.assertIn("CUTOVER-LOCK-1", register)
        self.assertIn("UNSUPPORTED_DIRECT_SCHEMA_CUTOVER", register)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

    def test_99h_agent_ingress_global_cutover_lock_regression_fails(self) -> None:
        """Dropping AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 must fail closed."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17_full = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        stripped = p17_full.replace(
            "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", "LOCK_REMOVED"
        ).replace("CUTOVER-LOCK-1", "LOCKFIX-REMOVED-1")
        self.assertNotIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", stripped)
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            (
                tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
            ).write_text(stripped, encoding="utf-8")
            for name in (
                "Platform_05_LLD.md",
                "Platform_06_Dataflow.md",
                "Platform_16_Installation_Upgrade_Rollback_and_Operations.md",
            ):
                (tmp_docs / name).write_text(
                    (docs / name).read_text(encoding="utf-8"),
                    encoding="utf-8",
                )
            (
                Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md"
            ).write_text(
                (
                    root / "Platform_Master_Build_Sequence_Register_v1.0.md"
                ).read_text(encoding="utf-8"),
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected global cutover lock failures")
            self.assertTrue(
                "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1" in joined
                or "CUTOVER-LOCK" in joined,
                joined,
            )

    def test_99i_agent_ingress_authority_checkpoint_recovery_contract(self) -> None:
        """Authority checkpoint + LEGACY_UNKNOWN supersession must stay frozen."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p16 = (
            docs / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        ).read_text(encoding="utf-8")
        p14 = (
            docs / "Platform_14_Test_Performance_and_Release_Quality.md"
        ).read_text(encoding="utf-8")
        p12 = (
            docs / "Platform_12_Decision_Source_Register.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        for needle in (
            "#### 7.1.10 Durable authority checkpoint",
            "UNVALIDATED",
            "AUTHORITY_VALIDATED",
            "LEGACY_UNKNOWN",
            "#### 7.1.11 Historical temporal authority",
            "assignmentGeneration",
            "SensorAcceptanceEvidenceV1",
            "PendingCanonicalBindingV1",
            "##### 7.1.11.4 UNVALIDATED exact retry",
            "J8_UNVALIDATED_CRASH_THEN_REVOCATION",
            "ERR_AGENT_ASSIGNMENT_REVOKED_AT_ACCEPTANCE",
            "ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN",
            "#### 7.1.12 AUTHORITY_VALIDATED retry",
            "#### 7.1.13 LEGACY_UNKNOWN migration and recovery",
            "LEGACY_PENDING_RECONCILIATION_REQUIRED",
            "#### 7.1.14 Publication-uncertain authority change — mandatory case J7",
            "U1 clean migration",
            "U2 dormant legacy pending migration",
            "U3 multi-tenant legacy pending",
            "U4 active publisher blocks migration",
            "U5A",
            "U5B",
            "U6 insufficient historical proof",
            "U7 accepted/settled history unchanged",
            "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            "publisher cannot claim `UNVALIDATED` or `LEGACY_UNKNOWN`",
            "authority checkpoint cannot authorize new sequences",
            "raw `acceptedAt` alone is insufficient",
            "0 <= clock_max_error_ms <= 5000",
            "0 <= clock_sync_age_ms <= 300000",
            "clock_max_error_ms = -1",
            "clock_sync_age_ms = -1",
            "acceptance_lower <= accepted_at_utc",
            "accepted_at_utc <= acceptance_upper",
            "acceptance_lower <= acceptance_upper",
            "UNQUALIFIED / INVALID",
            "MUST NEVER become more permissive than missing clock evidence",
            "No transition to `AUTHORITY_VALIDATED`",
            "Publisher claim before `AUTHORITY_VALIDATED` is forbidden",
            "replay uses persisted Entity A",
            "does **not** resolve to Entity B",
        ):
            self.assertIn(needle, p17, needle)

        self.assertIn(
            "SUPERSEDED: zero-pending as mandatory migration blocker",
            p16,
        )
        self.assertIn("LEGACY_UNKNOWN", p16)
        self.assertIn("U5A", p16)
        self.assertIn("U5B", p16)
        self.assertIn("### 3.2 Agent-ingress authority-checkpoint", p14)
        self.assertIn("U5A", p14)
        self.assertIn("U5B", p14)
        self.assertIn("J8_UNVALIDATED_CRASH_THEN_REVOCATION", p14)
        self.assertIn("### 29.10 Agent-ingress authority-checkpoint", p12)
        self.assertIn("SensorAcceptanceEvidenceV1", p12)
        self.assertIn("PendingCanonicalBindingV1", p12)
        self.assertIn(
            "supersedes zero-pending-as-mandatory-migration-blocker",
            register,
        )
        self.assertIn("AUTHORITY_CHECKPOINT_RECOVERY", register)
        self.assertIn("SENSOR_ACCEPTANCE_EVIDENCE_V1", register)
        self.assertIn("PENDING_CANONICAL_BINDING_V1", register)
        self.assertIn("J8_UNVALIDATED_CRASH_THEN_REVOCATION", register)
        # Preserve prior closures.
        self.assertIn("AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1", p17)
        self.assertIn("sFlow v5", register)
        for build_id, required in (
            ("BUILD-043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
            ("BUILD-051", "NOT_IMPLEMENTED"),
        ):
            row = next(
                line
                for line in register.splitlines()
                if line.startswith(f"| {build_id} |")
            )
            self.assertIn(required, row, build_id)
        row_042 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-042 |")
        )
        self.assertIn("PARTIAL", row_042)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

    def test_99j_authority_checkpoint_p1_qualification_negatives(self) -> None:
        """P1 freezes: clock qualification, UNVALIDATED retry, canonical binding."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p14 = (
            docs / "Platform_14_Test_Performance_and_Release_Quality.md"
        ).read_text(encoding="utf-8")
        p05 = (docs / "Platform_05_LLD.md").read_text(encoding="utf-8")

        # Raw acceptedAt alone is insufficient.
        self.assertIn("raw `acceptedAt` alone is insufficient", p17)
        self.assertIn("raw `acceptedAt` alone is insufficient", p14)
        self.assertNotRegex(
            p17,
            r"Sensor `acceptedAt`;\s*$",
            "raw acceptedAt must not remain the sole historical authority input",
        )

        # Clock qualification negatives (unsigned/nonnegative + ordered interval).
        self.assertIn("0 <= clock_max_error_ms <= 5000", p17)
        self.assertIn("0 <= clock_sync_age_ms <= 300000", p17)
        self.assertIn("0 <= clock_max_error_ms <= 5000", p14)
        self.assertIn("0 <= clock_sync_age_ms <= 300000", p14)
        self.assertIn("clock_max_error_ms = -1", p17)
        self.assertIn("clock_sync_age_ms = -1", p17)
        self.assertIn("clock_max_error_ms = -1", p14)
        self.assertIn("clock_sync_age_ms = -1", p14)
        self.assertIn("clock_max_error_ms > 5000", p14)
        self.assertIn("clock_sync_age_ms > 300000", p14)
        self.assertIn("clock_max_error_ms = 0", p17)
        self.assertIn("clock_sync_age_ms = 0", p17)
        self.assertIn("acceptance_lower <= accepted_at_utc", p17)
        self.assertIn("accepted_at_utc <= acceptance_upper", p17)
        self.assertIn("acceptance_lower <= acceptance_upper", p17)
        self.assertIn("acceptance_lower <= accepted_at_utc", p14)
        self.assertIn("accepted_at_utc <= acceptance_upper", p14)
        self.assertIn("acceptance_lower <= acceptance_upper", p14)
        self.assertIn("UNQUALIFIED / INVALID", p17)
        self.assertIn(
            "MUST NEVER become more permissive than missing clock evidence",
            p17,
        )
        self.assertIn("No transition to `AUTHORITY_VALIDATED`", p17)
        self.assertIn(
            "MUST NOT transition to `AUTHORITY_VALIDATED`",
            p14,
        )
        self.assertIn(
            "uncertainty overlapping revocation fails historical authority",
            p17,
        )
        self.assertIn(
            "Exact equality with the revocation boundary is **not** historically authorized",
            p17,
        )
        self.assertIn("acceptance_upper == revoked_at", p17)
        self.assertIn("acceptance_upper == revoked_at", p14)
        self.assertIn("ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN", p14)

        # UNVALIDATED qualified historical retry can become AUTHORITY_VALIDATED.
        self.assertIn("UNVALIDATED → AUTHORITY_VALIDATED", p17)
        self.assertIn("J8_UNVALIDATED_CRASH_THEN_REVOCATION", p17)
        self.assertIn(
            "qualified historical acceptance → `AUTHORITY_VALIDATED`",
            p17,
        )

        # Publisher cannot claim before AUTHORITY_VALIDATED.
        self.assertIn(
            "Publisher claim before `AUTHORITY_VALIDATED` is forbidden",
            p17,
        )
        self.assertIn(
            "Publisher claim MUST NOT occur unless exact pending state is `AUTHORITY_VALIDATED`",
            p17,
        )

        # Canonical binding snapshot required before AUTHORITY_VALIDATED publication.
        self.assertIn("PendingCanonicalBindingV1", p17)
        self.assertIn("pending_canonical_batch_sha256", p17)
        self.assertIn(
            "canonical binding snapshot is required before `AUTHORITY_VALIDATED` publication",
            p17,
        )
        self.assertIn(
            "PendingCanonicalBindingV1",
            p05,
        )

        # Exact replay remains Entity A after current binding becomes Entity B.
        self.assertIn("replay uses persisted Entity A", p17)
        self.assertIn("does **not** resolve to Entity B", p17)
        self.assertIn(
            "changed request body cannot use Entity A snapshot",
            p17,
        )
        self.assertIn(
            "do **NOT** re-resolve entity identity against the current live entity binding",
            p17,
        )
        self.assertIn(
            "A current entity-binding change cannot redirect already validated pending telemetry to another entity",
            p17,
        )

        # LEGACY_UNKNOWN without reproducible canonical identity → stream-local reconciliation.
        self.assertIn("U5B", p17)
        self.assertIn(
            "canonical mapping not reproducible",
            p17,
        )
        self.assertIn(
            "Current mutable entity resolution alone is **NOT** sufficient proof after a binding change",
            p17,
        )
        self.assertIn("U5B", p14)

        # Maturity honesty remains PENDING_IMPLEMENTATION (no runtime claim).
        self.assertIn(
            "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            p17,
        )
        self.assertIn(
            "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            p17,
        )
        self.assertIn(
            "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            p17,
        )
        self.assertIn(
            "does **NOT** implement authority-checkpoint recovery",
            p17,
        )

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

        # Negative: stripping SensorAcceptanceEvidenceV1 fails closed.
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            stripped = (
                p17.replace("SensorAcceptanceEvidenceV1", "REMOVED_EVIDENCE")
                .replace("PendingCanonicalBindingV1", "REMOVED_BINDING")
                .replace("J8_UNVALIDATED_CRASH_THEN_REVOCATION", "REMOVED_J8")
                .replace("U5A", "REMOVED_U5A")
                .replace("U5B", "REMOVED_U5B")
            )
            (
                tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
            ).write_text(stripped, encoding="utf-8")
            (tmp_docs / "Platform_05_LLD.md").write_text(
                "# LLD\nPlatform_17` §7.1\n"
                "A deterministic pre-publication non-terminal rejection MUST NOT "
                "leave an unpublished durable pending reservation blocking continuity.\n"
                "ERR_AGENT_IDENTITY_CONFLICT\n",
                encoding="utf-8",
            )
            (tmp_docs / "Platform_06_Dataflow.md").write_text(
                "# Dataflow\nPlatform_17` §7.1\n"
                "A deterministic pre-publication non-terminal rejection MUST NOT "
                "leave an unpublished durable pending reservation blocking continuity.\n"
                "### 119.21 Upgrade / rollback\n"
                "active Agent-ingress publisher/writer ownership = non-zero\n"
                "LEGACY_UNKNOWN\nSTAGE 1 drain mode\nSTAGE 2 hard cutover fence\n"
                "ERR_PLATFORM_UPGRADE_IN_PROGRESS\nAGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1\n"
                "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER\n"
                "No Core partial-schema state is valid\nAUTHORITY_CHECKPOINT_RECOVERY\n",
                encoding="utf-8",
            )
            (
                tmp_docs
                / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
            ).write_text(
                "# Ops\n"
                "### 5.1 Phase-1 standalone preflight — Agent-ingress "
                "pending-reservation upgrade gate\n"
                "SUPERSEDED: zero-pending as mandatory migration blocker\n"
                "active Agent-ingress publisher/writer ownership = non-zero\n"
                "LEGACY_UNKNOWN\n"
                "MUST NOT delete pending Agent-ingress rows manually\n"
                "Verify no active Agent-ingress publisher leases remain\n"
                "Verify no in-flight Agent-ingress writers\n"
                "STAGE 1 — DRAIN MODE\nSTAGE 2 — HARD CUTOVER FENCE\n"
                "ERR_PLATFORM_UPGRADE_IN_PROGRESS\n"
                "check-then-unlock-then-migrate gap is forbidden\n"
                "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1\n"
                "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER\n"
                "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW\n"
                "SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED\n"
                "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED\n"
                "Do not introduce a third witness/coordinator\n",
                encoding="utf-8",
            )
            (Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                "# Register\n"
                "| BUILD-003 | transport | x | y | CERTIFIED | does **not** reopen this certified record |\n"
                "| BUILD-043 | x | y | z | SPECIFIED_ONLY / NOT_IMPLEMENTED | a |\n"
                "| BUILD-044 | x | y | z | SPECIFIED_ONLY / NOT_IMPLEMENTED | a |\n"
                "| BUILD-051 | x | y | z | NOT_IMPLEMENTED | a |\n"
                "| BUILD-062 | runtime pending | x | y | Significant partial | AUTHORITY_CHECKPOINT_RECOVERY ERR_PLATFORM_UPGRADE_IN_PROGRESS AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 crash-after-mutable-validation-before-pending-release |\n"
                "| BUILD-127 | LEGACY_UNKNOWN zero-pending-as-mandatory-migration-blocker hard cutover fence AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 CUTOVER-LOCK-1 | x | y | Incomplete | U1 |\n"
                "| 127 | Upgrade | x | AUTHORITY_CHECKPOINT_RECOVERY | D | U1 clean migration U2 dormant legacy pending U4 active publisher manual pending deletion/reset is **not** accepted remediation | r | Incomplete |\n",
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected P1 freeze failures")
            self.assertTrue(
                "SensorAcceptanceEvidenceV1" in joined
                or "PendingCanonicalBindingV1" in joined
                or "J8_UNVALIDATED" in joined
                or "U5A" in joined
                or "U5B" in joined,
                joined,
            )

    def test_99k_sensor_acceptance_clock_range_evidence_regressions(self) -> None:
        """P1: nonnegative clock bounds, ordered interval, malformed fail-closed."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p17_path = docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        p14_path = docs / "Platform_14_Test_Performance_and_Release_Quality.md"
        p12_path = docs / "Platform_12_Decision_Source_Register.md"
        p17 = p17_path.read_text(encoding="utf-8")
        p14 = p14_path.read_text(encoding="utf-8")
        p12 = p12_path.read_text(encoding="utf-8")

        # Positive contract presence.
        for needle in (
            "0 <= clock_max_error_ms <= 5000",
            "0 <= clock_sync_age_ms <= 300000",
            "clock_max_error_ms = -1",
            "clock_sync_age_ms = -1",
            "clock_max_error_ms = 5001",
            "clock_sync_age_ms = 300001",
            "clock_max_error_ms = 0",
            "clock_sync_age_ms = 0",
            "acceptance_lower <= accepted_at_utc",
            "accepted_at_utc <= acceptance_upper",
            "acceptance_lower <= acceptance_upper",
            "acceptance_upper == revoked_at",
            "UNQUALIFIED / INVALID",
            "MUST NEVER become more permissive than missing clock evidence",
            "No transition to `AUTHORITY_VALIDATED`",
            "No wrapping, saturation or silent coercion",
            "checked duration arithmetic",
            "ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN",
        ):
            self.assertIn(needle, p17, needle)
        for needle in (
            "0 <= clock_max_error_ms <= 5000",
            "0 <= clock_sync_age_ms <= 300000",
            "clock_max_error_ms = -1",
            "clock_sync_age_ms = -1",
            "acceptance_lower <= accepted_at_utc",
            "accepted_at_utc <= acceptance_upper",
            "acceptance_lower <= acceptance_upper",
            "acceptance_upper == revoked_at",
            "MUST NEVER become more permissive than missing clock evidence",
            "MUST NOT transition to `AUTHORITY_VALIDATED`",
        ):
            self.assertIn(needle, p14, needle)
        self.assertIn("0 <= clock_max_error_ms <= 5000", p12)
        self.assertIn("0 <= clock_sync_age_ms <= 300000", p12)
        self.assertIn(
            "MUST NEVER be more permissive than missing clock evidence",
            p12,
        )

        # Maturity honesty preserved.
        self.assertIn(
            "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            p17,
        )
        self.assertIn(
            "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED",
            p17,
        )
        self.assertIn(
            "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION",
            p17,
        )

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_agent_ingress_pending_reservation_safety()
        self.assertEqual(validator.errors, [])

        # Regression: drop nonnegative floors / interval ordering / security text.
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            weakened = (
                p17.replace("0 <= clock_max_error_ms <= 5000", "clock_max_error_ms <= 5000")
                .replace("0 <= clock_sync_age_ms <= 300000", "clock_sync_age_ms <= 300000")
                .replace("clock_max_error_ms = -1", "REMOVED_NEG_ERROR")
                .replace("clock_sync_age_ms = -1", "REMOVED_NEG_AGE")
                .replace(
                    "acceptance_lower <= accepted_at_utc",
                    "REMOVED_LOWER_ORDER",
                )
                .replace(
                    "accepted_at_utc <= acceptance_upper",
                    "REMOVED_UPPER_ORDER",
                )
                .replace(
                    "acceptance_lower <= acceptance_upper",
                    "REMOVED_INTERVAL_ORDER",
                )
                .replace(
                    "MUST NEVER become more permissive than missing clock evidence",
                    "REMOVED_SECURITY_INVARIANT",
                )
                .replace(
                    "No transition to `AUTHORITY_VALIDATED`",
                    "malformed evidence MAY transition to AUTHORITY_VALIDATED",
                )
                .replace(
                    "Exact equality `acceptance_upper == revoked_at` is **NOT** authorized.",
                    "Exact equality `acceptance_upper == revoked_at` is authorized.",
                )
            )
            (
                tmp_docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
            ).write_text(weakened, encoding="utf-8")
            # Minimal companions so earlier required-doc checks do not dominate.
            for name, body in (
                (
                    "Platform_05_LLD.md",
                    "# LLD\nPlatform_17` §7.1\n"
                    "A deterministic pre-publication non-terminal rejection MUST NOT "
                    "leave an unpublished durable pending reservation blocking continuity.\n"
                    "ERR_AGENT_IDENTITY_CONFLICT\nSensorAcceptanceEvidenceV1\n"
                    "PendingCanonicalBindingV1\nJ8_UNVALIDATED_CRASH_THEN_REVOCATION\n"
                    "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n",
                ),
                (
                    "Platform_06_Dataflow.md",
                    "# Dataflow\nPlatform_17` §7.1\n"
                    "A deterministic pre-publication non-terminal rejection MUST NOT "
                    "leave an unpublished durable pending reservation blocking continuity.\n"
                    "### 119.21 Upgrade / rollback\n"
                    "active Agent-ingress publisher/writer ownership = non-zero\n"
                    "LEGACY_UNKNOWN\nSTAGE 1 drain mode\nSTAGE 2 hard cutover fence\n"
                    "ERR_PLATFORM_UPGRADE_IN_PROGRESS\nAGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1\n"
                    "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER\n"
                    "No Core partial-schema state is valid\nAUTHORITY_CHECKPOINT_RECOVERY\n"
                    "SENSOR_ACCEPTANCE_EVIDENCE_V1\nPENDING_CANONICAL_BINDING_V1\n"
                    "U5A\nU5B\n",
                ),
                (
                    "Platform_14_Test_Performance_and_Release_Quality.md",
                    "# Tests\n### 3.2 Agent-ingress authority-checkpoint\n"
                    "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "U5A\nU5B\nJ8_UNVALIDATED_CRASH_THEN_REVOCATION\n"
                    "raw `acceptedAt` alone is insufficient\n"
                    "0 <= clock_max_error_ms <= 5000\n0 <= clock_sync_age_ms <= 300000\n"
                    "clock_max_error_ms = -1\nclock_sync_age_ms = -1\n"
                    "clock_max_error_ms > 5000\nclock_sync_age_ms > 300000\n"
                    "acceptance_lower <= accepted_at_utc\n"
                    "accepted_at_utc <= acceptance_upper\n"
                    "acceptance_lower <= acceptance_upper\n"
                    "acceptance_upper == revoked_at\nUNQUALIFIED / INVALID\n"
                    "MUST NEVER become more permissive than missing clock evidence\n"
                    "MUST NOT transition to `AUTHORITY_VALIDATED`\n"
                    "ERR_AGENT_ACCEPTANCE_ORDER_UNCERTAIN\n",
                ),
                (
                    "Platform_12_Decision_Source_Register.md",
                    "# Decisions\n### 29.10 Agent-ingress authority-checkpoint\n"
                    "SensorAcceptanceEvidenceV1\nPendingCanonicalBindingV1\n"
                    "J8_UNVALIDATED_CRASH_THEN_REVOCATION\nU5A\nU5B\n"
                    "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "0 <= clock_max_error_ms <= 5000\n0 <= clock_sync_age_ms <= 300000\n"
                    "acceptance_lower <= accepted_at_utc <= acceptance_upper\n"
                    "MUST NEVER be more permissive than missing clock evidence\n"
                    "UNQUALIFIED / INVALID\n",
                ),
                (
                    "Platform_16_Installation_Upgrade_Rollback_and_Operations.md",
                    "# Ops\n"
                    "### 5.1 Phase-1 standalone preflight — Agent-ingress "
                    "pending-reservation upgrade gate\n"
                    "SUPERSEDED: zero-pending as mandatory migration blocker\n"
                    "active Agent-ingress publisher/writer ownership = non-zero\n"
                    "LEGACY_UNKNOWN\n"
                    "MUST NOT delete pending Agent-ingress rows manually\n"
                    "Verify no active Agent-ingress publisher leases remain\n"
                    "Verify no in-flight Agent-ingress writers\n"
                    "STAGE 1 — DRAIN MODE\nSTAGE 2 — HARD CUTOVER FENCE\n"
                    "ERR_PLATFORM_UPGRADE_IN_PROGRESS\n"
                    "check-then-unlock-then-migrate gap is forbidden\n"
                    "AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1\n"
                    "PROFILE_BLOCKED / UNSUPPORTED_DIRECT_SCHEMA_CUTOVER\n"
                    "GLOBAL CUTOVER SHARED → PER-SENSOR / ROW\n"
                    "SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED\n"
                    "AUTHORITY_CHECKPOINT_RECOVERY = SPECIFIED / PENDING_IMPLEMENTATION / NOT_CERTIFIED\n"
                    "SENSOR_ACCEPTANCE_EVIDENCE_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "PENDING_CANONICAL_BINDING_V1 = SPECIFIED / PENDING_IMPLEMENTATION\n"
                    "U5A\nU5B\n"
                    "Do not introduce a third witness/coordinator\n",
                ),
            ):
                (tmp_docs / name).write_text(body, encoding="utf-8")
            (Path(tmp) / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                "# Register\n"
                "| BUILD-003 | transport | x | y | CERTIFIED | does **not** reopen this certified record |\n"
                "| BUILD-043 | x | y | z | SPECIFIED_ONLY / NOT_IMPLEMENTED | a |\n"
                "| BUILD-044 | x | y | z | SPECIFIED_ONLY / NOT_IMPLEMENTED | a |\n"
                "| BUILD-051 | x | y | z | NOT_IMPLEMENTED | a |\n"
                "| BUILD-062 | runtime pending | x | y | Significant partial | AUTHORITY_CHECKPOINT_RECOVERY SENSOR_ACCEPTANCE_EVIDENCE_V1 PENDING_CANONICAL_BINDING_V1 J8_UNVALIDATED_CRASH_THEN_REVOCATION ERR_PLATFORM_UPGRADE_IN_PROGRESS AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 crash after mutable-validation failure before pending release |\n"
                "| BUILD-127 | LEGACY_UNKNOWN zero-pending-as-mandatory-migration-blocker hard cutover fence AGENT_INGRESS_SCHEMA_CUTOVER_LOCK_V1 CUTOVER-LOCK-1 | x | y | Incomplete | U1 |\n"
                "| 127 | Upgrade | x | AUTHORITY_CHECKPOINT_RECOVERY | D | U1 clean migration U2 dormant legacy pending U4 active publisher manual pending deletion/reset is **not** accepted remediation | r | Incomplete |\n"
                "Platform_17` §7.1\npending-reservation\nBUILD-024\n"
                "SensorAcceptanceEvidenceV1\nPendingCanonicalBindingV1\n"
                "J8_UNVALIDATED_CRASH_THEN_REVOCATION\nU5A\nU5B\n"
                "supersedes zero-pending-as-mandatory-migration-blocker\n"
                "CUTOVER-RACE-1\nCUTOVER-LOCK-1\nERR_PLATFORM_UPGRADE_IN_PROGRESS\n"
                "UNSUPPORTED_DIRECT_SCHEMA_CUTOVER\n",
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_agent_ingress_pending_reservation_safety()
            joined = "\n".join(validator.errors)
            self.assertTrue(
                validator.errors,
                "expected clock-range / uncertainty regression failures",
            )
            self.assertTrue(
                "0 <= clock_max_error_ms <= 5000" in joined
                or "nonnegative" in joined
                or "acceptance_lower <= accepted_at_utc" in joined
                or "more permissive" in joined
                or "AUTHORITY_VALIDATED" in joined
                or "revocation-boundary" in joined
                or "NOT authorized" in joined
                or "negative" in joined,
                joined,
            )

    def test_99g_build_042_counter_leaf_source_truth(self) -> None:
        """BUILD-042 must reflect PR #304 leaf without certification claims."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")

        required = (
            "PARTIAL — exact raw Counter64 + counter derivation leaf "
            "implemented; integration/benchmark/certification pending"
        )
        row_042 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-042 |")
        )
        ledger_042 = next(
            line
            for line in register.splitlines()
            if line.startswith("| 042 |")
        )
        self.assertIn(required, row_042)
        self.assertIn(required, ledger_042)
        self.assertNotEqual(
            [c.strip() for c in row_042.strip().strip("|").split("|")][4],
            "Raw Counter64 exists",
        )
        self.assertIn(required, p17)
        self.assertIn("PR #304", p17)
        self.assertIn("sensor/internal/protocol/snmp/counter/", p17)
        self.assertIn("no whole BUILD-042 certification", p17)
        self.assertIn("no BUILD-048 Network Sensor certification", p17)
        self.assertIn("no deployment claim", p17)
        self.assertNotIn(
            "That derived Counter32/Counter64 reset/wrap/discontinuity/"
            "rate layer remains **SPECIFIED but NOT IMPLEMENTED/CERTIFIED**",
            p17,
        )

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_build_042_counter_leaf_source_truth()
        self.assertEqual(validator.errors, [])

    def test_99h_build_042_obsolete_raw_only_status_fails(self) -> None:
        """Regressing BUILD-042 to Raw Counter64 exists must fail closed."""
        docs = Path(__file__).resolve().parents[1]
        with tempfile.TemporaryDirectory() as tmp:
            tmp_root = Path(tmp)
            bad = (
                "# Register\n\n"
                "| ID | Build unit | Primary scope | Depends on | Starting maturity | Exit gate |\n"
                "|---|---|---|---|---|---|\n"
                "| BUILD-042 | SNMP counters | counters | 041 | Raw Counter64 exists | exit |\n"
                "\n"
                "### BUILD-040..054 — Network, Flow, Syslog\n\n"
                "| BUILD | ① | ② | ③ delta | ④ D / I / C | ⑤ exit & acceptance | ⑥ rollout / rollback | ⑦ evidence / gate |\n"
                "|---|---|---|---|---|---|---|---|\n"
                "| 042 | SNMP counters | Core | delta | D | exit | n/a | Raw Counter64 exists |\n"
            )
            (tmp_root / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                bad, encoding="utf-8"
            )
            validator = MotherReferenceValidator(docs, repo_root=tmp_root)
            validator.errors = []
            validator.validate_build_042_counter_leaf_source_truth()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected BUILD-042 failures")
            self.assertIn("BUILD-042", joined)
            self.assertTrue(
                "Raw Counter64 exists" in joined
                or "PARTIAL — exact raw Counter64" in joined,
                joined,
            )

    def test_100_dependency_activation_any_all_combine(self) -> None:
        """DependencyActivationV1 MUST persist explicit ANY|ALL combination."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        p22 = (
            docs
            / "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md"
        ).read_text(encoding="utf-8")

        act = p17[
            p17.find("###### `DependencyActivationV1`") : p17.find(
                "###### `ThresholdExprV1` (discriminated union)"
            )
        ]
        for needle in (
            "combine: ANY | ALL",
            "The semantic-state criterion is ENABLED when `semantic_states` is non-empty",
            "The maintenance criterion is ENABLED when `maintenance_active = true`",
            "At least one criterion MUST be enabled",
            "`ANY` — activation is TRUE when at least one ENABLED criterion matches",
            "`ALL` — activation is TRUE only when every ENABLED criterion matches",
            "If only one criterion is enabled, `combine` MUST be `ANY`",
            "ALL` with only one enabled criterion is invalid",
            "No hidden/default combination behavior is permitted",
            "both criteria enabled and both match ⇒ TRUE for `ANY` and for `ALL`",
            "`combine=ALL`, both enabled, only one matches ⇒ FALSE",
            "`combine=ALL` configured with only one enabled criterion ⇒ validation failure",
            "stale/unqualified dependency evidence ⇒ `UNKNOWN` / no dependency action",
            "combination operator determines only whether dependency activation is TRUE",
        ):
            self.assertIn(needle, act, needle)

        body_dep = p22[
            p22.find("#### 6.1.9 Dependency threshold") : p22.find("#### 6.1.10")
        ]
        for needle in (
            "combine: ANY | ALL",
            "combine=ANY` ⇒ activation TRUE when ≥1 ENABLED criterion matches",
            "combine=ALL` ⇒ activation TRUE only when every ENABLED criterion matches",
            "if only one criterion is enabled, `combine` MUST be `ANY`",
            "no hidden/default combination behavior is permitted",
            "cannot activate `SUPPRESS`/`DEFER`",
            "##### SUPPRESS",
            "##### DEFER",
            "##### ANNOTATE",
        ):
            self.assertIn(needle, body_dep, needle)

        validator = MotherReferenceValidator(docs, repo_root=docs.parent)
        validator.errors = []
        validator.validate_dependency_activation_combine_operator()
        self.assertEqual(validator.errors, [])

    def test_101_build_051_flow_udp_profile_freeze(self) -> None:
        """BUILD-051 first UDP profile/gate/slices remain NOT_IMPLEMENTED."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        p10 = (
            docs / "Platform_10_Protocol_Security_Connector_Master.md"
        ).read_text(encoding="utf-8")
        p12 = (
            docs / "Platform_12_Decision_Source_Register.md"
        ).read_text(encoding="utf-8")
        p07 = (docs / "Platform_07_OSS_Catalog.md").read_text(encoding="utf-8")
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")

        summary = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-051 |")
        )
        section = register[register.find("### BUILD-040..054") :]
        nxt = section.find("\n### BUILD-", 1)
        if nxt != -1:
            section = section[:nxt]
        ledger = next(
            line for line in section.splitlines() if line.startswith("| 051 |")
        )

        for row in (summary, ledger):
            self.assertIn("FLOW_UDP_P1_PROFILE_V1", row)
            self.assertIn("FLOW_PARSER_DEPENDENCY_ADMISSION_GATE", row)
            self.assertIn("NOT_IMPLEMENTED", row)
            self.assertIn("SPECIFIED_NOT_IMPLEMENTED", row)
            cells = [c.strip() for c in row.strip().strip("|").split("|")]
            maturity = cells[4] if cells[0].startswith("BUILD-") else cells[-1]
            self.assertEqual(maturity, "NOT_IMPLEMENTED", maturity)
            self.assertNotIn("Partial", maturity)

        for needle in (
            "sensor/internal/protocol/flow/",
            "NetFlow v5",
            "NetFlow v9",
            "IPFIX v10",
            "sFlow v5",
            "S1 protocol-neutral",
            "S2 NetFlow v5",
            "S3 NetFlow v9",
            "S4 IPFIX v10 UDP",
            "S5 sFlow v5",
            "S6 receiver/admission shell",
            "S6 MUST NOT bypass BUILD-022/023",
            "template_refresh_timeout_seconds > 0",
            "NO_BUFFER_UNTEMPLATED_DATA",
            "MALFORMED_PACKET",
            "TEMPLATE_MISSING",
            "TEMPLATE_EXPIRED",
            "SEQUENCE_GAP",
            "EXPORTER_UNBOUND",
            "EXPORTER_AMBIGUOUS",
            "never Device Down",
            "do not delete previously durable authoritative Flow facts",
            "revert external parser dependency to prior admitted version",
        ):
            self.assertIn(needle, ledger, needle)

        self.assertIn("FLOW_PARSER_DEPENDENCY_ADMISSION_GATE", p12)
        self.assertIn("dependency admission only", p12)
        self.assertIn("FLOW_PARSER_DEPENDENCY_ADMISSION_GATE", p07)
        self.assertIn("FLOW_UDP_P1_PROFILE_V1", p10)
        self.assertIn("FLOW_PARSER_DEPENDENCY_ADMISSION_GATE", p10)
        self.assertIn("NO_BUFFER_UNTEMPLATED_DATA", p10)
        combined_lower = (ledger + p10).lower()
        self.assertIn("sflow flow vs counter", combined_lower)
        # IPFIX sequence distinction + restart/expiry ownership
        self.assertIn("record-based", combined_lower)
        self.assertIn("EMPTY", ledger)
        self.assertIn("FLOW_UDP_P1_PROFILE_V1", p17)
        self.assertIn("FLOW_PARSER_DEPENDENCY_ADMISSION_GATE", p17)
        self.assertIn("template_refresh_timeout_seconds > 0", p17)
        self.assertIn("NO_BUFFER_UNTEMPLATED_DATA", p17)

        # sFlow v5 datagram sequence contract (BUILD-051 S5)
        self.assertIn("sFlow v5 datagram sequence", p10)
        self.assertIn("Primary loss authority = datagram sequence", p10)
        self.assertIn("UINT32 wrap", p10)
        self.assertIn("DUPLICATE_DATAGRAM", p10)
        self.assertIn("OUT_OF_ORDER_OR_OLD_DATAGRAM", p10)
        self.assertIn("EXPORTER_RESTART_OR_DISCONTINUITY", p10)
        self.assertIn("SEQUENCE_DISCONTINUITY_UNKNOWN", p10)
        self.assertIn("SAMPLE_SEQUENCE_GAP", p10)
        self.assertIn("sample-sequence supplemental-only", ledger)
        self.assertIn("datagram-sequence primary loss authority", ledger)
        self.assertIn("collector-restart baseline", ledger)
        self.assertIn("sFlow v5 sequence quality envelope", p17)
        self.assertIn("sFlow v5 datagram sequence / discontinuity authority", p12)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_network_flow_source_reality_status()
        self.assertEqual(validator.errors, [])

    def test_101b_build_051_must_remain_not_implemented(self) -> None:
        """Promoting BUILD-051 to Partial after profile freeze must fail."""
        docs = Path(__file__).resolve().parents[1]
        with tempfile.TemporaryDirectory() as tmp:
            tmp_root = Path(tmp)
            bad = (
                "# Register\n\n"
                "| ID | Build unit | Primary scope | Depends on | Starting maturity | Exit gate |\n"
                "|---|---|---|---|---|---|\n"
                "| BUILD-043 | Interface | ifIndex | 041 | SPECIFIED_ONLY / NOT_IMPLEMENTED | exit |\n"
                "| BUILD-044 | Traps | traps | 041 | SPECIFIED_ONLY / NOT_IMPLEMENTED | exit |\n"
                "| BUILD-045 | gNMI | connector | DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE | "
                "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE PASS; NOT_IMPLEMENTED | exit |\n"
                "| BUILD-051 | Flow | FLOW_UDP_P1_PROFILE_V1 FLOW_PARSER_DEPENDENCY_ADMISSION_GATE "
                "SPECIFIED_NOT_IMPLEMENTED | 020 | Partial | exit |\n"
                "\n"
                "### BUILD-040..054 — Network, Flow, Syslog\n\n"
                "| BUILD | ① | ② | ③ delta | ④ D / I / C | ⑤ exit & acceptance | ⑥ rollout / rollback | ⑦ evidence / gate |\n"
                "|---|---|---|---|---|---|---|---|\n"
                "| 043 | Interface | Core | delta | D | exit | n/a | SPECIFIED_ONLY / NOT_IMPLEMENTED |\n"
                "| 044 | Traps | Sensor | delta | D | exit | n/a | SPECIFIED_ONLY / NOT_IMPLEMENTED |\n"
                "| 045 | gNMI; **NOT DISPATCHABLE until `DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE` PASS** | TARGET | Gate | D | exit | rb | "
                "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE PASS; NOT_IMPLEMENTED |\n"
                "| 051 | Flow FLOW_UDP_P1_PROFILE_V1 FLOW_PARSER_DEPENDENCY_ADMISSION_GATE "
                "SPECIFIED_NOT_IMPLEMENTED | Sensor | delta | D | exit | rb | Partial |\n"
            )
            (tmp_root / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                bad, encoding="utf-8"
            )
            validator = MotherReferenceValidator(docs, repo_root=tmp_root)
            validator.errors = []
            validator.validate_network_flow_source_reality_status()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected BUILD-051 maturity failure")
            self.assertIn("BUILD-051", joined)
            self.assertIn("Partial", joined)

    def test_101c_sflow_v5_datagram_sequence_contract(self) -> None:
        """sFlow datagram sequence / wrap / gap / discontinuity pins must hold."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p10 = (
            docs / "Platform_10_Protocol_Security_Connector_Master.md"
        ).read_text(encoding="utf-8")
        p12 = (
            docs / "Platform_12_Decision_Source_Register.md"
        ).read_text(encoding="utf-8")
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")

        section = register[register.find("### BUILD-040..054") :]
        nxt = section.find("\n### BUILD-", 1)
        if nxt != -1:
            section = section[:nxt]
        ledger = next(
            line for line in section.splitlines() if line.startswith("| 051 |")
        )
        maturity = [c.strip() for c in ledger.strip().strip("|").split("|")][-1]
        self.assertEqual(maturity, "NOT_IMPLEMENTED")

        for needle in (
            "Primary loss authority = datagram sequence",
            "sFlow datagram `sequence_number`",
            "Do **NOT** scope only by source IP",
            "Sample sequence numbers are supplemental",
            "UINT32 serial arithmetic",
            "distance = (C - P) mod 2^32",
            "DUPLICATE_DATAGRAM",
            "SEQUENCE_COLLISION",
            "SEQUENCE_GAP",
            "missing_datagrams = distance - 1",
            "OUT_OF_ORDER_OR_OLD_DATAGRAM",
            "UINT32 wrap",
            "0xffffffff → 0",
            "EXPORTER_RESTART_OR_DISCONTINUITY",
            "do not calculate datagram loss across the boundary",
            "current exporter uptime < previous exporter uptime",
            "NOT** legal when a trusted uptime decrease is present",
            "When `SEQUENCE_DISCONTINUITY_UNKNOWN` is legal",
            "trusted uptime decrease",
            "MUST NOT produce `SEQUENCE_DISCONTINUITY_UNKNOWN`",
            "SEQUENCE_DISCONTINUITY_UNKNOWN",
            "Duplicate and reorder do not become loss",
            "first valid datagram establishes a fresh collector-side baseline",
            "sample family (`FLOW` or `COUNTER`)",
            "SAMPLE_SEQUENCE_GAP",
            "MUST NOT change primary datagram loss counts",
            "sub_agent_id",
            "malformed packet does not mutate sequence high-water",
            "NOT_IMPLEMENTED",
        ):
            self.assertIn(needle, p10, needle)
        self.assertNotIn(
            "uptime decrease with otherwise plausible modular progression = "
            "discontinuity/`SEQUENCE_DISCONTINUITY_UNKNOWN`",
            p10,
        )

        for needle in (
            "datagram-sequence primary loss authority",
            "uint32 wrap",
            "duplicate/reorder/gap distinction",
            "restart/discontinuity epoch",
            "sample-sequence supplemental-only",
            "collector-restart baseline",
            "SEQUENCE_GAP",
            "DUPLICATE_DATAGRAM",
            "SAMPLE_SEQUENCE_GAP",
            "sub_agent_id",
            "NOT_IMPLEMENTED",
        ):
            self.assertIn(needle, ledger, needle)

        for needle in (
            "sFlow v5 sequence quality envelope",
            "DUPLICATE_DATAGRAM",
            "OUT_OF_ORDER_OR_OLD_DATAGRAM",
            "SEQUENCE_DISCONTINUITY_UNKNOWN",
            "SAMPLE_SEQUENCE_GAP",
            "datagram header `sequence_number`",
            "MUST NOT mutate primary datagram-loss counts",
        ):
            self.assertIn(needle, p17, needle)

        for needle in (
            "sFlow v5 datagram sequence / discontinuity authority",
            "EXPORTER_RESTART_OR_DISCONTINUITY",
            "current exporter uptime < previous exporter uptime",
            "ALWAYS classifies `EXPORTER_RESTART_OR_DISCONTINUITY`",
            "NOT** legal when a trusted uptime decrease is present",
            "prefer possible false discontinuity over fabricated giant loss",
            "NOT_IMPLEMENTED",
        ):
            self.assertIn(needle, p12, needle)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_sflow_v5_datagram_sequence_contract()
        self.assertEqual(validator.errors, [])

    def test_101e_sflow_uptime_decrease_must_not_be_unknown(self) -> None:
        """Trusted uptime decrease must not classify as SEQUENCE_DISCONTINUITY_UNKNOWN."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        with tempfile.TemporaryDirectory() as tmp:
            tmp_root = Path(tmp)
            docs_tmp = tmp_root / "project_documentation"
            docs_tmp.mkdir()
            p10_src = (
                docs / "Platform_10_Protocol_Security_Connector_Master.md"
            ).read_text(encoding="utf-8")
            # Regress fixture 8 to the contradictory UNKNOWN classification.
            p10_bad = p10_src.replace(
                "trusted uptime decrease (`current < previous`) with otherwise "
                "plausible modular progression = ALWAYS "
                "`EXPORTER_RESTART_OR_DISCONTINUITY` (epoch boundary; not "
                "confirmed reboot; MUST NOT produce "
                "`SEQUENCE_DISCONTINUITY_UNKNOWN`).",
                "uptime decrease with otherwise plausible modular progression = "
                "discontinuity/`SEQUENCE_DISCONTINUITY_UNKNOWN`, not confirmed reboot.",
            )
            self.assertIn(
                "uptime decrease with otherwise plausible modular progression = "
                "discontinuity/`SEQUENCE_DISCONTINUITY_UNKNOWN`",
                p10_bad,
            )
            (docs_tmp / "Platform_10_Protocol_Security_Connector_Master.md").write_text(
                p10_bad, encoding="utf-8"
            )
            (tmp_root / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                (root / "Platform_Master_Build_Sequence_Register_v1.0.md").read_text(
                    encoding="utf-8"
                ),
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(docs_tmp, repo_root=tmp_root)
            validator.errors = []
            validator.validate_sflow_v5_datagram_sequence_contract()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected fixture-8 UNKNOWN regression failure")
            self.assertIn("SEQUENCE_DISCONTINUITY_UNKNOWN", joined)

    def test_101d_sflow_sequence_regression_detects_loss(self) -> None:
        """Dropping datagram-sequence primary authority must fail validation."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        with tempfile.TemporaryDirectory() as tmp:
            tmp_root = Path(tmp)
            # Minimal register with BUILD-051 profile freeze but no S5 pins.
            bad_register = (
                "# Register\n\n"
                "| ID | Build unit | Primary scope | Depends on | Starting maturity | Exit gate |\n"
                "|---|---|---|---|---|---|\n"
                "| BUILD-051 | Flow | FLOW_UDP_P1_PROFILE_V1 FLOW_PARSER_DEPENDENCY_ADMISSION_GATE "
                "SPECIFIED_NOT_IMPLEMENTED | 020 | NOT_IMPLEMENTED | exit |\n"
                "\n"
                "### BUILD-040..054 — Network, Flow, Syslog\n\n"
                "| BUILD | ① | ② | ③ delta | ④ D / I / C | ⑤ exit & acceptance | ⑥ rollout / rollback | ⑦ evidence / gate |\n"
                "|---|---|---|---|---|---|---|---|\n"
                "| 051 | Flow FLOW_UDP_P1_PROFILE_V1 FLOW_PARSER_DEPENDENCY_ADMISSION_GATE "
                "SPECIFIED_NOT_IMPLEMENTED S5 sFlow v5 | Sensor | delta | D | exit | rb | NOT_IMPLEMENTED |\n"
            )
            (tmp_root / "Platform_Master_Build_Sequence_Register_v1.0.md").write_text(
                bad_register, encoding="utf-8"
            )
            # Point docs_root at a copy of Platform_10 with sequence contract removed.
            docs_tmp = tmp_root / "project_documentation"
            docs_tmp.mkdir()
            p10_src = (
                docs / "Platform_10_Protocol_Security_Connector_Master.md"
            ).read_text(encoding="utf-8")
            # Keep FLOW profile markers so the validator does not skip, but strip
            # the dedicated sFlow datagram sequence owner heading.
            p10_bad = p10_src.replace(
                "sFlow v5 datagram sequence / gap / duplicate / reorder / restart semantics",
                "sFlow v5 evidence notes",
            )
            self.assertNotIn("sFlow v5 datagram sequence", p10_bad)
            self.assertIn("FLOW_UDP_P1_PROFILE_V1", p10_bad)
            (docs_tmp / "Platform_10_Protocol_Security_Connector_Master.md").write_text(
                p10_bad, encoding="utf-8"
            )
            validator = MotherReferenceValidator(docs_tmp, repo_root=tmp_root)
            validator.errors = []
            validator.validate_sflow_v5_datagram_sequence_contract()
            joined = "\n".join(validator.errors)
            self.assertTrue(validator.errors, "expected sFlow sequence contract failure")
            self.assertIn("sFlow v5 datagram sequence", joined)

    def test_102_host_global_runtime_exclusion_contract(self) -> None:
        """Shared-host Docker runtime exclusion must stay frozen across owners."""
        docs = Path(__file__).resolve().parents[1]
        root = docs.parent
        p13 = (
            docs / "Platform_13_Engineering_Development_and_Docker_Standards.md"
        ).read_text(encoding="utf-8")
        p14 = (
            docs / "Platform_14_Test_Performance_and_Release_Quality.md"
        ).read_text(encoding="utf-8")
        p15 = (
            docs / "Platform_15_CI_CD_Packaging_and_Distribution.md"
        ).read_text(encoding="utf-8")

        lock_path = (
            "/home/gagan/amrudai-evidence/runtime/platform-dev-runner.lock"
        )
        for needle in (
            "## 22. Host-global shared Docker runtime exclusion",
            lock_path,
            "flock",
            "fcntl.flock",
            "Kernel lock ownership is authoritative",
            "Never unlink",
            "runtime-busy",
            "CODEX Desktop heavy E2E",
            "GitHub Actions jobs on `[self-hosted, Linux, X64, platform-ci]`",
            "agent-e2e-*",
            "SELF_HOSTED_RUNTIME_LOCK_IMPLEMENTATION = REQUIRED / PENDING_IMPLEMENTATION",
            ".github/workflows/platform-ci.yml",
            "PENDING_IMPLEMENTATION",
            "SPECIFIED / NOT YET EXECUTED",
            "Do **not** describe the lock as currently active in GitHub Actions",
        ):
            self.assertIn(needle, p13, needle)

        for needle in (
            "### 3.8 Self-hosted host-global runtime exclusion",
            "Integration, PostgreSQL, contract, security",
            lock_path,
            "before any Platform Docker mutation",
            "owned cleanup",
            "always()",
            "runtime-busy",
            "agent-e2e-*",
            "SELF_HOSTED_RUNTIME_LOCK_IMPLEMENTATION = REQUIRED / PENDING_IMPLEMENTATION",
            ".github/workflows/platform-ci.yml",
            "PENDING_IMPLEMENTATION",
            "SPECIFIED / NOT YET EXECUTED",
            "Do **not** describe the lock as currently active in GitHub Actions",
            "| `.github/workflows/platform-ci.yml` |",
        ):
            self.assertIn(needle, p15, needle)

        for needle in (
            "### Host-global runtime exclusion acceptance",
            "Persistence, Structured JSON Logs, and Workstation Isolation",
            "not** an allowlist for `agent-e2e-*`",
            "Local E2E owns the lock",
            "CI owns the lock",
            "Foreign containers remain unchanged throughout Test 20",
            "CI cancellation / process death releases the lock",
            "CI cleanup never deletes foreign `agent-e2e-*`",
            "Same-inode independent nonblocking acquisition",
            "Two GitHub Integration jobs cannot become simultaneous",
            "SELF_HOSTED_RUNTIME_LOCK_IMPLEMENTATION = REQUIRED / PENDING_IMPLEMENTATION",
            "SPECIFIED / NOT YET EXECUTED",
            "PENDING_IMPLEMENTATION",
        ):
            self.assertIn(needle, p14, needle)

        # Preserve prior P0 closures — these must not regress with this pin.
        register = (
            root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        ).read_text(encoding="utf-8")
        self.assertIn("sFlow v5", register)
        row_042 = next(
            line
            for line in register.splitlines()
            if line.startswith("| BUILD-042 |")
        )
        self.assertIn("PARTIAL", row_042)
        self.assertNotIn("Raw Counter64 exists", row_042)
        p17 = (
            docs / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        ).read_text(encoding="utf-8")
        self.assertIn("### 7.1 Agent-ingress pending-reservation", p17)
        self.assertIn("combine: ANY | ALL", p17)

        validator = MotherReferenceValidator(docs, repo_root=root)
        validator.errors = []
        validator.validate_host_global_runtime_exclusion_contract()
        self.assertEqual(validator.errors, [])

    def test_102b_host_global_runtime_exclusion_regression_fails(self) -> None:
        """Dropping the host-global lock owner section must fail closed."""
        docs = Path(__file__).resolve().parents[1]
        with tempfile.TemporaryDirectory() as tmp:
            tmp_docs = Path(tmp) / "project_documentation"
            tmp_docs.mkdir(parents=True)
            (tmp_docs / "Platform_13_Engineering_Development_and_Docker_Standards.md").write_text(
                "# Platform 13\n\n## 14. Compose profiles\n\n"
                "Docker Compose development without host exclusion.\n",
                encoding="utf-8",
            )
            (tmp_docs / "Platform_15_CI_CD_Packaging_and_Distribution.md").write_text(
                "# Platform 15\n\n### 3.1 Pipeline stages\n\nCI/CD stages.\n",
                encoding="utf-8",
            )
            (tmp_docs / "Platform_14_Test_Performance_and_Release_Quality.md").write_text(
                "# Platform 14\n\n### Current CI evidence boundary\n\n"
                "Four jobs exist.\n",
                encoding="utf-8",
            )
            validator = MotherReferenceValidator(tmp_docs, repo_root=Path(tmp))
            validator.errors = []
            validator.validate_host_global_runtime_exclusion_contract()
            joined = "\n".join(validator.errors)
            self.assertTrue(
                validator.errors,
                "expected host-global runtime exclusion failures",
            )
            self.assertIn("host-global", joined.lower())

    def test_103_nested_non_normative_strip_skips_child_overlap(self) -> None:
        """Parent non-normative strip must not resurrect nested sibling prose."""
        from validate_mother_references import strip_non_normative_heading_sections

        text = (
            "## Keep heading\n\nKeep.\n\n"
            "## Non-normative ownership index\n\n"
            "Parent intro `REQ-X-001`.\n\n"
            "### Summary-only nested catalogue\n\n"
            "Nested summary `REQ-X-001`.\n\n"
            "### Ordinary nested subsection\n\n"
            "Resurrected REQUIREMENT prose Platform SHALL enforce (`REQ-X-001`).\n\n"
            "## 2. Real section\n\n"
            "Real Platform SHALL enforce (`REQ-X-002`).\n"
        )
        cleaned = strip_non_normative_heading_sections(text)
        self.assertNotIn("Resurrected REQUIREMENT", cleaned)
        self.assertNotIn("Ordinary nested subsection", cleaned)
        self.assertNotIn("Nested summary", cleaned)
        self.assertIn("## Keep heading", cleaned)
        self.assertIn("## 2. Real section", cleaned)
        self.assertIn("REQ-X-002", cleaned)

    def test_103b_adjacent_non_normative_sections_both_stripped(self) -> None:
        """Adjacent non-normative siblings must both remain excluded."""
        from validate_mother_references import strip_non_normative_heading_sections

        text = (
            "## 1. Keep\n\nKeep body.\n\n"
            "## Non-normative ownership index\n\n"
            "First excluded `REQ-A-001`.\n\n"
            "## Summary-only catalogue\n\n"
            "Second excluded `REQ-A-002`.\n\n"
            "## 2. Real\n\n"
            "Platform SHALL enforce (`REQ-A-003`).\n"
        )
        cleaned = strip_non_normative_heading_sections(text)
        self.assertNotIn("First excluded", cleaned)
        self.assertNotIn("Second excluded", cleaned)
        self.assertIn("## 1. Keep", cleaned)
        self.assertIn("## 2. Real", cleaned)
        self.assertIn("REQ-A-003", cleaned)

    def test_103c_nested_synthetic_family_strip_skips_child_overlap(self) -> None:
        """Synthetic-family strip must not resurrect nested sibling prose."""
        from validate_mother_references import strip_synthetic_family_normative_sections

        text = (
            "## 6. Alerts\n\nKeep.\n\n"
            "### 6.0 Normative requirements — REQ-ALT family\n\n"
            "Parent synth.\n\n"
            "#### 6.0.0 Normative requirements — nested synth\n\n"
            "Nested synth body.\n\n"
            "#### 6.0.1 Ordinary nested under parent\n\n"
            "SHOULD NOT RESURRECT Platform SHALL (`REQ-ALT-099`).\n\n"
            "### 6.1 Real\n\n"
            "Real Platform SHALL (`REQ-ALT-001`).\n"
        )
        cleaned = strip_synthetic_family_normative_sections(text)
        self.assertNotIn("SHOULD NOT RESURRECT", cleaned)
        self.assertNotIn("Ordinary nested under parent", cleaned)
        self.assertNotIn("Nested synth body", cleaned)
        self.assertIn("### 6.1 Real", cleaned)
        self.assertIn("REQ-ALT-001", cleaned)

    def test_103d_adjacent_synthetic_family_sections_both_stripped(self) -> None:
        """Adjacent synthetic-family sections must both remain excluded."""
        from validate_mother_references import strip_synthetic_family_normative_sections

        text = (
            "## 6. Alerts\n\nKeep.\n\n"
            "### 6.0 Normative requirements — REQ-ALT family\n\n"
            "First synth `REQ-ALT-010`.\n\n"
            "### 7.0 Normative requirements — REQ-OSS family\n\n"
            "Second synth `REQ-OSS-010`.\n\n"
            "### 6.1 Real\n\n"
            "Real Platform SHALL (`REQ-ALT-001`).\n"
        )
        cleaned = strip_synthetic_family_normative_sections(text)
        self.assertNotIn("First synth", cleaned)
        self.assertNotIn("Second synth", cleaned)
        self.assertIn("### 6.1 Real", cleaned)
        self.assertIn("REQ-ALT-001", cleaned)

    def test_104_issue308_outbound_source_event_identity_contract(self) -> None:
        """Outbound operation_hash must bind immutable source-event identity."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (docs / "Platform_17_API_Event_and_Data_Model_Contracts.md").read_text(
            encoding="utf-8"
        )
        p06 = (docs / "Platform_06_Dataflow.md").read_text(encoding="utf-8")
        self.assertIn("source_event_id", p17)
        self.assertIn("created_in_txn_event_id", p17)
        self.assertRegex(p17, r"operation_hash[\s\S]{0,500}source_event_id")
        self.assertNotRegex(
            p17,
            r"Immutable hash of \(tenant_id, connector_id, object_type, "
            r"object_id, mapping_version, canonical payload\)",
        )
        self.assertIn("A→B→A", p17)
        self.assertIn("event_seq", p06)
        self.assertIn("A→B→A", p06)

    def test_105_issue308_policy_lifecycle_seq_contract(self) -> None:
        """Policy lifecycle events order by lifecycle_seq, not version_no alone."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (docs / "Platform_17_API_Event_and_Data_Model_Contracts.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("lifecycle_seq", p17)
        self.assertIn("`(policy_id, lifecycle_seq)`", p17)
        self.assertNotIn(
            "| PolicyChangePayload | tenant_id, policy_id UUID, version_no INT32, "
            "lifecycle_state",
            p17,
        )

    def test_106_issue308_discovery_backfill_parity_contract(self) -> None:
        """Backfill parity uses distinct derived identities + source-row coverage."""
        docs = Path(__file__).resolve().parents[1]
        p17 = (docs / "Platform_17_API_Event_and_Data_Model_Contracts.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("distinct derived", p17.lower())
        self.assertIn("source-row", p17.lower())
        self.assertNotIn(
            "count(`discovery_candidate`) eligible rows = count(`onboarding_candidate`)",
            p17,
        )

    def test_107_issue308_storage_byte_unit_factors(self) -> None:
        """Storage display units use SI 10^9/10^12 and IEC 2^30/2^40."""
        docs = Path(__file__).resolve().parents[1]
        p11 = (docs / "Platform_11_Sizing_Capacity_Storage_Master.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("10⁹", p11)
        self.assertIn("10¹²", p11)
        self.assertIn("2³⁰", p11)
        self.assertIn("2⁴⁰", p11)
        self.assertNotIn("GB unit (10³)", p11)
        self.assertNotIn("GB/TB** (10³/10⁶)", p11)
        self.assertNotIn("GiB/TiB** (2¹⁰/2²⁰)", p11)
        self.assertIn("1_000_000_000", p11)
        self.assertIn("1_073_741_824", p11)


if __name__ == "__main__":
    unittest.main()
