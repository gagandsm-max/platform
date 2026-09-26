#!/usr/bin/env python3
"""Strict structural contract for canonical Platform Skills (Issue #93 Phase B4).

Authoritative checker used by skill_depth_report.py, validate_agents/agent_pack,
and deterministic tests. Whole-file prose keywords and YAML frontmatter never
satisfy a required section.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

# Exact H2 titles in required relative order.
CANONICAL_HEADINGS: tuple[str, ...] = (
    "Purpose",
    "Use when",
    "Authoritative inputs",
    "Invariants",
    "Required procedure",
    "Negative / failure cases",
    "Security / tenant implications",
    "Required evidence",
    "Stop conditions",
    "Anti-patterns",
    "Handoff / output",
    "Non-ownership",
)

CANONICAL_HEADING_SET = frozenset(CANONICAL_HEADINGS)

# Map structural section titles to severity signal keys.
HEADING_TO_SIGNAL: dict[str, str] = {
    "Purpose": "trigger",
    "Use when": "trigger",
    "Authoritative inputs": "authoritative_inputs",
    "Invariants": "invariants",
    "Required procedure": "procedure",
    "Negative / failure cases": "negative_failure",
    "Security / tenant implications": "security_tenancy",
    "Required evidence": "evidence",
    "Stop conditions": "stop_conditions",
    "Anti-patterns": "anti_patterns",
    "Handoff / output": "handoff",
    "Non-ownership": "non_ownership",
}

# Signals used for P0/P1 escalation (executable procedure contract).
CRITICAL_SIGNALS = frozenset({"trigger", "invariants", "procedure", "evidence"})

EXECUTABLE_SIGNALS = frozenset(
    {
        "trigger",
        "authoritative_inputs",
        "invariants",
        "procedure",
        "negative_failure",
        "security_tenancy",
        "evidence",
        "stop_conditions",
        "anti_patterns",
        "handoff",
    }
)

# Skills whose structural incompleteness must escalate above generic P2.
OPERATIONAL_CRITICAL_SKILLS = frozenset(
    {
        "agent_architecture",
        "ai_evidence_safety",
        "alert_quality_evidence",
        "backup_restore",
        "bounded_implementation",
        "evidence_bundle",
        "exact_head_merge_gate",
        "ha_dr",
        "migration_safety",
        "pki_mtls",
        "replay_idempotency",
        "runtime_failure_injection",
        "secrets_handling",
        "security_negative_testing",
        "tenant_isolation",
    }
)

H2_RE = re.compile(r"^##\s+(.+?)\s*$", re.MULTILINE)
FRONTMATTER_RE = re.compile(r"\A---\r?\n.*?\r?\n---\r?\n?", re.DOTALL)


@dataclass
class SkillStructureResult:
    """Result of strict structural validation for one Skill body."""

    ok: bool
    errors: list[str] = field(default_factory=list)
    missing_headings: list[str] = field(default_factory=list)
    duplicate_headings: list[str] = field(default_factory=list)
    empty_sections: list[str] = field(default_factory=list)
    order_errors: list[str] = field(default_factory=list)
    present_signals: list[str] = field(default_factory=list)
    missing_signals: list[str] = field(default_factory=list)
    h2_titles: list[str] = field(default_factory=list)

    @property
    def structural_label(self) -> str:
        return "STRUCTURAL_OK" if self.ok else "STRUCTURAL_FAIL"


def strip_frontmatter(text: str) -> str:
    """Remove leading YAML frontmatter; frontmatter never satisfies sections."""
    match = FRONTMATTER_RE.match(text)
    if match:
        return text[match.end() :]
    return text


def iter_h2_sections(body: str) -> list[tuple[str, str]]:
    """Return (title, section_body) for each Markdown H2 in body."""
    matches = list(H2_RE.finditer(body))
    sections: list[tuple[str, str]] = []
    for index, match in enumerate(matches):
        title = match.group(1).strip()
        content_start = match.end()
        content_end = matches[index + 1].start() if index + 1 < len(matches) else len(body)
        sections.append((title, body[content_start:content_end]))
    return sections


def validate_skill_structure(text: str) -> SkillStructureResult:
    """Validate a canonical SKILL.md body against the B4 structural contract."""
    body = strip_frontmatter(text)
    sections = iter_h2_sections(body)
    titles = [title for title, _section in sections]
    result = SkillStructureResult(ok=True, h2_titles=titles)

    # Index first/last occurrence among all H2 titles.
    positions: dict[str, list[int]] = {}
    for index, title in enumerate(titles):
        positions.setdefault(title, []).append(index)

    for heading in CANONICAL_HEADINGS:
        idxs = positions.get(heading, [])
        if not idxs:
            result.missing_headings.append(heading)
            result.errors.append(f"missing required H2: ## {heading}")
        elif len(idxs) > 1:
            result.duplicate_headings.append(heading)
            result.errors.append(f"duplicate required H2: ## {heading} (count={len(idxs)})")

    # Canonical relative order: first-occurrence indexes must be strictly increasing.
    ordered_indexes: list[int] = []
    for heading in CANONICAL_HEADINGS:
        idxs = positions.get(heading, [])
        if len(idxs) == 1:
            ordered_indexes.append(idxs[0])
    if ordered_indexes != sorted(ordered_indexes):
        msg = "required H2 headings are not in canonical order"
        result.order_errors.append(msg)
        result.errors.append(msg)

    # Non-empty body for each required section (reject whitespace-only).
    body_by_title = {title: section for title, section in sections}
    for heading in CANONICAL_HEADINGS:
        if heading not in body_by_title:
            continue
        # If duplicates exist, still require every occurrence non-empty.
        for title, section in sections:
            if title != heading:
                continue
            if section.strip() == "":
                if heading not in result.empty_sections:
                    result.empty_sections.append(heading)
                result.errors.append(f"empty required section body: ## {heading}")
                break

    # Derived signal view (for severity). Trigger needs both Purpose and Use when.
    present: set[str] = set()
    for heading in CANONICAL_HEADINGS:
        if heading in result.missing_headings or heading in result.empty_sections:
            continue
        if heading not in positions:
            continue
        signal = HEADING_TO_SIGNAL[heading]
        if signal == "trigger":
            continue
        if signal != "non_ownership":
            present.add(signal)
    if (
        "Purpose" not in result.missing_headings
        and "Purpose" not in result.empty_sections
        and "Use when" not in result.missing_headings
        and "Use when" not in result.empty_sections
    ):
        present.add("trigger")

    result.present_signals = sorted(present & EXECUTABLE_SIGNALS)
    result.missing_signals = sorted(EXECUTABLE_SIGNALS - present)
    result.ok = not result.errors
    return result


def severity_for_structure(
    skill_name: str,
    structure: SkillStructureResult,
    *,
    operational_critical: frozenset[str],
    line_count: int,
) -> str:
    """Map structural result to OK/P0/P1/P2. Critical Skills never hide as P2."""
    if structure.ok:
        return "OK"

    missing = set(structure.missing_signals)
    for heading in structure.missing_headings + structure.empty_sections:
        missing.add(HEADING_TO_SIGNAL[heading])

    miss_crit = CRITICAL_SIGNALS & missing
    miss_exec = EXECUTABLE_SIGNALS & missing
    critical_skill = skill_name in operational_critical
    structural_hard = bool(
        structure.missing_headings
        or structure.duplicate_headings
        or structure.order_errors
        or structure.empty_sections
    )

    if critical_skill:
        if miss_crit or miss_exec or structural_hard:
            if miss_crit or line_count < 40 or structure.missing_headings or structure.duplicate_headings:
                return "P0"
            return "P1"
        return "P1"

    if miss_crit and line_count < 28:
        return "P0"
    if miss_crit or structure.missing_headings or structure.duplicate_headings or structure.order_errors:
        return "P1"
    return "P2"


def format_structure_errors(skill_name: str, structure: SkillStructureResult) -> list[str]:
    """Validator-facing error strings for a Skill."""
    if structure.ok:
        return []
    prefix = f"skill structural contract failed project_agents/skills/{skill_name}/SKILL.md"
    return [f"{prefix}: {err}" for err in structure.errors]
