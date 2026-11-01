"""Phase A: real Git trees/index/worktrees plus bounded remote/failure seams.

No GitHub writes. Fixture pushes target a disposable local bare repository.
Only remote_heads is replaced; production origin, native policy and Git checks
remain active. Authentication/CI certification is deliberately never simulated
as achieved by these tests.
"""
from __future__ import annotations

import contextlib
import io
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import traceback
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "project_agents/tools"))
import control_plane as native
import task_preflight as guard
from tests.engineering_orchestrator.test_control_plane_v2 import _task, _handoff

GOVERNANCE_DOCS = ("CHATGPT_AGENT_ROUTER.md", "AGENT_MATRIX.md", "START_HERE.md",
    "USAGE_PLAYBOOK.md", "TOOL_COMPATIBILITY.md", "TASK_PROMPT_TEMPLATE.md")


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
        for name in GOVERNANCE_DOCS:
            self.put("project_agents/" + name, "fixture governance\n")
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
        result = subprocess.run(["git", "-C", str(self.repo), *args], check=False,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=guard._environment())
        if result.returncode:
            # This helper handles only disposable local fixture Git commands,
            # never production credentials or the authenticated source adapter.
            raise AssertionError("fixture Git failed: " + result.stderr.decode(errors="replace"))
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

    def _check_governance_documents(self, mode):
        self.manifest["scope"] = ["project_agents"]
        for name in GOVERNANCE_DOCS:
            with self.subTest(document=name, mode=mode):
                path = "project_agents/" + name
                if mode == "hidden":
                    self.git("update-index", "--assume-unchanged", path)
                self.put(path, "tampered governance\n")
                if mode == "staged":
                    self.git("add", path)
                elif mode == "committed":
                    self.commit()
                self.blocked("GOVERNANCE_CHANGE")
                if mode == "hidden":
                    self.git("update-index", "--no-assume-unchanged", path)
                self.git("reset", "--hard", self.base)  # Disposable fixture only.
                self.manifest["exactHead"] = self.base

    def test_governance_documents_unstaged_rejected(self):
        self._check_governance_documents("unstaged")

    def test_governance_documents_staged_rejected(self):
        self._check_governance_documents("staged")

    def test_governance_documents_committed_rejected(self):
        self._check_governance_documents("committed")

    def test_governance_documents_hidden_rejected(self):
        self._check_governance_documents("hidden")

    def test_governance_documents_untracked_rejected(self):
        for name in GOVERNANCE_DOCS:
            self.git("rm", "project_agents/" + name)
        self.commit()
        self.base = self.manifest["exactHead"]
        self.manifest["authoritativeMain"] = self.base
        self.git("push", "-q", str(self.remote), "HEAD:main")
        self.manifest["scope"] = ["project_agents"]
        for name in GOVERNANCE_DOCS:
            with self.subTest(document=name):
                path = "project_agents/" + name
                self.put(path, "untracked governance\n")
                self.assertEqual(guard.changes(self.repo, self.base)["untracked"], [path])
                self.blocked("GOVERNANCE_CHANGE")
                (self.repo / path).unlink()

    def test_future_governance_file_rejected_without_enumeration(self):
        self.manifest["scope"] = ["project_agents"]
        self.put("project_agents/future/nested/policy.md", "new policy")
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
        original = guard._inventory
        calls = []
        def mutate(repo, base):
            calls.append(1)
            if len(calls) == 2:
                self.put("src/late", "late\n")
            return original(repo, base)
        with patch.object(guard, "_inventory", side_effect=mutate):
            self.blocked("SOURCE_MOVED_DURING_PREFLIGHT")

    def _malicious_filter(self, kind):
        self.publish()
        marker = self.directory / "filter-executed"
        self.git("config", "filter.hide." + kind,
            f"printf executed > '{marker}'; printf 'protected by scope\\n'")
        self.put(".git/info/attributes", "outside.txt filter=hide\n")
        self.put("outside.txt", "malicious raw bytes outside allowed scope\n")
        for state in ("IMPLEMENTATION", "SOURCE_REVIEW"):
            with self.subTest(state=state, filter=kind):
                self.manifest["status"] = state
                self.assertIn("outside.txt", guard.changes(self.repo, self.base)["unstaged"])
                self.blocked("OUTSIDE_SCOPE")
                self.assertFalse(marker.exists(), "target conversion filter executed")

    def test_clean_filter_never_executes_or_conceals_outside_edit(self):
        self._malicious_filter("clean")

    def test_process_filter_never_executes_or_conceals_outside_edit(self):
        self._malicious_filter("process")

    def test_worktree_attributes_cannot_hide_raw_changes_or_execute(self):
        marker = self.directory / "filter-executed"
        self.put("src/.gitattributes", "one.txt filter=hide\n")
        self.git("config", "filter.hide.clean", f"touch '{marker}'; printf 'base\\n'")
        self.put("src/one.txt", "different raw bytes\n")
        result = self.inspect()
        self.assertEqual(result["changes"]["unstaged"], ["src/one.txt"])
        self.assertFalse(marker.exists())

    def test_raw_mode_check_ignores_core_filemode_setting(self):
        self.git("config", "core.filemode", "false")
        (self.repo / "outside.txt").chmod(0o755)
        self.blocked("OUTSIDE_SCOPE")

    def test_raw_deleted_file_is_detected(self):
        (self.repo / "outside.txt").unlink()
        self.blocked("OUTSIDE_SCOPE")

    def test_crlf_normalization_does_not_hide_raw_bytes(self):
        self.git("config", "core.autocrlf", "true")
        (self.repo / "outside.txt").write_bytes(b"protected by scope\r\n")
        self.blocked("OUTSIDE_SCOPE")

    def test_lfs_style_clean_output_does_not_hide_materialized_bytes(self):
        self.publish()
        marker = self.directory / "lfs-executed"
        self.git("config", "filter.lfs.clean", f"touch '{marker}'; printf 'base\\n'")
        self.put(".git/info/attributes", "src/one.txt filter=lfs\n")
        self.put("src/one.txt", "materialized payload instead of index blob")
        self.manifest["status"] = "SOURCE_REVIEW"
        self.blocked("DIRTY_REVIEW_WORKTREE")
        self.assertFalse(marker.exists())

    def test_same_changed_path_mutation_during_inspection_is_detected(self):
        self.put("src/one.txt", "first\n")
        original = guard._inventory
        calls = []
        def mutate(repo, base):
            calls.append(1)
            if len(calls) == 2:
                self.put("src/one.txt", "other\n")
            return original(repo, base)
        with patch.object(guard, "_inventory", side_effect=mutate):
            self.blocked("SOURCE_MOVED_DURING_PREFLIGHT")

    def test_mutation_during_streaming_is_detected(self):
        original = guard.os.read
        changed = []
        def mutate(fd, amount):
            value = original(fd, amount)
            if value == b"protected by scope\n" and not changed:
                changed.append(True)
                self.put("outside.txt", "changed during read\n")
            return value
        with patch.object(guard.os, "read", side_effect=mutate):
            self.blocked("SOURCE_MOVED_DURING_PREFLIGHT")
        self.assertTrue(changed)

    def test_nested_git_metadata_rejected_in_writer_and_review(self):
        self.publish()
        for state in ("IMPLEMENTATION", "SOURCE_REVIEW"):
            self.manifest["status"] = state
            for parent in ("project_agents", "outside", "src"):
                for kind in ("directory", "file", "symlink"):
                    with self.subTest(state=state, parent=parent, kind=kind):
                        path = self.repo / parent / ".git"
                        path.parent.mkdir(exist_ok=True)
                        if kind == "directory":
                            path.mkdir()
                            (path / "AGENTS.md").write_text("hidden")
                        elif kind == "file":
                            path.write_text("not valid metadata")
                        else:
                            path.symlink_to(self.directory)
                        self.blocked("NESTED_GIT_METADATA")
                        if kind == "directory":
                            shutil.rmtree(path)  # Only this disposable fixture.
                        else:
                            path.unlink()

    def test_git_omitted_ignored_governance_file_is_still_found(self):
        self.put(".git/info/exclude", "project_agents/hidden/\n")
        self.put("project_agents/hidden/policy.txt", "hidden")
        self.blocked("GOVERNANCE_CHANGE")

    def test_inventory_does_not_depend_on_git_untracked_listing(self):
        self.put("outside/new.txt", "not in git output")
        with patch.object(guard, "_paths", return_value=[]):
            self.blocked("OUTSIDE_SCOPE")

    def test_ignored_review_file_and_empty_directory_block(self):
        self.publish()
        self.manifest["status"] = "SOURCE_REVIEW"
        self.put(".git/info/exclude", "src/hidden\n")
        self.put("src/hidden", "ignored bytes")
        self.blocked("DIRTY_REVIEW_WORKTREE")
        (self.repo / "src/hidden").unlink()
        (self.repo / "src/empty").mkdir()
        self.blocked("DIRTY_REVIEW_WORKTREE")

    def test_symlinked_directory_not_followed(self):
        outside = self.directory / "outside-tree"
        outside.mkdir()
        (outside / "unchanged").write_text("retain")
        (self.repo / "src/link").symlink_to(outside, target_is_directory=True)
        self.blocked("SYMLINK_PATH")
        self.assertEqual((outside / "unchanged").read_text(), "retain")

    def test_fifo_and_socket_are_rejected_without_opening(self):
        path = self.repo / "src/special"
        os.mkfifo(path)
        self.blocked("SPECIAL_FILE_PATH")
        path.unlink()
        with socket.socket(socket.AF_UNIX) as sock:
            sock.bind(str(path))
            self.blocked("SPECIAL_FILE_PATH")

    def test_inaccessible_file_and_directory_fail_closed(self):
        for relative in ("outside.txt", "src"):
            path = self.repo / relative
            old = path.stat().st_mode
            try:
                path.chmod(0)
                # Deterministic even if a reviewer invokes the suite as root.
                original = guard.os.open
                def denied(name, *args, **kwargs):
                    if str(name) == relative:
                        raise PermissionError("synthetic denied")
                    return original(name, *args, **kwargs)
                with patch.object(guard.os, "open", side_effect=denied):
                    self.blocked("FILESYSTEM_UNREADABLE_OR_MOVED")
            finally:
                path.chmod(old)

    def test_inventory_resource_limits_fail_closed(self):
        for limit in ("MAX_FILE_BYTES", "MAX_SCAN_BYTES", "MAX_SCAN_ENTRIES",
                      "MAX_SCAN_DEPTH", "MAX_SCAN_SECONDS"):
            with self.subTest(limit=limit), patch.object(guard, limit, 0):
                self.blocked("INVENTORY_LIMIT")

    def test_linked_worktree_metadata_is_verified(self):
        self.publish()
        linked = self.directory / "linked"
        self.git("worktree", "add", "--detach", str(linked), "HEAD")
        manifest = dict(self.manifest, worktree=str(linked), status="SOURCE_REVIEW")
        result = guard.inspect_task(linked, manifest)
        self.assertEqual(result["result"], "INSPECTION_ONLY")
        (linked / "src/.git").mkdir()
        with self.assertRaisesRegex(guard.PreflightError, "NESTED_GIT_METADATA"):
            guard.inspect_task(linked, manifest)

    def test_root_git_symlink_is_not_a_metadata_exception(self):
        original = self.repo / ".git"
        moved = self.directory / "moved-metadata"
        original.rename(moved)
        original.symlink_to(moved, target_is_directory=True)
        with self.assertRaisesRegex(guard.PreflightError, "INVALID_ROOT_GIT_METADATA"):
            guard.changes(self.repo, self.base)

    def test_linked_writer_and_invalid_metadata_backlink(self):
        linked = self.directory / "linked-writer"
        self.git("worktree", "add", "-b", "feat/linked", str(linked), "HEAD")
        manifest = dict(self.manifest, worktree=str(linked), branch="feat/linked")
        result = guard.inspect_task(linked, manifest)
        self.assertEqual(result["lane"], "WRITER_IN_PROGRESS")
        gitdir = Path((linked / ".git").read_text().strip()[8:])
        (gitdir / "gitdir").write_text(str(self.directory / "wrong/.git") + "\n")
        with self.assertRaisesRegex(guard.PreflightError, "INVALID_ROOT_GIT_METADATA"):
            guard.inspect_task(linked, manifest)

    def test_repeated_inspection_leaves_git_and_worktree_unchanged(self):
        self.put("src/one.txt", "unstaged\n")
        before = self.git("status", "--porcelain=v1")
        index = (self.repo / ".git/index").read_bytes()
        first, second = self.inspect(), self.inspect()
        self.assertEqual(first, second)
        self.assertEqual(before, self.git("status", "--porcelain=v1"))
        self.assertEqual(index, (self.repo / ".git/index").read_bytes())


class ImportAndObjectFormatTests(unittest.TestCase):
    def test_cli_and_package_imports_use_single_trusted_identity(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            repo = directory / "repo"
            repo.mkdir()
            subprocess.run(["git", "init", "-q", str(repo)], check=True, env=guard._environment())
            hostile = directory / "hostile"
            hostile.mkdir()
            marker = directory / "import-executed"
            for module in ("task_preflight", "control_plane"):
                (hostile / (module + ".py")).write_text(f"open({str(marker)!r}, 'w').write('executed')\n")
            manifest = directory / "manifest.json"
            manifest.write_text(json.dumps({"worktree": str(repo), "repository": "wrong"}))
            env = {"PATH": os.defpath, "PYTHONPATH": str(ROOT), "HOME": "/nonexistent"}
            program = f'''
from pathlib import Path
from project_agents.tools import control_plane as cp
from project_agents.tools import task_preflight as tp
import control_plane, task_preflight
assert cp is control_plane and tp is task_preflight
assert cp._load_preflight() is tp
assert tp.native is cp
try:
    cp.inspect_task_preflight(Path({str(repo)!r}), {{"worktree": {str(repo)!r}, "repository": "wrong"}})
except tp.PreflightError as error:
    assert error.code == "WRONG_REPOSITORY", error.code
else:
    raise AssertionError("inspection unexpectedly passed")
raise SystemExit(tp.main(["--worktree", {str(repo)!r}, "--manifest", {str(manifest)!r}]))
'''
            for safe in ([], ["-P"]):
                for invocation in ([str(ROOT / "project_agents/tools/task_preflight.py"),
                        "--worktree", str(repo), "--manifest", str(manifest)], ["-c", program]):
                    with self.subTest(safe=safe, entry=invocation[0]):
                        result = subprocess.run([sys.executable, "-B", *safe, *invocation],
                            cwd=hostile, env=env, capture_output=True, timeout=10)
                        self.assertEqual(result.returncode, 2, result.stderr.decode())
                        self.assertEqual(json.loads(result.stderr)["code"], "WRONG_REPOSITORY")
                        self.assertFalse(marker.exists())

    def test_sha256_raw_blob_identity_and_stream_bound(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            def git(*args):
                return subprocess.run(["git", "-C", str(repo), *args], check=True,
                    env=guard._environment(), capture_output=True).stdout.decode().strip()
            git("init", "-q", "--object-format=sha256")
            git("config", "user.name", "Synthetic Fixture")
            git("config", "user.email", "fixture@example.invalid")
            data = b"x" * (guard.READ_CHUNK_BYTES * 2 + 17)
            (repo / "file").write_bytes(data)
            git("add", "file")
            git("commit", "-qm", "fixture")
            base = git("rev-parse", "HEAD")
            self.assertEqual(len(base), 64)
            self.assertFalse(any(guard.changes(repo, base).values()))
            expected = hashlib.sha256(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
            self.assertEqual(guard._entries(repo, None, "sha256")["file"][1], expected)
            original = guard.os.read
            sizes = []
            def bounded(fd, size):
                sizes.append(size)
                return original(fd, size)
            (repo / "file").write_bytes(b"y" + data[1:])
            with patch.object(guard.os, "read", side_effect=bounded):
                self.assertEqual(guard.changes(repo, base)["unstaged"], ["file"])
            self.assertLessEqual(max(sizes), guard.READ_CHUNK_BYTES)


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


def source_payload(branch="feat/task"):
    return {"data": {"repository": {"nameWithOwner": guard.REPOSITORY, "isPrivate": True,
        "main": {"name": "main", "target": {"__typename": "Commit", "oid": "a" * 40}},
        "branch": {"name": branch, "target": {"__typename": "Commit", "oid": "b" * 40}}}}}


class SourceAdapterTests(unittest.TestCase):
    """Exercise a real adapter subprocess with synthetic credentials, no network."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.executable = self.root / "gh-fixture"
        self.executable.write_text(f'''#!{sys.executable}
import json, os, sys, time
if os.environ.get("GH_TOKEN") == "synthetic-timeout":
    print("synthetic-credential-canary", flush=True)
    time.sleep(30)
if os.environ.get("GH_TOKEN") != "synthetic-credential-canary":
    print("denied synthetic-credential-canary", file=sys.stderr)
    print("denied synthetic-credential-canary")
    sys.exit(1)
assert os.getcwd() == "/"
assert sys.argv[1:5] == ["api", "graphql", "--hostname", "github.com"]
assert sys.argv[5:] == ["-f", "query=" + {guard.SOURCE_QUERY!r}, "-f", "branch=refs/heads/feat/task"]
assert not any(k in os.environ for k in ["GH_DEBUG", "GH_HOST", "GIT_DIR", "HTTPS_PROXY", "LD_PRELOAD"])
print({json.dumps(source_payload())!r})
''')
        self.executable.chmod(0o755)
        real_run = subprocess.run
        def fixture_process(command, **kwargs):
            if command[0] == str(self.executable):
                # /tmp remains noexec. Execute fixture source with the trusted
                # interpreter; do not relax the container's mount protection.
                command = [sys.executable, *command]
            return real_run(command, **kwargs)
        for mock in (patch.object(guard, "GITHUB_CLI", str(self.executable)),
                     patch.object(guard.subprocess, "run", side_effect=fixture_process),
                     patch.dict(os.environ, {"GH_TOKEN": "synthetic-credential-canary",
                         "GH_DEBUG": "api", "GH_HOST": "evil.invalid", "GIT_DIR": "/untrusted",
                         "HTTPS_PROXY": "http://evil.invalid"}, clear=True)):
            mock.start()
            self.addCleanup(mock.stop)

    def test_authenticated_subprocess_success(self):
        self.assertEqual(guard.remote_heads("feat/task"), {"main": "a" * 40, "branch": "b" * 40})

    def test_denied_authentication_redacted_without_fallback(self):
        with patch.dict(os.environ, {"GH_TOKEN": "denied"}):
            with self.assertRaisesRegex(guard.PreflightError, "^SOURCE_ACCESS_FAILED$") as caught:
                guard.remote_heads("feat/task")
        self.assertIsNone(caught.exception.__context__)
        self.assertIsNone(caught.exception.__cause__)
        self.assertNotIn("synthetic-credential-canary", "".join(traceback.format_exception(caught.exception)))

    def test_real_subprocess_timeout_redacts_captured_output(self):
        with patch.dict(os.environ, {"GH_TOKEN": "synthetic-timeout"}), \
             patch.object(guard, "SOURCE_TIMEOUT_SECONDS", 0.15):
            with self.assertRaisesRegex(guard.PreflightError, "^SOURCE_TIMEOUT$") as caught:
                guard.remote_heads("feat/task")
        self.assertIsNone(caught.exception.__context__)
        self.assertIsNone(caught.exception.__cause__)
        self.assertNotIn("synthetic-credential-canary", "".join(traceback.format_exception(caught.exception)))

    def test_missing_trusted_adapter_is_closed(self):
        with patch.object(guard, "GITHUB_CLI", str(self.root / "absent")):
            with self.assertRaisesRegex(guard.PreflightError, "SOURCE_ADAPTER_UNAVAILABLE"):
                guard.remote_heads("feat/task")

    def test_installed_absolute_cli_and_trusted_auth_configuration(self):
        result = subprocess.CompletedProcess([], 0, stdout=json.dumps(source_payload()).encode())
        with patch.object(guard, "GITHUB_CLI", "/usr/bin/gh"), \
             patch.dict(os.environ, {"HOME": "/trusted-home", "GH_CONFIG_DIR": "/trusted-auth"}), \
             patch.object(guard.subprocess, "run", return_value=result) as run:
            guard.remote_heads("feat/task")
        args, options = run.call_args
        self.assertEqual(args[0][0], "/usr/bin/gh")
        self.assertEqual(options["cwd"], "/")
        self.assertEqual(options["env"]["GH_CONFIG_DIR"], "/trusted-auth")
        self.assertEqual(options["env"]["GH_TOKEN"], "synthetic-credential-canary")
        self.assertEqual(options["stderr"], subprocess.DEVNULL)
        self.assertEqual(options["timeout"], 20)
        self.assertNotIn("synthetic-credential-canary", " ".join(args[0]))

    def test_null_branch_requires_successful_authenticated_repository_read(self):
        payload = source_payload()
        payload["data"]["repository"]["branch"] = None
        with patch.object(guard, "_source_response", return_value=json.dumps(payload).encode()):
            self.assertEqual(guard.remote_heads("feat/task"), {"main": "a" * 40, "branch": None})

    def test_partial_errors_wrong_repo_and_invalid_ref_fail_closed(self):
        payloads = [dict(source_payload(), errors=[{"message": "synthetic-credential-canary"}]),
                    {"data": {"repository": None}}]
        for key, value in (("nameWithOwner", "other/repo"), ("isPrivate", "true"),
                           ("main", None), ("branch", {"name": "wrong"})):
            payload = source_payload()
            payload["data"]["repository"][key] = value
            payloads.append(payload)
        payload = source_payload()
        del payload["data"]["repository"]["branch"]
        payloads.append(payload)
        for payload in payloads:
            with self.subTest(payload=payload), patch.object(guard, "_source_response", return_value=json.dumps(payload).encode()):
                with self.assertRaises(guard.PreflightError):
                    guard.remote_heads("feat/task")

    def test_malformed_and_duplicate_json_have_no_secret_exception_context(self):
        for response in (b"synthetic-credential-canary{", b'{"data":1,"data":2}'):
            with self.subTest(response=response), patch.object(guard, "_source_response", return_value=response):
                with self.assertRaisesRegex(guard.PreflightError, "INVALID_REMOTE_RESPONSE") as caught:
                    guard.remote_heads("feat/task")
                self.assertIsNone(caught.exception.__context__)

    def test_cli_denial_does_not_emit_credentials(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = Path(tmp) / "manifest.json"
            manifest.write_text("{}")
            def probe(*args):
                return guard.remote_heads("feat/task")
            output = io.StringIO()
            with patch.dict(os.environ, {"GH_TOKEN": "denied"}), \
                 patch.object(native, "inspect_task_preflight", side_effect=probe), \
                 contextlib.redirect_stderr(output):
                self.assertEqual(guard.main(["--worktree", tmp, "--manifest", str(manifest)]), 2)
            result = json.loads(output.getvalue())
            self.assertEqual(result["code"], "SOURCE_ACCESS_FAILED")
            self.assertFalse(result["authorized"])
            self.assertNotIn("synthetic-credential-canary", output.getvalue())


class CredentialBoundaryTests(unittest.TestCase):
    """Synthetic regressions, never an authority for running PR code with secrets."""

    def test_removed_live_mode_rejects_without_consuming_open_stdin(self):
        # Leave the pipe open with no bytes: a credential reader would block.
        # The removed mode must reject before reading stdin or running any test.
        with subprocess.Popen([sys.executable, "-B", "-m",
                "tests.engineering_orchestrator.test_task_preflight",
                "--live-source-probe"], cwd=ROOT, stdin=subprocess.PIPE,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                env={"PATH": os.defpath, "HOME": "/nonexistent"}) as process:
            try:
                self.assertEqual(process.wait(timeout=10), 2)
            finally:
                if process.poll() is None:
                    process.kill()
            output, error = process.communicate()
        self.assertIn(b"unrecognized arguments: --live-source-probe", error)
        self.assertEqual(output, b"")
        self.assertNotIn(b"LIVE_PRIVATE_SOURCE", error)

    def test_no_real_credential_test_entrypoint(self):
        self.assertNotIn("run_live_source_probe", globals())
        # Production source remains callable only through synthetic fixture
        # replacement in this module, never through a live-auth test class.
        self.assertNotIn("LivePrivateSourceTests", globals())

    def test_compose_has_only_disconnected_stdin_closed_test_service(self):
        compose = (ROOT / "tests/engineering_orchestrator/preflight.compose.yaml").read_text()
        self.assertEqual(re.findall(r"^  ([\w-]+):", compose, re.MULTILINE), ["preflight-tests"])
        self.assertIn("    network_mode: none\n", compose)
        self.assertIn("    stdin_open: false\n", compose)
        self.assertIn('    entrypoint: [/bin/sh, -c, \'exec "$$@" </dev/null\', --]\n', compose)
        for forbidden in ("preflight-live-source", "--live-source-probe", "GH_TOKEN",
                "GITHUB_TOKEN", "GH_CONFIG_DIR", "/usr/bin/gh", "env_file:",
                "secrets:", "configs:", "extends:", "profiles:", "network_mode: bridge"):
            with self.subTest(forbidden=forbidden):
                self.assertNotIn(forbidden, compose)

    def test_documentation_has_no_credential_pipe_recipe(self):
        documentation = (ROOT / "project_agents/TASK_PREFLIGHT.md").read_text()
        # The unsafe mechanism remains described as history, not executable advice.
        commands = re.findall(r"```(?:sh|bash)\n(.*?)```", documentation, re.DOTALL)
        for command in commands:
            for forbidden in ("auth token", "preflight-live-source", "--live-source-probe"):
                with self.subTest(forbidden=forbidden):
                    self.assertNotIn(forbidden, command)
        self.assertIn("insufficient for security certification", documentation)


if __name__ == "__main__":
    unittest.main(verbosity=2)
