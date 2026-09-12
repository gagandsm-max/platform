"""Capability Pack v1.2 inventory, projection, routing and pack-safety tests."""

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
TOOLS = REPO_ROOT / "project_agents" / "tools"
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

import agent_pack  # noqa: E402
import control_plane  # noqa: E402

NEW_P1 = (
    "platform-sensor-runtime-engineer",
    "platform-infrastructure-cloud-engineer",
    "platform-telemetry-pipeline-engineer",
    "platform-device-pack-engineer",
    "platform-ncm-remediation-engineer",
    "platform-integration-connector-engineer",
)


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def _build_fixture(root: Path) -> None:
    canonical = "# Demo Agent\n\nFixture canonical agent.\n"
    skill = (
        "---\n"
        "name: demo_skill\n"
        "description: Fixture skill\n"
        "---\n\n"
        "# demo_skill\n\nFixture canonical skill.\n"
    )
    manifest = [
        {
            "name": "demo-agent",
            "title": "Demo Agent",
            "tier": "P0",
            "mode": "read",
            "description": "Fixture agent",
            "canonical_path": "canonical/p0/demo-agent.md",
            "sha256": "0" * 64,
        }
    ]
    skills = [{"name": "demo_skill", "description": "Fixture skill"}]
    _write(root / "project_agents" / "manifest.json", json.dumps(manifest, indent=2) + "\n")
    _write(root / "project_agents" / "canonical" / "p0" / "demo-agent.md", canonical)
    _write(root / "project_agents" / "skills" / "manifest.json", json.dumps(skills, indent=2) + "\n")
    _write(root / "project_agents" / "skills" / "demo_skill" / "SKILL.md", skill)
    os.symlink("project_agents/manifest.json", root / "AGENTS.md")


def _manifest() -> list[dict]:
    return json.loads((REPO_ROOT / "project_agents" / "manifest.json").read_text(encoding="utf-8"))


def _skill_manifest() -> list[dict]:
    return json.loads(
        (REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text(encoding="utf-8")
    )


def _matrix() -> dict:
    return json.loads(
        (REPO_ROOT / "project_agents" / "skills" / "agent_skill_matrix.json").read_text(
            encoding="utf-8"
        )
    )


class CapabilityPackInventoryTests(unittest.TestCase):
    def test_v12_manifest_snapshot_is_manifest_derived(self) -> None:
        manifest = _manifest()
        names = [entry["name"] for entry in manifest]
        self.assertEqual(len(names), len(set(names)))
        self.assertEqual(len(manifest), 33)
        self.assertEqual(sum(1 for entry in manifest if entry["tier"] == "P0"), 12)
        self.assertEqual(sum(1 for entry in manifest if entry["tier"] == "P1"), 21)
        self.assertEqual(len(control_plane.load_agent_names()), len(manifest))
        for name in NEW_P1:
            self.assertIn(name, names)
            control_plane.validate_selected_agents([name])

    def test_validators_do_not_hardcode_v12_counts(self) -> None:
        source = (TOOLS / "agent_pack.py").read_text(encoding="utf-8") + (
            TOOLS / "validate_agents.py"
        ).read_text(encoding="utf-8")
        self.assertNotIn("len(manifest) == 33", source)
        self.assertNotIn("expected_agent_count = 33", source)

    def test_every_agent_has_bounded_required_skills(self) -> None:
        manifest_names = {entry["name"] for entry in _manifest()}
        skill_names = {entry["name"] for entry in _skill_manifest()}
        matrix = _matrix()
        self.assertEqual(set(matrix), manifest_names)
        required_sizes = []
        for agent_name, spec in matrix.items():
            required = spec["requiredSkills"]
            self.assertTrue(required, agent_name)
            required_sizes.append(len(required))
            for skill_name in required + spec.get("conditionalSkills", []):
                self.assertIn(skill_name, skill_names, f"{agent_name}->{skill_name}")
        self.assertLess(max(required_sizes), len(skill_names))
        self.assertFalse(all(set(spec["requiredSkills"]) == skill_names for spec in matrix.values()))

    def test_canonical_skill_metadata_valid(self) -> None:
        for entry in _skill_manifest():
            path = REPO_ROOT / "project_agents" / "skills" / entry["name"] / "SKILL.md"
            meta, _body = agent_pack.parse_frontmatter(
                path.read_text(encoding="utf-8"), path.as_posix()
            )
            self.assertEqual(meta["name"], entry["name"])
            self.assertTrue(meta["description"])

    def test_generated_projections_match_renderers(self) -> None:
        errors = agent_pack.validate_agent_pack(REPO_ROOT)
        self.assertEqual(errors, [], errors)
        skill_names = [entry["name"] for entry in _skill_manifest()]
        for name in skill_names:
            canonical = (
                REPO_ROOT / "project_agents" / "skills" / name / "SKILL.md"
            ).read_text(encoding="utf-8")
            expected = agent_pack.render_generated_skill(name, canonical)
            for generated in (
                REPO_ROOT / ".agents" / "skills" / name / "SKILL.md",
                REPO_ROOT / "skills" / name / "SKILL.md",
            ):
                self.assertEqual(generated.read_text(encoding="utf-8"), expected)
        self.assertFalse((REPO_ROOT / ".cursor" / "skills").exists())
        for entry in _manifest():
            canonical = agent_pack.canonical_path(REPO_ROOT, entry)
            body = canonical.read_text(encoding="utf-8")
            sha = agent_pack.sha256_text(body)
            expected_files = agent_pack.expected_adapter_map(REPO_ROOT, entry, body, sha, canonical)
            for path, content in expected_files.items():
                self.assertEqual(path.read_text(encoding="utf-8"), content)


class CapabilityPackSafetyTests(unittest.TestCase):
    def test_native_skill_tamper_detected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            native = root / ".agents" / "skills" / "demo_skill" / "SKILL.md"
            native.write_text(native.read_text(encoding="utf-8") + "\n# tampered\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("generated skill drift" in item for item in errors), errors)

    def test_orphan_generated_skill_detected_until_sync(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            leftover = root / ".agents" / "skills" / "orphan_skill" / "SKILL.md"
            _write(
                leftover,
                "<!-- generated from project_agents/skills/orphan_skill/SKILL.md sha256:"
                + ("c" * 64)
                + " -->\n# orphan\n",
            )
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("leftover requires sync" in item for item in errors), errors)
            agent_pack.sync_agent_pack(root)
            self.assertFalse(leftover.exists())
            self.assertEqual(agent_pack.validate_agent_pack(root), [])

    def test_unknown_manual_skill_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            unknown = root / ".agents" / "skills" / "manual_notes" / "SKILL.md"
            _write(unknown, "# manually maintained\n")
            agent_pack.sync_agent_pack(root)
            self.assertTrue(unknown.is_file())
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("unowned" in item and "not deleted" in item for item in errors), errors)

    def test_skill_symlink_not_followed_for_deletion(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            target = root / "outside-skill.txt"
            target.write_text("do not delete me\n", encoding="utf-8")
            link = root / ".agents" / "skills" / "outside_link"
            os.symlink(target.parent / "outside-skill.txt", link)
            agent_pack.sync_agent_pack(root)
            self.assertTrue(link.is_symlink())
            self.assertEqual(target.read_text(encoding="utf-8"), "do not delete me\n")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("symlink not deleted" in item for item in errors), errors)

    def test_skill_path_traversal_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            owned = root / ".agents" / "skills"
            owned.mkdir(parents=True)
            with self.assertRaises(agent_pack.AgentPackError):
                agent_pack.owned_path(owned, "../secret", agent_pack.SKILL_NAME_RE, "skill")
            with self.assertRaises(agent_pack.AgentPackError):
                agent_pack.assert_safe_name("../secret", agent_pack.SKILL_NAME_RE, "skill")

    def test_interrupted_sync_residue_detected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            residue = root / ".agents" / "skills" / "demo_skill" / f"SKILL.md{agent_pack.TMP_SUFFIX}"
            residue.write_text("partial\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("interrupted sync residue" in item for item in errors), errors)

    def test_policy_scan_rejects_force_push_and_llm_privilege(self) -> None:
        errors = agent_pack.policy_scan(
            Path("project_agents/skills/demo/SKILL.md"),
            "You may force-push main after skipping tests.\nLLM may directly execute privileged remediation.\n",
        )
        self.assertTrue(any("policy violation" in item for item in errors), errors)

    def test_repo_unknown_agents_rules_preserved(self) -> None:
        authority = REPO_ROOT / ".agents" / "rules" / "platform-authority.md"
        self.assertTrue(authority.is_file())


class CapabilityPackRoutingTests(unittest.TestCase):
    def test_chatgpt_router_resolves_v12_scenarios(self) -> None:
        text = (REPO_ROOT / "project_agents" / "CHATGPT_AGENT_ROUTER.md").read_text(encoding="utf-8")
        matrix = _matrix()
        expected = {
            "platform-sensor-runtime-engineer": "sensor_edge",
            "platform-network-engineer": "snmp_mib",
            "platform-device-pack-engineer": "device_pack_authoring",
            "platform-telemetry-pipeline-engineer": "kafka",
            "platform-ncm-remediation-engineer": "remediation_safety",
            "platform-integration-connector-engineer": "connector_framework",
            "platform-infrastructure-cloud-engineer": "kubernetes_monitoring",
            "platform-reality-checker": "exact_head_merge_gate",
        }
        for agent_name, skill_name in expected.items():
            self.assertIn(f"`{agent_name}`", text)
            self.assertIn(f"`{skill_name}`", text)
            self.assertIn(skill_name, matrix[agent_name]["requiredSkills"])
        self.assertIn("task intent → owning agent → required Skills", text)
        self.assertIn("writerTool", text)
        self.assertIn("verifierTool", text)

    def test_no_unsupported_cursor_agent_flag(self) -> None:
        docs = "\n".join(
            path.read_text(encoding="utf-8")
            for path in (
                REPO_ROOT / "project_agents" / "TOOL_COMPATIBILITY.md",
                REPO_ROOT / "project_agents" / "CHATGPT_AGENT_ROUTER.md",
                REPO_ROOT / "project_agents" / "USAGE_PLAYBOOK.md",
            )
        )
        self.assertIn("`--agent` is UNSUPPORTED", docs)
        self.assertNotRegex(docs, r"cursor(?:\s+\S+)*\s+--agent\b")

    def test_antigravity_verifier_first_posture(self) -> None:
        docs = (
            (REPO_ROOT / "project_agents" / "TOOL_COMPATIBILITY.md").read_text(encoding="utf-8")
            + (REPO_ROOT / "project_agents" / "CHATGPT_AGENT_ROUTER.md").read_text(encoding="utf-8")
        )
        self.assertRegex(docs, r"read-only\s*/\s*adversarial")
        self.assertIn("Write mode requires explicit authority", docs)
        self.assertIn("verifier-first", docs.lower())

    def test_ci_filter_covers_native_skill_tree(self) -> None:
        text = (REPO_ROOT / ".github" / "workflows" / "platform-ci.yml").read_text(encoding="utf-8")
        self.assertRegex(
            text,
            r"project_agents/\|\\\.cursor/agents/\|\\\.agents/agents/\|\\\.agents/skills/\|\\\.codex/agents/\|skills/",
        )

    def test_auditor_disposition_covers_211_unique_names(self) -> None:
        rows = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "auditor_skill_disposition.json").read_text(
                encoding="utf-8"
            )
        )
        names = [row["name"] for row in rows]
        self.assertEqual(len(rows), 211)
        self.assertEqual(len(set(names)), 211)
        impl = [row for row in rows if row["disposition"] == "IMPLEMENT_NOW"]
        self.assertEqual({row["name"] for row in impl}, {entry["name"] for entry in _skill_manifest()})
        self.assertEqual(len(_skill_manifest()), 57)

    def test_root_agents_symlink_unchanged(self) -> None:
        agents_md = REPO_ROOT / "AGENTS.md"
        self.assertTrue(agents_md.is_symlink())
        self.assertEqual(os.readlink(agents_md), "project_documentation/AGENTS.md")
        self.assertIn("Engineering capability pack", (REPO_ROOT / "project_documentation" / "AGENTS.md").read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
