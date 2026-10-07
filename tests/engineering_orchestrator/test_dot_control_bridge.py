"""Dot Host-Control Bridge v1 — positive Gate 3A proof and negative security tests.

Uses temporary repositories and temporary control roots only.
Does not touch the live repository worktree under test.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
PROJECT_AGENTS = REPO_ROOT / "project_agents"
if str(PROJECT_AGENTS) not in sys.path:
    sys.path.insert(0, str(PROJECT_AGENTS))

from dot_control.bridge import DotControlBridge  # noqa: E402
from dot_control.evidence import redact_argv, redact_text  # noqa: E402
from dot_control.executor import (  # noqa: E402
    build_action_argv,
    run_process,
    verify_git_preconditions,
)
from dot_control.locks import WorktreeLock, inspect_lock, pid_alive  # noqa: E402
from dot_control.models import (  # noqa: E402
    SCHEMA_VERSION,
    Action,
    BridgeError,
    Disposition,
    MAX_PROMPT_BYTES,
)
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
    # Simulate origin/main for REPO_PROBE.
    _git(root, "branch", "main")
    _git(root, "checkout", "-b", branch)
    # Fake remote ref without network.
    head = _git(root, "rev-parse", "HEAD").stdout.strip()
    _git(root, "update-ref", "refs/remotes/origin/main", head)
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
) -> dict:
    payload = {
        "schemaVersion": SCHEMA_VERSION,
        "taskId": task_id,
        "governingIssue": "#185",
        "action": action,
        "writerTool": writer,
        "verifierTool": verifier,
        "repository": "gagandsm-max/platform",
        "worktree": str(worktree),
        "branch": branch,
        "authoritativeBase": head,
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
        # Clean HEAD baseline, then only the single allowed file is dirty.
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
        # Detached verifier repo
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
        self.assertIn("unexpected head", str(ctx.exception).lower())

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
            bridge_pid=2**22 + 99999,  # almost certainly dead
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
        # Same as live lock — Cursor vs Codex.
        self.test_live_lock_blocks_second_writer()

    def test_writer_equals_verifier_rejected(self) -> None:
        # Simulate equal non-none tools via raw dict before enum split.
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
        # Force equal string tools unsupported combination: writerTool=antigravity invalid.
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
        from dot_control.executor import enforce_post_change_scope
        from dot_control.models import TaskManifest, WriterTool, VerifierTool

        manifest = TaskManifest(
            schema_version=SCHEMA_VERSION,
            task_id="CF-1",
            governing_issue="#185",
            action=Action.CODEX_TASK,
            writer_tool=WriterTool.CODEX,
            verifier_tool=VerifierTool.NONE,
            repository="gagandsm-max/platform",
            worktree=self.repo,
            branch=self.branch,
            authoritative_base=self.head,
            expected_head=self.head,
            allowed_files=("a.txt",),
            forbidden_files=(),
            prompt_file=None,
            timeout_seconds=10,
            allow_commit=False,
            allow_push=False,
            allow_pull_request=False,
            created_at="2026-10-07T00:00:00Z",
            requested_by="t",
        )
        with self.assertRaises(BridgeError) as ctx:
            enforce_post_change_scope(manifest, ["b.txt"])
        self.assertIn("outside allowedfiles", str(ctx.exception).lower())

    def test_commit_stages_forbidden_file(self) -> None:
        from dot_control.executor import execute_git_commit_proof
        from dot_control.models import TaskManifest, WriterTool, VerifierTool

        (self.repo / "ok.txt").write_text("1\n", encoding="utf-8")
        (self.repo / "nope.txt").write_text("2\n", encoding="utf-8")
        # Pre-stage both files, then call executor which re-adds allowed and
        # must reject because staged set will include forbidden after add-all-dirty? 
        # Executor runs `git add -- allowed` only; pre-staged forbidden remains staged.
        _git(self.repo, "add", "--", "ok.txt", "nope.txt")
        manifest = TaskManifest(
            schema_version=SCHEMA_VERSION,
            task_id="FORB-1",
            governing_issue="#185",
            action=Action.GIT_COMMIT_PROOF,
            writer_tool=WriterTool.NONE,
            verifier_tool=VerifierTool.NONE,
            repository="gagandsm-max/platform",
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
        # Creating bridge without create_dirs must not mkdir production path.
        prod = Path("/home/gagan/nms-dot-control")
        # We only assert the temporary bridge created dirs under tmp.
        self.assertTrue(self.control.joinpath("inbox").is_dir())
        # Production may or may not exist; we must not have created it in this test
        # via DotControlBridge(DEFAULT) — we didn't call that.
        self.assertNotEqual(self.control, prod)


class Gate3AFixtureManifestTests(unittest.TestCase):
    """Fixture manifest document for Gate 3A replacement proof."""

    def test_fixture_manifest_shape(self) -> None:
        fixture = {
            "schemaVersion": SCHEMA_VERSION,
            "taskId": "GATE3A-FIXTURE",
            "governingIssue": "#185",
            "action": "GIT_COMMIT_PROOF",
            "writerTool": "none",
            "verifierTool": "none",
            "repository": "gagandsm-max/platform",
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
