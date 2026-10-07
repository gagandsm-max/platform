#!/usr/bin/env python3
"""Deterministic Skill-depth contract tests for Issue #93 B3 batches."""

from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
REPORT_TOOL = REPO_ROOT / "project_agents" / "tools" / "skill_depth_report.py"
TOOLS = REPO_ROOT / "project_agents" / "tools"

EXPLICIT_HEADINGS = (
    "## Purpose",
    "## Use when",
    "## Authoritative inputs",
    "## Invariants",
    "## Required procedure",
    "## Negative / failure cases",
    "## Security / tenant implications",
    "## Required evidence",
    "## Stop conditions",
    "## Anti-patterns",
    "## Handoff / output",
    "## Non-ownership",
)


def _load_depth_report():
    import sys

    if str(TOOLS) not in sys.path:
        sys.path.insert(0, str(TOOLS))
    spec = importlib.util.spec_from_file_location("skill_depth_report", REPORT_TOOL)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class SkillDepthBatchTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.sdr = _load_depth_report()

    def _assert_batch_depth_complete(self, focus: frozenset[str]) -> None:
        for name in sorted(focus):
            path = REPO_ROOT / "project_agents" / "skills" / name / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            present, missing = self.sdr.scan_skill(text)
            lines = len(text.splitlines())
            self.assertEqual(missing, [], msg=f"{name} missing {missing}")
            self.assertEqual(self.sdr.priority(name, missing, lines), "OK", msg=name)
            for signal in self.sdr.EXECUTABLE_SIGNALS:
                self.assertIn(signal, present, msg=f"{name}:{signal}")
            for heading in EXPLICIT_HEADINGS:
                self.assertIn(heading, text, msg=f"{name} lacks explicit {heading}")

    def test_report_matches_disk(self) -> None:
        report, counts = self.sdr.build_report()
        on_disk = Path(self.sdr.REPORT_PATH).read_text(encoding="utf-8")
        self.assertEqual(on_disk, report)
        self.assertEqual(counts["P0"], 0)
        self.assertEqual(counts["P1"], 0)
        self.assertEqual(counts["P2"], 0)
        self.assertEqual(counts["OK"], 63)

    def test_agent_architecture_is_operational_critical(self) -> None:
        self.assertIn("agent_architecture", self.sdr.OPERATIONAL_CRITICAL_SKILLS)

    def test_b3d_skills_are_depth_complete(self) -> None:
        self._assert_batch_depth_complete(self.sdr.B3D_FOCUS)

    def test_b3e_skills_are_depth_complete(self) -> None:
        self._assert_batch_depth_complete(self.sdr.B3E_FOCUS)

    def test_incomplete_agent_architecture_would_not_hide_as_p2(self) -> None:
        # Severity guard: critical Skills with missing executable signals escalate.
        self.assertEqual(
            self.sdr.priority("agent_architecture", ["negative_failure"], 80),
            "P1",
        )
        self.assertEqual(
            self.sdr.priority("agent_architecture", ["negative_failure"], 32),
            "P0",
        )
        self.assertEqual(
            self.sdr.priority("agent_architecture", ["procedure"], 80),
            "P0",
        )
        self.assertNotEqual(
            self.sdr.priority("agent_architecture", ["negative_failure"], 80),
            "P2",
        )

    def test_projection_counts(self) -> None:
        agents = json.loads((REPO_ROOT / "project_agents" / "manifest.json").read_text(encoding="utf-8"))
        skills = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text(encoding="utf-8")
        )
        skill_names = {entry["name"] for entry in skills}
        self.assertEqual(len(agents), 34)
        self.assertEqual(len(skill_names), 63)
        for root in (".cursor/agents", ".agents/agents", ".codex/agents"):
            files = [p for p in (REPO_ROOT / root).iterdir() if p.is_file()]
            self.assertEqual(len(files), 34, msg=root)
        for root in (".agents/skills", "skills"):
            mirrored = [
                p
                for p in (REPO_ROOT / root).iterdir()
                if p.is_dir() and p.name in skill_names and (p / "SKILL.md").is_file()
            ]
            self.assertEqual(len(mirrored), 63, msg=root)


if __name__ == "__main__":
    unittest.main()
