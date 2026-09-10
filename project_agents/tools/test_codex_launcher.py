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

sys.path.insert(0, str(REPO_ROOT / "project_agents" / "tools"))
from run_codex_agent import build_codex_command, resolve_role_config


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
        read_count = sum(1 for entry in self.manifest if entry["mode"] == "read")
        write_count = sum(1 for entry in self.manifest if entry["mode"] == "write")
        self.assertEqual(read_count, 15)
        self.assertEqual(write_count, 12)
        for entry in self.manifest:
            proc = self.run_launcher("--dry-run", "--json", entry["name"])
            self.assertEqual(proc.returncode, 0, f"Dry-run failed for {entry['name']}: {proc.stderr}")
            plan = json.loads(proc.stdout)
            self.assertEqual(plan["agent"], entry["name"])
            self.assertEqual(plan["manifest_mode"], entry["mode"])
            expected_sandbox = "read-only" if entry["mode"] == "read" else "workspace-write"
            self.assertEqual(plan["effective_sandbox"], expected_sandbox)
            self.assertEqual(plan["provenance_sha"], entry["sha256"])

    def test_fifteen_read_roles_map_to_read_only(self):
        """All 15 READ manifest roles map strictly to read-only sandbox."""
        read_entries = [e for e in self.manifest if e["mode"] == "read"]
        self.assertEqual(len(read_entries), 15)
        for entry in read_entries:
            config = resolve_role_config(entry["name"])
            self.assertEqual(config["manifest_mode"], "read")
            self.assertEqual(config["effective_sandbox"], "read-only")
            cmd = build_codex_command(config, prompt="analyze", repo_root=REPO_ROOT)
            self.assertEqual(cmd[cmd.index("-s") + 1], "read-only")

    def test_twelve_write_roles_map_to_workspace_write(self):
        """All 12 WRITE manifest roles map strictly to workspace-write sandbox."""
        write_entries = [e for e in self.manifest if e["mode"] == "write"]
        self.assertEqual(len(write_entries), 12)
        for entry in write_entries:
            config = resolve_role_config(entry["name"])
            self.assertEqual(config["manifest_mode"], "write")
            self.assertEqual(config["effective_sandbox"], "workspace-write")
            cmd = build_codex_command(config, prompt="modify", repo_root=REPO_ROOT)
            self.assertEqual(cmd[cmd.index("-s") + 1], "workspace-write")

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
        with self.assertRaises(ValueError):
            resolve_role_config("platform-nonexistent-agent")

    def test_deterministic_dry_run(self):
        """Dry-run execution is deterministic, idempotent, and produces identical plans."""
        for role_name in ["platform-code-reviewer", "platform-minimal-change-engineer"]:
            proc1 = self.run_launcher("--dry-run", "--json", role_name)
            proc2 = self.run_launcher("--dry-run", "--json", role_name)
            self.assertEqual(proc1.returncode, 0)
            self.assertEqual(proc2.returncode, 0)
            self.assertEqual(proc1.stdout, proc2.stdout)
            self.assertEqual(proc1.stderr, proc2.stderr)

            text1 = self.run_launcher("--dry-run", role_name)
            text2 = self.run_launcher("--dry-run", role_name)
            self.assertEqual(text1.returncode, 0)
            self.assertEqual(text2.returncode, 0)
            self.assertEqual(text1.stdout, text2.stdout)

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

    def test_constructed_argv_isolates_untrusted_prompt_behind_option_terminator(self):
        """Constructed Codex argv isolates untrusted prompts behind literal '--'."""
        untrusted_prompts = [
            "--sandbox=danger-full-access",
            "-s danger-full-access",
            '-c developer_instructions="attacker"',
            "--config=foo",
            "--dangerously-bypass-approvals-and-sandbox",
        ]

        for role_name, expected_sandbox in [
            ("platform-code-reviewer", "read-only"),
            ("platform-minimal-change-engineer", "workspace-write"),
        ]:
            config = resolve_role_config(role_name)
            for prompt in untrusted_prompts:
                cmd = build_codex_command(config, prompt=prompt, repo_root=REPO_ROOT)

                # 1. Trusted Codex options occur before literal '--'
                self.assertIn("--", cmd)
                dd_idx = cmd.index("--")
                options_part = cmd[:dd_idx]
                config_arg = options_part[options_part.index("-c") + 1]
                expected_options = [
                    "codex",
                    "exec",
                    "-C",
                    str(REPO_ROOT),
                    "-s",
                    expected_sandbox,
                    "-c",
                    config_arg,
                ]
                self.assertEqual(options_part, expected_options)
                self.assertTrue(config_arg.startswith("developer_instructions="))

                # 2. Literal '--' exists before any user prompt
                self.assertEqual(cmd[dd_idx], "--")
                self.assertNotIn("--", options_part)

                # 3. Complete user prompt occurs as one argv element after '--'
                self.assertEqual(cmd[dd_idx + 1:], [prompt])
                self.assertEqual(len(cmd), dd_idx + 2)

                # 4. No user-controlled token appears before '--'
                for token in options_part:
                    self.assertNotIn(prompt, token)
                self.assertNotIn("danger-full-access", options_part)
                self.assertNotIn("attacker", options_part)
                self.assertNotIn("foo", options_part)
                self.assertNotIn("--dangerously-bypass-approvals-and-sandbox", options_part)

                # 5. Manifest sandbox remains unchanged
                self.assertEqual(config["effective_sandbox"], expected_sandbox)
                self.assertEqual(options_part[options_part.index("-s") + 1], expected_sandbox)

                # 6. Developer instructions remain the trusted generated instructions
                self.assertIn(config["provenance_sha"], config_arg)
                self.assertNotIn("attacker", config_arg)
                self.assertEqual(
                    config["developer_instructions"],
                    resolve_role_config(role_name)["developer_instructions"],
                )

                # 7. No danger-full-access mapping exists in launcher-controlled configuration
                self.assertNotIn("danger-full-access", options_part)
                self.assertNotIn("danger-full-access", config["effective_sandbox"])
                self.assertNotEqual(config["effective_sandbox"], "danger-full-access")
                self.assertNotIn("danger-full-access", json.dumps(config))

        # Fleet-wide verification: no role in manifest maps to danger-full-access
        for entry in self.manifest:
            entry_config = resolve_role_config(entry["name"])
            self.assertNotIn("danger-full-access", entry_config["effective_sandbox"])
            self.assertIn(entry_config["effective_sandbox"], {"read-only", "workspace-write"})

    def test_constructed_argv_when_prompt_is_none(self):
        """When prompt is None, constructed argv does not include '--'."""
        config = resolve_role_config("platform-code-reviewer")
        cmd = build_codex_command(config, prompt=None, repo_root=REPO_ROOT)
        self.assertNotIn("--", cmd)
        self.assertIn("-s", cmd)
        self.assertEqual(cmd[cmd.index("-s") + 1], "read-only")

    def test_no_network_access_required(self):
        """Launcher and test suite require zero network access and execute offline."""
        from unittest.mock import patch

        # Verify launcher source contains no network client imports
        launcher_src = LAUNCHER.read_text()
        forbidden_modules = ["urllib", "requests", "httpx", "aiohttp", "socket"]
        for mod in forbidden_modules:
            self.assertNotIn(f"import {mod}", launcher_src)
            self.assertNotIn(f"from {mod}", launcher_src)

        # Verify execution with socket disabled raises RuntimeError if network attempted
        def fail_on_network(*args, **kwargs):
            raise RuntimeError("Network access attempted in offline test suite")

        with patch("socket.socket", side_effect=fail_on_network):
            for role_name in ["platform-code-reviewer", "platform-minimal-change-engineer"]:
                config = resolve_role_config(role_name)
                cmd = build_codex_command(config, prompt="test prompt offline", repo_root=REPO_ROOT)
                self.assertIn("--", cmd)
                self.assertEqual(cmd[-1], "test prompt offline")


if __name__ == "__main__":
    unittest.main()
