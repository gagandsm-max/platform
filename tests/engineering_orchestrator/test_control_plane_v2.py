"""Control Plane v2 contract, pack-safety and validator tests."""

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

SHA_A = "a" * 40
SHA_B = "b" * 40
SHA_C = "c" * 40


def _task(**overrides):
    payload = {
        "schemaVersion": "v2",
        "taskId": "CPV2-1",
        "taskType": "implementation",
        "objective": "repair control plane contracts",
        "repository": "gagandsm-max/platform",
        "authoritativeMain": SHA_A,
        "branch": "feature/control-plane-v2",
        "worktree": "/tmp/worktree",
        "governingRequirements": ["REQ-CPV2"],
        "selectedAgents": ["platform-minimal-change-engineer"],
        "selectedSkills": ["root_cause_first"],
        "scope": ["project_agents/"],
        "nonGoals": ["runtime product changes"],
        "securityBoundaries": ["no secret embedding"],
        "migrationReservations": [],
        "dependencies": [],
        "requiredTests": ["tests/engineering_orchestrator/test_control_plane_v2.py"],
        "requiredEvidence": ["tests/engineering_orchestrator/test_control_plane_v2.py"],
        "stopConditions": ["head drift"],
        "writerTool": "cursor-cli",
        "verifierTool": "chatgpt-github",
        "gatekeeper": "founder",
        "status": "INTAKE",
        "createdAt": "2026-09-12T00:00:00Z",
        "updatedAt": "2026-09-12T00:00:00Z",
    }
    payload.update(overrides)
    return payload


def _handoff(**overrides):
    payload = {
        "schemaVersion": "v2",
        "taskId": "CPV2-1",
        "phase": "IMPLEMENTATION",
        "inputHead": SHA_B,
        "outputHead": SHA_C,
        "filesChanged": ["project_agents/tools/control_plane.py"],
        "requirementsAddressed": ["REQ-CPV2"],
        "testsExecuted": ["tests/engineering_orchestrator/test_control_plane_v2.py"],
        "testResults": [{"name": "control-plane-v2", "status": "PASS"}],
        "runtimeEvidence": [],
        "securityEvidence": [],
        "knownFailures": [],
        "unresolvedRisks": [],
        "claims": [
            {
                "claimType": "EXECUTION_EVIDENCE",
                "statement": "focused tests passed",
                "evidencePointer": "tests/engineering_orchestrator/test_control_plane_v2.py",
            }
        ],
        "nextState": "LOCAL_VALIDATION",
        "nextOwner": "verifierTool",
        "timestamp": "2026-09-12T00:00:00Z",
    }
    payload.update(overrides)
    return payload


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def _build_fixture(root: Path) -> None:
    canonical = (
        "# Demo Agent\n\n"
        "Fixture canonical agent.\n\n"
        "## Required Skills\n"
        "- `demo_skill`\n\n"
        "## Conditional Skills\n"
        "- `demo_conditional_skill`\n"
    )
    skill = (
        "---\n"
        "name: demo_skill\n"
        "description: Fixture skill\n"
        "---\n\n"
        "# demo_skill\n\nFixture canonical skill.\n"
    )
    conditional_skill = (
        "---\n"
        "name: demo_conditional_skill\n"
        "description: Fixture conditional skill\n"
        "---\n\n"
        "# demo_conditional_skill\n\nFixture conditional skill.\n"
    )
    manifest = [
        {
            "name": "demo-agent",
            "title": "Demo Agent",
            "tier": "P0",
            "mode": "write",
            "description": "Fixture agent",
            "canonical_path": "canonical/p0/demo-agent.md",
            "sha256": "0" * 64,
        }
    ]
    skills = [
        {"name": "demo_skill", "description": "Fixture skill"},
        {"name": "demo_conditional_skill", "description": "Fixture conditional skill"},
    ]
    matrix = {
        "demo-agent": {
            "requiredSkills": ["demo_skill"],
            "conditionalSkills": ["demo_conditional_skill"],
        }
    }
    authority = json.loads((REPO_ROOT / "project_agents" / "tool_authority.json").read_text(encoding="utf-8"))
    _write(root / "project_agents" / "manifest.json", json.dumps(manifest, indent=2) + "\n")
    _write(root / "project_agents" / "canonical" / "p0" / "demo-agent.md", canonical)
    _write(root / "project_agents" / "skills" / "manifest.json", json.dumps(skills, indent=2) + "\n")
    _write(root / "project_agents" / "skills" / "demo_skill" / "SKILL.md", skill)
    _write(root / "project_agents" / "skills" / "demo_conditional_skill" / "SKILL.md", conditional_skill)
    _write(
        root / "project_agents" / "skills" / "agent_skill_matrix.json",
        json.dumps(matrix, indent=2) + "\n",
    )
    _write(root / "project_agents" / "tool_authority.json", json.dumps(authority, indent=2) + "\n")
    os.symlink("project_documentation/AGENTS.md", root / "AGENTS.md")
    _write(root / "project_documentation" / "AGENTS.md", "# Fixture AGENTS\n")


class ControlPlaneContractTests(unittest.TestCase):
    def test_manifest_derived_inventories(self) -> None:
        agents = control_plane.load_agent_names()
        skills = control_plane.load_skill_names()
        manifest = control_plane.load_json(control_plane.MANIFEST_PATH)
        skill_manifest = control_plane.load_json(control_plane.SKILLS_MANIFEST_PATH)
        self.assertEqual(len(agents), len(manifest))
        self.assertEqual(len(skills), len(skill_manifest))
        self.assertIn("platform-code-reviewer", agents)
        self.assertIn("root_cause_first", skills)

    def test_selected_agent_and_skill_resolution(self) -> None:
        control_plane.validate_selected_agents(["platform-minimal-change-engineer"])
        control_plane.validate_selected_skills(["evidence_bundle"])
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_selected_agents(["not-a-real-agent"])
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_selected_skills(["not_a_real_skill"])

    def test_schema_enums_match_state_machine(self) -> None:
        control_plane.assert_schema_state_enums_match_machine()
        states = control_plane.authoritative_states()
        self.assertIn("MERGE_READY", states)
        self.assertIn("POST_MERGE_VERIFY", states)
        self.assertIn("BLOCKED", states)
        self.assertIn("REJECTED", states)

    def test_phase_aware_task_validation(self) -> None:
        control_plane.validate_task_manifest(_task(status="INTAKE", selectedAgents=[], selectedSkills=[], scope=[]))
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(_task(status="ROUTING", selectedAgents=["platform-minimal-change-engineer"]))
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(
                    status="IMPLEMENTATION",
                    exactHead=SHA_B,
                    scope=[],
                    selectedAgents=["platform-minimal-change-engineer"],
                    selectedSkills=["root_cause_first"],
                )
            )
        control_plane.validate_task_manifest(
            _task(
                status="IMPLEMENTATION",
                exactHead=SHA_B,
                selectedAgents=["platform-minimal-change-engineer"],
                selectedSkills=["root_cause_first"],
                scope=["project_agents/"],
            )
        )

    def test_routing_rejects_empty_selected_skills(self) -> None:
        control_plane.validate_task_manifest(
            _task(status="INTAKE", selectedAgents=[], selectedSkills=[], scope=[])
        )
        control_plane.validate_task_manifest(
            _task(status="DIAGNOSIS", selectedAgents=[], selectedSkills=[], scope=[])
        )
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    status="ROUTING",
                    exactHead=SHA_B,
                    selectedAgents=["platform-minimal-change-engineer"],
                    selectedSkills=[],
                    scope=[],
                )
            )
        self.assertIn("selectedSkills must be resolved before ROUTING", str(ctx.exception))
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(
                    status="ROUTING",
                    exactHead=SHA_B,
                    selectedAgents=["platform-minimal-change-engineer"],
                    selectedSkills=["not_a_real_skill"],
                    scope=[],
                )
            )
        control_plane.validate_task_manifest(
            _task(
                status="ROUTING",
                exactHead=SHA_B,
                selectedAgents=["platform-minimal-change-engineer"],
                selectedSkills=["source_truth_rebaseline", "bounded_implementation"],
                scope=[],
            )
        )

    def test_writer_verifier_aliases_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(_task(writer="cursor-cloud"))

    def test_head_and_main_drift_rejected(self) -> None:
        manifest = _task(status="IMPLEMENTATION", exactHead=SHA_B)
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(manifest, observed_main=SHA_C, observed_head=SHA_B)
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(manifest, observed_main=SHA_A, observed_head=SHA_C)

    def test_evidence_pointer_required_for_source_facts(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_handoff(
                _handoff(
                    claims=[{"claimType": "SOURCE_FACT", "statement": "file exists"}],
                )
            )

    def test_unverified_cannot_satisfy_evidence_gate(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_handoff(
                _handoff(
                    claims=[
                        {
                            "claimType": "UNVERIFIED",
                            "statement": "tests probably passed",
                        }
                    ],
                    nextState="LOCAL_VALIDATION",
                ),
                required_evidence=["tests/engineering_orchestrator/test_control_plane_v2.py"],
            )

    def test_writer_cannot_self_certify(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_legal_transition(
                "CI_VALIDATION", "REALITY_GATE", actor_role="writer"
            )
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_legal_transition(
                "REALITY_GATE", "MERGE_READY", actor_role="writer"
            )
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_legal_transition(
                "MERGE_READY", "POST_MERGE_VERIFY", actor_role="writer"
            )

    def test_illegal_and_legal_transitions(self) -> None:
        control_plane.assert_legal_transition("INTAKE", "REBASELINE")
        control_plane.assert_legal_transition("IMPLEMENTATION", "DIAGNOSIS")
        control_plane.assert_legal_transition("IMPLEMENTATION", "ROUTING")
        control_plane.assert_legal_transition("MERGE_READY", "POST_MERGE_VERIFY")
        control_plane.assert_legal_transition("POST_MERGE_VERIFY", "COMPLETE")
        control_plane.assert_legal_transition("DIAGNOSIS", "BLOCKED")
        control_plane.assert_legal_transition("BLOCKED", "IMPLEMENTATION")
        control_plane.assert_legal_transition("ROUTING", "REJECTED")
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_legal_transition("MERGE_READY", "COMPLETE")
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_legal_transition("INTAKE", "IMPLEMENTATION")

    def test_frozen_scope_rejects_silent_expansion(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.assert_write_paths_in_scope(
                ["core/src/Main.java"],
                ["project_agents/"],
                "IMPLEMENTATION",
            )


class AgentPackSafetyTests(unittest.TestCase):
    def test_sync_validate_sync_zero_second_diff(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            first = {
                path.relative_to(root).as_posix(): path.read_bytes()
                for path in root.rglob("*")
                if path.is_file() and not path.is_symlink()
            }
            self.assertEqual(agent_pack.validate_agent_pack(root), [])
            self.assertTrue((root / ".agents" / "skills" / "demo_skill" / "SKILL.md").is_file())
            self.assertTrue((root / "skills" / "demo_skill" / "SKILL.md").is_file())
            self.assertFalse((root / ".cursor" / "skills").exists())
            agent_pack.sync_agent_pack(root)
            second = {
                path.relative_to(root).as_posix(): path.read_bytes()
                for path in root.rglob("*")
                if path.is_file() and not path.is_symlink()
            }
            self.assertEqual(first, second)

    def test_generated_tampering_detected_then_restored(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            adapter = root / ".cursor" / "agents" / "demo-agent.md"
            original = adapter.read_text(encoding="utf-8")
            adapter.write_text(original + "\n# tampered\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("adapter drift" in item for item in errors), errors)
            agent_pack.sync_agent_pack(root)
            self.assertEqual(agent_pack.validate_agent_pack(root), [])
            self.assertEqual(adapter.read_text(encoding="utf-8"), original)

    def test_unknown_adapter_is_not_deleted(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            unknown = root / ".cursor" / "agents" / "manual-notes.md"
            unknown.write_text("# manually maintained\n", encoding="utf-8")
            agent_pack.sync_agent_pack(root)
            self.assertTrue(unknown.is_file())
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("unowned adapter requires review" in item for item in errors), errors)

    def test_unknown_root_skill_is_not_deleted(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            unknown = root / "skills" / "manual_review_skill" / "SKILL.md"
            _write(unknown, "# manually maintained skill\n")
            agent_pack.sync_agent_pack(root)
            self.assertTrue(unknown.is_file())
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(
                any("unowned" in item and "requires review" in item for item in errors),
                errors,
            )

    def test_proven_generated_leftovers_are_pruned(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            leftover_adapter = root / ".cursor" / "agents" / "retired-agent.md"
            leftover_adapter.write_text(
                "<!-- generated from project_agents/canonical/p0/retired-agent.md sha256:"
                + ("a" * 64)
                + " -->\n# retired\n",
                encoding="utf-8",
            )
            leftover_skill = root / "skills" / "retired_skill" / "SKILL.md"
            leftover_native = root / ".agents" / "skills" / "retired_skill" / "SKILL.md"
            marker = (
                "<!-- generated from project_agents/skills/retired_skill/SKILL.md sha256:"
                + ("b" * 64)
                + " -->\n# retired\n"
            )
            _write(leftover_skill, marker)
            _write(leftover_native, marker)
            agent_pack.sync_agent_pack(root)
            self.assertFalse(leftover_adapter.exists())
            self.assertFalse(leftover_skill.exists())
            self.assertFalse(leftover_native.exists())
            self.assertEqual(agent_pack.validate_agent_pack(root), [])

    def test_symlink_adapters_are_not_followed_for_deletion(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            target = root / "outside.txt"
            target.write_text("do not delete me\n", encoding="utf-8")
            link = root / ".cursor" / "agents" / "outside-link.md"
            os.symlink(target, link)
            agent_pack.sync_agent_pack(root)
            self.assertTrue(link.is_symlink())
            self.assertEqual(target.read_text(encoding="utf-8"), "do not delete me\n")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("symlink not deleted" in item for item in errors), errors)

    def test_path_traversal_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            owned = root / ".cursor" / "agents"
            owned.mkdir(parents=True)
            with self.assertRaises(agent_pack.AgentPackError):
                agent_pack.owned_path(owned, "../AGENTS.md", agent_pack.AGENT_NAME_RE, "adapter")
            with self.assertRaises(agent_pack.AgentPackError):
                agent_pack.assert_safe_name("../AGENTS.md", agent_pack.AGENT_NAME_RE, "agent")

    def test_root_agents_symlink_in_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            self.assertTrue((root / "AGENTS.md").is_symlink())
            self.assertEqual(agent_pack.validate_agent_pack(root), [])


class RepositoryGovernanceTests(unittest.TestCase):
    def test_current_repo_pack_and_symlink(self) -> None:
        errors = agent_pack.validate_agent_pack(REPO_ROOT)
        self.assertEqual(errors, [], errors)
        self.assertTrue((REPO_ROOT / "AGENTS.md").is_symlink())
        self.assertEqual(
            os.readlink(REPO_ROOT / "AGENTS.md"),
            "project_documentation/AGENTS.md",
        )

    def test_ci_filter_includes_root_skills(self) -> None:
        text = (REPO_ROOT / ".github" / "workflows" / "platform-ci.yml").read_text(encoding="utf-8")
        self.assertIn("|skills/)", text)
        self.assertIn(".agents/skills/", text)
        self.assertIn("project_agents/", text)
        self.assertIn(".cursor/agents/", text)
        self.assertIn(".agents/agents/", text)
        self.assertIn(".codex/agents/", text)
        self.assertIn("python3 project_agents/tools/validate_agents.py", text)

    def test_task_schema_uses_writer_tool_names(self) -> None:
        schema = json.loads(
            (REPO_ROOT / "project_agents" / "schemas" / "task_manifest.schema.json").read_text(
                encoding="utf-8"
            )
        )
        self.assertIn("writerTool", schema["properties"])
        self.assertIn("verifierTool", schema["properties"])
        self.assertNotIn("writer", schema["properties"])
        self.assertNotIn("verifier", schema["properties"])
        self.assertNotIn("state", schema["properties"])

    def test_no_hardcoded_inventory_policy_in_validator(self) -> None:
        source = (TOOLS / "agent_pack.py").read_text(encoding="utf-8") + (
            TOOLS / "validate_agents.py"
        ).read_text(encoding="utf-8")
        self.assertNotIn("len(manifest) == 27", source)
        self.assertNotIn("len(manifest) == 33", source)
        self.assertNotIn("len(manifest) == 34", source)
        self.assertNotIn("P0=12", source)
        self.assertNotIn("P1=15", source)
        self.assertNotIn("P1=21", source)
        self.assertNotIn("P1=22", source)


class ToolAuthorityTests(unittest.TestCase):
    def test_prohibited_cursor_cloud_writer_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(_task(writerTool="cursor-cloud"))
        self.assertIn("prohibited", str(ctx.exception).lower())

    def test_illegal_writer_verifier_pairing_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(writerTool="cursor-cli", verifierTool="cursor-cli")
            )
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(writerTool="cursor-cli", verifierTool="github-copilot")
            )

    def test_writer_self_certification_as_gatekeeper_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(
                    writerTool="cursor-cli",
                    verifierTool="chatgpt-github",
                    gatekeeper="cursor-cli",
                    status="MERGE_READY",
                    exactHead=SHA_B,
                    scope=["project_agents/"],
                    requiredEvidence=["tests/engineering_orchestrator/test_control_plane_v2.py"],
                )
            )

    def test_authorized_cursor_cli_writer_accepted(self) -> None:
        control_plane.validate_task_manifest(
            _task(writerTool="cursor-cli", verifierTool="antigravity", gatekeeper="founder")
        )

    def test_antigravity_and_codex_default_read_only_renderers(self) -> None:
        entry = {
            "name": "platform-minimal-change-engineer",
            "description": "demo",
            "mode": "write",
            "tier": "P0",
        }
        body = "# demo\n"
        sha = "a" * 64
        canonical = REPO_ROOT / "project_agents" / "canonical" / "p0" / "platform-minimal-change-engineer.md"
        agy = agent_pack.render_antigravity(entry, body, sha, canonical, REPO_ROOT)
        self.assertNotIn("replace_file_content", agy.split("---", 2)[1])
        codex = agent_pack.render_codex(entry, body, sha, canonical, REPO_ROOT)
        self.assertIn('sandbox_mode = "read-only"', codex)
        self.assertNotIn("workspace-write", codex)

    def test_duplicate_required_skill_rejected(self) -> None:
        with self.assertRaises(agent_pack.AgentPackError) as ctx:
            agent_pack.assert_unique_skill_list(
                ["evidence_bundle", "evidence_bundle"],
                "demo requiredSkills",
            )
        self.assertIn("duplicate", str(ctx.exception).lower())

    def test_duplicate_conditional_skill_rejected(self) -> None:
        with self.assertRaises(agent_pack.AgentPackError) as ctx:
            agent_pack.assert_unique_skill_list(
                ["kafka", "kafka"],
                "demo conditionalSkills",
            )
        self.assertIn("duplicate", str(ctx.exception).lower())

    def test_unknown_skill_in_matrix_fails_pack_validation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            matrix_path = root / "project_agents" / "skills" / "agent_skill_matrix.json"
            matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
            matrix["demo-agent"]["requiredSkills"] = ["demo_skill", "not_a_real_skill"]
            matrix_path.write_text(json.dumps(matrix, indent=2) + "\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(
                any("unknown skill" in item for item in errors),
                errors,
            )

    def test_canonical_agent_matrix_skill_mismatch_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            matrix_path = root / "project_agents" / "skills" / "agent_skill_matrix.json"
            matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
            matrix["demo-agent"]["requiredSkills"] = ["demo_conditional_skill"]
            matrix["demo-agent"]["conditionalSkills"] = ["demo_skill"]
            matrix_path.write_text(json.dumps(matrix, indent=2) + "\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(
                any("mismatch between canonical agent and matrix" in item for item in errors),
                errors,
            )

    def test_missing_codex_adapter_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            codex = root / ".codex" / "agents" / "demo-agent.toml"
            self.assertTrue(codex.is_file())
            codex.unlink()
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(
                any("missing Codex adapter" in item or "missing adapters" in item for item in errors),
                errors,
            )

    def test_adapter_drift_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            adapter = root / ".cursor" / "agents" / "demo-agent.md"
            adapter.write_text(adapter.read_text(encoding="utf-8") + "\n# drift\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(any("adapter drift" in item for item in errors), errors)

    def test_skill_mirror_drift_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            _build_fixture(root)
            agent_pack.sync_agent_pack(root)
            mirror = root / "skills" / "demo_skill" / "SKILL.md"
            mirror.write_text(mirror.read_text(encoding="utf-8") + "\n# drift\n", encoding="utf-8")
            errors = agent_pack.validate_agent_pack(root)
            self.assertTrue(
                any("generated skill drift" in item for item in errors),
                errors,
            )

    def test_tool_authority_source_present_and_cursor_cloud_prohibited(self) -> None:
        authority = control_plane.load_tool_authority()
        self.assertEqual(authority["tools"]["cursor-cloud"]["status"], "prohibited")
        self.assertIn("cursor-cli", authority["legalCombinations"]["writerTools"])
        self.assertNotIn("cursor-cloud", authority["legalCombinations"]["writerTools"])


if __name__ == "__main__":
    unittest.main()
