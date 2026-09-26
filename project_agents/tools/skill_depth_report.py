#!/usr/bin/env python3
"""Deterministic Skill procedural-depth gap report (Issue #93 Phase B4).

Recomputes project_agents/skills/SKILL_DEPTH_GAP_REPORT.md from canonical
SKILL.md bodies using the strict structural contract in skill_structure.py.

After B4, structural failures also fail validate_agents.py. Do not hand-edit
the report into a false PASS — regenerate with this script after Skill changes.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from skill_structure import (
    CANONICAL_HEADINGS,
    EXECUTABLE_SIGNALS,
    OPERATIONAL_CRITICAL_SKILLS,
    validate_skill_structure,
    severity_for_structure,
)

REPO_ROOT = Path(__file__).resolve().parents[2]
SKILLS_ROOT = REPO_ROOT / "project_agents" / "skills"
REPORT_PATH = SKILLS_ROOT / "SKILL_DEPTH_GAP_REPORT.md"

P0_FOCUS = frozenset(
    {
        "ai_evidence_safety",
        "alert_quality_evidence",
        "bounded_implementation",
        "evidence_bundle",
        "migration_safety",
        "security_negative_testing",
    }
)

B3A_FOCUS = frozenset(
    {
        "ha_dr",
        "replay_idempotency",
        "runtime_failure_injection",
        "exact_head_merge_gate",
        "backup_restore",
        "secrets_handling",
    }
)

B3B_FOCUS = frozenset(
    {
        "source_truth_rebaseline",
        "mother_reference_reconciliation",
        "root_cause_first",
        "post_merge_rebaseline",
        "ci_cd",
        "install_upgrade",
        "docker_compose",
        "postgresql",
        "clickhouse",
        "capacity_performance",
    }
)

B3C_FOCUS = frozenset(
    {
        "api_event_contracts",
        "connector_framework",
        "device_control_protocols",
        "flow_telemetry",
        "snmp_mib",
        "syslog_traps",
        "topology_routing",
        "monitoring_work_units",
        "incident_flight_recorder",
        "remediation_safety",
    }
)

B3D_FOCUS = frozenset(
    {
        "agent_architecture",
        "change_intelligence",
        "cloud_monitoring",
        "device_pack_authoring",
        "icmp_path",
        "itsm_change_telemetry",
        "kubernetes_monitoring",
        "linux_systems",
        "virtualization_storage",
        "windows_monitoring",
    }
)

B3E_FOCUS = frozenset(
    {
        "accessibility_inclusive_design",
        "customer_value_mttr",
        "rca_hypothesis_fusion",
        "sbom_provenance",
        "temporal_operational_graph",
        "typescript_react",
        "ux_operational_design",
        "web_interface_quality",
    }
)

B4_FOCUS = frozenset(
    {
        "distributed_systems",
        "go_runtime",
        "java_spring",
        "kafka",
        "oauth_oidc_clients",
        "pki_mtls",
        "python_runtime",
        "sensor_edge",
        "tenant_isolation",
        "threat_modelling",
    }
)

B4_HEADING_NORMALIZATIONS = frozenset(
    {
        "ai_evidence_safety",
        "alert_quality_evidence",
        "bounded_implementation",
        "evidence_bundle",
        "migration_safety",
        "security_negative_testing",
    }
)


def scan_skill(text: str) -> tuple[list[str], list[str]]:
    """Compatibility wrapper: present/missing executable signals via strict parse."""
    result = validate_skill_structure(text)
    return result.present_signals, result.missing_signals


def priority(name: str, missing: list[str], lines: int, text: str | None = None) -> str:
    """Deterministic severity from strict structure (critical never hides as P2)."""
    if text is None:
        # Fallback path for older tests that only pass missing lists.
        from skill_structure import SkillStructureResult

        synthetic = SkillStructureResult(
            ok=not missing,
            missing_signals=list(missing),
            missing_headings=[],
            errors=[f"missing signal {m}" for m in missing] if missing else [],
        )
        return severity_for_structure(
            name,
            synthetic,
            operational_critical=OPERATIONAL_CRITICAL_SKILLS,
            line_count=lines,
        )
    structure = validate_skill_structure(text)
    return severity_for_structure(
        name,
        structure,
        operational_critical=OPERATIONAL_CRITICAL_SKILLS,
        line_count=lines,
    )


def _focus_status(rows: list[tuple[str, object, int]], names: frozenset[str]) -> list[str]:
    out: list[str] = []
    for name in sorted(names):
        match = next((r for r in rows if r[0] == name), None)
        if match is None:
            out.append(f"- `{name}`: **MISSING from manifest**")
            continue
        _name, structure, nlines = match
        pri = severity_for_structure(
            name,
            structure,
            operational_critical=OPERATIONAL_CRITICAL_SKILLS,
            line_count=nlines,
        )
        if structure.ok and pri == "OK":
            state = "STRUCTURAL_OK"
        else:
            detail = ", ".join(structure.errors) if structure.errors else ", ".join(structure.missing_signals)
            state = f"{pri} {detail}"
        out.append(f"- `{name}` ({nlines} lines): **{state}**")
    return out


def build_report(skills_root: Path = SKILLS_ROOT) -> tuple[str, dict[str, int]]:
    manifest = json.loads((skills_root / "manifest.json").read_text(encoding="utf-8"))
    rows: list[tuple[str, object, int]] = []
    for entry in manifest:
        name = entry["name"]
        text = (skills_root / name / "SKILL.md").read_text(encoding="utf-8")
        structure = validate_skill_structure(text)
        rows.append((name, structure, len(text.splitlines())))

    counts = {"OK": 0, "P0": 0, "P1": 0, "P2": 0, "STRUCTURAL_OK": 0}
    lines_out = [
        "# Skill procedural-depth gap report (Phase B4 strict structural)",
        "",
        "Generated by `python3 project_agents/tools/skill_depth_report.py`.",
        "",
        "B4 uses **strict Markdown H2 section parsing**. Whole-file prose keywords and "
        "YAML frontmatter do **not** satisfy required sections. Structural failures "
        "**fail** `validate_agents.py`.",
        "",
        f"- Canonical skills scanned: **{len(rows)}**",
        f"- Operationally critical Skills: **{len(OPERATIONAL_CRITICAL_SKILLS)}**",
        f"- Required H2 sections ({len(CANONICAL_HEADINGS)}): "
        + ", ".join(f"`## {h}`" for h in CANONICAL_HEADINGS),
        "",
        "| Priority | Structural | Skill | Lines | Missing/Errors |",
        "|---|---|---|---:|---|",
    ]
    sorted_rows = sorted(
        rows,
        key=lambda r: (
            {"P0": 0, "P1": 1, "P2": 2, "OK": 3}[
                severity_for_structure(
                    r[0], r[1], operational_critical=OPERATIONAL_CRITICAL_SKILLS, line_count=r[2]
                )
            ],
            r[0],
        ),
    )
    for name, structure, nlines in sorted_rows:
        pri = severity_for_structure(
            name,
            structure,
            operational_critical=OPERATIONAL_CRITICAL_SKILLS,
            line_count=nlines,
        )
        counts[pri] += 1
        if structure.ok:
            counts["STRUCTURAL_OK"] += 1
        critical_mark = " critical" if name in OPERATIONAL_CRITICAL_SKILLS else ""
        err = "; ".join(structure.errors) if structure.errors else "_none_"
        lines_out.append(
            f"| {pri} | {structure.structural_label} | `{name}`{critical_mark} | {nlines} | {err} |"
        )

    lines_out[7:7] = [
        f"- STRUCTURAL_OK: **{counts['STRUCTURAL_OK']}**",
        f"- Depth OK (strict structure + signals): **{counts['OK']}**",
        f"- P0: **{counts['P0']}**",
        f"- P1: **{counts['P1']}**",
        f"- P2: **{counts['P2']}**",
        "",
    ]

    lines_out += [
        "",
        "## Executable procedure signals",
        ", ".join(f"`{s}`" for s in sorted(EXECUTABLE_SIGNALS)),
        "",
        "## Severity rule for operationally critical Skills",
        "Skills in the operational-critical set failing structural/executable contract are "
        "classified **P0/P1**, never generic **P2**.",
        "",
        "## Issue #93 Phase B2 heading-normalization focus",
        *_focus_status(rows, B4_HEADING_NORMALIZATIONS),
        "",
        "## Issue #93 Phase B3A critical Skill focus",
        *_focus_status(rows, B3A_FOCUS),
        "",
        "## Issue #93 Phase B3B Skill-depth focus",
        *_focus_status(rows, B3B_FOCUS),
        "",
        "## Issue #93 Phase B3C Skill-depth focus",
        *_focus_status(rows, B3C_FOCUS),
        "",
        "## Issue #93 Phase B3D Skill-depth focus",
        *_focus_status(rows, B3D_FOCUS),
        "",
        "## Issue #93 Phase B3E Skill-depth focus",
        *_focus_status(rows, B3E_FOCUS),
        "",
        "## Issue #93 Phase B4 foundational Skill focus",
        *_focus_status(rows, B4_FOCUS),
        "",
        "## Phase B4 status",
        "Strict structural scanning is authoritative. Semantic quality remains independently audited.",
    ]
    return "\n".join(lines_out) + "\n", counts


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="exit 1 if on-disk report differs from recomputation (no write)",
    )
    parser.add_argument(
        "--write",
        action="store_true",
        help="write SKILL_DEPTH_GAP_REPORT.md (default when neither flag set)",
    )
    args = parser.parse_args(argv)
    report, counts = build_report()
    if args.check:
        existing = REPORT_PATH.read_text(encoding="utf-8") if REPORT_PATH.is_file() else ""
        if existing != report:
            print("SKILL_DEPTH_GAP_REPORT.md is stale; re-run without --check", file=sys.stderr)
            return 1
        print(
            f"depth report current STRUCTURAL_OK={counts['STRUCTURAL_OK']} "
            f"OK={counts['OK']} P0={counts['P0']} P1={counts['P1']} P2={counts['P2']}"
        )
        return 0
    REPORT_PATH.write_text(report, encoding="utf-8")
    print(
        f"wrote {REPORT_PATH.relative_to(REPO_ROOT)} "
        f"STRUCTURAL_OK={counts['STRUCTURAL_OK']} "
        f"OK={counts['OK']} P0={counts['P0']} P1={counts['P1']} P2={counts['P2']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
