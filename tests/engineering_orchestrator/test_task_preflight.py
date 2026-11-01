"""Phase A: real Git trees/index/worktrees plus bounded remote/failure seams.

No GitHub writes. Fixture pushes target a disposable local bare repository.
Only remote_heads is replaced; production origin, native policy and Git checks
remain active. Authentication/CI certification is deliberately never simulated
as achieved by these tests.
"""
from __future__ import annotations

import contextlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "project_agents/tools"))
import control_plane as native
import task_preflight as guard
from tests.engineering_orchestrator.test_control_plane_v2 import _task, _handoff


class PhysicalPreflightTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.repo = self.directory / "writer"
        self.repo.mkdir()
        self.remote = self.directory / "remote.git"
        self.git("init", "-q", "--initial-branch=main")
        self.git("config", "user.name", "Synthetic Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        for name in guard.NATIVE_INPUTS:
            dest = self.repo / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, dest)
        self.put("src/one.txt", "base\n")
        self.put("src/two.txt", "base\n")
        self.put("outside.txt", "protected by scope\n")
        self.put("project_documentation/AGENTS.md", "fixture governance\n")
        (self.repo / "AGENTS.md").symlink_to("project_documentation/AGENTS.md")
        self.commit()
        self.base = self.git("rev-parse", "HEAD")
        self.git("init", "-q", "--bare", "--initial-branch=main", str(self.remote))
        self.git("push", "-q", str(self.remote), "main")
        self.git("remote", "add", "origin", guard.REMOTE)
        self.git("switch", "-q", "-c", "feat/task")
        self.manifest = _task(writerTool="codex", verifierTool="antigravity",
            gatekeeper="chatgpt-github", worktree=str(self.repo), authoritativeMain=self.base,
            exactHead=self.base, branch="feat/task", status="IMPLEMENTATION", scope=["src/"])
        self.remote_patch = patch.object(guard, "remote_heads", side_effect=self.remote_observation)
        self.remote_patch.start()
        self.addCleanup(self.remote_patch.stop)

    def git(self, *args):
        result = subprocess.run(["git", "-C", str(self.repo), *args], check=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=guard._environment())
        return result.stdout.decode().strip()

    def put(self, path, content):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def commit(self):
        self.git("add", "--all")
        self.git("commit", "-qm", "fixture commit")
        if hasattr(self, "manifest"):
            self.manifest["exactHead"] = self.git("rev-parse", "HEAD")

    def publish(self):
        self.git("push", "-q", str(self.remote), "HEAD:refs/heads/feat/task")

    def remote_observation(self, branch):
        rows = self.git("ls-remote", "--refs", str(self.remote), "refs/heads/main", "refs/heads/" + branch)
        refs = dict(reversed(row.split("\t")) for row in rows.splitlines())
        return {"main": refs["refs/heads/main"], "branch": refs.get("refs/heads/" + branch)}

    def inspect(self):
        return native.inspect_task_preflight(self.repo, self.manifest)

    def blocked(self, code):
        with self.assertRaisesRegex(guard.PreflightError, "^" + code + "$"):
            self.inspect()

    def test_new_writer_branch_is_inspection_only(self):
        result = self.inspect()
        self.assertEqual(result["lane"], "WRITER_IN_PROGRESS")
        self.assertIsNone(result["remoteBranchHead"])
        self.assertFalse(result["authorized"])
        self.assertFalse(result["mergeReady"])
        self.assertEqual(result["authorization"], "UNVERIFIED")
        self.assertEqual(result["ciEvidence"], "UNVERIFIED")
        self.assertEqual(result["independentReview"], "UNVERIFIED")
        self.assertEqual(result["result"], "INSPECTION_ONLY")

    def test_unpushed_commit_on_existing_remote_is_valid_writer_progress(self):
        self.publish()
        self.put("src/one.txt", "local unpushed commit\n")
        self.commit()
        result = self.inspect()
        self.assertEqual(result["remoteBranchHead"], self.base)
        self.assertEqual(result["exactHead"], self.manifest["exactHead"])
        self.assertEqual(result["changes"]["committed"], ["src/one.txt"])
        self.assertFalse(result["authorized"])

    def test_unpushed_commit_rejected_for_every_review_state(self):
        self.publish()
        self.put("src/one.txt", "local\n")
        self.commit()
        for state in guard.REVIEW_STATES - {"REALITY_GATE"}:
            with self.subTest(state=state):
                self.manifest["status"] = state
                self.blocked("UNPUBLISHED_REVIEW_HEAD")

    def test_missing_remote_review_rejected(self):
        self.manifest["status"] = "SOURCE_REVIEW"
        self.blocked("UNPUBLISHED_REVIEW_HEAD")

    def test_published_detached_review_is_candidate_only(self):
        self.put("src/one.txt", "published\n")
        self.commit()
        self.publish()
        self.git("switch", "--detach", "-q", "HEAD")
        self.manifest["status"] = "SOURCE_REVIEW"
        result = self.inspect()
        self.assertEqual(result["lane"], "PUBLISHED_REVIEW_CANDIDATE")
        self.assertEqual(result["independentReview"], "UNVERIFIED")

    def test_detached_writer_rejected(self):
        self.git("switch", "--detach", "-q", "HEAD")
        self.blocked("GIT_COMMAND_FAILED")

    def test_wrong_branch_rejected(self):
        self.git("switch", "-q", "-c", "feat/wrong")
        self.blocked("WRONG_WRITER_BRANCH")

    def test_main_writer_rejected(self):
        self.manifest["branch"] = "main"
        self.blocked("INVALID_TASK_BRANCH")

    def test_wrong_worktree_rejected(self):
        self.manifest["worktree"] = str(self.directory)
        self.blocked("WRONG_WORKTREE")

    def test_symlink_worktree_alias_rejected(self):
        alias = self.directory / "alias"
        alias.symlink_to(self.repo, target_is_directory=True)
        with self.assertRaisesRegex(guard.PreflightError, "WORKTREE_ALIAS"):
            guard.inspect_task(alias, self.manifest)

    def test_wrong_origin_and_push_origin_rejected(self):
        for key in ("remote.origin.url", "remote.origin.pushurl"):
            with self.subTest(key=key):
                self.git("config", key, "https://example.invalid/other.git")
                self.blocked("WRONG_ORIGIN")
                self.git("config", key, guard.REMOTE)

    def test_wrong_repository_rejected(self):
        self.manifest["repository"] = "other/platform"
        self.blocked("WRONG_REPOSITORY")

    def test_stale_main_and_head_rejected(self):
        for key, code in (("authoritativeMain", "STALE_MAIN"), ("exactHead", "STALE_HEAD")):
            with self.subTest(key=key):
                self.manifest[key] = "a" * 40
                self.blocked(code)
                self.manifest[key] = self.base

    def test_remote_ahead_or_diverged_blocks_writer(self):
        self.put("src/one.txt", "remote ahead\n")
        self.commit()
        self.publish()
        self.git("reset", "--hard", self.base)  # Only disposable fixture.
        self.manifest["exactHead"] = self.base
        self.blocked("GIT_COMMAND_FAILED")
        self.put("src/two.txt", "local diverged\n")
        self.commit()
        self.blocked("GIT_COMMAND_FAILED")

    def test_staged_unstaged_cancellation_keeps_both_surfaces(self):
        self.put("src/one.txt", "staged\n")
        self.git("add", "src/one.txt")
        self.put("src/one.txt", "base\n")
        result = self.inspect()["changes"]
        self.assertEqual(result["staged"], ["src/one.txt"])
        self.assertEqual(result["unstaged"], ["src/one.txt"])

    def test_outside_scope_staged_cancellation_rejected(self):
        self.put("outside.txt", "staged\n")
        self.git("add", "outside.txt")
        self.put("outside.txt", "protected by scope\n")
        self.blocked("OUTSIDE_SCOPE")

    def test_staged_only_and_untracked_inventory(self):
        self.put("src/one.txt", "stage\n")
        self.git("add", "src/one.txt")
        self.put("src/untracked.txt", "untracked\n")
        result = self.inspect()["changes"]
        self.assertEqual(result["staged"], ["src/one.txt"])
        self.assertEqual(result["untracked"], ["src/untracked.txt"])

    def test_untracked_outside_scope_rejected(self):
        self.put("foreign.txt", "retain\n")
        self.blocked("OUTSIDE_SCOPE")
        self.assertEqual((self.repo / "foreign.txt").read_text(), "retain\n")

    def test_ignored_untracked_outside_scope_rejected(self):
        self.put(".git/info/exclude", "hidden\n")
        self.put("hidden", "retain\n")
        self.blocked("OUTSIDE_SCOPE")

    def test_rename_checks_both_endpoints(self):
        self.git("mv", "outside.txt", "src/moved.txt")
        self.blocked("OUTSIDE_SCOPE")
        self.assertEqual(guard.changes(self.repo, self.base)["staged"], ["outside.txt", "src/moved.txt"])

    def test_in_scope_rename_reports_both_endpoints(self):
        self.git("mv", "src/one.txt", "src/renamed.txt")
        self.assertEqual(self.inspect()["changes"]["staged"], ["src/one.txt", "src/renamed.txt"])

    def test_scope_prefix_collision(self):
        self.put("src-evil/file", "x")
        self.blocked("OUTSIDE_SCOPE")

    def test_dirty_review_rejected(self):
        self.publish()
        self.manifest["status"] = "SOURCE_REVIEW"
        self.put("src/untracked", "x")
        self.blocked("DIRTY_REVIEW_WORKTREE")

    def test_symlink_and_parent_symlink_rejected(self):
        target = self.repo / "src/link"
        target.symlink_to(self.directory)
        self.blocked("SYMLINK_PATH")
        with self.assertRaisesRegex(guard.PreflightError, "SYMLINK_PATH"):
            guard._no_symlink(self.repo, "src/link/anything")

    def test_staged_symlink_hidden_by_regular_worktree_file_rejected(self):
        target = self.repo / "src/link"
        target.symlink_to("one.txt")
        self.git("add", "src/link")
        target.unlink()
        target.write_text("regular\n")
        self.blocked("SYMLINK_OR_SUBMODULE_CHANGE")

    def test_submodule_gitlink_rejected(self):
        self.git("update-index", "--add", "--cacheinfo", "160000," + self.base + ",src/submodule")
        self.blocked("SYMLINK_OR_SUBMODULE_CHANGE")

    def test_governance_change_cannot_be_allowed_by_scope(self):
        self.manifest["scope"] = ["project_agents"]
        self.put("project_agents/tool_authority.json", "{}")
        self.blocked("GOVERNANCE_CHANGE")

    def test_governance_staged_only_and_cancelled_edit_rejected(self):
        path = "project_agents/tool_authority.json"
        original = (self.repo / path).read_text()
        self.put(path, "{}")
        self.git("add", path)
        self.blocked("GOVERNANCE_CHANGE")
        self.put(path, original)
        self.blocked("GOVERNANCE_CHANGE")

    def test_assume_unchanged_policy_edit_rejected(self):
        path = "project_agents/tool_authority.json"
        self.git("update-index", "--assume-unchanged", path)
        self.put(path, "{}")
        self.blocked("GOVERNANCE_CHANGE")

    def test_skip_worktree_policy_edit_rejected(self):
        path = "project_agents/tool_authority.json"
        self.git("update-index", "--skip-worktree", path)
        self.put(path, "{}")
        self.blocked("GOVERNANCE_CHANGE")

    def test_nested_agent_policy_rejected(self):
        self.put("src/AGENTS.md", "override scope")
        self.blocked("GOVERNANCE_CHANGE")

    def test_hidden_nonpolicy_changes_rejected(self):
        self.git("update-index", "--assume-unchanged", "outside.txt")
        self.put("outside.txt", "hidden modification")
        self.blocked("HIDDEN_INDEX_FLAGS")

    def test_unmerged_index_rejected(self):
        blob = self.git("rev-parse", "HEAD:src/one.txt")
        data = f"0 {'0' * 40}\tsrc/one.txt\n100644 {blob} 1\tsrc/one.txt\n100644 {blob} 2\tsrc/one.txt\n"
        subprocess.run(["git", "-C", str(self.repo), "update-index", "--index-info"],
            input=data.encode(), check=True, env=guard._environment())
        self.blocked("UNMERGED_INDEX")

    def test_untrusted_python_policy_never_executes(self):
        marker = self.directory / "executed"
        self.put("project_agents/tools/control_plane.py", f"open({str(marker)!r}, 'w').write('executed')\n")
        self.blocked("GOVERNANCE_CHANGE")
        self.assertFalse(marker.exists())

    def test_target_external_diff_and_fsmonitor_never_execute(self):
        marker = self.directory / "executed"
        hook = self.directory / "untrusted-hook"
        hook.write_text(f"#!/bin/sh\ntouch '{marker}'\n")
        hook.chmod(0o755)
        self.git("config", "diff.external", str(hook))
        self.git("config", "core.fsmonitor", str(hook))
        self.put("src/one.txt", "modified\n")
        self.assertEqual(self.inspect()["structuralStatus"], "PASS")
        self.assertFalse(marker.exists())

    def test_native_installation_mismatch_rejected(self):
        with patch.object(native, "REPO_ROOT", self.repo):
            self.put("project_agents/schemas/task_manifest.schema.json", "{}")
            self.blocked("STALE_NATIVE_POLICY")

    def test_unauthorized_role_combinations_rejected(self):
        for key, value in (("writerTool", "antigravity"), ("writerTool", "cursor-cloud"),
                           ("writerTool", "github-copilot"), ("verifierTool", "codex"),
                           ("gatekeeper", "codex")):
            with self.subTest(key=key, value=value):
                previous = self.manifest[key]
                self.manifest[key] = value
                self.blocked("INVALID_NATIVE_MANIFEST")
                self.manifest[key] = previous

    def test_actor_claim_cannot_authorize(self):
        for actor in ("chatgpt-github", "antigravity", "codex"):
            with self.subTest(actor=actor):
                self.manifest["actor"] = actor
                self.blocked("INVALID_NATIVE_MANIFEST")

    def test_green_ancestor_claim_cannot_open_certification_states(self):
        self.publish()  # A was published; its green-CI claim has no trusted input.
        self.put("src/one.txt", "B\n")
        self.commit()
        self.publish()
        for state in guard.CERTIFY_STATES:
            with self.subTest(state=state):
                self.manifest["status"] = state
                self.blocked("TRUSTED_GATE_EVIDENCE_REQUIRED")

    def test_ci_claim_strings_are_not_native_fields(self):
        self.manifest["ciEvidence"] = {"sha": self.base, "status": "PASS", "actor": "chatgpt-github"}
        self.blocked("INVALID_NATIVE_MANIFEST")

    def test_scope_traversal_and_globs_rejected(self):
        for scope in ("../", "/tmp", ".", "src/../outside.txt", "src/*", ".git", "src\\evil"):
            with self.subTest(scope=scope):
                self.manifest["scope"] = [scope]
                self.blocked("UNSAFE_PATH")

    def test_network_failure_has_no_cached_fallback(self):
        with patch.object(guard, "remote_heads", side_effect=guard.PreflightError("GIT_COMMAND_FAILED")):
            self.blocked("GIT_COMMAND_FAILED")

    def test_mid_inspection_remote_drift_rejected(self):
        initial = self.remote_observation("feat/task")
        with patch.object(guard, "remote_heads", side_effect=[initial, {**initial, "main": "a" * 40}]):
            self.blocked("SOURCE_MOVED_DURING_PREFLIGHT")

    def test_mutation_during_inventory_rejected(self):
        original = guard.changes
        calls = []
        def mutate(repo, base):
            calls.append(1)
            if len(calls) == 2:
                self.put("src/late", "late\n")
            return original(repo, base)
        with patch.object(guard, "changes", side_effect=mutate):
            self.blocked("SOURCE_MOVED_DURING_PREFLIGHT")

    def test_repeated_inspection_leaves_git_and_worktree_unchanged(self):
        self.put("src/one.txt", "unstaged\n")
        before = self.git("status", "--porcelain=v1")
        index = (self.repo / ".git/index").read_bytes()
        first, second = self.inspect(), self.inspect()
        self.assertEqual(first, second)
        self.assertEqual(before, self.git("status", "--porcelain=v1"))
        self.assertEqual(index, (self.repo / ".git/index").read_bytes())


class InputAndNativeRegressionTests(unittest.TestCase):
    def test_actual_git_remote_failure_is_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaisesRegex(guard.PreflightError, "GIT_COMMAND_FAILED"):
                guard._git(None, "ls-remote", str(Path(tmp) / "absent.git"))

    def test_git_timeout_and_oserror_are_redacted(self):
        for error in (OSError("SECRET"), subprocess.TimeoutExpired("SECRET", 20)):
            with self.subTest(error=type(error)), patch.object(guard.subprocess, "run", side_effect=error):
                with self.assertRaisesRegex(guard.PreflightError, "^GIT_UNAVAILABLE_OR_TIMEOUT$"):
                    guard._git(None, "ls-remote")

    def test_git_error_stderr_is_not_exposed(self):
        result = subprocess.CompletedProcess([], 1, stdout=b"", stderr=b"SECRET")
        with patch.object(guard.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(guard.PreflightError, "^GIT_COMMAND_FAILED$"):
                guard._git(None, "ls-remote")

    def test_remote_command_does_not_use_target_config(self):
        with patch.object(guard, "_text", return_value="a" * 40 + "\trefs/heads/main\n") as call:
            self.assertIsNone(guard.remote_heads("feat/task")["branch"])
            self.assertEqual(call.call_args.args[:4], (None, "ls-remote", "--refs", guard.REMOTE))

    def test_inherited_git_authority_overrides_removed(self):
        with patch.dict(os.environ, {"GIT_DIR": "/wrong", "GIT_CONFIG_COUNT": "1", "GIT_SSH_COMMAND": "evil"}):
            env = guard._environment()
            for key in ("GIT_DIR", "GIT_CONFIG_COUNT", "GIT_SSH_COMMAND"):
                self.assertNotIn(key, env)
            self.assertEqual(env["GIT_CONFIG_NOSYSTEM"], "1")

    def test_path_injection_and_non_utf8_rejected(self):
        for value in (b"src/a\nsecret\0", b"src/\xff\0", b"../escape\0", b"src/path"):
            with self.subTest(value=value), patch.object(guard, "_git", return_value=value):
                with self.assertRaises(guard.PreflightError):
                    guard._paths(Path("/fixture"), "diff")

    def test_json_invalid_duplicate_oversized_and_nonobject_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "manifest.json"
            for value in ('{"actor":1,"actor":2}', "{bad", "[]", " " * (guard.MAX_JSON_BYTES + 1)):
                with self.subTest(length=len(value)):
                    path.write_text(value)
                    with self.assertRaises(guard.PreflightError):
                        guard.read_manifest(path)

    def test_native_writer_self_certification_remains_forbidden(self):
        for state in guard.CERTIFY_STATES:
            with self.subTest(state=state), self.assertRaises(native.ControlPlaneError):
                native.validate_handoff(_handoff(nextState=state), actor_role="writer")

    def test_native_unverified_cannot_satisfy_evidence(self):
        with self.assertRaisesRegex(native.ControlPlaneError, "UNVERIFIED"):
            native.validate_handoff(_handoff(claims=[{"claimType": "UNVERIFIED", "statement": "green"}]),
                required_evidence=["proof"], actor_role="writer")

    def test_cli_error_is_fail_closed_and_redacted(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "task.json"
            path.write_text("{SECRET")
            output = io.StringIO()
            with contextlib.redirect_stderr(output):
                code = guard.main(["--worktree", tmp, "--manifest", str(path)])
            self.assertEqual(code, 2)
            result = json.loads(output.getvalue())
            self.assertFalse(result["authorized"])
            self.assertFalse(result["mergeReady"])
            self.assertNotIn("SECRET", output.getvalue())


if __name__ == "__main__":
    unittest.main(verbosity=2)
