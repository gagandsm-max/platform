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
CANONICAL_MOTHER_REFERENCE_VERSION = "3.3.0"
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

# Non-normative ownership placeholders rejected as sole owner evidence.
OWNERSHIP_INDEX_START = "<!-- RTM FORMAL-ID OWNERSHIP INDEX -->"
OWNERSHIP_INDEX_END = "<!-- END RTM FORMAL-ID OWNERSHIP INDEX -->"
OWNERSHIP_INDEX_HEADING_RE = re.compile(
    r"^#{1,6}\s+RTM formal-ID ownership index\b.*$",
    re.I | re.M,
)
# Front-matter / family catalogue markers that only duplicate RTM one-liners.
RTM_SECTION_FORMAL_START_RE = re.compile(
    r"<!--\s*RTM SECTION FORMAL REQUIREMENTS:\s*([^>]+?)\s*-->",
    re.I,
)
RTM_SECTION_FORMAL_END_RE = re.compile(
    r"<!--\s*END RTM SECTION FORMAL REQUIREMENTS:\s*([^>]+?)\s*-->",
    re.I,
)
NON_NORMATIVE_SECTION_HEADING_RE = re.compile(
    r"^#{1,6}\s+.*\b("
    r"non-normative|ownership index|summary[- ]only|catalogue only|"
    r"formal-id ownership|rtm section formal requirements|"
    r"family catalogue|index[- ]only|summary catalogue"
    r")\b.*$",
    re.I | re.M,
)
# Synthetic family ownership catalogues (rejected as primary semantic owners).
SYNTHETIC_FAMILY_SECTION_HEADING_RE = re.compile(
    r"^(#{2,6})\s+(\d+(?:\.\d+)*)\.0\s+Normative requirements\b.*$",
    re.I | re.M,
)
SYNTHETIC_FAMILY_LOCATOR_RE = re.compile(
    r"(?:§|Section\s+)\d+(?:\.\d+)*\.0\s+Normative requirements\b"
    r"|Normative requirements\s*[—\-]\s*`?REQ-[A-Z]+`?\s+family",
    re.I,
)
# Trailing/footer blocks that only list identifiers without requirement prose.
BARE_ID_BULLET_RE = re.compile(
    r"^\s*[-*]\s+`?((?:REQ|PRD)-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)`?\s*$",
    re.M,
)
BARE_ID_TABLE_ROW_RE = re.compile(
    r"^\|\s*`?((?:REQ|PRD)-[A-Z]+(?:-[A-Z]+)*-\d{3,}(?:\.\d+)?)`?\s*\|\s*$",
    re.M,
)
# Two-column summary catalogues that merely restatement RTM one-liners.
SUMMARY_REQ_TABLE_HEADER_RE = re.compile(
    r"^\|\s*Requirement ID\s*\|\s*Normative requirement\s*\|\s*$",
    re.I | re.M,
)
# Four-column synthetic family boilerplate tables.
FOUR_COLUMN_FAMILY_TABLE_HEADER_RE = re.compile(
    r"^\|\s*Requirement ID\s*\|\s*Normative SHALL contract\s*\|\s*"
    r"Parent-section envelope\s*\|\s*Boundary\s*/\s*non-goal\s*\|\s*$",
    re.I | re.M,
)
# Extract numbered section references from Platform_08 locators.
# Require an explicit section marker (§ / Section / leading N. title form).
# Do not treat issue numbers, versions (v3.3) or ID suffixes (-001) as sections.
LOCATOR_SECTION_RE = re.compile(
    r"(?:§|Section\s+|sec(?:tion)?\s+)(\d+(?:\.\d+){0,4})\b"
    r"|^\s*(\d+(?:\.\d+){0,4})\.\s+\S"
    r"|(?<![A-Za-z0-9._#])(\d+(?:\.\d+){1,4})\s+[A-Za-z]",
    re.I | re.M,
)
MARKDOWN_NUMBERED_HEADING_RE = re.compile(
    # Accept "## 3. Title" (bare int requires trailing ". ") and
    # "### 3.1 Title" / "#### 3.1.1 Title". Reject date headings like
    # "## 2026-10-30" and letter-suffixed "### 3.1A".
    r"^(#{2,6})\s+("
    r"\d+(?:\.\d+)+"  # dotted subsection ids
    r"|"
    r"\d+(?=\.\s)"  # bare top-level id only when followed by ". "
    r")(?:\.\s|\s)\S[^\n]*$",
    re.M,
)

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


def strip_all_marker_blocks(text: str, start: str, end: str) -> str:
    while start in text and end in text:
        text = strip_marker_block(text, start, end)
    return text


def strip_rtm_section_formal_requirement_blocks(text: str) -> str:
    """Remove <!-- RTM SECTION FORMAL REQUIREMENTS --> catalogue blocks.

    These markers historically hosted two-column summary tables that duplicated
    RTM one-liners. They are non-normative ownership evidence when used alone.
    """
    while True:
        start = RTM_SECTION_FORMAL_START_RE.search(text)
        if not start:
            break
        end = RTM_SECTION_FORMAL_END_RE.search(text, start.end())
        if not end:
            # Unclosed marker: drop from start marker to EOF so it cannot prove
            # ownership.
            text = text[: start.start()]
            break
        text = text[: start.start()] + text[end.end() :]
    return text


def _heading_level_at(text: str, pos: int) -> int | None:
    """Return markdown heading level for a match starting at pos, else None."""
    m = re.match(r"#{1,6}", text[pos:])
    return len(m.group(0)) if m else None


def _end_of_section_at_level(text: str, body_start: int, level: int) -> int:
    """Return absolute end index of a section body starting at body_start.

    Consumes nested child headings and stops at the next heading of equal or
    higher level (fewer '#'), or EOF.
    """
    for hm in re.finditer(r"\n(#{1,6})\s+\S", text[body_start:]):
        child_level = len(hm.group(1))
        if child_level <= level:
            return body_start + hm.start()
    return len(text)


def strip_non_normative_heading_sections(text: str) -> str:
    """Drop sections whose heading declares index/summary/non-normative ownership.

    Stripping continues through nested child headings until the next heading of
    equal or higher level (not an arbitrary first child heading).
    """
    if not NON_NORMATIVE_SECTION_HEADING_RE.search(text):
        return text
    parts: list[str] = []
    idx = 0
    for match in NON_NORMATIVE_SECTION_HEADING_RE.finditer(text):
        parts.append(text[idx : match.start()])
        level = _heading_level_at(text, match.start()) or 2
        end = _end_of_section_at_level(text, match.end(), level)
        idx = end
    parts.append(text[idx:])
    return "".join(parts)


def _strip_markdown_tables_matching(text: str, header_re: re.Pattern[str]) -> str:
    """Remove markdown tables whose header line matches header_re."""
    if not header_re.search(text):
        return text
    parts: list[str] = []
    idx = 0
    for match in header_re.finditer(text):
        line_start = text.rfind("\n", 0, match.start()) + 1
        parts.append(text[idx:line_start])
        cursor = match.end()
        if cursor < len(text) and text[cursor] == "\n":
            cursor += 1
        while cursor < len(text):
            eol = text.find("\n", cursor)
            line = text[cursor:] if eol < 0 else text[cursor:eol]
            if line.startswith("|"):
                cursor = len(text) if eol < 0 else eol + 1
                continue
            break
        idx = cursor
    parts.append(text[idx:])
    return "".join(parts)


def strip_two_column_summary_req_tables(text: str) -> str:
    """Remove two-column Requirement ID | Normative requirement summary catalogues.

    These tables historically duplicated RTM one-liners and must not prove
    ownership by themselves. Semantic sufficiency of retained prose remains an
    independent-audit concern.
    """
    return _strip_markdown_tables_matching(text, SUMMARY_REQ_TABLE_HEADER_RE)


def strip_four_column_family_boilerplate_tables(text: str) -> str:
    """Remove four-column synthetic family SHALL/envelope/boundary tables."""
    return _strip_markdown_tables_matching(text, FOUR_COLUMN_FAMILY_TABLE_HEADER_RE)


def strip_synthetic_family_normative_sections(text: str) -> str:
    """Drop `.0 Normative requirements — REQ-* family` catalogue sections.

    These headings and their boilerplate tables are not accepted as primary
    semantic requirement ownership. Substantive numbered sections remain.
    Stripping continues through nested children until equal/higher heading.
    """
    if not SYNTHETIC_FAMILY_SECTION_HEADING_RE.search(text):
        return text
    parts: list[str] = []
    idx = 0
    for match in SYNTHETIC_FAMILY_SECTION_HEADING_RE.finditer(text):
        parts.append(text[idx : match.start()])
        level = len(match.group(1))
        end = _end_of_section_at_level(text, match.end(), level)
        idx = end
    parts.append(text[idx:])
    return "".join(parts)


def strip_ownership_index_placeholders(text: str) -> str:
    """Remove index-only ownership placeholders that must not prove RTM ownership.

    Structural ownership indexes, RTM SECTION FORMAL REQUIREMENTS catalogues,
    bare identifier bullets/rows, two-/four-column summary catalogues, synthetic
    `.0 Normative requirements` family sections and labelled
    non-normative/summary sections are non-normative. Ownership must still bind
    through an exact Platform_08 section locator into remaining substantive
    owner text. A static validator cannot certify semantic completeness of that
    prose; independent review remains mandatory.
    """
    text = strip_all_marker_blocks(text, OWNERSHIP_INDEX_START, OWNERSHIP_INDEX_END)
    text = strip_rtm_section_formal_requirement_blocks(text)
    text = strip_synthetic_family_normative_sections(text)
    text = strip_non_normative_heading_sections(text)
    text = strip_two_column_summary_req_tables(text)
    text = strip_four_column_family_boilerplate_tables(text)
    # Drop the ownership-index heading and following bare-ID list until the next
    # heading, when present without markers.
    if OWNERSHIP_INDEX_HEADING_RE.search(text):
        parts: list[str] = []
        idx = 0
        for match in OWNERSHIP_INDEX_HEADING_RE.finditer(text):
            parts.append(text[idx : match.start()])
            level = _heading_level_at(text, match.start()) or 2
            idx = _end_of_section_at_level(text, match.end(), level)
        parts.append(text[idx:])
        text = "".join(parts)
    # Remove bare identifier-only bullets/rows so they cannot satisfy presence.
    text = BARE_ID_BULLET_RE.sub("", text)
    text = BARE_ID_TABLE_ROW_RE.sub("", text)
    return text


def extract_locator_section_ids(locator: str) -> list[str]:
    """Return ordered unique numbered section ids referenced by a locator."""
    found: list[str] = []
    seen: set[str] = set()
    for match in LOCATOR_SECTION_RE.finditer(locator or ""):
        sid = next((g for g in match.groups() if g), None)
        if sid and sid not in seen:
            seen.add(sid)
            found.append(sid)
    return found


class DuplicateSectionError(ValueError):
    """Raised when a numbered section identifier appears more than once."""


def section_body_map(text: str) -> dict[str, str]:
    """Map numbered heading ids to body text until the next same/higher heading.

    Duplicate numeric section identifiers are a hard failure — this function
    never silently overwrites an earlier section key.
    """
    matches = list(MARKDOWN_NUMBERED_HEADING_RE.finditer(text))
    bodies: dict[str, str] = {}
    first_line: dict[str, str] = {}
    for i, match in enumerate(matches):
        level = len(match.group(1))
        sid = match.group(2)
        start = match.end()
        end = len(text)
        for nxt in matches[i + 1 :]:
            if len(nxt.group(1)) <= level:
                end = nxt.start()
                break
        if sid in bodies:
            raise DuplicateSectionError(
                f"duplicate numbered section identifier {sid!r} "
                f"(first={first_line[sid]!r}; later={match.group(0)!r})"
            )
        bodies[sid] = text[start:end]
        first_line[sid] = match.group(0)
    return bodies


def locator_binds_id(locator: str, rid: str, owner_text: str) -> tuple[bool, str]:
    """Structural check: locator names a real section and the ID appears there.

    Returns (ok, reason). Empty/coarse locators without numbered sections fail.
    Synthetic `.0 Normative requirements — REQ-* family` locators always fail.
    Catalogue/index/summary sections cannot satisfy primary ownership because
    they are stripped before this check.
    """
    if SYNTHETIC_FAMILY_LOCATOR_RE.search(locator or ""):
        return (
            False,
            "locator points at synthetic .0 Normative requirements family catalogue",
        )
    section_ids = extract_locator_section_ids(locator)
    if not section_ids:
        return False, "locator lacks numbered section reference"
    try:
        bodies = section_body_map(owner_text)
    except DuplicateSectionError as exc:
        return False, f"ambiguous section locator (duplicate ids): {exc}"
    # Also allow the heading line itself to contain the ID. Reject ambiguous
    # heading maps the same way section_body_map rejects duplicate bodies.
    heading_only: dict[str, str] = {}
    for m in MARKDOWN_NUMBERED_HEADING_RE.finditer(owner_text):
        sid = m.group(2)
        if sid in heading_only:
            return (
                False,
                f"ambiguous section locator (duplicate heading id {sid})",
            )
        heading_only[sid] = m.group(0)
    matched_existing = False
    for sid in section_ids:
        if sid not in bodies and sid not in heading_only:
            continue
        matched_existing = True
        chunk = heading_only.get(sid, "") + "\n" + bodies.get(sid, "")
        if rid.startswith("REQ-"):
            present = rid in REQ_ID_RE.findall(chunk)
        elif rid.startswith("PRD-"):
            present = rid in PRD_ID_RE.findall(chunk)
        else:
            present = rid in chunk
        if present:
            return True, ""
    if not matched_existing:
        return (
            False,
            "locator section not found in owner: " + ", ".join(section_ids),
        )
    return False, "formal ID absent from locator section(s)"


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
        # Ownership proven solely via trailing ID indexes / bare ID placeholders
        # is rejected. Substantive normative tables and prose remain.
        text = strip_ownership_index_placeholders(text)
        return text

    def _validate_registry_owners(
        self,
        rows: list[dict[str, str]],
        kind: str,
        paths: dict[str, Path],
    ) -> None:
        owner_text_cache: dict[str, str | None] = {}
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
            # Reject synthetic family locators before presence checks so the
            # failure names the catalogue defect rather than a stripped ID.
            if locator and SYNTHETIC_FAMILY_LOCATOR_RE.search(locator):
                self.fail(
                    f"{kind} {rid}: source locator does not bind ID to a "
                    f"substantive owner section (locator points at synthetic "
                    f".0 Normative requirements family catalogue): "
                    f"locator={locator!r} owner={owner}"
                )
                continue
            if owner not in owner_text_cache:
                owner_text_cache[owner] = self._owner_text_without_primary_registry(
                    owner, paths, kind
                )
            owner_text = owner_text_cache[owner]
            if owner_text is None or not self._id_occurs_in_text(rid, owner_text):
                self.fail(
                    f"{kind} {rid}: formal ID absent from declared owner "
                    f"outside primary registry: {owner}"
                )
                continue
            # Structural semantic-ownership gate: Platform_08 locator must name
            # an exact numbered owner section that contains the ID after
            # non-normative catalogues are stripped.
            if not locator:
                continue
            ok, reason = locator_binds_id(locator, rid, owner_text)
            if not ok:
                self.fail(
                    f"{kind} {rid}: source locator does not bind ID to a "
                    f"substantive owner section ({reason}): locator={locator!r} "
                    f"owner={owner}"
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

    def validate_unique_section_identifiers(
        self, paths: dict[str, Path]
    ) -> None:
        """Reject duplicate numeric section IDs at every numbered heading depth.

        Applies to all Mother Reference markdown files. Duplicate `## N`,
        `### N.M`, `#### N.M.x`, etc. fail closed; `section_body_map` must never
        silently overwrite a colliding key.
        """
        for rel, path in paths.items():
            if not rel.endswith(".md") or not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
            # Strip fenced code so example markdown cannot create false collisions.
            cleaned_parts: list[str] = []
            in_fence = False
            for line in text.splitlines(keepends=True):
                if line.lstrip().startswith("```"):
                    in_fence = not in_fence
                    continue
                if not in_fence:
                    cleaned_parts.append(line)
            cleaned = "".join(cleaned_parts)
            try:
                section_body_map(cleaned)
            except DuplicateSectionError as exc:
                self.fail(f"{rel}: {exc}")
            # Also report via Counter for clearer multi-collision messages.
            ids = [
                m.group(2) for m in MARKDOWN_NUMBERED_HEADING_RE.finditer(cleaned)
            ]
            for sid, count in Counter(ids).items():
                if count > 1:
                    msg = (
                        f"{rel}: duplicate numbered section identifier "
                        f"{sid} (count={count})"
                    )
                    if msg not in self.errors and not any(
                        rel in e and f"duplicate numbered section identifier {sid!r}" in e
                        for e in self.errors
                    ):
                        self.fail(msg)

    _VARCHAR_ENUM_FIELD_RE = re.compile(
        r"`?([A-Za-z_][A-Za-z0-9_]*)`?\s+"
        r"VARCHAR\((\d+)\)\s+NN?"
        r"[^\n]*?"
        r"(?:CHECK\s+IN\s*\(([^)]+)\)|\(([^)]+)\))",
        re.I,
    )

    def validate_postgres_enum_varchar_widths(
        self, paths: dict[str, Path]
    ) -> None:
        """Fail when a declared VARCHAR(n) cannot hold every listed enum literal."""
        rel = "Platform_17_API_Event_and_Data_Model_Contracts.md"
        path = paths.get(rel)
        if path is None or not path.is_file():
            return
        text = path.read_text(encoding="utf-8")
        for m in self._VARCHAR_ENUM_FIELD_RE.finditer(text):
            field, width_s, check_list, paren_list = m.groups()
            width = int(width_s)
            raw = check_list or paren_list or ""
            # Only treat as enum list when tokens look like quoted/UPPER literals.
            literals = re.findall(r"'([A-Z][A-Z0-9_]*)'|\b([A-Z][A-Z0-9_]{2,})\b", raw)
            values = [a or b for a, b in literals]
            if len(values) < 2:
                continue
            # Skip likely non-enum parenthetical notes.
            if any(v in {"UUID", "JSONB", "BYTEA", "TIMESTAMPTZ", "INET"} for v in values):
                continue
            longest = max(values, key=len)
            if len(longest) > width:
                self.fail(
                    f"{rel}: VARCHAR({width}) too narrow for {field} enum "
                    f"(longest literal {longest!r} len={len(longest)})"
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

    # Same-statement historical / non-normative markers.
    # "supersed" matches supersedes / superseded / superseding.
    _LICENCE_HISTORICAL_MARKERS: tuple[str, ...] = (
        "historical",
        "supersed",
        "non-normative",
        "provenance only",
        "provenance-only",
        "record only",
        "record-only",
    )

    # Normative unresolved licence-grace token (and flexible whitespace).
    _FOUNDER_LIC_GRACE_UNRESOLVED_RE = re.compile(
        r"founder-lic-grace\s+unresolved",
        re.IGNORECASE,
    )

    # Bounded semantic forms of seven-day POST-expiry grace (not T-7 pre-expiry).
    # Deliberately excludes pre-expiry / T-7 banner wording.
    _SEVEN_DAY_POST_EXPIRY_RE = re.compile(
        r"(?:"
        r"post[-\s]?expiry\s+grace(?:\s+(?:duration|period))?"
        r"(?:\s*(?::|=)\s*|\s+is\s+)(?:7|seven)\s+days?"
        r"|"
        r"(?:7|seven)[-\s]?days?\s+post[-\s]?expiry\s+grace"
        r"|"
        r"(?:7|seven)[-\s]?days?\s+(?:default\s+)?grace\s+after\s+expiry"
        r"|"
        r"grace\s+(?:period\s+)?after\s+expiry\s+is\s+(?:7|seven)\s+days?"
        r"|"
        r"7-day\s+default\s+grace\s+after\s+expiry"
        r"|"
        r"governed\s+seven-day\s+grace"
        r"|"
        r"existing\s+seven-day\s+grace\s+requirement"
        r"|"
        r"seven-day\s+default\s+post-expiry\s+grace"
        r"|"
        r"seven-day\s+licence\s+grace\s+remains\s+governed"
        r"|"
        r"existing\s+seven-day\s+licence\s+grace"
        r")",
        re.IGNORECASE,
    )

    _LIST_ITEM_RE = re.compile(r"^(?:[-*+]|\d+\.)\s+")
    _SENTENCE_BOUND_RE = re.compile(r"(?<=[.!?])\s+")

    def validate_normalized_rtm_licence_grace(
        self, paths: dict[str, Path]
    ) -> None:
        """Enforce Founder-approved 15-day post-expiry licence grace.

        Guards REQ-LIC-004 / REQ-EGC-007 / PRD-EGC-007 and rejects normative
        reintroduction of FOUNDER-LIC-GRACE UNRESOLVED or seven-day post-expiry
        grace. Preserves T-7 PRE-EXPIRY banner semantics. Historical/superseded
        provenance mentioning seven-day/unresolved may remain when that
        individual occurrence's own Markdown line / table row / sentence (or
        list-clause) unambiguously labels it historical/superseded/
        non-normative/provenance-only — not via a nearby unrelated statement.

        The Platform_08 registry 15-day / T-7 row checks remain NORMALIZED_V1-
        gated with Platform_08. The constitutional stale-regression scan
        (FOUNDER-LIC-GRACE UNRESOLVED and seven-day post-expiry forms) applies
        to every constitutional Mother Reference path supplied in ``paths``,
        including TRANSITIONAL_LEGACY documents — normalization state does not
        waive this invariant.
        """
        rel = "Platform_08_Requirements_Traceability.md"
        if CANONICAL_NORMALIZATION.get(rel) != "NORMALIZED_V1":
            return
        path = paths.get(rel)
        if path is None or not path.is_file():
            return
        text = path.read_text(encoding="utf-8")

        self._reject_normative_seven_day_post_expiry(rel, text)
        self._reject_normative_founder_lic_grace_unresolved(rel, text)

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
            blob_l = blob.lower()
            # Row-level UNRESOLVED: each match must be locally historical.
            for m in self._FOUNDER_LIC_GRACE_UNRESOLVED_RE.finditer(blob):
                if not self._licence_local_is_historical(blob, m.start(), m.end()):
                    self.fail(
                        f"{rel}: {rid} must not state normative "
                        f"FOUNDER-LIC-GRACE UNRESOLVED "
                        f"(controlling truth: 15-day post-expiry grace)"
                    )
            has_15 = (
                "15-day" in blob_l
                or "15 day" in blob_l
                or "expired_grace_15d" in blob_l
                or "15 days" in blob_l
            )
            if not has_15:
                self.fail(
                    f"{rel}: {rid} primary registry row must state Founder-approved "
                    f"15-day post-expiry grace / EXPIRED_GRACE_15D"
                )

        lic003 = req_rows.get("REQ-LIC-003")
        if lic003 is not None:
            banner_blob = " ".join(
                [
                    lic003.get("summary", ""),
                    lic003.get("evidence", ""),
                    lic003.get("locator", ""),
                ]
            ).lower()
            has_temporal = bool(
                re.search(
                    r"(?:\bt-7\b|7-day|7\s+day|seven-day|"
                    r"seven\s+days?\s+before\s+expir)",
                    banner_blob,
                )
            )
            has_pre_expiry = (
                "pre-expiry" in banner_blob
                or "before expiry" in banner_blob
                or (
                    "expiry" in banner_blob
                    and ("warning" in banner_blob or "banner" in banner_blob)
                )
            )
            if not (has_temporal and has_pre_expiry):
                self.fail(
                    f"{rel}: REQ-LIC-003 must retain T-7 / 7-day pre-expiry banner semantics"
                )

        # Constitutional licence-grace regression: scan ALL mother-reference paths
        # regardless of NORMALIZED_V1 vs TRANSITIONAL_LEGACY.
        for doc_rel, doc_path in paths.items():
            if doc_rel == rel:
                continue
            if doc_rel not in CANONICAL_NORMALIZATION:
                continue
            if not doc_path.is_file():
                continue
            doc_text = doc_path.read_text(encoding="utf-8")
            self._reject_normative_founder_lic_grace_unresolved(doc_rel, doc_text)
            self._reject_normative_seven_day_post_expiry(doc_rel, doc_text)

    def _reject_normative_founder_lic_grace_unresolved(
        self, doc_rel: str, text: str
    ) -> None:
        """Fail when any FOUNDER-LIC-GRACE UNRESOLVED occurrence lacks same-statement labels."""
        for m in self._FOUNDER_LIC_GRACE_UNRESOLVED_RE.finditer(text):
            if not self._licence_local_is_historical(text, m.start(), m.end()):
                self.fail(
                    f"{doc_rel}: normative FOUNDER-LIC-GRACE UNRESOLVED is obsolete; "
                    f"label as historical/superseded or state 15-day RESOLVED truth"
                )

    def _reject_normative_seven_day_post_expiry(
        self, doc_rel: str, text: str
    ) -> None:
        """Fail normative seven-day POST-expiry grace; allow same-statement historical only.

        Explicit `_SEVEN_DAY_POST_EXPIRY_RE` matches are never excused by nearby
        T-7 / pre-expiry wording. T-7 banner text must not match the regex.
        """
        for m in self._SEVEN_DAY_POST_EXPIRY_RE.finditer(text):
            if self._licence_local_is_historical(text, m.start(), m.end()):
                continue
            self.fail(
                f"{doc_rel}: stale fixed post-expiry licence-grace wording "
                f"must not appear as normative truth: {m.group(0)!r}"
            )

    @classmethod
    def _licence_statement_text(cls, text: str, start: int, end: int) -> str:
        """Return the logical statement containing [start:end] (line/row/sentence/list)."""
        if start < 0:
            start = 0
        if end > len(text):
            end = len(text)
        line_start = text.rfind("\n", 0, start) + 1
        line_end = text.find("\n", end)
        if line_end < 0:
            line_end = len(text)
        line = text[line_start:line_end]
        stripped = line.lstrip()

        # Table row: entire row is the statement.
        if stripped.startswith("|"):
            return line

        # List item: include contiguous prior list items plus one intro line above.
        if cls._LIST_ITEM_RE.match(stripped):
            block_start = line_start
            while block_start > 0:
                prev_end = block_start - 1  # index of preceding '\n'
                prev_start = text.rfind("\n", 0, prev_end) + 1
                prev_line = text[prev_start:prev_end]
                prev_s = prev_line.strip()
                if not prev_s or prev_s.startswith("#") or prev_s.startswith("|"):
                    break
                if cls._LIST_ITEM_RE.match(prev_s):
                    block_start = prev_start
                    continue
                # Immediate non-list intro line for this list block.
                block_start = prev_start
                break
            return text[block_start:line_end]

        # Prose line: prefer the sentence containing the match.
        rel_start = start - line_start
        rel_end = end - line_start
        cursor = 0
        for m in cls._SENTENCE_BOUND_RE.finditer(line):
            if cursor <= rel_start and rel_end <= m.start():
                return line[cursor : m.start()]
            cursor = m.end()
        if cursor <= rel_start:
            return line[cursor:]
        return line

    @classmethod
    def _licence_local_is_historical(
        cls, text: str, start: int, end: int
    ) -> bool:
        """True when the occurrence's own statement labels it non-normative."""
        statement = cls._licence_statement_text(text, start, end).lower()
        return any(marker in statement for marker in cls._LICENCE_HISTORICAL_MARKERS)

    @classmethod
    def _licence_stale_is_historical(cls, text: str, stale: str) -> bool:
        """True when every stale-phrase occurrence is same-statement historical."""
        lower = text.lower()
        needle = stale.lower()
        idx = 0
        while True:
            pos = lower.find(needle, idx)
            if pos < 0:
                return True
            if not cls._licence_local_is_historical(
                text, pos, pos + len(needle)
            ):
                return False
            idx = pos + len(needle)

    def validate_network_flow_source_reality_status(self) -> None:
        """Pin BUILD-043/044/045/051 status truth to main 92f0fb37 source reality.

        Documentation candidate based on canonical main
        ``92f0fb37805c234be2869619abcc01ad26b5f30b`` must not overstate
        interface reconciliation, SNMP trap/inform, device-control connector,
        or Flow receiver implementation. Skipped when the programme register
        is absent (minimal fixture trees).
        """
        register_path = (
            self.repo_root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        )
        if not register_path.is_file():
            return
        text = register_path.read_text(encoding="utf-8")

        def summary_row(build_id: str) -> str:
            prefix = f"| {build_id} |"
            for line in text.splitlines():
                if line.startswith(prefix):
                    return line
            self.fail(f"Build Register missing summary row for {build_id}")
            return ""

        def ledger_row(num: str) -> str:
            section = text
            marker = "### BUILD-040..054"
            if marker in text:
                section = text[text.find(marker) :]
                nxt = section.find("\n### BUILD-", 1)
                if nxt != -1:
                    section = section[:nxt]
            prefix = f"| {num} |"
            for line in section.splitlines():
                if line.startswith(prefix):
                    return line
            self.fail(
                f"Build Register missing executable-plan ledger row for BUILD-{num}"
            )
            return ""

        def maturity_cell(row: str) -> str:
            cells = [c.strip() for c in row.strip().strip("|").split("|")]
            if not cells:
                return ""
            # Summary: ID | unit | scope | depends | Starting maturity | Exit
            # Ledger: BUILD | ... | ⑦ evidence / gate (last cell)
            if cells[0].startswith("BUILD-"):
                return cells[4] if len(cells) > 4 else ""
            return cells[-1]

        checks: list[tuple[str, str, str, tuple[str, ...]]] = [
            (
                "BUILD-043",
                "043",
                "SPECIFIED_ONLY / NOT_IMPLEMENTED",
                ("Partial/spec", "Partial"),
            ),
            (
                "BUILD-044",
                "044",
                "SPECIFIED_ONLY / NOT_IMPLEMENTED",
                ("Partial/spec", "Partial"),
            ),
            (
                "BUILD-045",
                "045",
                (
                    "**NOT DISPATCHABLE** until DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE "
                    "PASS; NOT_IMPLEMENTED"
                ),
                ("Mostly not implemented",),
            ),
            (
                "BUILD-051",
                "051",
                "NOT_IMPLEMENTED",
                ("Not production", "Next lane"),
            ),
        ]

        for build_id, num, required, banned in checks:
            for label, row in (
                ("summary", summary_row(build_id)),
                ("ledger", ledger_row(num)),
            ):
                if not row:
                    continue
                maturity = maturity_cell(row)
                if maturity != required and required not in maturity:
                    self.fail(
                        f"Build Register {label} {build_id} maturity must be "
                        f"{required!r} (source reality at main "
                        f"92f0fb37805c234be2869619abcc01ad26b5f30b); got {maturity!r}"
                    )
                for bad in banned:
                    if bad == "Partial":
                        # Exact cell or tokenized maturity, not substring of
                        # SPECIFIED_ONLY / NOT_IMPLEMENTED.
                        if maturity == "Partial" or maturity.startswith("Partial;"):
                            self.fail(
                                f"Build Register {label} {build_id} must not use "
                                f"overstated maturity {maturity!r}"
                            )
                        continue
                    if bad in maturity:
                        self.fail(
                            f"Build Register {label} {build_id} must not retain "
                            f"overstated maturity token {bad!r}"
                        )

        for row in (summary_row("BUILD-045"), ledger_row("045")):
            if not row:
                continue
            if "DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE" not in row:
                self.fail(
                    "BUILD-045 must retain DEVICE_CONTROL_P1_PROFILE_FREEZE_GATE"
                )
            if "NOT DISPATCHABLE" not in row:
                self.fail("BUILD-045 must retain NOT DISPATCHABLE until gate PASS")

        # BUILD-051 first UDP profile + GoFlow2 admission gate (docs only).
        # Maturity must remain NOT_IMPLEMENTED; profile freeze is not Partial.
        for row in (summary_row("BUILD-051"), ledger_row("051")):
            if not row:
                continue
            for required in (
                "FLOW_UDP_P1_PROFILE_V1",
                "FLOW_PARSER_DEPENDENCY_ADMISSION_GATE",
                "NOT_IMPLEMENTED",
                "SPECIFIED_NOT_IMPLEMENTED",
            ):
                if required not in row:
                    self.fail(
                        f"BUILD-051 must retain {required!r} in summary/ledger "
                        "(first UDP profile freeze; GoFlow2 admission; "
                        "source reality NOT_IMPLEMENTED)"
                    )
            for banned in ("Partial", "Next lane"):
                maturity = maturity_cell(row)
                if banned == "Partial":
                    if maturity == "Partial" or maturity.startswith("Partial;"):
                        self.fail(
                            "BUILD-051 must not promote documentation freeze to "
                            f"Partial maturity ({maturity!r})"
                        )
                elif banned in maturity:
                    self.fail(
                        f"BUILD-051 must not retain overstated maturity token "
                        f"{banned!r}"
                    )

        self.validate_sflow_v5_datagram_sequence_contract()
        self.validate_build_042_counter_leaf_source_truth()

    def validate_build_042_counter_leaf_source_truth(self) -> None:
        """Pin BUILD-042 status after PR #304 counter derivation leaf merge.

        Must not regress to obsolete ``Raw Counter64 exists`` alone, and must
        not claim whole-BUILD / BUILD-048 / deployment certification. Skipped
        when the programme register is absent (minimal fixture trees).
        """
        register_path = (
            self.repo_root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        )
        if not register_path.is_file():
            return
        text = register_path.read_text(encoding="utf-8")

        required = (
            "PARTIAL — exact raw Counter64 + counter derivation leaf "
            "implemented; integration/benchmark/certification pending"
        )
        banned_exact = ("Raw Counter64 exists",)

        summary = ""
        ledger = ""
        section = text
        marker = "### BUILD-040..054"
        if marker in text:
            section = text[text.find(marker) :]
            nxt = section.find("\n### BUILD-", 1)
            if nxt != -1:
                section = section[:nxt]
        for line in text.splitlines():
            if line.startswith("| BUILD-042 |"):
                summary = line
                break
        for line in section.splitlines():
            if line.startswith("| 042 |"):
                ledger = line
                break
        if not summary:
            self.fail("Build Register missing summary row for BUILD-042")
            return
        if not ledger:
            self.fail(
                "Build Register missing executable-plan ledger row for BUILD-042"
            )
            return

        for label, row in (("summary", summary), ("ledger", ledger)):
            if required not in row:
                self.fail(
                    f"Build Register {label} BUILD-042 must record "
                    f"{required!r} after PR #304 leaf merge"
                )
            for bad in banned_exact:
                # Allow historical discussion only if the required PARTIAL
                # status is present; ban obsolete sole maturity cell.
                cells = [c.strip() for c in row.strip().strip("|").split("|")]
                maturity = ""
                if cells and cells[0].startswith("BUILD-"):
                    maturity = cells[4] if len(cells) > 4 else ""
                else:
                    maturity = cells[-1] if cells else ""
                if maturity == bad:
                    self.fail(
                        f"Build Register {label} BUILD-042 must not regress to "
                        f"obsolete maturity {bad!r}"
                    )

        p17_path = (
            self.docs_root
            / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        )
        if p17_path.is_file():
            p17 = p17_path.read_text(encoding="utf-8")
            for needle in (
                required,
                "PR #304",
                "sensor/internal/protocol/snmp/counter/",
                "no whole BUILD-042 certification",
                "no BUILD-048 Network Sensor certification",
                "no deployment claim",
            ):
                if needle not in p17:
                    self.fail(
                        "Platform_17 §6 must reconcile BUILD-042 leaf source "
                        f"truth (missing {needle!r})"
                    )
            # Stale sole claim that derivation remains unimplemented.
            stale = (
                "That derived Counter32/Counter64 reset/wrap/discontinuity/"
                "rate layer remains **SPECIFIED but NOT IMPLEMENTED/CERTIFIED**"
            )
            if stale in p17:
                self.fail(
                    "Platform_17 §6 must not retain obsolete "
                    "SPECIFIED-but-NOT-IMPLEMENTED derivation claim after PR #304"
                )

    def validate_sflow_v5_datagram_sequence_contract(self) -> None:
        """Pin BUILD-051 S5 sFlow datagram sequence / discontinuity contract.

        Detailed semantics live in Platform_10; Build Register cross-references
        S5 fixtures. BUILD-051 maturity must remain NOT_IMPLEMENTED.
        Skipped when Platform_10 / Build Register are absent (minimal fixtures).
        """
        p10_path = (
            self.docs_root
            / "Platform_10_Protocol_Security_Connector_Master.md"
        )
        register_path = (
            self.repo_root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        )
        if not p10_path.is_file() or not register_path.is_file():
            return

        p10 = p10_path.read_text(encoding="utf-8")
        register = register_path.read_text(encoding="utf-8")

        if "sFlow v5 datagram sequence" not in p10:
            # Minimal fixtures without the Flow handoff section.
            if "FLOW_UDP_P1_PROFILE_V1" not in p10 and "BUILD-051" not in register:
                return
            self.fail(
                "Platform_10 must own sFlow v5 datagram sequence / gap / "
                "duplicate / reorder / restart semantics for BUILD-051 S5"
            )

        required_p10 = (
            "Primary loss authority = datagram sequence",
            "sFlow datagram `sequence_number`",
            "Do **NOT** scope only by source IP",
            "Sample sequence numbers are supplemental",
            "MUST NOT be interpreted as sequence-per-`source_id`",
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
            "SEQUENCE_DISCONTINUITY_UNKNOWN",
            "Duplicate and reorder do not become loss",
            "Stream epoch state",
            "first valid datagram establishes a fresh collector-side baseline",
            "Do not silently persist sequence state by exporter IP alone",
            "sample family (`FLOW` or `COUNTER`)",
            "SAMPLE_SEQUENCE_GAP",
            "SAMPLE_DUPLICATE",
            "SAMPLE_OUT_OF_ORDER",
            "MUST NOT change primary datagram loss counts",
            "sub_agent_id",
            "malformed packet does not mutate sequence high-water",
            "NOT_IMPLEMENTED",
        )
        for needle in required_p10:
            if needle not in p10:
                self.fail(
                    "Platform_10 sFlow v5 datagram sequence contract missing "
                    f"{needle!r}"
                )

        # Build Register must cross-reference S5 ownership without promoting
        # maturity, and retain the primary-authority / wrap / epoch pins.
        section = register[register.find("### BUILD-040..054") :]
        nxt = section.find("\n### BUILD-", 1)
        if nxt != -1:
            section = section[:nxt]
        ledger = ""
        for line in section.splitlines():
            if line.startswith("| 051 |"):
                ledger = line
                break
        if not ledger:
            self.fail("BUILD-051 ledger row missing for sFlow sequence contract")
            return

        for required in (
            "NOT_IMPLEMENTED",
            "S5 sFlow v5",
            "datagram-sequence primary loss authority",
            "uint32 wrap",
            "duplicate/reorder/gap distinction",
            "restart/discontinuity epoch",
            "sample-sequence supplemental-only",
            "Platform_10",
            "SEQUENCE_GAP",
            "DUPLICATE_DATAGRAM",
            "OUT_OF_ORDER_OR_OLD_DATAGRAM",
            "EXPORTER_RESTART_OR_DISCONTINUITY",
            "SEQUENCE_DISCONTINUITY_UNKNOWN",
            "SAMPLE_SEQUENCE_GAP",
            "sub_agent_id",
            "collector-restart baseline",
        ):
            if required not in ledger:
                self.fail(
                    f"BUILD-051 S5 ledger must retain sFlow sequence pin {required!r}"
                )

        maturity = [c.strip() for c in ledger.strip().strip("|").split("|")][-1]
        if maturity != "NOT_IMPLEMENTED":
            self.fail(
                "BUILD-051 must remain NOT_IMPLEMENTED after sFlow sequence "
                f"contract freeze; got {maturity!r}"
            )

        p17_path = (
            self.docs_root
            / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        )
        if p17_path.is_file():
            p17 = p17_path.read_text(encoding="utf-8")
            if "FLOW_UDP_P1_PROFILE_V1" in p17:
                for required in (
                    "sFlow v5 sequence quality envelope",
                    "DUPLICATE_DATAGRAM",
                    "SEQUENCE_COLLISION",
                    "OUT_OF_ORDER_OR_OLD_DATAGRAM",
                    "SEQUENCE_DISCONTINUITY_UNKNOWN",
                    "SAMPLE_SEQUENCE_GAP",
                    "datagram header `sequence_number`",
                    "MUST NOT mutate primary datagram-loss counts",
                ):
                    if required not in p17:
                        self.fail(
                            "Platform_17 Flow quality envelope missing sFlow "
                            f"sequence outcome {required!r}"
                        )

        p12_path = (
            self.docs_root / "Platform_12_Decision_Source_Register.md"
        )
        if p12_path.is_file():
            p12 = p12_path.read_text(encoding="utf-8")
            if "FLOW_PARSER_DEPENDENCY_ADMISSION_GATE" in p12:
                for required in (
                    "sFlow v5 datagram sequence / discontinuity authority",
                    "EXPORTER_RESTART_OR_DISCONTINUITY",
                    "prefer possible false discontinuity over fabricated giant loss",
                    "NOT_IMPLEMENTED",
                ):
                    if required not in p12:
                        self.fail(
                            "Platform_12 must register sFlow sequence decision "
                            f"containing {required!r}"
                        )

    def validate_dependency_activation_combine_operator(self) -> None:
        """Pin DependencyActivationV1 explicit ANY|ALL combination operator.

        Skipped when Platform_17 / Platform_22 are absent (minimal fixtures).
        """
        p17_path = (
            self.docs_root
            / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        )
        p22_path = (
            self.docs_root
            / "Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md"
        )
        if not p17_path.is_file() or not p22_path.is_file():
            return
        p17 = p17_path.read_text(encoding="utf-8")
        p22 = p22_path.read_text(encoding="utf-8")
        if "###### `DependencyActivationV1`" not in p17:
            return
        act = p17[
            p17.find("###### `DependencyActivationV1`") : p17.find(
                "###### `ThresholdExprV1` (discriminated union)"
            )
        ]
        for required in (
            "combine: ANY | ALL",
            "combine=ANY",
            "combine=ALL",
            "At least one criterion MUST be enabled",
            "No hidden/default combination behavior is permitted",
            "ALL` with only one enabled criterion is invalid",
            "stale/unqualified dependency evidence ⇒ `UNKNOWN`",
        ):
            if required not in act:
                self.fail(
                    "Platform_17 DependencyActivationV1 must freeze "
                    f"combination operator contract containing {required!r}"
                )
        body_dep = p22[
            p22.find("#### 6.1.9 Dependency threshold") : p22.find("#### 6.1.10")
        ]
        for required in (
            "combine: ANY | ALL",
            "combine=ANY",
            "combine=ALL",
            "no hidden/default combination behavior is permitted",
            "cannot activate `SUPPRESS`/`DEFER`",
        ):
            if required not in body_dep:
                self.fail(
                    "Platform_22 §6.1.9 must freeze DependencyActivation "
                    f"ANY|ALL semantics containing {required!r}"
                )

    def validate_agent_ingress_pending_reservation_safety(self) -> None:
        """Pin Platform_17 §7.1 Agent-ingress pending-reservation invariant.

        A deterministic pre-publication non-terminal rejection must not leave an
        unpublished durable pending reservation blocking continuity. Bindings in
        Platform_05/06 and BUILD-062/024 must reference §7.1 without dropping the
        frozen invariant phrase. Also pins the zero-pending upgrade/migration
        gate (Platform_16 §5.1 / §7.1.9 / BUILD-127) and crash/restart exact
        retry fixture under BUILD-062. Skipped when Platform_17 is absent.
        """
        p17_path = (
            self.docs_root
            / "Platform_17_API_Event_and_Data_Model_Contracts.md"
        )
        if not p17_path.is_file():
            return
        p17 = p17_path.read_text(encoding="utf-8")

        # Minimal fixture trees often stub Platform_17 without the PR #81
        # durability note. Enforce only when that contract surface is present.
        has_durability = "## 7. Telemetry durability" in p17
        has_section_71 = "### 7.1 Agent-ingress pending-reservation" in p17
        if not has_durability and not has_section_71:
            return

        frozen = (
            "A deterministic pre-publication non-terminal rejection MUST NOT "
            "leave an unpublished durable pending reservation blocking continuity."
        )
        if frozen not in p17:
            self.fail(
                "Platform_17 must freeze Agent-ingress pending-reservation "
                f"invariant: {frozen!r}"
            )

        # Prefer the dedicated §7.1 owner; fail if the frozen phrase exists only
        # as a dangling cross-reference without the section heading.
        if not has_section_71:
            self.fail(
                "Platform_17 must own pending-reservation safety under "
                "### 7.1 Agent-ingress pending-reservation …"
            )

        required_p17 = (
            "### 7.1.1 Disposition classes",
            "### 7.1.2 Normative processing order",
            "### 7.1.3 Stale / unknown / revoked / fenced assignment authority",
            "### 7.1.4 Pending reservation safety and conditional release",
            "### 7.1.5 Retryable infrastructure / publication failure",
            "### 7.1.6 Cleanup failure",
            "### 7.1.7 Terminal disposition remains distinct",
            "### 7.1.8 Mandatory acceptance tests / fixtures",
            "### 7.1.9 Schema migration / release-guard upgrade precondition",
            "TERMINALLY_REJECTED",
            "ERR_AGENT_IDENTITY_CONFLICT",
            "ERR_AGENT_ASSIGNMENT_UNKNOWN",
            "pending_first_sequence IS NULL",
            "historical disposition lookup",
            "mutable assignment",
            "conditional release",
            "publisher-owned",
            "Sensor retains custody",
            "same unsettled sequence",
            "Historical pending publication state is **not inferable**",
            "zero-pending",
            "Crash after mutable validation failure, before pending release",
            "Manual pending deletion/reset is **not** accepted remediation",
            "STAGE 1 — DRAIN MODE",
            "STAGE 2 — HARD CUTOVER FENCE",
            "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
            "check-then-unlock-then-migrate gap",
            "CUTOVER-RACE-1",
            "CUTOVER-RACE-2",
            "CUTOVER-RACE-3",
            "CUTOVER-RACE-4",
            "database-level migration/cutover exclusion",
            "per-Sensor lock alone does **not** fence",
        )
        for needle in required_p17:
            if needle not in p17:
                self.fail(
                    f"Platform_17 §7.1 missing required pending-reservation "
                    f"contract text: {needle!r}"
                )

        # Processing-order steps must remain ahead of mutable validation.
        order_markers = (
            "Authenticate Sensor",
            "Derive server-authoritative tenant and Sensor identity",
            "Validate immutable request structure",
            "exact durable prior-disposition",
            "mutable assignment / entity / binding validation",
        )
        for needle in order_markers:
            if needle not in p17:
                self.fail(
                    f"Platform_17 §7.1 processing order missing step text: "
                    f"{needle!r}"
                )

        p05_path = self.docs_root / "Platform_05_LLD.md"
        if p05_path.is_file():
            p05 = p05_path.read_text(encoding="utf-8")
            for needle in (
                "Platform_17` §7.1",
                frozen,
                "ERR_AGENT_IDENTITY_CONFLICT",
            ):
                if needle not in p05:
                    self.fail(
                        f"Platform_05 must bind Platform_17 §7.1 "
                        f"(missing {needle!r})"
                    )

        p06_path = self.docs_root / "Platform_06_Dataflow.md"
        if p06_path.is_file():
            p06 = p06_path.read_text(encoding="utf-8")
            for needle in (
                "Platform_17` §7.1",
                frozen,
                "### 119.21 Upgrade / rollback",
                "pending Agent-ingress = non-zero",
                "STAGE 1 drain mode",
                "STAGE 2 hard cutover fence",
                "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
                "No Core partial-schema state is valid",
            ):
                if needle not in p06:
                    self.fail(
                        f"Platform_06 must bind Platform_17 §7.1 / "
                        f"zero-pending upgrade preflight (missing {needle!r})"
                    )

        p16_path = (
            self.docs_root
            / "Platform_16_Installation_Upgrade_Rollback_and_Operations.md"
        )
        if p16_path.is_file():
            p16 = p16_path.read_text(encoding="utf-8")
            for needle in (
                "### 5.1 Phase-1 standalone preflight — Agent-ingress "
                "pending-reservation upgrade gate",
                "pending Agent-ingress = non-zero",
                "MUST NOT delete pending Agent-ingress rows manually",
                "Verify all Agent-ingress pending reservations are zero",
                "Verify no active Agent-ingress publisher leases remain",
                "STAGE 1 — DRAIN MODE",
                "STAGE 2 — HARD CUTOVER FENCE",
                "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
                "check-then-unlock-then-migrate gap is forbidden",
                "database-level migration/cutover exclusion",
            ):
                if needle not in p16:
                    self.fail(
                        "Platform_16 must freeze Agent-ingress zero-pending "
                        f"upgrade gate (missing {needle!r})"
                    )

        register_path = (
            self.repo_root / "Platform_Master_Build_Sequence_Register_v1.0.md"
        )
        if register_path.is_file():
            register = register_path.read_text(encoding="utf-8")
            for needle in (
                "Platform_17` §7.1",
                "pending-reservation",
                "BUILD-062",
                "BUILD-024",
                "BUILD-127",
                "zero-pending",
                "crash after mutable-validation failure before pending release",
                "CUTOVER-RACE-1",
                "ERR_PLATFORM_UPGRADE_IN_PROGRESS",
                "hard cutover fence",
            ):
                if needle not in register:
                    self.fail(
                        "Build Register must bind Platform_17 §7.1 "
                        f"pending-reservation safety (missing {needle!r})"
                    )
            # Ownership must stay on 062/024; do not reopen BUILD-003 CERTIFIED.
            summary_003 = ""
            summary_062 = ""
            summary_127 = ""
            for line in register.splitlines():
                if line.startswith("| BUILD-003 |"):
                    summary_003 = line
                elif line.startswith("| BUILD-062 |"):
                    summary_062 = line
                elif line.startswith("| BUILD-127 |"):
                    summary_127 = line
            if summary_003 and "CERTIFIED" not in summary_003:
                self.fail(
                    "BUILD-003 historical CERTIFIED status must remain intact "
                    "when binding §7.1 pending-reservation safety"
                )
            if summary_003 and "does **not** reopen this certified record" not in summary_003:
                self.fail(
                    "BUILD-003 summary must state §7.1 does not reopen the "
                    "certified record"
                )
            if summary_062 and "runtime" not in summary_062.lower():
                self.fail(
                    "BUILD-062 must own runtime pending/replay behavior for "
                    "Agent-ingress pending-reservation safety"
                )
            if summary_127 and "zero-pending" not in summary_127:
                self.fail(
                    "BUILD-127 must own zero-pending Agent-ingress migration "
                    "preflight acceptance"
                )
            if summary_127 and "hard cutover fence" not in summary_127:
                self.fail(
                    "BUILD-127 must own race-safe hard cutover fence acceptance"
                )
            if summary_062 and "ERR_PLATFORM_UPGRADE_IN_PROGRESS" not in summary_062:
                self.fail(
                    "BUILD-062 must own hard-cutover fence admission "
                    "(ERR_PLATFORM_UPGRADE_IN_PROGRESS)"
                )
            # Preserve prior Network/Flow status truth closures.
            for build_id, required in (
                ("BUILD-043", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
                ("BUILD-044", "SPECIFIED_ONLY / NOT_IMPLEMENTED"),
                ("BUILD-051", "NOT_IMPLEMENTED"),
            ):
                row = ""
                for line in register.splitlines():
                    if line.startswith(f"| {build_id} |"):
                        row = line
                        break
                if row and required not in row:
                    self.fail(
                        f"{build_id} maturity truth must remain {required!r} "
                        "when binding Agent-ingress upgrade gate"
                    )

    def validate_host_global_runtime_exclusion_contract(self) -> None:
        """Pin shared-host Docker runtime exclusion across Platform_13/14/15.

        Concurrent Compose/runtime mutation on the self-hosted AmrudAi
        development/CI host is forbidden. One OS advisory lock
        (``flock`` / ``fcntl.flock``) on the stable path is mandatory; CI
        Integration is a heavy owner; Test 20 foreign-container failure must
        not be weakened by an ``agent-e2e-*`` allowlist. Skipped when the
        Docker standards owner document is absent (minimal fixture trees).
        """
        p13_path = (
            self.docs_root
            / "Platform_13_Engineering_Development_and_Docker_Standards.md"
        )
        if not p13_path.is_file():
            return
        p13 = p13_path.read_text(encoding="utf-8")
        if "## 22. Host-global shared Docker runtime exclusion" not in p13:
            # Minimal stubs without the Docker contract surface are skipped.
            if "Docker Compose" not in p13 and "Compose profiles" not in p13:
                return
            self.fail(
                "Platform_13 must own host-global shared Docker runtime "
                "exclusion under ## 22. Host-global shared Docker runtime "
                "exclusion"
            )
            return

        lock_path = (
            "/home/gagan/amrudai-evidence/runtime/platform-dev-runner.lock"
        )
        for required in (
            lock_path,
            "flock",
            "fcntl.flock",
            "Kernel lock ownership is authoritative",
            "Never unlink",
            "runtime-busy",
            "agent-e2e-*",
            "CODEX Desktop heavy E2E",
            "GitHub Actions jobs on `[self-hosted, Linux, X64, platform-ci]`",
        ):
            if required not in p13:
                self.fail(
                    "Platform_13 §22 host-global runtime exclusion missing "
                    f"{required!r}"
                )

        p15_path = (
            self.docs_root / "Platform_15_CI_CD_Packaging_and_Distribution.md"
        )
        if p15_path.is_file():
            p15 = p15_path.read_text(encoding="utf-8")
            if "### 3." in p15 or "CI/CD" in p15:
                for required in (
                    "### 3.8 Self-hosted host-global runtime exclusion",
                    "Integration, PostgreSQL, contract, security",
                    lock_path,
                    "before any Platform Docker mutation",
                    "owned cleanup",
                    "always()",
                    "runtime-busy",
                    "agent-e2e-*",
                ):
                    if required not in p15:
                        self.fail(
                            "Platform_15 §3.8 Integration runtime-lock "
                            f"binding missing {required!r}"
                        )

        p14_path = (
            self.docs_root
            / "Platform_14_Test_Performance_and_Release_Quality.md"
        )
        if p14_path.is_file():
            p14 = p14_path.read_text(encoding="utf-8")
            if "Current CI evidence boundary" in p14 or "release quality" in p14.lower():
                for required in (
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
                ):
                    if required not in p14:
                        self.fail(
                            "Platform_14 host-global runtime exclusion "
                            f"acceptance missing {required!r}"
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
        self.validate_unique_section_identifiers(paths)
        self.validate_postgres_enum_varchar_widths(paths)
        self.validate_maturity_language(paths)
        self.validate_normalized_rtm_licence_grace(paths)
        self.validate_a23_oss_namespace_and_ui_truth(manifest, paths)
        self.validate_network_flow_source_reality_status()
        self.validate_dependency_activation_combine_operator()
        self.validate_agent_ingress_pending_reservation_safety()
        self.validate_host_global_runtime_exclusion_contract()
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
