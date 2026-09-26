#!/usr/bin/env python3
"""Deterministic Skill procedural-depth gap report (informational).

Recomputes project_agents/skills/SKILL_DEPTH_GAP_REPORT.md from canonical
SKILL.md bodies. Does not fail validate_agents.py. Do not hand-edit the report
into a false PASS — regenerate with this script after Skill changes.

Severity is not heading-presence alone for operationally critical Skills:
incompleteness that can affect security, durability, HA/recovery, migration or
certification is escalated above generic P2.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SKILLS_ROOT = REPO_ROOT / "project_agents" / "skills"
REPORT_PATH = SKILLS_ROOT / "SKILL_DEPTH_GAP_REPORT.md"

# Heuristic regexes matching the Phase B2 audit used at control-plane hardening.
SECTIONS: dict[str, list[str]] = {
    "trigger": [
        r"(?i)^##\s+use when\b",
        r"(?i)^##\s+trigger",
        r"(?i)\btypical triggers\b",
        r"(?i)^##\s+when to use",
        r"(?i)^##\s+purpose\b",
    ],
    "authoritative_inputs": [
        r"(?i)always read",
        r"(?i)authoritative",
        r"(?i)required reading",
        r"(?i)^##\s+inputs",
        r"(?i)mother reference",
        r"(?i)governing",
        r"(?i)^##\s+authoritative inputs",
    ],
    "invariants": [
        r"(?i)^##\s+guardrails",
        r"(?i)\binvariant",
        r"(?i)non-negotiable",
        r"(?i)^##\s+invariants",
    ],
    "procedure": [
        r"(?i)^##\s+required procedure",
        r"(?i)^##\s+procedure",
        r"(?i)^##\s+workflow",
        r"(?i)^##\s+steps",
    ],
    "negative_failure": [
        r"(?i)fail[- ]closed",
        r"(?i)negative",
        r"(?i)^##\s+failure",
        r"(?i)^##\s+negative",
        r"(?i)reject\b",
        r"(?i)do not weaken",
    ],
    "security_tenancy": [
        r"(?i)\btenant\b",
        r"(?i)\brbac\b",
        r"(?i)\bsecret",
        r"(?i)\bsecurity\b",
        r"(?i)no secret",
        r"(?i)^##\s+security",
    ],
    "evidence": [
        r"(?i)^##\s+required evidence",
        r"(?i)\bevidence\b",
        r"(?i)\bunverified\b",
        r"(?i)source_fact",
    ],
    "stop_conditions": [
        r"(?i)^##\s+non-ownership",
        r"(?i)out of scope",
        r"(?i)non-goal",
        r"(?i)^##\s+stop",
        r"(?i)explicit non",
    ],
    "anti_patterns": [
        r"(?i)anti-pattern",
        r"(?i)^##\s+anti-patterns",
        r"(?i)do not\b",
        r"(?i)never\b",
        r"(?i)forbidden",
        r"(?i)cannot",
    ],
    "handoff": [
        r"(?i)\bhandoff\b",
        r"(?i)next action",
        r"(?i)return through",
        r"(?i)^##\s+handoff",
        r"(?i)cannot self-certify",
    ],
}

# Signals whose absence breaks executable procedure for any Skill.
CRITICAL_SIGNALS = frozenset({"trigger", "invariants", "procedure", "evidence"})

# Executable procedure contract expected of operationally critical Skills.
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

# Skills whose incompleteness can directly affect security, durability/data
# integrity, HA/recovery, migration or certification. Missing executable signals
# on these Skills must not be classified as generic P2.
OPERATIONAL_CRITICAL_SKILLS = frozenset(
    {
        # security
        "secrets_handling",
        "security_negative_testing",
        "tenant_isolation",
        "pki_mtls",
        "ai_evidence_safety",
        # durability / data integrity
        "replay_idempotency",
        "runtime_failure_injection",
        "evidence_bundle",
        # HA / recovery
        "ha_dr",
        "backup_restore",
        # migration
        "migration_safety",
        # certification / bounded change
        "exact_head_merge_gate",
        "alert_quality_evidence",
        "bounded_implementation",
    }
)

# Historical Issue #93 Phase B2 P0 focus (kept for continuity in the report).
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

# Phase B3A critical batch named by Master/Antigravity operational priority.
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

# Phase B3B governance/runtime batch (Issue #93 controlled Skill-depth).
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

# Phase B3C monitoring/protocol/integration batch (Issue #93 controlled Skill-depth).
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


def priority(name: str, missing: list[str], lines: int) -> str:
    """Deterministic severity. Critical Skills never hide as P2 when incomplete."""
    missing_set = set(missing)
    miss_crit = CRITICAL_SIGNALS & missing_set
    miss_exec = EXECUTABLE_SIGNALS & missing_set
    critical_skill = name in OPERATIONAL_CRITICAL_SKILLS

    if not missing_set:
        return "OK"

    if critical_skill:
        # Any missing executable/critical signal on a critical Skill is P0/P1.
        if miss_crit or miss_exec:
            if miss_crit or lines < 40:
                return "P0"
            return "P1"
        # Should be unreachable while EXECUTABLE_SIGNALS is the full set; keep safe.
        return "P1"

    if miss_crit and lines < 28:
        return "P0"
    if miss_crit:
        return "P1"
    return "P2"


def scan_skill(text: str) -> tuple[list[str], list[str]]:
    present: list[str] = []
    missing: list[str] = []
    for key, pats in SECTIONS.items():
        if any(re.search(p, text, re.M) for p in pats):
            present.append(key)
        else:
            missing.append(key)
    return present, missing


def _focus_status(
    rows: list[tuple[str, list[str], list[str], int]], names: frozenset[str]
) -> list[str]:
    out: list[str] = []
    for name in sorted(names):
        match = next((r for r in rows if r[0] == name), None)
        if match is None:
            out.append(f"- `{name}`: **MISSING from manifest**")
            continue
        _, _present, missing, nlines = match
        pri = priority(name, missing, nlines)
        state = (
            "depth-complete"
            if pri == "OK"
            else f"{pri} missing={', '.join(missing) or '_none_'}"
        )
        out.append(f"- `{name}` ({nlines} lines): **{state}**")
    return out


def build_report(skills_root: Path = SKILLS_ROOT) -> tuple[str, dict[str, int]]:
    manifest = json.loads((skills_root / "manifest.json").read_text(encoding="utf-8"))
    rows: list[tuple[str, list[str], list[str], int]] = []
    for entry in manifest:
        name = entry["name"]
        text = (skills_root / name / "SKILL.md").read_text(encoding="utf-8")
        present, missing = scan_skill(text)
        rows.append((name, present, missing, len(text.splitlines())))

    counts = {"OK": 0, "P0": 0, "P1": 0, "P2": 0}
    lines_out = [
        "# Skill procedural-depth gap report (Phase B2/B3)",
        "",
        "Generated by `python3 project_agents/tools/skill_depth_report.py`.",
        "",
        "This report is **informational**. It does **not** fail `validate_agents.py` in this tranche.",
        "Do **not** mass-template generic filler to clear this list. Author real procedures in Phase B2/B3.",
        "",
        f"- Canonical skills scanned: **{len(rows)}**",
        f"- Operationally critical Skills (security/durability/HA/migration/certification): "
        f"**{len(OPERATIONAL_CRITICAL_SKILLS)}**",
        "",
        "| Priority | Skill | Lines | Present | Missing |",
        "|---|---|---:|---|---|",
    ]
    sorted_rows = sorted(
        rows,
        key=lambda r: (
            {"P0": 0, "P1": 1, "P2": 2, "OK": 3}[priority(r[0], r[2], r[3])],
            r[0],
        ),
    )
    for name, present, missing, nlines in sorted_rows:
        pri = priority(name, missing, nlines)
        counts[pri] += 1
        critical_mark = " critical" if name in OPERATIONAL_CRITICAL_SKILLS else ""
        lines_out.append(
            f"| {pri} | `{name}`{critical_mark} | {nlines} | "
            f"{', '.join(present) if present else '_none_'} | "
            f"{', '.join(missing) if missing else '_none_'} |"
        )

    lines_out[7:7] = [
        f"- Depth OK (all 10 signals detected): **{counts['OK']}**",
        f"- P0 (thin/critical missing executable signals): **{counts['P0']}**",
        f"- P1 (missing critical/executable signals): **{counts['P1']}**",
        f"- P2 (non-critical Skills missing non-critical signals only): **{counts['P2']}**",
        "",
    ]

    lines_out += [
        "",
        "## Critical signals",
        "`trigger`/`purpose`, `invariants`/`guardrails`, `procedure`, `evidence`",
        "",
        "## Executable procedure signals",
        ", ".join(f"`{s}`" for s in sorted(EXECUTABLE_SIGNALS)),
        "",
        "## Severity rule for operationally critical Skills",
        "Skills in the operational-critical set missing any executable procedure signal are "
        "classified **P0/P1**, never generic **P2**.",
        "",
        "## Issue #93 Phase B2 P0 Skill focus",
        *_focus_status(rows, P0_FOCUS),
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
        "## Phase B3 recommendation",
        "Continue remaining non-critical P2 Skills in controlled domain batches—"
        "author real procedures, not cosmetic headings.",
        "",
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
            f"depth report current OK={counts['OK']} P0={counts['P0']} "
            f"P1={counts['P1']} P2={counts['P2']}"
        )
        return 0
    REPORT_PATH.write_text(report, encoding="utf-8")
    print(
        f"wrote {REPORT_PATH.relative_to(REPO_ROOT)} "
        f"OK={counts['OK']} P0={counts['P0']} P1={counts['P1']} P2={counts['P2']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
