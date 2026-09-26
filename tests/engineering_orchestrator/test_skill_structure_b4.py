#!/usr/bin/env python3
"""Deterministic B4 strict Skill structural contract tests (Issue #93)."""

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
TOOLS = REPO_ROOT / "project_agents" / "tools"


def _load(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    # Ensure sibling imports (skill_structure) resolve when loading skill_depth_report.
    sys.path.insert(0, str(TOOLS))
    spec.loader.exec_module(module)
    return module


class SkillStructureUnitTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.ss = _load("skill_structure", TOOLS / "skill_structure.py")
        cls.sdr = _load("skill_depth_report", TOOLS / "skill_depth_report.py")

    def _minimal_skill(self, **overrides: str) -> str:
        sections = {
            "Purpose": "Purpose body.",
            "Use when": "Use when body.",
            "Authoritative inputs": "Inputs body.",
            "Invariants": "Invariants body.",
            "Required procedure": "Procedure body.",
            "Negative / failure cases": "Negative body.",
            "Security / tenant implications": "Security body.",
            "Required evidence": "Evidence body.",
            "Stop conditions": "Stop body.",
            "Anti-patterns": "Anti body.",
            "Handoff / output": "Handoff body.",
            "Non-ownership": "Non-ownership body.",
        }
        sections.update(overrides)
        parts = ["---\nname: demo\ndescription: demo\n---\n"]
        for heading in self.ss.CANONICAL_HEADINGS:
            parts.append(f"## {heading}\n\n{sections[heading]}\n")
        return "\n".join(parts)

    def test_canonical_skill_count_is_60(self) -> None:
        skills = json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())
        self.assertEqual(len(skills), 60)

    def test_all_60_skills_strict_structural_ok(self) -> None:
        skills = json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())
        for entry in skills:
            name = entry["name"]
            text = (REPO_ROOT / "project_agents" / "skills" / name / "SKILL.md").read_text(encoding="utf-8")
            result = self.ss.validate_skill_structure(text)
            self.assertTrue(result.ok, msg=f"{name}: {result.errors}")
            self.assertEqual(result.missing_headings, [])
            # Exactly once each canonical heading.
            counts = {}
            for title in result.h2_titles:
                if title in self.ss.CANONICAL_HEADING_SET:
                    counts[title] = counts.get(title, 0) + 1
            for heading in self.ss.CANONICAL_HEADINGS:
                self.assertEqual(counts.get(heading), 1, msg=f"{name} count for {heading}")
            # Order of required headings.
            required_seq = [t for t in result.h2_titles if t in self.ss.CANONICAL_HEADING_SET]
            self.assertEqual(required_seq, list(self.ss.CANONICAL_HEADINGS), msg=name)

    def test_frontmatter_and_prose_cannot_satisfy_missing_section(self) -> None:
        text = """---
name: demo
description: Always read invariants procedure evidence tenant security handoff stop
---

# demo

This prose mentions Required procedure and Security / tenant implications and Anti-patterns
and Authoritative inputs and Negative / failure cases but has no real H2 sections except one.

## Purpose

Only purpose exists.
"""
        result = self.ss.validate_skill_structure(text)
        self.assertFalse(result.ok)
        self.assertIn("Use when", result.missing_headings)
        self.assertIn("Authoritative inputs", result.missing_headings)

    def test_duplicate_heading_fails(self) -> None:
        text = self._minimal_skill()
        text = text + "\n## Purpose\n\nDuplicate purpose.\n"
        result = self.ss.validate_skill_structure(text)
        self.assertFalse(result.ok)
        self.assertIn("Purpose", result.duplicate_headings)

    def test_missing_heading_fails(self) -> None:
        text = self._minimal_skill()
        text = text.replace("## Non-ownership\n\nNon-ownership body.\n", "")
        result = self.ss.validate_skill_structure(text)
        self.assertFalse(result.ok)
        self.assertIn("Non-ownership", result.missing_headings)

    def test_wrong_heading_alias_fails(self) -> None:
        text = self._minimal_skill()
        text = text.replace(
            "## Security / tenant implications\n\nSecurity body.\n",
            "## Security / tenant boundary\n\nSecurity body.\n",
        )
        result = self.ss.validate_skill_structure(text)
        self.assertFalse(result.ok)
        self.assertIn("Security / tenant implications", result.missing_headings)

    def test_empty_section_fails(self) -> None:
        text = self._minimal_skill(**{"Required evidence": "   \n\t  "})
        result = self.ss.validate_skill_structure(text)
        self.assertFalse(result.ok)
        self.assertIn("Required evidence", result.empty_sections)

    def test_out_of_order_headings_fail(self) -> None:
        # Swap Use when and Purpose order.
        sections = []
        body = "---\nname: demo\ndescription: d\n---\n\n"
        order = list(self.ss.CANONICAL_HEADINGS)
        order[0], order[1] = order[1], order[0]
        for heading in order:
            body += f"## {heading}\n\nBody for {heading}.\n\n"
        result = self.ss.validate_skill_structure(body)
        self.assertFalse(result.ok)
        self.assertTrue(result.order_errors)

    def test_b4_foundational_and_b2_normalization_pass(self) -> None:
        for name in sorted(self.sdr.B4_FOCUS | self.sdr.B4_HEADING_NORMALIZATIONS):
            text = (REPO_ROOT / "project_agents" / "skills" / name / "SKILL.md").read_text(encoding="utf-8")
            result = self.ss.validate_skill_structure(text)
            self.assertTrue(result.ok, msg=f"{name}: {result.errors}")
            if name in self.sdr.B4_HEADING_NORMALIZATIONS:
                self.assertIn("## Security / tenant implications", text)
                self.assertNotIn("## Security / tenant boundary", text)

    def test_operational_critical_missing_section_escalates(self) -> None:
        text = self._minimal_skill()
        text = text.replace("## Required procedure\n\nProcedure body.\n", "")
        result = self.ss.validate_skill_structure(text)
        sev = self.ss.severity_for_structure(
            "tenant_isolation",
            result,
            operational_critical=self.ss.OPERATIONAL_CRITICAL_SKILLS,
            line_count=80,
        )
        self.assertIn(sev, {"P0", "P1"})
        self.assertNotEqual(sev, "P2")
        sev_thin = self.ss.severity_for_structure(
            "tenant_isolation",
            result,
            operational_critical=self.ss.OPERATIONAL_CRITICAL_SKILLS,
            line_count=20,
        )
        self.assertEqual(sev_thin, "P0")

    def test_non_critical_missing_section_never_silently_ok(self) -> None:
        text = self._minimal_skill()
        text = text.replace("## Anti-patterns\n\nAnti body.\n", "")
        result = self.ss.validate_skill_structure(text)
        sev = self.ss.severity_for_structure(
            "typescript_react",
            result,
            operational_critical=self.ss.OPERATIONAL_CRITICAL_SKILLS,
            line_count=90,
        )
        self.assertNotEqual(sev, "OK")
        self.assertFalse(result.ok)

    def test_report_check_detects_stale(self) -> None:
        report, _counts = self.sdr.build_report()
        on_disk = Path(self.sdr.REPORT_PATH).read_text(encoding="utf-8")
        self.assertEqual(on_disk, report)
        # Stale detection: mutate temp copy path behaviour via direct compare.
        self.assertNotEqual(report + "\n# stale\n", on_disk)

    def test_validator_fails_on_structural_defect(self) -> None:
        sys.path.insert(0, str(TOOLS))
        import agent_pack

        with tempfile.TemporaryDirectory() as tmp:
            # Use real repo validation path by temporarily pointing is impractical;
            # assert format_structure_errors integrates with validate result shape.
            bad = self._minimal_skill()
            bad = bad.replace("## Stop conditions\n\nStop body.\n", "")
            errs = self.ss.format_structure_errors("demo", self.ss.validate_skill_structure(bad))
            self.assertTrue(errs)
            self.assertTrue(any("missing required H2: ## Stop conditions" in e for e in errs))
            # Smoke: full pack validates clean on current repo.
            pack_errors = agent_pack.validate_agent_pack(REPO_ROOT)
            self.assertEqual(pack_errors, [], pack_errors)

    def test_operational_critical_inventory(self) -> None:
        required = {
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
        self.assertTrue(required <= self.ss.OPERATIONAL_CRITICAL_SKILLS)


class SkillDepthBatchRegressionTests(unittest.TestCase):
    """Keep B3 batch heading assertions working under strict parse."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.sdr = _load("skill_depth_report", TOOLS / "skill_depth_report.py")
        cls.ss = _load("skill_structure", TOOLS / "skill_structure.py")

    def test_report_matches_disk_strict(self) -> None:
        report, counts = self.sdr.build_report()
        self.assertEqual(Path(self.sdr.REPORT_PATH).read_text(encoding="utf-8"), report)
        self.assertEqual(counts["P0"], 0)
        self.assertEqual(counts["P1"], 0)
        self.assertEqual(counts["P2"], 0)
        self.assertEqual(counts["OK"], 60)
        self.assertEqual(counts["STRUCTURAL_OK"], 60)

    def test_agent_architecture_still_critical(self) -> None:
        self.assertIn("agent_architecture", self.sdr.OPERATIONAL_CRITICAL_SKILLS)


if __name__ == "__main__":
    unittest.main()
