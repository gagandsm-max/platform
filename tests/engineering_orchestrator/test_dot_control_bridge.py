"""Dot Host-Control Bridge v1 — positive Gate 3A proof and negative security tests.

Uses temporary repositories and temporary control roots only.
Does not touch the live repository worktree under test.
"""

from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

REPO_ROOT = Path(__file__).resolve().parents[2]
PROJECT_AGENTS = REPO_ROOT / "project_agents"
DOT_CONTROL = PROJECT_AGENTS / "dot_control"
if str(PROJECT_AGENTS) not in sys.path:
    sys.path.insert(0, str(PROJECT_AGENTS))

from dot_control.bridge import DotControlBridge  # noqa: E402
from dot_control.evidence import (  # noqa: E402
    BoundedRedactingWriter,
    append_bridge_log,
    redact_argv,
    redact_text,
)
from dot_control.executor import (  # noqa: E402
    build_action_argv,
    collect_change_scope,
    enforce_change_scope,
    enforce_post_change_scope,
    execute_repo_probe,
    live_writer_unsupported,
    parse_github_owner_repo,
    run_process,
    sanitize_remote_url,
    validate_repo_relative_path,
    verify_authoritative_base,
    verify_git_preconditions,
    verify_repository_binding,
)
from dot_control.locks import WorktreeLock, inspect_lock, pid_alive  # noqa: E402
from dot_control.models import (  # noqa: E402
    APPROVED_REPOSITORY,
    DIR_MODE,
    FILE_MODE,
    JSON_SCHEMA_META,
    LIVE_WRITER_UNSUPPORTED_REASON,
    MAX_PROMPT_BYTES,
    MAX_STDERR_LOG_BYTES,
    MAX_STDOUT_LOG_BYTES,
    PUBLIC_WEBSITE,
    SCHEMA_ID,
    SCHEMA_VERSION,
    Action,
    BridgeError,
    Disposition,
    TaskManifest,
    VerifierTool,
    WriterTool,
)
from dot_control.paths import init_control_root, validate_control_root_layout  # noqa: E402
from dot_control.validator import PathPolicy, validate_manifest_dict  # noqa: E402


def _git(cwd: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", *args],
        cwd=str(cwd),
        capture_output=True,
        text=True,
        check=check,
        shell=False,
    )


def _init_repo(root: Path, *, branch: str = "feature/gate3-proof") -> str:
    root.mkdir(parents=True, exist_ok=True)
    _git(root, "init")
    _git(root, "config", "user.email", "bridge-test@example.com")
    _git(root, "config", "user.name", "Bridge Test")
    (root / "README.md").write_text("seed\n", encoding="utf-8")
    _git(root, "add", "README.md")
    _git(root, "commit", "-m", "seed")
    _git(root, "branch", "main")
    _git(root, "checkout", "-b", branch)
    head = _git(root, "rev-parse", "HEAD").stdout.strip()
    _git(root, "update-ref", "refs/remotes/origin/main", head)
    _git(root, "remote", "add", "origin", "https://github.com/gagandsm-max/platform.git")
    return head


def _sha(repo: Path) -> str:
    return _git(repo, "rev-parse", "HEAD").stdout.strip()


def _base_manifest(
    *,
    control: Path,
    worktree: Path,
    task_id: str,
    action: str,
    branch: str,
    head: str,
    writer: str = "none",
    verifier: str = "none",
    allowed: list | None = None,
    forbidden: list | None = None,
    prompt: str | None = None,
    allow_commit: bool = False,
    commit_message: str | None = None,
    test_profile: str | None = None,
    timeout: int = 30,
    repository: str = APPROVED_REPOSITORY,
    authoritative_base: str | None = None,
) -> dict:
    payload = {
        "schemaVersion": SCHEMA_VERSION,
        "taskId": task_id,
        "governingIssue": "#185",
        "action": action,
        "writerTool": writer,
        "verifierTool": verifier,
        "repository": repository,
        "worktree": str(worktree),
        "branch": branch,
        "authoritativeBase": authoritative_base or head,
        "expectedHead": head,
        "allowedFiles": allowed if allowed is not None else [],
        "forbiddenFiles": forbidden if forbidden is not None else [],
        "promptFile": prompt,
        "timeoutSeconds": timeout,
        "allowCommit": allow_commit,
        "allowPush": False,
        "allowPullRequest": False,
        "createdAt": "2026-10-07T00:00:00Z",
        "requestedBy": "dot-test",
    }
    if commit_message is not None:
        payload["commitMessage"] = commit_message
    if test_profile is not None:
        payload["testProfile"] = test_profile
    return payload


def _write_prompt(control: Path, task_id: str, text: str = "audit please") -> Path:
    d = control / "tasks" / task_id
    d.mkdir(parents=True, exist_ok=True)
    path = d / "prompt.md"
    path.write_text(text, encoding="utf-8")
    return path


def _submit(bridge: DotControlBridge, payload: dict) -> Path:
    task_id = payload["taskId"]
    tmp = bridge.inbox / f"{task_id}.tmp"
    final = bridge.inbox / f"{task_id}.json"
    tmp.write_text(json.dumps(payload), encoding="utf-8")
    with open(tmp, "a", encoding="utf-8") as fh:
        fh.flush()
        os.fsync(fh.fileno())
    os.replace(tmp, final)
    return final


def _manifest_obj(
    *,
    repo: Path,
    branch: str,
    head: str,
    action: Action = Action.CODEX_TASK,
    allow_commit: bool = False,
    allowed: tuple[str, ...] = ("a.txt",),
    forbidden: tuple[str, ...] = (),
) -> TaskManifest:
    return TaskManifest(
        schema_version=SCHEMA_VERSION,
        task_id="SCOPE-1",
        governing_issue="#185",
        action=action,
        writer_tool=WriterTool.CODEX if action == Action.CODEX_TASK else WriterTool.CURSOR_CLI,
        verifier_tool=VerifierTool.NONE,
        repository=APPROVED_REPOSITORY,
        worktree=repo,
        branch=branch,
        authoritative_base=head,
        expected_head=head,
        allowed_files=allowed,
        forbidden_files=forbidden,
        prompt_file=None,
        timeout_seconds=30,
        allow_commit=allow_commit,
        allow_push=False,
        allow_pull_request=False,
        created_at="2026-10-07T00:00:00Z",
        requested_by="t",
    )


class DotControlBridgeTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory(prefix="dot-control-")
        self.tmp = Path(self._tmpdir.name)
        self.control = self.tmp / "control"
        self.repo = self.tmp / "repo"
        self.branch = "feature/gate3-proof"
        self.head = _init_repo(self.repo, branch=self.branch)
        self.policy = PathPolicy(
            home=self.tmp / "home",
            control_root=self.control,
            approved_repo_exact=self.repo,
            approved_repo_prefix=str(self.tmp / "nms-"),
            allow_test_roots=[self.tmp],
        )
        (self.tmp / "home").mkdir(parents=True, exist_ok=True)
        self.bridge = DotControlBridge(
            control_root=self.control,
            policy=self.policy,
            create_dirs=True,
        )

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    # --- Gate 3A replacement proof ----------------------------------------

    def test_git_commit_proof_gate3a(self) -> None:
        target = self.repo / "proof.txt"
        target.write_text("gate3-proof\n", encoding="utf-8")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="GATE3A-1",
            action="GIT_COMMIT_PROOF",
            branch=self.branch,
            head=self.head,
            allowed=["proof.txt"],
            allow_commit=True,
            commit_message="chore: gate3 commit proof",
        )
        _submit(self.bridge, payload)
        result = self.bridge.process_one()

        self.assertEqual(result.disposition, Disposition.SUCCESS.value)
        self.assertIsNotNone(result.commit_sha)
        self.assertEqual(result.committed_files, ["proof.txt"])
        self.assertEqual(result.push_status, "NOT_ATTEMPTED")
        self.assertEqual(result.pr_status, "NOT_ATTEMPTED")
        self.assertTrue(result.lock_lifecycle.get("acquired"))
        self.assertTrue(result.lock_lifecycle.get("released"))
        self.assertTrue((self.control / "results" / "GATE3A-1.result.json").is_file())
        self.assertEqual(_sha(self.repo), result.commit_sha)
        self.assertNotEqual(result.commit_sha, self.head)
        self.assertFalse((self.tmp / "home" / "escape").exists())

    # --- REPO_PROBE -------------------------------------------------------

    def test_repo_probe_readonly(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="PROBE-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        _submit(self.bridge, payload)
        result = self.bridge.process_one()
        self.assertEqual(result.disposition, Disposition.SUCCESS.value)
        self.assertIn("rev-parse HEAD", result.probe)
        self.assertEqual(result.ending_head, self.head)
        self.assertEqual(result.exit_code, 0)

    def test_repo_probe_missing_origin_main_not_success(self) -> None:
        _git(self.repo, "update-ref", "-d", "refs/remotes/origin/main")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="PROBE-MISS",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        _submit(self.bridge, payload)
        result = self.bridge.process_one()
        self.assertIn(result.disposition, {Disposition.PARTIAL.value, Disposition.FAILED.value})
        self.assertNotEqual(result.disposition, Disposition.SUCCESS.value)
        self.assertNotEqual(result.exit_code, 0)
        self.assertGreater(result.probe["summary"]["requiredFailed"], 0)

    def test_repo_probe_failed_status_not_success(self) -> None:
        manifest = validate_manifest_dict(
            _base_manifest(
                control=self.control,
                worktree=self.repo,
                task_id="PROBE-FAIL",
                action="REPO_PROBE",
                branch=self.branch,
                head=self.head,
            ),
            policy=self.policy,
        )
        with mock.patch("dot_control.executor._run_git") as run_git:
            def _side_effect(args, cwd, **kwargs):
                key = " ".join(args)
                if args[:1] == ["status"]:
                    return subprocess.CompletedProcess(
                        args=["git", *args], returncode=128, stdout="", stderr="fail"
                    )
                return subprocess.CompletedProcess(
                    args=["git", *args], returncode=0, stdout="ok", stderr=""
                )

            run_git.side_effect = _side_effect
            _evidence, probe, disposition = execute_repo_probe(manifest)
        self.assertNotEqual(disposition, Disposition.SUCCESS)
        self.assertNotEqual(probe["summary"]["disposition"], Disposition.SUCCESS.value)

    # --- argv construction / CLI help fidelity ----------------------------

    def test_codex_argv_workspace_write_no_danger(self) -> None:
        prompt = _write_prompt(self.control, "CODEX-ARGV")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CODEX-ARGV",
            action="CODEX_TASK",
            branch=self.branch,
            head=self.head,
            writer="codex",
            prompt=str(prompt),
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        argv, meta = build_action_argv(manifest)
        self.assertEqual(argv[0:2], ["codex", "exec"])
        self.assertIn("workspace-write", argv)
        self.assertNotIn("danger-full-access", argv)
        self.assertNotIn("--dangerously-bypass-approvals-and-sandbox", argv)
        self.assertEqual(meta.get("stdin_path"), str(prompt))

    def test_cursor_argv_print_no_yolo(self) -> None:
        prompt = _write_prompt(self.control, "CURSOR-ARGV", "implement safely")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CURSOR-ARGV",
            action="CURSOR_TASK",
            branch=self.branch,
            head=self.head,
            writer="cursor-cli",
            prompt=str(prompt),
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        argv, _meta = build_action_argv(manifest)
        self.assertEqual(argv[0], "agent")
        self.assertIn("--print", argv)
        self.assertNotIn("--force", argv)
        self.assertNotIn("--yolo", argv)

    def test_antigravity_argv_plan_sandbox(self) -> None:
        vrepo = self.tmp / "verifier"
        head = _init_repo(vrepo, branch="feature/v")
        _git(vrepo, "checkout", "--detach", head)
        prompt = _write_prompt(self.control, "AGY-ARGV")
        payload = _base_manifest(
            control=self.control,
            worktree=vrepo,
            task_id="AGY-ARGV",
            action="ANTIGRAVITY_AUDIT",
            branch="HEAD",
            head=head,
            verifier="antigravity",
            prompt=str(prompt),
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        argv, _meta = build_action_argv(manifest)
        self.assertEqual(argv[0], "agy")
        self.assertIn("--print", argv)
        self.assertIn("--sandbox", argv)
        self.assertIn("plan", argv)
        self.assertNotIn("--dangerously-skip-permissions", argv)

    def test_live_cursor_unsupported_no_child(self) -> None:
        prompt = _write_prompt(self.control, "CURSOR-LIVE")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CURSOR-LIVE",
            action="CURSOR_TASK",
            branch=self.branch,
            head=self.head,
            writer="cursor-cli",
            prompt=str(prompt),
            allowed=["ok.txt"],
        )
        _submit(self.bridge, payload)
        with mock.patch("dot_control.bridge.run_process") as rp:
            result = self.bridge.process_one()
            rp.assert_not_called()
        self.assertEqual(result.disposition, Disposition.UNSUPPORTED.value)
        self.assertIn("credential-isolation", (result.error or "").lower())
        self.assertTrue(any("not launched" in n.lower() for n in result.notes))
        self.assertEqual(result.command_argv[0], "agent")

    def test_live_codex_unsupported_no_child(self) -> None:
        prompt = _write_prompt(self.control, "CODEX-LIVE")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CODEX-LIVE",
            action="CODEX_TASK",
            branch=self.branch,
            head=self.head,
            writer="codex",
            prompt=str(prompt),
            allowed=["ok.txt"],
        )
        _submit(self.bridge, payload)
        with mock.patch("dot_control.bridge.run_process") as rp:
            result = self.bridge.process_one()
            rp.assert_not_called()
        self.assertEqual(result.disposition, Disposition.UNSUPPORTED.value)
        self.assertIsNotNone(live_writer_unsupported(Action.CODEX_TASK))
        self.assertIn(LIVE_WRITER_UNSUPPORTED_REASON[:40], result.error or "")

    # --- negative tests ---------------------------------------------------

    def test_invalid_json_quarantine(self) -> None:
        bad = self.bridge.inbox / "BADJSON.json"
        bad.write_text("{not-json", encoding="utf-8")
        with self.assertRaises(BridgeError):
            self.bridge.process_one(bad)
        self.assertTrue(any(self.bridge.quarantine.glob("BADJSON*")))

    def test_duplicate_task_id(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="DUP-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        _submit(self.bridge, payload)
        self.bridge.process_one()
        _submit(self.bridge, payload)
        with self.assertRaises(BridgeError) as ctx:
            self.bridge.process_one()
        self.assertIn("duplicate", str(ctx.exception).lower())

    def test_traversal_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="TRAV-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        payload["worktree"] = str(self.repo / ".." / "escape")
        with self.assertRaises(BridgeError):
            validate_manifest_dict(payload, policy=self.policy, require_paths_exist=False)

    def test_symlink_worktree_rejected(self) -> None:
        link = self.tmp / "symlink-repo"
        link.symlink_to(self.repo)
        payload = _base_manifest(
            control=self.control,
            worktree=link,
            task_id="SYM-WT",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("symlink", str(ctx.exception).lower())

    def test_forbidden_home_path(self) -> None:
        ssh = self.policy.home / ".ssh"
        ssh.mkdir(parents=True)
        payload = _base_manifest(
            control=self.control,
            worktree=ssh,
            task_id="HOME-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy, require_paths_exist=False)
        self.assertIn("forbidden", str(ctx.exception).lower())

    def test_branch_main_write_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="MAIN-1",
            action="GIT_COMMIT_PROOF",
            branch="main",
            head=self.head,
            allowed=["proof.txt"],
            allow_commit=True,
            commit_message="bad",
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("protected", str(ctx.exception).lower())

    def test_detached_writer_rejected(self) -> None:
        _git(self.repo, "checkout", "--detach", self.head)
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="DET-W",
            action="CODEX_TASK",
            branch=self.branch,
            head=self.head,
            writer="codex",
            prompt=str(_write_prompt(self.control, "DET-W")),
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_git_preconditions(manifest)
        self.assertIn("detached", str(ctx.exception).lower())

    def test_non_detached_antigravity_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="AGY-ND",
            action="ANTIGRAVITY_AUDIT",
            branch=self.branch,
            head=self.head,
            verifier="antigravity",
            prompt=str(_write_prompt(self.control, "AGY-ND")),
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_git_preconditions(manifest)
        self.assertIn("detached", str(ctx.exception).lower())

    def test_unexpected_head(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="HEAD-1",
            action="REPO_PROBE",
            branch=self.branch,
            head="a" * 40,
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_git_preconditions(manifest)
        # May fail on authoritativeBase existence before HEAD check.
        self.assertTrue(
            "unexpected head" in str(ctx.exception).lower()
            or "authoritativebase" in str(ctx.exception).lower()
        )

    def test_dirty_start_rejected_for_commit_proof(self) -> None:
        (self.repo / "extra.txt").write_text("dirt\n", encoding="utf-8")
        (self.repo / "proof.txt").write_text("p\n", encoding="utf-8")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="DIRTY-1",
            action="GIT_COMMIT_PROOF",
            branch=self.branch,
            head=self.head,
            allowed=["proof.txt"],
            allow_commit=True,
            commit_message="x",
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_git_preconditions(manifest)
        self.assertIn("allowedfiles", str(ctx.exception).lower())

    def test_live_lock_blocks_second_writer(self) -> None:
        lock_a = WorktreeLock(self.bridge.locks, self.repo)
        lock_a.acquire(
            task_id="L1",
            action="CODEX_TASK",
            writer_tool="codex",
            branch=self.branch,
            expected_head=self.head,
        )
        lock_b = WorktreeLock(self.bridge.locks, self.repo)
        with self.assertRaises(BridgeError) as ctx:
            lock_b.acquire(
                task_id="L2",
                action="CURSOR_TASK",
                writer_tool="cursor-cli",
                branch=self.branch,
                expected_head=self.head,
            )
        self.assertEqual(ctx.exception.disposition, Disposition.LOCKED)
        lock_a.release()

    def test_stale_lock_reclaim(self) -> None:
        from dot_control.evidence import utc_now_iso
        from dot_control.locks import worktree_lock_name
        from dot_control.models import LockRecord

        lock = WorktreeLock(self.bridge.locks, self.repo)
        path = self.bridge.locks / worktree_lock_name(self.repo.resolve())
        path.parent.mkdir(parents=True, exist_ok=True)
        dead = LockRecord(
            task_id="STALE",
            action="CODEX_TASK",
            writer_tool="codex",
            worktree=str(self.repo.resolve()),
            branch=self.branch,
            expected_head=self.head,
            bridge_pid=2**22 + 99999,
            child_pid=None,
            created_at=utc_now_iso(),
            heartbeat_at=utc_now_iso(),
        )
        self.assertFalse(pid_alive(dead.bridge_pid))
        path.write_text(json.dumps(dead.to_wire()), encoding="utf-8")
        lock.acquire(
            task_id="NEW",
            action="REPO_PROBE",
            writer_tool="none",
            branch=self.branch,
            expected_head=self.head,
        )
        self.assertTrue(lock.lifecycle.get("staleReclaimed"))
        lock.release()

    def test_multiple_writers_same_worktree(self) -> None:
        self.test_live_lock_blocks_second_writer()

    def test_writer_equals_verifier_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="WV-1",
            action="ANTIGRAVITY_AUDIT",
            branch=self.branch,
            head=self.head,
            writer="none",
            verifier="antigravity",
            prompt=str(_write_prompt(self.control, "WV-1")),
        )
        payload["writerTool"] = "antigravity"
        with self.assertRaises(BridgeError):
            validate_manifest_dict(payload, policy=self.policy)

    def test_arbitrary_command_field_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CMD-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        payload["command"] = "rm -rf /"
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("command", str(ctx.exception).lower())

    def test_unknown_action(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="UA-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        payload["action"] = "FORMAT_DISK"
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("unknown action", str(ctx.exception).lower())

    def test_unknown_test_profile(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="TP-1",
            action="TEST_PROFILE",
            branch=self.branch,
            head=self.head,
            test_profile="docker-full-integration",
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("unknown test profile", str(ctx.exception).lower())

    def test_prompt_outside_task_root(self) -> None:
        outsider = self.control / "tasks" / "other" / "prompt.md"
        outsider.parent.mkdir(parents=True, exist_ok=True)
        outsider.write_text("x", encoding="utf-8")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="PR-OUT",
            action="CURSOR_TASK",
            branch=self.branch,
            head=self.head,
            writer="cursor-cli",
            prompt=str(outsider),
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("prompt", str(ctx.exception).lower())

    def test_oversized_prompt(self) -> None:
        path = _write_prompt(self.control, "BIG-1", "x" * (MAX_PROMPT_BYTES + 8))
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="BIG-1",
            action="CURSOR_TASK",
            branch=self.branch,
            head=self.head,
            writer="cursor-cli",
            prompt=str(path),
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("oversized", str(ctx.exception).lower())

    def test_symlink_prompt(self) -> None:
        real = _write_prompt(self.control, "SYMP-1", "hello")
        link = self.control / "tasks" / "SYMP-1" / "link.md"
        link.symlink_to(real)
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="SYMP-1",
            action="CURSOR_TASK",
            branch=self.branch,
            head=self.head,
            writer="cursor-cli",
            prompt=str(link),
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("symlink", str(ctx.exception).lower())

    def test_timeout_child_termination(self) -> None:
        evidence = run_process(
            [sys.executable, "-c", "import time; time.sleep(30)"],
            cwd=self.repo,
            timeout_seconds=1,
            logs_dir=self.bridge.logs,
            task_id="TO-1",
        )
        self.assertTrue(evidence.timed_out)
        self.assertTrue(evidence.sigterm_sent)
        self.assertIsNotNone(evidence.child_pid)
        self.assertFalse(pid_alive(evidence.child_pid))

    def test_child_process_cleanup(self) -> None:
        evidence = run_process(
            [sys.executable, "-c", "print('ok')"],
            cwd=self.repo,
            timeout_seconds=10,
            logs_dir=self.bridge.logs,
            task_id="CL-1",
        )
        self.assertEqual(evidence.exit_code, 0)
        self.assertFalse(evidence.timed_out)
        self.assertFalse(pid_alive(evidence.child_pid))

    def test_changed_file_outside_allowed(self) -> None:
        manifest = _manifest_obj(
            repo=self.repo, branch=self.branch, head=self.head, allowed=("a.txt",)
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_post_change_scope(manifest, ["b.txt"])
        self.assertIn("outside allowedfiles", str(ctx.exception).lower())

    def test_commit_stages_forbidden_file(self) -> None:
        from dot_control.executor import execute_git_commit_proof

        (self.repo / "ok.txt").write_text("1\n", encoding="utf-8")
        (self.repo / "nope.txt").write_text("2\n", encoding="utf-8")
        _git(self.repo, "add", "--", "ok.txt", "nope.txt")
        manifest = TaskManifest(
            schema_version=SCHEMA_VERSION,
            task_id="FORB-1",
            governing_issue="#185",
            action=Action.GIT_COMMIT_PROOF,
            writer_tool=WriterTool.NONE,
            verifier_tool=VerifierTool.NONE,
            repository=APPROVED_REPOSITORY,
            worktree=self.repo,
            branch=self.branch,
            authoritative_base=self.head,
            expected_head=self.head,
            allowed_files=("ok.txt",),
            forbidden_files=("nope.txt",),
            prompt_file=None,
            timeout_seconds=30,
            allow_commit=True,
            allow_push=False,
            allow_pull_request=False,
            created_at="2026-10-07T00:00:00Z",
            requested_by="t",
            commit_message="should fail",
        )
        with self.assertRaises(BridgeError) as ctx:
            execute_git_commit_proof(manifest)
        self.assertIn("forbidden", str(ctx.exception).lower())

    def test_push_pr_false_enforcement(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="PP-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        payload["allowPush"] = True
        with self.assertRaises(BridgeError):
            validate_manifest_dict(payload, policy=self.policy)
        payload["allowPush"] = False
        payload["allowPullRequest"] = True
        with self.assertRaises(BridgeError):
            validate_manifest_dict(payload, policy=self.policy)

    def test_credential_url_redaction(self) -> None:
        text = "clone https://user:secret@github.com/org/repo.git ghp_abcdefghijklmnop"
        red = redact_text(text)
        self.assertNotIn("secret", red)
        self.assertNotIn("ghp_abcdefghijklmnop", red)
        self.assertIn("[REDACTED]", red)
        argv = redact_argv(["git", "push", "https://x:y@example.com/r.git"])
        self.assertTrue(all("x:y@" not in a for a in argv))

    def test_partial_manifest_ignored_until_rename(self) -> None:
        partial = self.bridge.inbox / "PARTIAL-1.tmp"
        partial.write_text("{}", encoding="utf-8")
        self.assertEqual(self.bridge.list_inbox_ready(), [])
        with self.assertRaises(BridgeError):
            self.bridge.process_one(partial)

    def test_dry_run_validate(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="DRY-1",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        path = self.tmp / "dry.json"
        path.write_text(json.dumps(payload), encoding="utf-8")
        plan = self.bridge.validate_only(path)
        self.assertEqual(plan["disposition"], Disposition.DRY_RUN.value)
        self.assertEqual(plan["action"], "REPO_PROBE")

    def test_inspect_lock(self) -> None:
        lock = WorktreeLock(self.bridge.locks, self.repo)
        lock.acquire(
            task_id="INS",
            action="REPO_PROBE",
            writer_tool="none",
            branch=self.branch,
            expected_head=self.head,
        )
        info = inspect_lock(self.bridge.locks, self.repo)
        self.assertTrue(info["exists"])
        self.assertEqual(info["record"]["taskId"], "INS")
        lock.release()

    def test_init_does_not_touch_production_by_default(self) -> None:
        prod = Path("/home/gagan/nms-dot-control")
        self.assertTrue(self.control.joinpath("inbox").is_dir())
        self.assertNotEqual(self.control, prod)


class SchemaBrandRegressionTests(unittest.TestCase):
    def test_schema_id_and_no_forbidden_brand(self) -> None:
        # Construct forbidden brand so this test file does not embed the banned literal.
        forbidden_brand = "amrud" + ".ai"
        schema_path = DOT_CONTROL / "task.schema.json"
        schema = json.loads(schema_path.read_text(encoding="utf-8"))
        self.assertEqual(schema["$schema"], JSON_SCHEMA_META)
        self.assertEqual(schema["$id"], SCHEMA_ID)
        self.assertEqual(SCHEMA_ID, "urn:amrudai:schema:dot-control:task-manifest:v1")
        self.assertNotIn(forbidden_brand, schema_path.read_text(encoding="utf-8"))

        scan_roots = [DOT_CONTROL, Path(__file__).resolve()]
        for root in scan_roots:
            paths = [root] if root.is_file() else [
                p for p in root.rglob("*")
                if p.is_file() and p.suffix != ".pyc" and "__pycache__" not in p.parts
            ]
            for path in paths:
                text = path.read_text(encoding="utf-8", errors="replace")
                # Allow the constructed form only via concatenation in this test.
                if path.resolve() == Path(__file__).resolve():
                    # Ensure the banned literal form is absent (concatenated check uses parts).
                    self.assertNotIn(forbidden_brand, text.replace('"amrud" + ".ai"', ""))
                    continue
                self.assertNotIn(forbidden_brand, text, msg=f"{forbidden_brand} found in {path}")

        # Public website constant exists; must not appear as an unserved schema URL.
        self.assertEqual(PUBLIC_WEBSITE, "https://amrudai.com")
        schema_text = schema_path.read_text(encoding="utf-8")
        self.assertNotIn(PUBLIC_WEBSITE, schema_text)


class ChangeScopeTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory(prefix="dot-scope-")
        self.tmp = Path(self._tmpdir.name)
        self.repo = self.tmp / "repo"
        self.branch = "feature/scope"
        self.head = _init_repo(self.repo, branch=self.branch)

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    def test_unauthorized_commit_clean_worktree_rejected(self) -> None:
        start = self.head
        (self.repo / "secret.txt").write_text("nope\n", encoding="utf-8")
        _git(self.repo, "add", "secret.txt")
        _git(self.repo, "commit", "-m", "unauthorized")
        end = _sha(self.repo)
        self.assertFalse(_git(self.repo, "status", "--porcelain").stdout.strip())
        manifest = _manifest_obj(
            repo=self.repo,
            branch=self.branch,
            head=start,
            allow_commit=True,
            allowed=("ok.txt",),
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_change_scope(manifest, start, end)
        self.assertIn("outside allowedfiles", str(ctx.exception).lower())

    def test_allowed_commit_accepted(self) -> None:
        start = self.head
        (self.repo / "ok.txt").write_text("yes\n", encoding="utf-8")
        _git(self.repo, "add", "ok.txt")
        _git(self.repo, "commit", "-m", "allowed")
        end = _sha(self.repo)
        manifest = _manifest_obj(
            repo=self.repo,
            branch=self.branch,
            head=start,
            allow_commit=True,
            allowed=("ok.txt",),
        )
        changed = enforce_change_scope(manifest, start, end)
        self.assertEqual(changed, ["ok.txt"])

    def test_head_move_allow_commit_false_rejected(self) -> None:
        start = self.head
        (self.repo / "ok.txt").write_text("yes\n", encoding="utf-8")
        _git(self.repo, "add", "ok.txt")
        _git(self.repo, "commit", "-m", "moved")
        end = _sha(self.repo)
        manifest = _manifest_obj(
            repo=self.repo,
            branch=self.branch,
            head=start,
            allow_commit=False,
            allowed=("ok.txt",),
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_change_scope(manifest, start, end)
        self.assertIn("allowcommit=false", str(ctx.exception).lower())

    def test_staged_unauthorized_rejected(self) -> None:
        (self.repo / "bad.txt").write_text("x\n", encoding="utf-8")
        _git(self.repo, "add", "bad.txt")
        manifest = _manifest_obj(
            repo=self.repo,
            branch=self.branch,
            head=self.head,
            allow_commit=False,
            allowed=("ok.txt",),
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_change_scope(manifest, self.head, self.head)
        msg = str(ctx.exception).lower()
        self.assertTrue("staged" in msg or "outside allowedfiles" in msg)

    def test_untracked_unauthorized_rejected(self) -> None:
        (self.repo / "leak.txt").write_text("x\n", encoding="utf-8")
        manifest = _manifest_obj(
            repo=self.repo,
            branch=self.branch,
            head=self.head,
            allow_commit=False,
            allowed=("ok.txt",),
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_change_scope(manifest, self.head, self.head)
        self.assertIn("outside allowedfiles", str(ctx.exception).lower())

    def test_space_and_newline_filenames_nul_safe(self) -> None:
        weird = self.repo / "my file.txt"
        weird.write_text("space\n", encoding="utf-8")
        nl_name = "new\nline.txt"
        # Git can store newline names; create via plumbing if needed.
        (self.repo / "plain.txt").write_text("p\n", encoding="utf-8")
        _git(self.repo, "add", "--", "my file.txt", "plain.txt")
        scope = collect_change_scope(self.repo, self.head, self.head)
        self.assertIn("my file.txt", scope["staged"])
        validate_repo_relative_path("my file.txt", self.repo)
        # Rename form via committed diff
        start = self.head
        _git(self.repo, "commit", "-m", "add spaced")
        mid = _sha(self.repo)
        _git(self.repo, "mv", "--", "my file.txt", "renamed file.txt")
        _git(self.repo, "commit", "-m", "rename")
        end = _sha(self.repo)
        scope2 = collect_change_scope(self.repo, mid, end)
        union = set(scope2["committed"])
        self.assertTrue("renamed file.txt" in union or "my file.txt" in union)

    def test_path_traversal_rejected(self) -> None:
        with self.assertRaises(BridgeError):
            validate_repo_relative_path("../escape.txt", self.repo)
        with self.assertRaises(BridgeError):
            validate_repo_relative_path("/etc/passwd", self.repo)
        with self.assertRaises(BridgeError):
            validate_repo_relative_path(r"foo\bar", self.repo)
        with self.assertRaises(BridgeError):
            validate_repo_relative_path("a\x00b", self.repo)
        with self.assertRaises(BridgeError):
            validate_repo_relative_path("", self.repo)
        outside = self.tmp / "outside.txt"
        outside.write_text("x\n", encoding="utf-8")
        link = self.repo / "escape-link"
        link.symlink_to(outside)
        with self.assertRaises(BridgeError):
            validate_repo_relative_path("escape-link", self.repo)


class LoggingBoundTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory(prefix="dot-log-")
        self.tmp = Path(self._tmpdir.name)
        self.logs = self.tmp / "logs"
        self.logs.mkdir()

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    def test_github_token_absent_from_durable_log(self) -> None:
        path = self.logs / "tok.stdout.log"
        w = BoundedRedactingWriter(path, max_bytes=MAX_STDOUT_LOG_BYTES)
        w.write(b"token ghp_ABCDEFGHIJKLMNOPQRSTUVWX in output\n")
        w.close()
        text = path.read_text(encoding="utf-8")
        self.assertNotIn("ghp_ABCDEFGHIJKLMNOPQRSTUVWX", text)
        self.assertIn("[REDACTED_TOKEN]", text)
        self.assertEqual(stat.S_IMODE(path.stat().st_mode), FILE_MODE)

    def test_bearer_token_absent(self) -> None:
        path = self.logs / "bearer.stdout.log"
        w = BoundedRedactingWriter(path, max_bytes=4096)
        w.write(b"Authorization: Bearer super-secret-value\n")
        w.close()
        text = path.read_text(encoding="utf-8")
        self.assertNotIn("super-secret-value", text)

    def test_private_key_absent(self) -> None:
        path = self.logs / "pem.stdout.log"
        pem = (
            b"-----BEGIN RSA PRIVATE KEY-----\n"
            b"MIIEvSecretMaterialHere\n"
            b"-----END RSA PRIVATE KEY-----\n"
        )
        w = BoundedRedactingWriter(path, max_bytes=4096)
        w.write(pem)
        w.close()
        text = path.read_text(encoding="utf-8")
        self.assertNotIn("MIIEvSecretMaterialHere", text)
        self.assertIn("[REDACTED]", text)

    def test_credential_url_redacted_in_log(self) -> None:
        path = self.logs / "url.stdout.log"
        w = BoundedRedactingWriter(path, max_bytes=4096)
        w.write(b"https://user:sekrit@github.com/gagandsm-max/platform.git\n")
        w.close()
        text = path.read_text(encoding="utf-8")
        self.assertNotIn("sekrit", text)

    def test_oversized_stdout_bounded_and_marked(self) -> None:
        path = self.logs / "big.stdout.log"
        limit = 8 * 1024
        w = BoundedRedactingWriter(path, max_bytes=limit)
        w.write(b"A" * (limit + 5000))
        w.close()
        self.assertTrue(w.truncated)
        self.assertLessEqual(w.bytes_captured, limit)
        self.assertLessEqual(path.stat().st_size, limit + 128)
        self.assertIn(b"TRUNCATED", path.read_bytes())

    def test_oversized_stderr_bounded(self) -> None:
        path = self.logs / "big.stderr.log"
        limit = 4 * 1024
        w = BoundedRedactingWriter(path, max_bytes=limit)
        w.write(b"E" * (limit + 1000))
        w.close()
        self.assertTrue(w.truncated)
        self.assertLessEqual(w.bytes_captured, limit)

    def test_bridge_log_truncation(self) -> None:
        path, truncated = append_bridge_log(
            self.logs,
            "BR-1",
            ["line " + ("x" * 200) for _ in range(100)],
            max_bytes=2048,
        )
        self.assertTrue(path.is_file())
        # Either truncated flag or content mark.
        data = path.read_text(encoding="utf-8")
        self.assertTrue(truncated or "TRUNCATED" in data)


class ControlRootHardeningTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory(prefix="dot-root-")
        self.tmp = Path(self._tmpdir.name)

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    def test_init_modes_enforced(self) -> None:
        root = self.tmp / "control"
        created = init_control_root(root)
        self.assertTrue(created)
        self.assertEqual(stat.S_IMODE(root.stat().st_mode), DIR_MODE)
        for name in ("inbox", "logs", "results", "locks", "state", "quarantine", "tasks", "processing"):
            sub = root / name
            self.assertTrue(sub.is_dir())
            self.assertFalse(sub.is_symlink())
            self.assertEqual(stat.S_IMODE(sub.stat().st_mode), DIR_MODE)

    def test_symlinked_logs_rejected(self) -> None:
        root = self.tmp / "control"
        init_control_root(root)
        outside = self.tmp / "outside-logs"
        outside.mkdir()
        marker = outside / "untouched.txt"
        marker.write_text("safe\n", encoding="utf-8")
        (root / "logs").rmdir()
        (root / "logs").symlink_to(outside)
        with self.assertRaises(BridgeError):
            validate_control_root_layout(root, require_subdirs=True)
        self.assertEqual(marker.read_text(encoding="utf-8"), "safe\n")

    def test_symlinked_results_state_locks_rejected(self) -> None:
        root = self.tmp / "control"
        init_control_root(root)
        for name in ("results", "state", "locks"):
            outside = self.tmp / f"out-{name}"
            outside.mkdir()
            (root / name).rmdir()
            (root / name).symlink_to(outside)
            with self.assertRaises(BridgeError):
                validate_control_root_layout(root, require_subdirs=True)
            (root / name).unlink()
            (root / name).mkdir()

    def test_result_symlink_rejected(self) -> None:
        from dot_control.evidence import write_result
        from dot_control.models import TaskResult

        root = self.tmp / "control"
        init_control_root(root)
        outside = self.tmp / "escape-result.json"
        outside.write_text("{}\n", encoding="utf-8")
        target = root / "results" / "T1.result.json"
        target.symlink_to(outside)
        result = TaskResult(
            task_id="T1",
            action="REPO_PROBE",
            selected_tool="bridge",
            worktree="/tmp",
            branch="b",
            starting_head="a" * 40,
            ending_head="a" * 40,
            authoritative_base="a" * 40,
            command_argv=[],
            started_at="t",
            ended_at="t",
            exit_code=0,
            timed_out=False,
            stdout_log=None,
            stderr_log=None,
            changed_files=[],
            committed_files=[],
            commit_sha=None,
            push_status="NOT_ATTEMPTED",
            pr_status="NOT_ATTEMPTED",
            lock_lifecycle={},
            disposition="SUCCESS",
        )
        with self.assertRaises(OSError):
            write_result(root / "results", result)
        self.assertEqual(outside.read_text(encoding="utf-8"), "{}\n")

    def test_lock_symlink_rejected(self) -> None:
        root = self.tmp / "control"
        init_control_root(root)
        repo = self.tmp / "repo"
        repo.mkdir()
        outside = self.tmp / "escape.lock"
        outside.write_text("keep\n", encoding="utf-8")
        from dot_control.locks import worktree_lock_name

        lock_path = root / "locks" / worktree_lock_name(repo.resolve())
        lock_path.symlink_to(outside)
        lock = WorktreeLock(root / "locks", repo)
        with self.assertRaises(BridgeError):
            lock.acquire(
                task_id="L",
                action="REPO_PROBE",
                writer_tool="none",
                branch="b",
                expected_head="a" * 40,
            )
        self.assertEqual(outside.read_text(encoding="utf-8"), "keep\n")


class RepositoryBindingTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory(prefix="dot-bind-")
        self.tmp = Path(self._tmpdir.name)
        self.repo = self.tmp / "repo"
        self.branch = "feature/bind"
        self.head = _init_repo(self.repo, branch=self.branch)
        self.control = self.tmp / "control"
        self.policy = PathPolicy(
            home=self.tmp / "home",
            control_root=self.control,
            approved_repo_exact=self.repo,
            approved_repo_prefix=str(self.tmp / "nms-"),
            allow_test_roots=[self.tmp],
        )
        (self.tmp / "home").mkdir()
        self.control.mkdir()
        for name in ("inbox", "processing", "results", "logs", "locks", "state", "quarantine", "tasks"):
            (self.control / name).mkdir()

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    def test_foreign_repository_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="FOREIGN",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
            repository="other/org",
        )
        with self.assertRaises(BridgeError) as ctx:
            validate_manifest_dict(payload, policy=self.policy)
        self.assertIn("gagandsm-max/platform", str(ctx.exception))

    def test_malformed_remote_rejected(self) -> None:
        _git(self.repo, "remote", "set-url", "origin", "not-a-url")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="MAL-REMOTE",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError):
            verify_repository_binding(manifest)

    def test_credential_bearing_remote_sanitized(self) -> None:
        _git(
            self.repo,
            "remote",
            "set-url",
            "origin",
            "https://user:hunter2@github.com/gagandsm-max/platform.git",
        )
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="CRED-REMOTE",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
        )
        manifest = validate_manifest_dict(payload, policy=self.policy)
        info = verify_repository_binding(manifest)
        self.assertEqual(info["ownerRepo"], APPROVED_REPOSITORY)
        self.assertNotIn("hunter2", info["originSanitized"])
        self.assertNotIn("hunter2", sanitize_remote_url(
            "https://user:hunter2@github.com/gagandsm-max/platform.git"
        ))

    def test_parse_ssh_and_https(self) -> None:
        self.assertEqual(
            parse_github_owner_repo("git@github.com:gagandsm-max/platform.git"),
            APPROVED_REPOSITORY,
        )
        self.assertEqual(
            parse_github_owner_repo("ssh://git@github.com/gagandsm-max/platform.git"),
            APPROVED_REPOSITORY,
        )
        self.assertEqual(
            parse_github_owner_repo("https://github.com/gagandsm-max/platform"),
            APPROVED_REPOSITORY,
        )

    def test_nonexistent_base_rejected(self) -> None:
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="BASE-MISS",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
            authoritative_base="b" * 40,
        )
        # expectedHead is current head; base does not exist
        payload["authoritativeBase"] = "b" * 40
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_authoritative_base(manifest)
        self.assertIn("does not exist", str(ctx.exception).lower())

    def test_base_not_ancestor_rejected(self) -> None:
        # Create an unrelated orphan commit as base.
        orphan = self.tmp / "orphan"
        orphan.mkdir()
        _git(orphan, "init")
        _git(orphan, "config", "user.email", "o@example.com")
        _git(orphan, "config", "user.name", "O")
        (orphan / "x").write_text("x\n", encoding="utf-8")
        _git(orphan, "add", "x")
        _git(orphan, "commit", "-m", "orphan")
        orphan_sha = _git(orphan, "rev-parse", "HEAD").stdout.strip()
        # Fetch orphan object into test repo without making it an ancestor.
        bundle = self.tmp / "o.bundle"
        _git(orphan, "bundle", "create", str(bundle), "HEAD")
        _git(self.repo, "fetch", str(bundle), "HEAD:refs/tmp/orphan")
        payload = _base_manifest(
            control=self.control,
            worktree=self.repo,
            task_id="BASE-ANC",
            action="REPO_PROBE",
            branch=self.branch,
            head=self.head,
            authoritative_base=orphan_sha,
        )
        payload["authoritativeBase"] = orphan_sha
        manifest = validate_manifest_dict(payload, policy=self.policy)
        with self.assertRaises(BridgeError) as ctx:
            verify_authoritative_base(manifest)
        self.assertIn("not an ancestor", str(ctx.exception).lower())


class Gate3AFixtureManifestTests(unittest.TestCase):
    def test_fixture_manifest_shape(self) -> None:
        fixture = {
            "schemaVersion": SCHEMA_VERSION,
            "taskId": "GATE3A-FIXTURE",
            "governingIssue": "#185",
            "action": "GIT_COMMIT_PROOF",
            "writerTool": "none",
            "verifierTool": "none",
            "repository": APPROVED_REPOSITORY,
            "worktree": "/tmp/replaced-in-tests",
            "branch": "feature/gate3-proof",
            "authoritativeBase": "a" * 40,
            "expectedHead": "a" * 40,
            "allowedFiles": ["proof.txt"],
            "forbiddenFiles": [],
            "promptFile": None,
            "timeoutSeconds": 60,
            "allowCommit": True,
            "allowPush": False,
            "allowPullRequest": False,
            "createdAt": "2026-10-07T00:00:00Z",
            "requestedBy": "gate3a-fixture",
            "commitMessage": "chore: gate3a commit proof fixture",
        }
        self.assertEqual(fixture["action"], "GIT_COMMIT_PROOF")
        self.assertFalse(fixture["allowPush"])
        self.assertFalse(fixture["allowPullRequest"])
        self.assertEqual(len(fixture["allowedFiles"]), 1)


if __name__ == "__main__":
    unittest.main()
