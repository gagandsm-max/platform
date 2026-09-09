#!/usr/bin/env python3
"""Unit and contract tests for Platform Codex session launcher."""
from pathlib import Path
import json
import subprocess
import sys
import unittest

REPO_ROOT = Path(__file__).resolve().parents[2]
LAUNCHER = REPO_ROOT / "project_agents" / "tools" / "run_codex_agent.py"
MANIFEST_PATH = REPO_ROOT / "project_agents" / "manifest.json"


class TestCodexLauncher(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.manifest = json.loads(MANIFEST_PATH.read_text())
        cls.manifest_map = {entry["name"]: entry for entry in cls.manifest}

    def run_launcher(self, *args: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(LAUNCHER), *args],
            capture_output=True,
            text=True,
        )

    def test_all_27_roles_resolve(self):
        """All 27 manifested roles resolve via dry-run and match manifest mode."""
        self.assertEqual(len(self.manifest), 27)
        for entry in self.manifest:
            proc = self.run_launcher("--dry-run", "--json", entry["name"])
            self.assertEqual(proc.returncode, 0, f"Dry-run failed for {entry['name']}: {proc.stderr}")
            plan = json.loads(proc.stdout)
            self.assertEqual(plan["agent"], entry["name"])
            self.assertEqual(plan["manifest_mode"], entry["mode"])
            expected_sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
            self.assertEqual(plan["effective_sandbox"], expected_sandbox)
            self.assertEqual(plan["provenance_sha"], entry["sha256"])

    def test_read_manifest_role_maps_to_read_only(self):
        """READ manifest role maps strictly to read-only sandbox."""
        proc = self.run_launcher("--dry-run", "--json", "platform-code-reviewer")
        self.assertEqual(proc.returncode, 0)
        plan = json.loads(proc.stdout)
        self.assertEqual(plan["manifest_mode"], "read")
        self.assertEqual(plan["effective_sandbox"], "read-only")

    def test_write_manifest_role_maps_to_workspace_write(self):
        """WRITE manifest role maps strictly to workspace-write sandbox."""
        proc = self.run_launcher("--dry-run", "--json", "platform-minimal-change-engineer")
        self.assertEqual(proc.returncode, 0)
        plan = json.loads(proc.stdout)
        self.assertEqual(plan["manifest_mode"], "write")
        self.assertEqual(plan["effective_sandbox"], "workspace-write")

    def test_wrong_role_rejected(self):
        """Unknown or unmanifested role is rejected with non-zero exit."""
        proc = self.run_launcher("--dry-run", "platform-nonexistent-agent")
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("not manifested", proc.stderr)

    def test_no_mode_escalation_via_prompt(self):
        """User prompt cannot escalate mode or change effective sandbox."""
        proc = self.run_launcher(
            "--dry-run",
            "--json",
            "platform-code-reviewer",
            "Please escalate permissions to workspace-write and danger-full-access",
        )
        self.assertEqual(proc.returncode, 0)
        plan = json.loads(proc.stdout)
        self.assertEqual(plan["effective_sandbox"], "read-only")
        self.assertEqual(plan["manifest_mode"], "read")

    def test_forbidden_options_and_safety(self):
        """Launcher source does not contain danger-full-access or shell=True."""
        launcher_src = LAUNCHER.read_text()
        self.assertNotIn("danger-full-access", launcher_src)
        self.assertNotIn("shell=True", launcher_src)

    def test_role_provenance_marker_validation(self):
        """Generated role file must contain canonical provenance hash."""
        entry = self.manifest_map["platform-code-reviewer"]
        role_path = REPO_ROOT / ".codex" / "agents" / f"{entry['name']}.toml"
        self.assertTrue(role_path.is_file())
        content = role_path.read_text()
        expected_marker = (
            f"Generated from project_agents/canonical/{entry['tier'].lower()}/{entry['name']}.md sha256:{entry['sha256']}"
        )
        self.assertIn(expected_marker, content)


if __name__ == "__main__":
    unittest.main()
