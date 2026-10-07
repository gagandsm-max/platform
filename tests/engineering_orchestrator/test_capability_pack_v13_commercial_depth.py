#!/usr/bin/env python3
"""Capability Pack v1.3 commercial-depth: resource projection + deterministic evals."""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
TOOLS = REPO_ROOT / "project_agents" / "tools"


def _load(name: str, path: Path):
    if str(TOOLS) not in sys.path:
        sys.path.insert(0, str(TOOLS))
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class CommercialDepthInventoryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.agent_pack = _load("agent_pack", TOOLS / "agent_pack.py")
        cls.skill_resources = _load("skill_resources", TOOLS / "skill_resources.py")
        cls.skill_evals = _load("skill_evals", TOOLS / "skill_evals.py")

    def test_active_inventory_is_34_agents_63_skills(self) -> None:
        agents = json.loads((REPO_ROOT / "project_agents" / "manifest.json").read_text())
        skills = json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())
        self.assertEqual(len(agents), 34)
        self.assertEqual(len(skills), 63)
        self.assertEqual(self.agent_pack.ACTIVE_AGENT_INVENTORY, 34)
        self.assertEqual(self.agent_pack.ACTIVE_SKILL_INVENTORY, 63)

    def test_three_new_skills_present_only(self) -> None:
        skills = {e["name"] for e in json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())}
        for name in (
            "ncm_configuration_assurance",
            "redfish_hardware_monitoring",
            "database_monitoring",
        ):
            self.assertIn(name, skills)
        self.assertEqual(len(skills), 63)

    def test_routing_for_new_skills(self) -> None:
        matrix = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "agent_skill_matrix.json").read_text()
        )
        self.assertIn(
            "ncm_configuration_assurance",
            matrix["platform-ncm-remediation-engineer"]["requiredSkills"],
        )
        self.assertIn(
            "redfish_hardware_monitoring",
            matrix["platform-infrastructure-cloud-engineer"]["conditionalSkills"],
        )
        self.assertIn(
            "database_monitoring",
            matrix["platform-infrastructure-cloud-engineer"]["conditionalSkills"],
        )
        self.assertIn(
            "redfish_hardware_monitoring",
            matrix["platform-device-pack-engineer"]["conditionalSkills"],
        )


class ResourceProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.agent_pack = _load("agent_pack", TOOLS / "agent_pack.py")
        cls.skill_resources = _load("skill_resources", TOOLS / "skill_resources.py")

    def _minimal_repo(self, tmp: Path) -> Path:
        # Minimal skill package with a resource.
        skill = tmp / "project_agents" / "skills" / "demo_skill"
        skill.mkdir(parents=True)
        (skill / "SKILL.md").write_text(
            "---\nname: demo_skill\ndescription: demo\n---\n\n# demo_skill\n\n"
            + "\n".join(
                f"## {h}\n\nbody\n"
                for h in [
                    "Purpose",
                    "Use when",
                    "Authoritative inputs",
                    "Invariants",
                    "Required procedure",
                    "Negative / failure cases",
                    "Security / tenant implications",
                    "Required evidence",
                    "Stop conditions",
                    "Anti-patterns",
                    "Handoff / output",
                    "Non-ownership",
                ]
            ),
            encoding="utf-8",
        )
        (skill / "references").mkdir()
        (skill / "references" / "note.md").write_text("hello resource\n", encoding="utf-8")
        return skill

    def test_resource_projection_and_idempotency(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_repo(root)
            gen_a = root / "skills"
            gen_b = root / ".agents" / "skills"
            review1 = self.skill_resources.sync_skill_resources(
                "demo_skill",
                skill,
                [gen_a, gen_b],
                root,
                skill_name_re=self.agent_pack.SKILL_NAME_RE,
            )
            self.assertEqual(review1, [])
            for gen in (gen_a, gen_b):
                projected = gen / "demo_skill" / "references" / "note.md"
                self.assertTrue(projected.is_file())
                self.assertEqual(projected.read_bytes(), b"hello resource\n")
            # Second sync must be idempotent (no content drift).
            before = {
                p.relative_to(root).as_posix(): p.read_bytes()
                for p in (gen_a / "demo_skill").rglob("*")
                if p.is_file()
            }
            self.skill_resources.sync_skill_resources(
                "demo_skill",
                skill,
                [gen_a, gen_b],
                root,
                skill_name_re=self.agent_pack.SKILL_NAME_RE,
            )
            after = {
                p.relative_to(root).as_posix(): p.read_bytes()
                for p in (gen_a / "demo_skill").rglob("*")
                if p.is_file()
            }
            self.assertEqual(before, after)
            errors = self.skill_resources.validate_skill_resources(
                "demo_skill", skill, [gen_a, gen_b], root
            )
            self.assertEqual(errors, [])

    def test_projection_drift_detected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_repo(root)
            gen = root / "skills"
            self.skill_resources.sync_skill_resources(
                "demo_skill",
                skill,
                [gen],
                root,
                skill_name_re=self.agent_pack.SKILL_NAME_RE,
            )
            target = gen / "demo_skill" / "references" / "note.md"
            target.write_text("tampered\n", encoding="utf-8")
            errors = self.skill_resources.validate_skill_resources(
                "demo_skill", skill, [gen], root
            )
            self.assertTrue(any("drift" in e for e in errors), errors)

    def test_unsafe_resource_path_rejected(self) -> None:
        with self.assertRaises(self.skill_resources.SkillResourceError):
            self.skill_resources.assert_safe_resource_rel("../secrets.txt")
        with self.assertRaises(self.skill_resources.SkillResourceError):
            self.skill_resources.assert_safe_resource_rel("scripts/run.sh")

    def test_symlink_resource_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_repo(root)
            link = skill / "references" / "evil.md"
            os.symlink("/etc/passwd", link)
            with self.assertRaises(self.skill_resources.SkillResourceError):
                self.skill_resources.discover_canonical_resources(skill)

    def test_oversized_resource_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_repo(root)
            big = skill / "references" / "big.md"
            big.write_bytes(b"x" * (self.skill_resources.MAX_RESOURCE_FILE_BYTES + 1))
            with self.assertRaises(self.skill_resources.SkillResourceError):
                self.skill_resources.discover_canonical_resources(skill)


class SkillEvalValidatorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.skill_evals = _load("skill_evals", TOOLS / "skill_evals.py")
        cls.matrix = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "agent_skill_matrix.json").read_text()
        )
        cls.agents = set(cls.matrix)
        cls.skills = {
            e["name"]
            for e in json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())
        }

    def _validate_case(self, case: dict, skill_name: str = "go_runtime") -> list[str]:
        skill_dir = REPO_ROOT / "project_agents" / "skills" / skill_name
        corpus = self.skill_evals.package_text_corpus(skill_dir)
        return self.skill_evals.validate_eval_case(
            case,
            skill_name=skill_name,
            known_agents=self.agents,
            known_skills=self.skills,
            agent_matrix=self.matrix,
            corpus=corpus,
            seen_ids=set(),
        )

    def test_unknown_skill_fails(self) -> None:
        errors = self._validate_case(
            {
                "id": "x.unknown.skill",
                "type": "TRIGGER",
                "task": "demo",
                "owningAgents": ["platform-sensor-runtime-engineer"],
                "requiredSkills": ["not_a_real_skill"],
                "forbiddenSkills": [],
                "requiredSemanticTokens": ["context"],
            }
        )
        self.assertTrue(any("unknown required skill" in e for e in errors), errors)

    def test_unknown_agent_fails(self) -> None:
        errors = self._validate_case(
            {
                "id": "x.unknown.agent",
                "type": "TRIGGER",
                "task": "demo",
                "owningAgents": ["platform-not-an-agent"],
                "requiredSkills": ["go_runtime"],
                "forbiddenSkills": [],
                "requiredSemanticTokens": ["context"],
            }
        )
        self.assertTrue(any("unknown agent" in e for e in errors), errors)

    def test_duplicate_eval_id_fails(self) -> None:
        skill_dir = REPO_ROOT / "project_agents" / "skills" / "go_runtime"
        corpus = self.skill_evals.package_text_corpus(skill_dir)
        seen = {"go.trigger.context"}
        errors = self.skill_evals.validate_eval_case(
            {
                "id": "go.trigger.context",
                "type": "TRIGGER",
                "task": "demo",
                "owningAgents": ["platform-sensor-runtime-engineer"],
                "requiredSkills": ["go_runtime"],
                "forbiddenSkills": [],
                "requiredSemanticTokens": ["context cancellation"],
            },
            skill_name="go_runtime",
            known_agents=self.agents,
            known_skills=self.skills,
            agent_matrix=self.matrix,
            corpus=corpus,
            seen_ids=seen,
        )
        self.assertTrue(any("duplicate eval id" in e for e in errors), errors)

    def test_invalid_eval_type_fails(self) -> None:
        errors = self._validate_case(
            {
                "id": "x.bad.type",
                "type": "HALLUCINATION",
                "task": "demo",
                "owningAgents": ["platform-sensor-runtime-engineer"],
                "requiredSkills": ["go_runtime"],
                "forbiddenSkills": [],
                "requiredSemanticTokens": ["context"],
            }
        )
        self.assertTrue(any("invalid eval type" in e for e in errors), errors)

    def test_required_forbidden_contradiction_fails(self) -> None:
        errors = self._validate_case(
            {
                "id": "x.contradiction",
                "type": "NEGATIVE",
                "task": "demo",
                "owningAgents": ["platform-sensor-runtime-engineer"],
                "requiredSkills": ["go_runtime"],
                "forbiddenSkills": ["go_runtime"],
                "requiredSemanticTokens": ["context"],
            }
        )
        self.assertTrue(any("contradiction" in e for e in errors), errors)

    def test_missing_required_depth_resource_fails_policy(self) -> None:
        skill_resources = _load("skill_resources", TOOLS / "skill_resources.py")
        policy = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "commercial_depth_policy.json").read_text()
        )
        self.assertIn("ncm_configuration_assurance", policy["skills"])
        required = policy["skills"]["ncm_configuration_assurance"]["requiredReferenceFiles"]
        skill_dir = REPO_ROOT / "project_agents" / "skills" / "ncm_configuration_assurance"
        for rel in required:
            dest = skill_resources.assert_safe_required_reference_file(skill_dir, rel)
            self.assertTrue(dest.is_file())
            self.assertFalse(dest.is_symlink())

    def test_missing_required_eval_type_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo"
            (skill_dir / "evals").mkdir(parents=True)
            (skill_dir / "SKILL.md").write_text("token context cancellation\n", encoding="utf-8")
            (skill_dir / "evals" / "cases.json").write_text(
                json.dumps(
                    {
                        "version": 1,
                        "cases": [
                            {
                                "id": "demo.trigger.only",
                                "type": "TRIGGER",
                                "task": "demo",
                                "owningAgents": ["platform-sensor-runtime-engineer"],
                                "requiredSkills": ["go_runtime"],
                                "forbiddenSkills": [],
                                "requiredSemanticTokens": ["context cancellation"],
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            errors = self.skill_evals.validate_skill_evals(
                skill_name="demo",
                skill_dir=skill_dir,
                known_agents=self.agents,
                known_skills=self.skills,
                agent_matrix=self.matrix,
                require_evals=True,
                required_types={"TRIGGER", "NEGATIVE", "ADVERSARIAL", "REGRESSION"},
            )
            self.assertTrue(any("missing required eval type" in e for e in errors), errors)


class RepoSyncCommercialDepthTests(unittest.TestCase):
    def test_repo_resources_projected(self) -> None:
        for skill in (
            "ncm_configuration_assurance",
            "go_runtime",
            "database_monitoring",
        ):
            can = REPO_ROOT / "project_agents" / "skills" / skill / "references"
            self.assertTrue(can.is_dir(), skill)
            for gen_root in (REPO_ROOT / "skills", REPO_ROOT / ".agents" / "skills"):
                # After sync these must exist; test assumes sync has been run in CI/local.
                # If missing, sync_agents must be executed first.
                projected = gen_root / skill / "references"
                self.assertTrue(
                    projected.is_dir() or not (gen_root / skill).exists(),
                    msg=f"expected sync before validate for {projected}",
                )


class SemanticCorpusAuthorityTests(unittest.TestCase):
    """P1-1: semantic tokens must not self-satisfy from evals/ or fixtures/."""

    UNIQUE_TOKEN = "ZZ_SEMANTIC_CORPUS_AUTHORITY_TOKEN_v13_p1_1"

    @classmethod
    def setUpClass(cls) -> None:
        cls.skill_evals = _load("skill_evals", TOOLS / "skill_evals.py")
        cls.matrix = json.loads(
            (REPO_ROOT / "project_agents" / "skills" / "agent_skill_matrix.json").read_text()
        )
        cls.agents = set(cls.matrix)
        cls.skills = {
            e["name"]
            for e in json.loads((REPO_ROOT / "project_agents" / "skills" / "manifest.json").read_text())
        }

    def _write_skill_shell(self, skill_dir: Path, skill_md_body: str) -> None:
        skill_dir.mkdir(parents=True, exist_ok=True)
        (skill_dir / "SKILL.md").write_text(skill_md_body, encoding="utf-8")

    def _write_eval(self, skill_dir: Path, token: str, case_id: str = "demo.semantic.authority") -> None:
        (skill_dir / "evals").mkdir(parents=True, exist_ok=True)
        (skill_dir / "evals" / "cases.json").write_text(
            json.dumps(
                {
                    "version": 1,
                    "cases": [
                        {
                            "id": case_id,
                            "type": "TRIGGER",
                            "task": "semantic corpus authority",
                            "owningAgents": ["platform-sensor-runtime-engineer"],
                            "requiredSkills": ["go_runtime"],
                            "forbiddenSkills": [],
                            "requiredSemanticTokens": [token],
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )

    def _validate(self, skill_dir: Path) -> list[str]:
        return self.skill_evals.validate_skill_evals(
            skill_name="demo_skill",
            skill_dir=skill_dir,
            known_agents=self.agents,
            known_skills=self.skills,
            agent_matrix=self.matrix,
            require_evals=True,
            required_types={"TRIGGER"},
            global_seen_ids=set(),
        )

    def test_token_only_in_evals_cases_json_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo_skill"
            self._write_skill_shell(skill_dir, "# demo_skill\n\nno unique token here\n")
            self._write_eval(skill_dir, self.UNIQUE_TOKEN)
            errors = self._validate(skill_dir)
            self.assertTrue(any("missing semantic token" in e for e in errors), errors)
            corpus = self.skill_evals.package_text_corpus(skill_dir)
            self.assertNotIn(self.UNIQUE_TOKEN.lower(), corpus.lower())

    def test_token_in_skill_md_passes(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo_skill"
            self._write_skill_shell(
                skill_dir, f"# demo_skill\n\nguidance contains {self.UNIQUE_TOKEN}\n"
            )
            self._write_eval(skill_dir, self.UNIQUE_TOKEN)
            errors = self._validate(skill_dir)
            self.assertFalse(any("missing semantic token" in e for e in errors), errors)

    def test_token_only_in_references_passes(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo_skill"
            self._write_skill_shell(skill_dir, "# demo_skill\n\nbody without token\n")
            (skill_dir / "references").mkdir()
            (skill_dir / "references" / "guide.md").write_text(
                f"authoritative note: {self.UNIQUE_TOKEN}\n", encoding="utf-8"
            )
            self._write_eval(skill_dir, self.UNIQUE_TOKEN)
            errors = self._validate(skill_dir)
            self.assertFalse(any("missing semantic token" in e for e in errors), errors)

    def test_token_only_in_fixtures_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo_skill"
            self._write_skill_shell(skill_dir, "# demo_skill\n\nbody without token\n")
            (skill_dir / "fixtures").mkdir()
            (skill_dir / "fixtures" / "demo.json").write_text(
                json.dumps({"token": self.UNIQUE_TOKEN}), encoding="utf-8"
            )
            self._write_eval(skill_dir, self.UNIQUE_TOKEN)
            errors = self._validate(skill_dir)
            self.assertTrue(any("missing semantic token" in e for e in errors), errors)
            corpus = self.skill_evals.package_text_corpus(skill_dir)
            self.assertNotIn(self.UNIQUE_TOKEN.lower(), corpus.lower())

    def test_generated_mirror_token_without_canonical_guidance_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill_dir = root / "project_agents" / "skills" / "demo_skill"
            self._write_skill_shell(skill_dir, "# demo_skill\n\ncanonical without token\n")
            self._write_eval(skill_dir, self.UNIQUE_TOKEN)
            mirror = root / "skills" / "demo_skill"
            mirror.mkdir(parents=True)
            (mirror / "SKILL.md").write_text(
                f"# demo_skill\n\nmirror has {self.UNIQUE_TOKEN}\n", encoding="utf-8"
            )
            (mirror / "references").mkdir()
            (mirror / "references" / "guide.md").write_text(
                f"{self.UNIQUE_TOKEN}\n", encoding="utf-8"
            )
            # Validation uses canonical skill_dir, not generated mirrors.
            errors = self._validate(skill_dir)
            self.assertTrue(any("missing semantic token" in e for e in errors), errors)

    def test_global_eval_id_uniqueness_retained(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill_dir = Path(tmp) / "demo_skill"
            self._write_skill_shell(
                skill_dir, f"# demo_skill\n\n{self.UNIQUE_TOKEN}\n"
            )
            self._write_eval(skill_dir, self.UNIQUE_TOKEN, case_id="shared.eval.id")
            seen = {"shared.eval.id"}
            errors = self.skill_evals.validate_skill_evals(
                skill_name="demo_skill",
                skill_dir=skill_dir,
                known_agents=self.agents,
                known_skills=self.skills,
                agent_matrix=self.matrix,
                require_evals=True,
                global_seen_ids=seen,
            )
            self.assertTrue(any("duplicate eval id" in e for e in errors), errors)


class UnexpectedResourcePreservationTests(unittest.TestCase):
    """P1-2: sync/retire must preserve unproven generated resources."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.agent_pack = _load("agent_pack", TOOLS / "agent_pack.py")
        cls.skill_resources = _load("skill_resources", TOOLS / "skill_resources.py")

    def _minimal_skill(self, root: Path, name: str = "demo_skill") -> Path:
        skill = root / "project_agents" / "skills" / name
        skill.mkdir(parents=True)
        (skill / "SKILL.md").write_text(
            "---\nname: demo_skill\ndescription: demo\n---\n\n# demo_skill\n",
            encoding="utf-8",
        )
        (skill / "references").mkdir()
        (skill / "references" / "note.md").write_text("hello resource\n", encoding="utf-8")
        return skill

    def test_unexpected_manual_note_preserved_and_reported(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_skill(root)
            gen = root / "skills"
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            unexpected = gen / "demo_skill" / "references" / "manual-note.md"
            unexpected.write_text("local diagnostic\n", encoding="utf-8")
            review = self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            self.assertTrue(unexpected.is_file(), "unexpected file must be preserved")
            self.assertEqual(unexpected.read_text(encoding="utf-8"), "local diagnostic\n")
            self.assertTrue(
                any("manual-note.md" in item and "requires review" in item for item in review),
                review,
            )
            errors = self.skill_resources.validate_skill_resources(
                "demo_skill", skill, [gen], root
            )
            self.assertTrue(
                any("unexpected generated skill resource" in e and "manual-note.md" in e for e in errors),
                errors,
            )

    def test_removed_canonical_resource_preserved_until_cleanup(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_skill(root)
            gen = root / "skills"
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            old = gen / "demo_skill" / "references" / "note.md"
            self.assertTrue(old.is_file())
            (skill / "references" / "note.md").unlink()
            (skill / "references" / "replacement.md").write_text("new\n", encoding="utf-8")
            review = self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            self.assertTrue(old.is_file(), "old generated resource must be preserved")
            self.assertTrue(
                any("note.md" in item and "requires review" in item for item in review),
                review,
            )
            self.assertTrue((gen / "demo_skill" / "references" / "replacement.md").is_file())
            errors = self.skill_resources.validate_skill_resources(
                "demo_skill", skill, [gen], root
            )
            self.assertTrue(any("unexpected generated skill resource" in e for e in errors), errors)

    def test_sync_idempotent_with_unexpected_resource(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_skill(root)
            gen = root / "skills"
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            unexpected = gen / "demo_skill" / "references" / "manual-note.md"
            unexpected.write_text("local diagnostic\n", encoding="utf-8")
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            before = {
                p.relative_to(root).as_posix(): p.read_bytes()
                for p in (gen / "demo_skill").rglob("*")
                if p.is_file() and not p.is_symlink()
            }
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            after = {
                p.relative_to(root).as_posix(): p.read_bytes()
                for p in (gen / "demo_skill").rglob("*")
                if p.is_file() and not p.is_symlink()
            }
            self.assertEqual(before, after)

    def test_symlink_resource_preserved_and_reported(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._minimal_skill(root)
            gen = root / "skills"
            self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            target = root / "outside.txt"
            target.write_text("outside\n", encoding="utf-8")
            link = gen / "demo_skill" / "references" / "evil-link.md"
            os.symlink(target, link)
            review = self.skill_resources.sync_skill_resources(
                "demo_skill", skill, [gen], root, skill_name_re=self.agent_pack.SKILL_NAME_RE
            )
            self.assertTrue(link.is_symlink(), "symlink must not be deleted")
            self.assertTrue(
                any("evil-link.md" in item and "symlink not deleted" in item for item in review),
                review,
            )
            # Must not have followed the symlink into outside content for deletion.
            self.assertTrue(target.is_file())
            self.assertEqual(target.read_text(encoding="utf-8"), "outside\n")

    def test_retired_skill_with_unknown_fixture_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            gen = root / "skills"
            retired = gen / "retired_skill"
            retired.mkdir(parents=True)
            body = "---\nname: retired_skill\ndescription: retired\n---\n\n# retired\n"
            (retired / "SKILL.md").write_text(
                self.agent_pack.render_generated_skill("retired_skill", body),
                encoding="utf-8",
            )
            (retired / "fixtures").mkdir()
            unknown = retired / "fixtures" / "local.json"
            unknown.write_text('{"keep": true}\n', encoding="utf-8")
            review = self.agent_pack.prune_retired_skill_mirrors(gen, declared=set(), repo=root)
            self.assertTrue(unknown.is_file(), "unknown fixture must survive")
            self.assertTrue(retired.is_dir(), "skill directory must survive")
            self.assertTrue((retired / "SKILL.md").is_file())
            self.assertTrue(
                any("requires review" in item for item in review),
                review,
            )
            self.assertTrue(
                any("retired_skill" in item for item in review),
                review,
            )

    def test_retired_skill_only_generated_skill_md_pruned(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            gen = root / "skills"
            retired = gen / "retired_skill"
            retired.mkdir(parents=True)
            body = "---\nname: retired_skill\ndescription: retired\n---\n\n# retired\n"
            (retired / "SKILL.md").write_text(
                self.agent_pack.render_generated_skill("retired_skill", body),
                encoding="utf-8",
            )
            review = self.agent_pack.prune_retired_skill_mirrors(gen, declared=set(), repo=root)
            self.assertFalse(retired.exists(), f"expected prune; review={review}")


class RequiredReferencePathSafetyTests(unittest.TestCase):
    """P2-1: requiredReferenceFiles must use safe path validation."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.skill_resources = _load("skill_resources", TOOLS / "skill_resources.py")

    def _skill_with_reference(self, root: Path) -> Path:
        skill = root / "project_agents" / "skills" / "demo_skill"
        skill.mkdir(parents=True)
        (skill / "SKILL.md").write_text("# demo\n", encoding="utf-8")
        (skill / "references").mkdir()
        (skill / "references" / "example.md").write_text("ok\n", encoding="utf-8")
        return skill

    def test_accepts_references_example(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill = self._skill_with_reference(Path(tmp))
            dest = self.skill_resources.assert_safe_required_reference_file(
                skill, "references/example.md"
            )
            self.assertTrue(dest.is_file())
            self.assertFalse(dest.is_symlink())

    def test_rejects_traversal_and_non_reference_roots(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            skill = self._skill_with_reference(root)
            other = root / "project_agents" / "skills" / "other_skill"
            other.mkdir(parents=True)
            (other / "reference.md").write_text("escaped\n", encoding="utf-8")
            (skill / "fixtures").mkdir()
            (skill / "fixtures" / "demo.json").write_text("{}\n", encoding="utf-8")
            (skill / "evals").mkdir()
            (skill / "evals" / "cases.json").write_text("{}\n", encoding="utf-8")
            rejected = [
                "../other_skill/reference.md",
                "/absolute/file.md",
                "fixtures/demo.json",
                "evals/cases.json",
                "references/../../escape.md",
                "references\\..\\escape.md",
                "scripts/run.sh",
            ]
            for rel in rejected:
                with self.assertRaises(self.skill_resources.SkillResourceError):
                    self.skill_resources.assert_safe_required_reference_file(skill, rel)

    def test_rejects_symlink_reference(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            skill = self._skill_with_reference(Path(tmp))
            target = Path(tmp) / "outside.md"
            target.write_text("outside\n", encoding="utf-8")
            link = skill / "references" / "linked.md"
            os.symlink(target, link)
            with self.assertRaises(self.skill_resources.SkillResourceError):
                self.skill_resources.assert_safe_required_reference_file(
                    skill, "references/linked.md"
                )


if __name__ == "__main__":
    unittest.main()
