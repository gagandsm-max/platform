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
from tests.engineering_orchestrator.helpers import fixture_skill_markdown  # noqa: E402

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
    skill = fixture_skill_markdown("demo_skill", "Fixture skill")
    conditional_skill = fixture_skill_markdown(
        "demo_conditional_skill", "Fixture conditional skill"
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

    def test_selected_tool_statuses_fail_closed(self) -> None:
        for tool in ("cursor-cli", "chatgpt-github", "founder"):
            for status in ("unknown", "suspended", "prohibited", None):
                with self.subTest(tool=tool, status=status):
                    authority = control_plane.load_tool_authority()
                    if status is None:
                        del authority["tools"][tool]["status"]
                    else:
                        authority["tools"][tool]["status"] = status
                    with self.assertRaises(control_plane.ControlPlaneError):
                        control_plane.assert_legal_tool_roles(
                            writer_tool="cursor-cli", verifier_tool="chatgpt-github",
                            gatekeeper="founder", status="INTAKE", authority=authority,
                        )

    def test_exact_identifiers_and_timestamps(self) -> None:
        for overrides in (
            {"taskId": "CPV2-1\n"},
            {"authoritativeMain": SHA_A + "\n"},
            {"exactHead": None},
            {"exactHead": ""},
            {"createdAt": "2026-02-30T00:00:00Z"},
            {"updatedAt": "2026-09-12T00:00:00+00:60"},
        ):
            with self.subTest(overrides=overrides), self.assertRaises(control_plane.ControlPlaneError):
                control_plane.validate_task_manifest(_task(**overrides))
        for overrides in (
            {"taskId": "CPV2-1\n"},
            {"outputHead": SHA_C + "\n"},
            {"timestamp": "2026-01-01T25:00:00Z"},
        ):
            with self.subTest(overrides=overrides), self.assertRaises(control_plane.ControlPlaneError):
                control_plane.validate_handoff(_handoff(**overrides))

    def test_write_paths_require_valid_syntax_and_frozen_scope(self) -> None:
        for paths, scope, state, message in (
            ([], [], "UNKNOWN", "unknown"),
            ([], ["../core"], "DIAGNOSIS", "safe repository-relative"),
            (["/core"], [], "ROUTING", "safe repository-relative"),
            (["core/file"], ["core/"], "ROUTING", "frozen scope"),
            (["core/file"], ["core/"], "BLOCKED", "frozen scope"),
        ):
            with self.subTest(state=state, paths=paths), self.assertRaisesRegex(
                control_plane.ControlPlaneError, message
            ):
                control_plane.assert_write_paths_in_scope(paths, scope, state)
        control_plane.assert_write_paths_in_scope([], ["core/"], "DIAGNOSIS")

    def test_handoff_checks_changed_path_syntax_and_phase(self) -> None:
        for path in ("../file", "/file", "C:/file", "file/", ".Git/config", "file\n"):
            with self.subTest(path=path), self.assertRaisesRegex(
                control_plane.ControlPlaneError, "safe repository-relative"
            ):
                control_plane.validate_handoff(_handoff(filesChanged=[path]))
        with self.assertRaisesRegex(control_plane.ControlPlaneError, "phase"):
            control_plane.validate_handoff(
                _handoff(phase="ROUTING"), previous_state="IMPLEMENTATION"
            )

    def test_formal_gate_requires_gatekeeper_and_evidence_context(self) -> None:
        handoff = _handoff(phase="CI_VALIDATION", nextState="REALITY_GATE")
        requirements = [handoff["claims"][0]["evidencePointer"]]
        context = {
            "previous_state": "CI_VALIDATION",
            "required_evidence": requirements,
        }
        self.assertEqual(control_plane.validate_handoff(handoff), handoff)
        self.assertEqual(
            control_plane.assert_legal_transition("CI_VALIDATION", "REALITY_GATE"),
            "structural_only",
        )
        for role in (None, "", "unknown", "writer", "verifier"):
            with self.subTest(role=role), self.assertRaises(control_plane.ControlPlaneError):
                control_plane.validate_handoff(handoff, actor_role=role, promotion=True, **context)
        with self.assertRaisesRegex(control_plane.ControlPlaneError, "gatekeeper role"):
            control_plane.validate_handoff(handoff, actor_role="verifier", **context)
        with self.assertRaisesRegex(control_plane.ControlPlaneError, "previous_state"):
            control_plane.validate_handoff(handoff, actor_role="gatekeeper", promotion=True)
        with self.assertRaisesRegex(control_plane.ControlPlaneError, "required evidence"):
            control_plane.validate_handoff(
                handoff, actor_role="gatekeeper", previous_state="CI_VALIDATION", promotion=True
            )
        for changes in (
            {"claims": []},
            {"claims": [{"claimType": "INFERENCE", "statement": "likely", "evidencePointer": requirements[0]}]},
            {"claims": [dict(handoff["claims"][0], evidencePointer="other-evidence")]},
            {"testResults": [{"name": "focused", "status": "FAIL"}]},
            {"testResults": [{"name": "focused", "status": "ERROR"}]},
            {"knownFailures": ["test fixture failed"]},
        ):
            with self.subTest(changes=changes), self.assertRaises(control_plane.ControlPlaneError):
                control_plane.validate_handoff(
                    dict(handoff, **changes), actor_role="gatekeeper", promotion=True, **context
                )
        failure_record = dict(handoff, knownFailures=["test fixture failed"])
        self.assertEqual(control_plane.validate_handoff(failure_record), failure_record)
        self.assertEqual(
            control_plane.validate_handoff(handoff, actor_role="gatekeeper", promotion=True, **context),
            handoff,
        )

    def test_writer_cannot_submit_independent_review_for_promotion(self) -> None:
        handoff = _handoff(phase="SOURCE_REVIEW", nextState="ADVERSARIAL_RUNTIME")
        with self.assertRaisesRegex(control_plane.ControlPlaneError, "self-review"):
            control_plane.validate_handoff(
                handoff, previous_state="SOURCE_REVIEW", actor_role="writer", promotion=True,
                required_evidence=[handoff["claims"][0]["evidencePointer"]],
            )

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

    def test_manifest_scope_rejects_unsafe_entries(self) -> None:
        for entry in ("/core", ".", "project_agents/../core", "project_agents//tools",
                      "C:/core", "project_agents/.git/config", "project_agents/GIT~1/config"):
            with self.subTest(entry=entry), self.assertRaisesRegex(
                control_plane.ControlPlaneError, "safe repository-relative path"
            ):
                control_plane.validate_task_manifest(_task(scope=[entry]))

    def test_frozen_scope_checks_observed_worktree(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            allowed = root / "allowed"
            allowed.mkdir()
            (root / "outside").mkdir()
            (allowed / "alias").symlink_to(root / "outside", target_is_directory=True)
            (allowed / "inside-alias").symlink_to(allowed, target_is_directory=True)
            (allowed / "nested").mkdir()
            (allowed / "nested" / ".git").write_text("gitdir: elsewhere")
            (allowed / "nested-dir" / ".git").mkdir(parents=True)
            control_plane.assert_write_paths_in_scope(
                ["allowed/new.py"], ["allowed/"], "IMPLEMENTATION", worktree_root=root
            )
            for path, reason in (
                ("allowed/../outside/file", "safe repository-relative path"),
                ("allowed/.GIT/config", "safe repository-relative path"),
                ("allowed/alias/file", "symlink"),
                ("allowed/inside-alias/file", "symlink"),
                ("allowed/nested/file", "nested Git worktree"),
                ("allowed/nested-dir/file", "nested Git worktree"),
                ("allowed-other/file", "outside frozen scope"),
            ):
                with self.subTest(path=path), self.assertRaisesRegex(
                    control_plane.ControlPlaneError, reason
                ):
                    control_plane.assert_write_paths_in_scope(
                        [path], ["allowed/"], "IMPLEMENTATION", worktree_root=root
                    )
            with self.assertRaisesRegex(control_plane.ControlPlaneError, "symlink"):
                control_plane.assert_write_paths_in_scope(
                    [], ["allowed/alias/"], "IMPLEMENTATION", worktree_root=root
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

    def test_default_cursor_cli_rejects_unexpected_elevation(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="cursor-cli",
                    writerElevation={
                        "authorizedBy": "founder",
                        "reason": "unnecessary",
                        "isolatedWorkspace": True,
                    },
                )
            )
        self.assertIn("must not include writerElevation", str(ctx.exception))

    def test_authorized_antigravity_writer_accepted(self) -> None:
        control_plane.validate_task_manifest(_task(writerTool="antigravity"))

    def test_antigravity_writer_elevation_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="antigravity",
                    writerElevation={
                        "authorizedBy": "founder",
                        "reason": "attempt to elevate verifier",
                        "isolatedWorkspace": True,
                    },
                )
            )
        self.assertIn("must not include writerElevation", str(ctx.exception))

    def test_authorized_codex_writer_accepted(self) -> None:
        control_plane.validate_task_manifest(
            _task(writerTool="codex", verifierTool="antigravity", gatekeeper="founder")
        )

    def test_codex_rejects_writer_elevation(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="codex",
                    verifierTool="antigravity",
                    gatekeeper="chatgpt-github",
                    writerElevation={
                        "authorizedBy": "founder",
                        "reason": "unnecessary",
                        "isolatedWorkspace": True,
                    },
                )
            )
        self.assertIn("must not include writerElevation", str(ctx.exception))

    def test_codex_writer_plus_codex_verifier_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(writerTool="codex", verifierTool="codex")
            )
        self.assertIn("cannot self-verify", str(ctx.exception))

    def test_codex_writer_plus_codex_gatekeeper_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(writerTool="codex", verifierTool="antigravity", gatekeeper="codex")
            )
        self.assertIn("prohibited", str(ctx.exception).lower())

    def test_cursor_and_codex_cannot_both_own_one_manifest(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError):
            control_plane.validate_task_manifest(
                _task(writerTool=["cursor-cli", "codex"])
            )

    def test_cursor_cloud_elevation_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="cursor-cloud",
                    writerElevation={
                        "authorizedBy": "founder",
                        "reason": "attempt to elevate prohibited cloud writer",
                        "isolatedWorkspace": True,
                    },
                )
            )
        self.assertIn("prohibited", str(ctx.exception).lower())

    def test_github_copilot_elevation_rejected(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="github-copilot",
                    writerElevation={
                        "authorizedBy": "founder",
                        "reason": "attempt to elevate copilot",
                        "isolatedWorkspace": True,
                    },
                )
            )
        self.assertIn("must not include writerElevation", str(ctx.exception))

    def test_writer_equals_verifier_fails(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="codex",
                    verifierTool="codex",
                    gatekeeper="founder",
                )
            )
        self.assertIn("cannot self-verify", str(ctx.exception))

    def test_writer_equals_gatekeeper_fails(self) -> None:
        with self.assertRaises(control_plane.ControlPlaneError) as ctx:
            control_plane.validate_task_manifest(
                _task(
                    writerTool="codex",
                    verifierTool="antigravity",
                    gatekeeper="codex",
                )
            )
        message = str(ctx.exception).lower()
        self.assertTrue(
            "gatekeeper" in message or "self-certify" in message or "authorized gatekeeper" in message,
            message,
        )

    def test_antigravity_read_only_and_codex_mode_derived_renderers(self) -> None:
        write_entry = {
            "name": "platform-minimal-change-engineer",
            "description": "demo",
            "mode": "write",
            "tier": "P0",
        }
        body = "# demo\n"
        sha = "a" * 64
        canonical = REPO_ROOT / "project_agents" / "canonical" / "p0" / "platform-minimal-change-engineer.md"
        agy = agent_pack.render_antigravity(write_entry, body, sha, canonical, REPO_ROOT)
        self.assertNotIn("replace_file_content", agy.split("---", 2)[1])
        codex_write = agent_pack.render_codex(write_entry, body, sha, canonical, REPO_ROOT)
        self.assertIn('sandbox_mode = "workspace-write"', codex_write)
        self.assertNotIn("danger-full-access", codex_write)
        self.assertIn("operate only in the assigned worktree and task scope", codex_write)
        self.assertIn("do not merge, certify, self-verify", codex_write)

        read_entry = dict(write_entry, mode="read")
        codex_read = agent_pack.render_codex(read_entry, body, sha, canonical, REPO_ROOT)
        self.assertIn('sandbox_mode = "read-only"', codex_read)
        self.assertNotIn("workspace-write", codex_read)

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
        self.assertIn("cursor-cli", authority["legalCombinations"]["defaultWriterTools"])
        self.assertIn("cursor-cli", authority["legalCombinations"]["writerTools"])
        self.assertNotIn("cursor-cloud", authority["legalCombinations"]["writerTools"])
        self.assertEqual(authority["legalCombinations"]["exceptionalWriterTools"], [])
        self.assertEqual(
            set(authority["legalCombinations"]["writerTools"]),
            {"cursor-cli", "codex", "antigravity", "github-copilot"},
        )
        self.assertEqual(
            set(authority["legalCombinations"]["verifierTools"]),
            {"chatgpt-github", "antigravity", "cursor-cli", "codex", "github-copilot"},
        )
        self.assertIn("codex", authority["legalCombinations"]["writerTools"])
        self.assertEqual(authority["tools"]["codex"]["status"], "authorized")
        self.assertNotIn("codex", authority["legalCombinations"]["neverWriterTools"])
        self.assertNotIn("codex", authority["legalCombinations"]["prohibitedWriterTools"])
        self.assertIn("cursor-cloud", authority["legalCombinations"]["neverWriterTools"])
        self.assertTrue(authority["tools"]["antigravity"]["mayWriteProductCode"])
        self.assertFalse(authority["tools"]["antigravity"]["mayCertify"])
        self.assertTrue(authority["tools"]["codex"]["mayWriteProductCode"])
        self.assertFalse(authority["tools"]["codex"]["mayCertify"])
        self.assertEqual(
            authority["tools"]["antigravity"]["defaultGeneratedCapability"], "read-only"
        )
        self.assertEqual(authority["tools"]["codex"]["readAgentSandboxMode"], "read-only")
        self.assertEqual(
            authority["tools"]["codex"]["writeAgentSandboxMode"], "workspace-write"
        )
        self.assertEqual(
            set(authority["tools"]["codex"]["clientInterfaces"]),
            {"codex-cli", "chatgpt-desktop-codex-mode"},
        )
        self.assertIn("auditor", authority["tools"]["antigravity"]["roles"])
        self.assertIn("certification_gate", authority["tools"]["antigravity"]["roles"])
        self.assertEqual(authority["tools"]["cursor-cli"]["roles"], ["writer", "test", "verifier"])
        self.assertTrue(authority["tools"]["cursor-cli"]["mayWriteProductCode"])
        self.assertTrue(authority["tools"]["cursor-cli"]["defaultWriter"])
        self.assertFalse(authority["tools"]["cursor-cli"]["mayMerge"])
        self.assertFalse(authority["tools"]["cursor-cli"]["mayCertify"])
        self.assertIn("writer", authority["tools"]["codex"]["roles"])
        self.assertFalse(authority["tools"]["codex"]["defaultWriter"])
        self.assertFalse(authority["tools"]["codex"]["mayMerge"])
        self.assertFalse(authority["tools"]["chatgpt-github"]["mayWriteProductCode"])
        self.assertTrue(authority["tools"]["chatgpt-github"]["mayMerge"])
        self.assertTrue(authority["tools"]["chatgpt-github"]["mayCertify"])

    def test_schema_writer_elevation_conditionals_match_runtime(self) -> None:
        schema = json.loads(
            (REPO_ROOT / "project_agents" / "schemas" / "task_manifest.schema.json").read_text(
                encoding="utf-8"
            )
        )
        control_plane.assert_schema_writer_elevation_conditionals(schema)
        control_plane.assert_schema_tool_enums_match_authority(task_schema=schema)
        writer_enum = schema["properties"]["writerTool"]["enum"]
        self.assertEqual(writer_enum, ["cursor-cli", "codex", "antigravity", "github-copilot"])
        verifier_enum = schema["properties"]["verifierTool"]["enum"]
        self.assertEqual(verifier_enum, ["chatgpt-github", "antigravity", "cursor-cli", "codex", "github-copilot"])
        for never in ("cursor-cloud", "chatgpt-github", "founder"):
            self.assertNotIn(never, writer_enum)

    def test_schema_and_runtime_agree_on_writer_elevation_matrix(self) -> None:
        """Deterministic agreement matrix; optional JSON Schema engine is best-effort."""
        import importlib.util

        valid_elevation = {
            "authorizedBy": "founder",
            "reason": "isolated exceptional write lane",
            "isolatedWorkspace": True,
        }
        writers = ("cursor-cli", "codex", "antigravity", "github-copilot")
        cases = [
            (f"{writer}-reviewed-by-{verifier}", {"writerTool": writer, "verifierTool": verifier},
             writer != verifier)
            for writer in writers
            for verifier in (*writers, "chatgpt-github")
        ]
        for writer in writers:
            cases.extend([
                (f"{writer}-with-null", {"writerTool": writer, "writerElevation": None}, True),
                (f"{writer}-with-elevation",
                 {"writerTool": writer, "writerElevation": valid_elevation}, False),
                (f"{writer}-with-false-elevation",
                 {"writerTool": writer, "writerElevation": False}, False),
                (f"{writer}-as-gatekeeper", {"gatekeeper": writer}, False),
            ])
        cases.extend([
            ("unknown-writer", {"writerTool": "unknown"}, False),
            ("cloud-writer", {"writerTool": "cursor-cloud"}, False),
            ("master-writer", {"writerTool": "chatgpt-github"}, False),
            ("founder-writer", {"writerTool": "founder"}, False),
            ("cloud-verifier", {"verifierTool": "cursor-cloud"}, False),
            ("cloud-gatekeeper", {"gatekeeper": "cursor-cloud"}, False),
            ("codex-cli-alias", {"writerTool": "codex-cli"}, False),
            ("codex-desktop-alias", {"writerTool": "codex",
                                     "verifierTool": "chatgpt-desktop-codex-mode"}, False),
        ])

        schema = json.loads(
            (REPO_ROOT / "project_agents" / "schemas" / "task_manifest.schema.json").read_text(
                encoding="utf-8"
            )
        )
        control_plane.assert_schema_writer_elevation_conditionals(schema)

        jsonschema_mod = None
        if importlib.util.find_spec("jsonschema") is not None:
            import jsonschema  # type: ignore

            jsonschema_mod = jsonschema

        engine_results: list[str] = []
        for name, overrides, expect_valid in cases:
            payload = _task(**overrides)
            runtime_ok = True
            try:
                control_plane.validate_task_manifest(payload)
            except control_plane.ControlPlaneError:
                runtime_ok = False
            self.assertEqual(
                runtime_ok,
                expect_valid,
                f"runtime mismatch for {name}: expected_valid={expect_valid}",
            )

            # Evaluate the schema's role and elevation restrictions without optional dependencies.
            props = schema["properties"]
            structural_ok = all(
                payload[field] in props[field]["enum"]
                for field in ("writerTool", "verifierTool", "gatekeeper")
            )
            structural_ok &= payload.get("writerElevation") is None
            structural_ok &= not any(
                all(payload[field] == rule["const"]
                    for field, rule in clause["not"]["properties"].items())
                for clause in schema["allOf"]
            )
            self.assertEqual(structural_ok, expect_valid, name)

            if jsonschema_mod is None:
                engine_results.append(f"{name}=NOT_RUN")
            else:
                try:
                    jsonschema_mod.Draft202012Validator(schema).validate(payload)
                    engine_ok = True
                except Exception:
                    engine_ok = False
                self.assertEqual(
                    engine_ok,
                    expect_valid,
                    f"jsonschema engine mismatch for {name}",
                )
                engine_results.append(f"{name}={'PASS' if engine_ok else 'FAIL'}")

        if jsonschema_mod is None:
            # Keep an explicit evidence marker when no standards engine is wired into CI deps.
            self.assertTrue(all(item.endswith("=NOT_RUN") for item in engine_results))


if __name__ == "__main__":
    unittest.main()
