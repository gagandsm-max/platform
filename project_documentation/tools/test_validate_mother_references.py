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

from validate_mother_references import SCHEMA_VERSION, validate_docs_root  # noqa: E402

MINIMAL_DOCS = [
    ("AGENTS.md", None, "TRANSITIONAL_LEGACY", "navigation_engineering_governance"),
    ("Platform_00_Document_Index.md", "PLATFORM-INDEX-001", "NORMALIZED_V1", "mother_reference"),
    ("Platform_01_BRD.md", "PLATFORM-BRD-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_02_PRD.md", "PLATFORM-PRD-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_03_Architecture.md", "PLATFORM-ARC-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_04_HLD.md", "PLATFORM-HLD-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_05_LLD.md", "PLATFORM-LLD-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_06_Dataflow.md", "PLATFORM-DF-001", "TRANSITIONAL_LEGACY", "mother_reference"),
    ("Platform_07_OSS_Catalog.md", "PLATFORM-OSS-001", "TRANSITIONAL_LEGACY", "mother_reference"),
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


def _req_registry(rows: list[tuple[str, str]]) -> str:
    lines = [
        "<!-- GLOBAL PRIMARY REQ REGISTRY -->",
        "### GLOBAL PRIMARY REQ REGISTRY",
        "",
        "| Requirement ID | Summary | Primary owner | Source locator | Traceability state | Maturity | Evidence / blocker pointer |",
        "|---|---|---|---|---|---|---|",
    ]
    for rid, summary in rows:
        lines.append(
            f"| {rid} | {summary} | `Platform_08_Requirements_Traceability.md` | §2 | BASE_RTM_INDEXED | SPECIFIED | n/a |"
        )
    lines.append("<!-- END GLOBAL PRIMARY REQ REGISTRY -->")
    return "\n".join(lines)


def _prd_registry(rows: list[tuple[str, str]]) -> str:
    lines = [
        "<!-- GLOBAL PRIMARY PRD REGISTRY -->",
        "### GLOBAL PRIMARY PRD REGISTRY",
        "",
        "| PRD ID | Product requirement summary | Owner document | Source locator | Related REQ family | Maturity / implementation boundary |",
        "|---|---|---|---|---|---|",
    ]
    for rid, summary in rows:
        lines.append(
            f"| {rid} | {summary} | `Platform_02_PRD.md` | §1 | — | SPECIFIED |"
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
        req_rows: list[tuple[str, str]] | None = None,
        prd_rows: list[tuple[str, str]] | None = None,
        legacy: list[dict] | None = None,
        extra_files: dict[str, str] | None = None,
        omit_files: set[str] | None = None,
        version: str = "3.2.2",
        p00_sections: str | None = None,
        p08_extra: str = "",
        p02_body: str | None = None,
        normalize_overrides: dict[str, str] | None = None,
        doc_id_overrides: dict[str, str] | None = None,
        include_platform_24: bool = False,
        duplicate_platform_file: str | None = None,
        broken_ref: str | None = None,
        skip_manifest_legacy: bool = False,
    ) -> Path:
        omit_files = omit_files or set()
        req_rows = req_rows or [("REQ-P-001", "Quality")]
        prd_rows = prd_rows or [("PRD-HA-001", "HA pair")]
        legacy = legacy if legacy is not None else []

        docs_meta = []
        for i, (fname, doc_id, state, kind) in enumerate(MINIMAL_DOCS, start=1):
            if fname in omit_files:
                continue
            state = (normalize_overrides or {}).get(fname, state)
            docs_meta.append(
                {
                    "file": fname,
                    "ordinal": i,
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
        }
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
            "## 6. Precedence\n\n"
        )

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
                    + p08_extra
                    + "\n"
                )
                if broken_ref:
                    body += f"\nSee `{broken_ref}` for details.\n"
            elif fname == "Platform_02_PRD.md":
                body = _header(doc_id, version) + (
                    p02_body
                    or ("\n".join(f"| `{rid}` | {summary} |" for rid, summary in prd_rows) + "\n")
                )
            else:
                # Ensure REQ/PRD discovery has at least the fixture IDs somewhere
                # for non-index docs only when needed; put REQ-P-001 only in P08/P01
                body = _header(
                    (doc_id_overrides or {}).get(fname, doc_id), version
                ) + "## 1. Section\n\nContent.\n"
                if fname == "Platform_01_BRD.md":
                    body += "\n".join(f"Mentions {rid}." for rid, _ in req_rows if rid.startswith("REQ-"))
                    body += "\n"
            # Apply doc id override already handled
            if doc_id_overrides and fname in doc_id_overrides and fname not in {
                "Platform_00_Document_Index.md",
                "Platform_08_Requirements_Traceability.md",
                "Platform_02_PRD.md",
                "AGENTS.md",
            }:
                body = _header(doc_id_overrides[fname], version) + "## 1. Section\n\nContent.\n"
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
            legacy=[{"id": "OSS-008", "kind": "formal_requirement_legacy", "reason": "test"}],
            req_rows=[("REQ-P-001", "Quality"), ("OSS-008", "Air-gap")],
        )
        # Put OSS-008 only in registry; also need Platform_08 table form for discovery
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_02_missing_mother_reference_fails(self) -> None:
        root = self._write_minimal(omit_files={"Platform_03_Architecture.md"})
        # still declare in manifest — recreate manifest with full list but delete file
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").unlink()
        errors = validate_docs_root(root)
        self.assertTrue(any("missing authoritative file: Platform_03_Architecture.md" in e for e in errors), errors)

    def test_03_unexpected_numbered_document_fails(self) -> None:
        root = self._write_minimal(
            extra_files={
                "Platform_99_Unexpected.md": _header("PLATFORM-X-001") + "## 1. X\n"
            }
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("unexpected numbered document" in e for e in errors), errors)

    def test_04_platform_24_fails(self) -> None:
        root = self._write_minimal(include_platform_24=True)
        errors = validate_docs_root(root)
        self.assertTrue(any("Platform_24+" in e for e in errors), errors)

    def test_05_duplicate_platform_number_fails(self) -> None:
        root = self._write_minimal(duplicate_platform_file="Platform_03_Duplicate.md")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("unexpected numbered document" in e or "duplicate Platform number" in e for e in errors),
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
        # Manifest still says 3.2.2 by rewriting
        man = json.loads((root / "mother_reference_manifest.json").read_text())
        man["motherReferenceVersion"] = "3.2.2"
        (root / "mother_reference_manifest.json").write_text(json.dumps(man), encoding="utf-8")
        errors = validate_docs_root(root)
        self.assertTrue(any("version mismatch" in e for e in errors), errors)

    def test_08_req_absent_from_global_registry_fails(self) -> None:
        root = self._write_minimal(req_rows=[("REQ-P-001", "Quality")])
        p01 = (root / "Platform_01_BRD.md").read_text(encoding="utf-8")
        (root / "Platform_01_BRD.md").write_text(p01 + "\nREQ-NET-001 must be indexed.\n", encoding="utf-8")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("REQ in source but absent from global primary registry: REQ-NET-001" in e for e in errors),
            errors,
        )

    def test_09_duplicate_primary_req_row_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[("REQ-P-001", "Quality"), ("REQ-P-001", "Dup")]
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("duplicate primary REQ row: REQ-P-001" in e for e in errors), errors)

    def test_10_prd_absent_from_global_registry_fails(self) -> None:
        root = self._write_minimal(prd_rows=[("PRD-HA-001", "HA")])
        p02 = (root / "Platform_02_PRD.md").read_text(encoding="utf-8")
        (root / "Platform_02_PRD.md").write_text(p02 + "\nPRD-SNMP-001 appears here.\n", encoding="utf-8")
        errors = validate_docs_root(root)
        self.assertTrue(
            any("PRD in source but absent from global primary PRD registry: PRD-SNMP-001" in e for e in errors),
            errors,
        )

    def test_11_duplicate_primary_prd_row_fails(self) -> None:
        root = self._write_minimal(
            prd_rows=[("PRD-HA-001", "HA"), ("PRD-HA-001", "Dup")]
        )
        errors = validate_docs_root(root)
        self.assertTrue(any("duplicate primary PRD row: PRD-HA-001" in e for e in errors), errors)

    def test_12_unknown_owner_source_file_fails(self) -> None:
        # Primary PRD with no source occurrence anywhere
        root = self._write_minimal(prd_rows=[("PRD-HA-001", "HA")])
        (root / "Platform_02_PRD.md").write_text(
            _header("PLATFORM-PRD-001") + "## 1. Empty of this PRD\n", encoding="utf-8"
        )
        # Keep PRD only in primary registry
        errors = validate_docs_root(root)
        self.assertTrue(
            any("no authoritative source" in e or "unknown owner" in e for e in errors),
            errors,
        )

    def test_13_broken_platform_md_reference_fails(self) -> None:
        root = self._write_minimal(broken_ref="Platform_99_Does_Not_Exist.md")
        errors = validate_docs_root(root)
        self.assertTrue(any("broken Platform_*.md reference" in e for e in errors), errors)

    def test_14_invalid_formal_id_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[("REQ-P-001", "Quality"), ("OSS-999", "Bad legacy")],
            skip_manifest_legacy=True,
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("invalid formal ID" in e or "unregistered malformed" in e for e in errors),
            errors,
        )

    def test_15_registered_oss008_legacy_passes(self) -> None:
        root = self._write_minimal(
            req_rows=[("REQ-P-001", "Quality"), ("OSS-008", "Air-gap")],
            legacy=[{"id": "OSS-008", "kind": "formal_requirement_legacy", "reason": "Issue #93"}],
        )
        errors = validate_docs_root(root)
        self.assertEqual(errors, [], errors)

    def test_16_unregistered_malformed_legacy_fails(self) -> None:
        root = self._write_minimal(
            req_rows=[("REQ-P-001", "Quality"), ("OSS-008", "Air-gap")],
            skip_manifest_legacy=True,
        )
        errors = validate_docs_root(root)
        self.assertTrue(
            any("OSS-008" in e and ("invalid" in e or "unregistered" in e) for e in errors),
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
            any("duplicate normalized top-level section number: 1" in e for e in errors),
            errors,
        )

    def test_18_transitional_legacy_duplicate_sections_not_blocked(self) -> None:
        # Platform_03 is TRANSITIONAL_LEGACY; duplicate ## 1 should not fail normalized checks
        root = self._write_minimal()
        (root / "Platform_03_Architecture.md").write_text(
            _header("PLATFORM-ARC-001")
            + "## 1. One\n\n## 1. Two\n\n## 18.1 Nested historical\n\n",
            encoding="utf-8",
        )
        errors = validate_docs_root(root)
        self.assertFalse(
            any("Platform_03_Architecture.md: duplicate normalized" in e for e in errors),
            errors,
        )


if __name__ == "__main__":
    unittest.main()
