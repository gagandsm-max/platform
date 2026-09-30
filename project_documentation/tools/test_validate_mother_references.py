#!/usr/bin/env python3
"""Unit tests for Mother Reference structural validator (stdlib only)."""

from __future__ import annotations

import json
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


def _header(doc_id: str | None, version: str = "3.2.2") -> str:
    if doc_id is None:
        return f"# AGENTS\n**Version:** {version}\n\n"
    return (
        f"# Doc\n"
        f"**Document ID:** {doc_id}\n"
        f"**Version:** {version}\n\n"
    )


def _req_registry(rows: list[tuple[str, str, str]]) -> str:
    """rows: (id, summary, owner)."""
    lines = [
        "<!-- GLOBAL PRIMARY REQ REGISTRY -->",
        "### GLOBAL PRIMARY REQ REGISTRY",
        "",
        "| Requirement ID | Summary | Primary owner | Source locator | Traceability state | Maturity | Evidence / blocker pointer |",
        "|---|---|---|---|---|---|---|",
    ]
    for rid, summary, owner in rows:
        lines.append(
            f"| {rid} | {summary} | `{owner}` | §2 | BASE_RTM_INDEXED | SPECIFIED | n/a |"
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
            f"| {rid} | {summary} | `{owner}` | §1 | — | SPECIFIED |"
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
        version: str = "3.2.2",
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
                body = _header(doc_id, version) + (
                    p02_body
                    or (
                        "\n".join(
                            f"| `{rid}` | {summary} |" for rid, summary, _owner in prd_rows
                        )
                        + "\n"
                    )
                )
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
        man["motherReferenceVersion"] = "3.2.2"
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

    def test_18_transitional_legacy_duplicate_sections_not_blocked(self) -> None:
        # Platform_09+ remain TRANSITIONAL_LEGACY; duplicate ## numbers allowed.
        root = self._write_minimal()
        (root / "Platform_09_UI_UX_Master.md").write_text(
            _header("PLATFORM-UIUX-001")
            + "## 1. One\n\n## 1. Two\n\n## 18.1 Nested historical\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any(
                "Platform_09_UI_UX_Master.md: duplicate normalized" in e for e in errors
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
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §2 |",
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
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §2 | BASE_RTM_INDEXED | SPECIFIED | n/a |",
            "| REQ-P-001 | Quality | `Platform_08_Requirements_Traceability.md` | §2 |",
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


if __name__ == "__main__":
    unittest.main()
