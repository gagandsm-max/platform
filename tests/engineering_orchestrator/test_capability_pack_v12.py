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

UI_SKILLS = (
    "ux_operational_design",
    "web_interface_quality",
    "accessibility_inclusive_design",
)

UI_AUDITOR_MERGE = {
    "usability_information_architecture": "ux_operational_design",
    "product_ui_visual_interaction": "ux_operational_design",
    "ux_writing_content_intent": "ux_operational_design",
    "dashboard_decision_design": "ux_operational_design",
    "noc_operational_ux": "ux_operational_design",
    "large_estate_dense_ui": "ux_operational_design",
    "design_system_architecture": "web_interface_quality",
    "web_visual_runtime_validation": "web_interface_quality",
    "frontend_state_completeness": "web_interface_quality",
    "inclusive_keyboard_semantics": "accessibility_inclusive_design",
}

UPSTREAM_PROVENANCE_SHAS = {
    "vercel-labs/web-interface-guidelines": "e3d624baaf29dc1fc645aff3e38f03e564d2d6b1",
    "hueyexe/frontend-agent-skills": "2841c079dd8a9c634882227194dc42e25227710d",
    "ca-who-codes/ultimate.UIUX.design.skills": "c87ae4e906ed4508634f1ead0f79431f5c62ff4e",
    "plugin87/full-stack-design-skills": "61311065938a3f7e978b05476c52c65385b52ba7",
    "mares29/dashboard-design-skill": "bbed535a08f0e1768b40087996fe019370312374",
}


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
        self.assertEqual(len(manifest), 34)
        self.assertEqual(sum(1 for entry in manifest if entry["tier"] == "P0"), 12)
        self.assertEqual(sum(1 for entry in manifest if entry["tier"] == "P1"), 22)
        self.assertEqual(len(control_plane.load_agent_names()), len(manifest))
        for name in NEW_P1:
            self.assertIn(name, names)
            control_plane.validate_selected_agents([name])
        self.assertIn("platform-product-ui-designer", names)
        control_plane.validate_selected_agents(["platform-product-ui-designer"])

    def test_validators_do_not_hardcode_v12_counts(self) -> None:
        source = (TOOLS / "agent_pack.py").read_text(encoding="utf-8") + (
            TOOLS / "validate_agents.py"
        ).read_text(encoding="utf-8")
        self.assertNotIn("len(manifest) == 33", source)
        self.assertNotIn("expected_agent_count = 33", source)
        self.assertNotIn("len(manifest) == 34", source)
        self.assertNotIn("expected_agent_count = 34", source)

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
            "platform-product-ui-designer": "ux_operational_design",
            "platform-frontend-developer": "web_interface_quality",
            "platform-ux-architect": "ux_operational_design",
            "platform-accessibility-auditor": "accessibility_inclusive_design",
            "platform-ui-finish-gate-reviewer": "web_interface_quality",
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

    def test_auditor_disposition_tracks_manifest_and_ui_merge(self) -> None:
        rows = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "auditor_skill_disposition.json").read_text(
                encoding="utf-8"
            )
        )
        names = [row["name"] for row in rows]
        self.assertEqual(len(names), len(set(names)))
        self.assertGreaterEqual(len(rows), 211)
        impl = [row for row in rows if row["disposition"] == "IMPLEMENT_NOW"]
        skill_names = {entry["name"] for entry in _skill_manifest()}
        self.assertEqual({row["name"] for row in impl}, skill_names)
        self.assertLessEqual(len(_skill_manifest()), 60)
        self.assertEqual(len(_skill_manifest()), 60)
        for skill_name in UI_SKILLS:
            self.assertIn(skill_name, skill_names)
            control_plane.validate_selected_skills([skill_name])
        by_name = {row["name"]: row for row in rows}
        for proposed, target in UI_AUDITOR_MERGE.items():
            self.assertEqual(by_name[proposed]["disposition"], f"MERGE_INTO:{target}")
        self.assertEqual(by_name["ui_no_generic_dashboard"]["disposition"], "MERGE_INTO:ux_operational_design")
        self.assertEqual(
            by_name["wcag_operational_screens"]["disposition"], "MERGE_INTO:accessibility_inclusive_design"
        )
        self.assertFalse(any(str(row["disposition"]).startswith("ALIAS") for row in rows))

    def test_root_agents_symlink_unchanged(self) -> None:
        agents_md = REPO_ROOT / "AGENTS.md"
        self.assertTrue(agents_md.is_symlink())
        self.assertEqual(os.readlink(agents_md), "project_documentation/AGENTS.md")
        self.assertIn("Engineering capability pack", (REPO_ROOT / "project_documentation" / "AGENTS.md").read_text(encoding="utf-8"))

    def test_implementation_routing_examples_declare_selected_agents_and_skills(self) -> None:
        docs = "\n".join(
            path.read_text(encoding="utf-8")
            for path in (
                REPO_ROOT / "project_agents" / "START_HERE.md",
                REPO_ROOT / "project_agents" / "USAGE_PLAYBOOK.md",
                REPO_ROOT / "project_agents" / "CHATGPT_AGENT_ROUTER.md",
                REPO_ROOT / "project_agents" / "TASK_PROMPT_TEMPLATE.md",
            )
        )
        self.assertGreaterEqual(docs.count("selectedAgents"), 4)
        self.assertGreaterEqual(docs.count("selectedSkills"), 4)
        self.assertIn("platform-product-ui-designer", docs)
        self.assertIn("ux_operational_design", docs)
        self.assertIn("web_interface_quality", docs)
        self.assertIn("accessibility_inclusive_design", docs)
        self.assertIn("Tiny UI", docs)


class CapabilityPackUiAmendmentTests(unittest.TestCase):
    def _canonical(self, name: str) -> str:
        path = REPO_ROOT / "project_agents" / "canonical" / "p1" / f"{name}.md"
        return path.read_text(encoding="utf-8")

    def test_ui_ownership_separation(self) -> None:
        designer = self._canonical("platform-product-ui-designer")
        frontend = self._canonical("platform-frontend-developer")
        ux = self._canonical("platform-ux-architect")
        auditor = self._canonical("platform-accessibility-auditor")
        finish = self._canonical("platform-ui-finish-gate-reviewer")
        self.assertIn("you do not implement `ui/` source", designer)
        self.assertIn("React/TypeScript implementation", designer)
        self.assertIn("cannot certify accessibility or final UI finish", designer)
        self.assertIn("Does not own product design authority", frontend)
        self.assertIn("Do not redesign material UX while coding", frontend)
        self.assertIn("Do not self-certify accessibility or final finish", frontend)
        self.assertIn("Do not produce pixel/component specifications", ux)
        self.assertIn("independent of the designer and frontend writer", auditor)
        self.assertIn("Designer intent is not certification", auditor)
        self.assertIn("READ-ONLY", finish)
        self.assertIn("Independent of designer and frontend writer", finish)
        matrix = _matrix()
        self.assertNotIn("ux_operational_design", matrix["platform-frontend-developer"]["requiredSkills"])
        self.assertIn("ux_operational_design", matrix["platform-product-ui-designer"]["requiredSkills"])
        self.assertIn("ux_operational_design", matrix["platform-ux-architect"]["requiredSkills"])
        self.assertNotIn("web_interface_quality", matrix["platform-ux-architect"]["requiredSkills"])
        self.assertIn("accessibility_inclusive_design", matrix["platform-accessibility-auditor"]["requiredSkills"])
        self.assertIn("evidence_bundle", matrix["platform-accessibility-auditor"]["requiredSkills"])
        self.assertIn("evidence_bundle", matrix["platform-ui-finish-gate-reviewer"]["requiredSkills"])

    def test_third_party_provenance_records_exact_shas_and_licences(self) -> None:
        text = (REPO_ROOT / "project_agents" / "skills" / "THIRD_PARTY_PROVENANCE.md").read_text(
            encoding="utf-8"
        )
        for repo, sha in UPSTREAM_PROVENANCE_SHAS.items():
            self.assertIn(repo, text)
            self.assertIn(sha, text)
        self.assertGreaterEqual(text.lower().count("declared licence"), 5)
        self.assertIn("MIT", text)
        self.assertIn("No substantial upstream text was copied", text)
        self.assertIn("Do not import Tailwind", text)
        combined_skills = "\n".join(
            (REPO_ROOT / "project_agents" / "skills" / skill_name / "SKILL.md").read_text(encoding="utf-8")
            for skill_name in UI_SKILLS
        )
        for skill_name in UI_SKILLS:
            self.assertIn(f"`{skill_name}`", text)
        self.assertNotRegex(combined_skills, r"(?i)adopt Tailwind|use Material Design|use Bootstrap")
        self.assertNotRegex(combined_skills, r"(?i)npm install|add a new npm")
        self.assertIn("Do not import Tailwind", combined_skills)

    def test_handoff_contract_and_acceptance_matrix(self) -> None:
        designer = self._canonical("platform-product-ui-designer")
        ux_skill = (REPO_ROOT / "project_agents" / "skills" / "ux_operational_design" / "SKILL.md").read_text(
            encoding="utf-8"
        )
        for required in (
            "Goal / user job",
            "Primary decision/action",
            "Information architecture",
            "Layout/hierarchy",
            "Key components",
            "Relevant design tokens",
            "Interaction model",
            "Applicable UI states",
            "Empty/error/recovery behaviour",
            "Responsive behaviour",
            "Accessibility intent",
            "Large-estate considerations",
            "Performance considerations",
            "RBAC/tenant considerations",
            "Open assumptions",
            "Acceptance criteria",
        ):
            self.assertTrue(required in designer or required in ux_skill, required)
        combined = designer + ux_skill
        for row in (
            "PASS",
            "FAIL",
            "NOT_APPLICABLE",
            "NOT_RUN",
            "freshness/trust",
            "keyboard",
            "large-estate",
            "destructive-action safety",
        ):
            self.assertIn(row, combined)
        self.assertIn("Never hide `NOT_RUN`", combined)

    def test_effective_diff_excludes_product_runtime(self) -> None:
        import subprocess

        def _has_commit(ref: str) -> bool:
            return (
                subprocess.run(
                    ["git", "cat-file", "-e", f"{ref}^{{commit}}"],
                    cwd=REPO_ROOT,
                    capture_output=True,
                    check=False,
                ).returncode
                == 0
            )

        candidates = [
            os.environ.get("BASE_SHA", "").strip(),
            os.environ.get("GITHUB_BASE_SHA", "").strip(),
            "origin/main",
            "main",
        ]
        base = next((item for item in candidates if item and _has_commit(item)), None)
        if base is None:
            self.skipTest("no resolvable base commit for runtime-diff check")
        named = subprocess.check_output(
            ["git", "diff", "--name-only", f"{base}...HEAD"],
            cwd=REPO_ROOT,
            text=True,
        )
        forbidden_prefixes = ("ui/", "core/", "sensor/", "agent/")
        forbidden_files = (
            "ui/package.json",
            "core/pom.xml",
            "package-lock.json",
        )
        for line in named.splitlines():
            self.assertFalse(
                any(line == prefix.rstrip("/") or line.startswith(prefix) for prefix in forbidden_prefixes),
                line,
            )
            self.assertNotIn(line, forbidden_files)


if __name__ == "__main__":
    unittest.main()
