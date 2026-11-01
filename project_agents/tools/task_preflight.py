#!/usr/bin/env python3
"""Issue #285 Phase A: read-only structural inspection, NOT authorization.

Run from a trusted control-plane installation, never as privileged PR code.
The CLI cannot accept an actor, credentials, a policy override or CI claims.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
from typing import Any, Mapping

import control_plane as native

REPOSITORY = "gagandsm-max/platform"
REMOTE = "https://github.com/gagandsm-max/platform.git"
REMOTE_URLS = frozenset({REMOTE, REMOTE[:-4],
    "git@github.com:gagandsm-max/platform.git", "git@github.com:gagandsm-max/platform",
    "ssh://git@github.com/gagandsm-max/platform.git", "ssh://git@github.com/gagandsm-max/platform"})
WRITER_STATES = frozenset({"IMPLEMENTATION", "LOCAL_VALIDATION"})
REVIEW_STATES = frozenset({"SOURCE_REVIEW", "ADVERSARIAL_RUNTIME",
    "SECURITY_DOMAIN_REVIEW", "CI_VALIDATION", "REALITY_GATE"})
CERTIFY_STATES = native.WRITER_SELF_CERTIFY_FORBIDDEN
# Deliberate policy changes require a separate governance lane. A task scope
# cannot grant itself an exception. Also protect nested AGENTS.md anywhere.
POLICY_PREFIXES = ("project_documentation", "project_agents",
    ".github", ".agents", ".cursor", ".codex", "skills")
POLICY_FILES = frozenset({"AGENTS.md", "project_agents/tool_authority.json",
    "project_agents/control_plane_state_machine.json", "project_agents/manifest.json"})
# These installed inputs must match current main before the new tool can inspect
# an ordinary lane. No target Python is imported, even if its bytes match.
NATIVE_INPUTS = ("project_agents/tools/control_plane.py", "project_agents/tool_authority.json",
    "project_agents/control_plane_state_machine.json", "project_agents/manifest.json",
    "project_agents/skills/manifest.json", "project_agents/schemas/task_manifest.schema.json",
    "project_agents/schemas/handoff.schema.json")
MAX_JSON_BYTES = 1_048_576
GITHUB_CLI = "/usr/bin/gh"
SOURCE_TIMEOUT_SECONDS = 20
# A read-only query, never supplied by the task/checkout. Authentication is
# performed by the installed CLI, independently of local Git configuration.
SOURCE_QUERY = '''query($branch:String!){repository(owner:"gagandsm-max",name:"platform"){
nameWithOwner isPrivate main:ref(qualifiedName:"refs/heads/main"){name target{__typename oid}}
branch:ref(qualifiedName:$branch){name target{__typename oid}}}}'''


class PreflightError(ValueError):
    """Stable fail-closed error; details never include Git stderr or credentials."""

    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


def fail(code: str) -> None:
    raise PreflightError(code)


def _environment() -> dict[str, str]:
    # Exclude inherited GIT_DIR, alternate object stores, config injection,
    # SSH commands and credential helpers from target inspection.
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("GIT_", "GH_", "GITHUB_"))}
    env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               GIT_TERMINAL_PROMPT="0", GIT_NO_REPLACE_OBJECTS="1", LC_ALL="C")
    return env


def _git(repo: Path | None, *args: str) -> bytes:
    command = ["git", "--no-optional-locks", "-c", "core.fsmonitor=false",
        "-c", "core.hooksPath=/dev/null", "-c", "core.attributesFile=/dev/null",
        "-c", "protocol.ext.allow=never", "-c", "diff.ignoreSubmodules=none"]
    if repo is not None:
        command += ["-C", str(repo)]
    try:
        result = subprocess.run(command + list(args), cwd="/", env=_environment(),
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20, check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise PreflightError("GIT_UNAVAILABLE_OR_TIMEOUT") from None
    if result.returncode:
        fail("GIT_COMMAND_FAILED")
    return result.stdout


def _text(repo: Path | None, *args: str) -> str:
    try:
        return _git(repo, *args).decode("utf-8", errors="strict").rstrip("\n")
    except UnicodeError:
        raise PreflightError("INVALID_GIT_ENCODING") from None


def _sha(value: Any) -> str:
    if not isinstance(value, str) or not native.GIT_SHA_RE.fullmatch(value):
        fail("INVALID_SHA")
    return value


def _path(value: Any) -> str:
    if (not isinstance(value, str) or not value or value.startswith(("/", "-"))
            or "\\" in value or any(ord(c) < 32 or ord(c) == 127 for c in value)
            or any(p in {"", ".", "..", ".git"} for p in value.split("/"))
            or any(c in value for c in "*?[]")):
        fail("UNSAFE_PATH")
    return value


def _paths(repo: Path, *args: str) -> list[str]:
    data = _git(repo, *args)
    if data and not data.endswith(b"\0"):
        fail("INVALID_GIT_PATH_OUTPUT")
    try:
        return [_path(p.decode("utf-8", errors="strict")) for p in data.split(b"\0") if p]
    except UnicodeError:
        raise PreflightError("INVALID_PATH_ENCODING") from None


def _branch(value: Any) -> str:
    if (not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9._/-]+", value)
            or value.startswith(("-", "refs/")) or value == "main"
            or ".." in value or "//" in value
            or any(p.startswith(".") or p.endswith((".lock", ".")) for p in value.split("/"))
            or value.endswith("/")):
        fail("INVALID_TASK_BRANCH")
    return value


def _source_environment() -> dict[str, str]:
    # Only trusted launcher authentication settings reach gh. No GH_DEBUG,
    # host/URL/proxy/TLS overrides, repo config, extensions or task actor string.
    # A credential able to read source does NOT attest a writer/verifier role.
    keys = ("HOME", "XDG_CONFIG_HOME", "GH_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN")
    env = {key: os.environ[key] for key in keys if key in os.environ}
    env.update(PATH="/usr/bin:/bin", GH_PROMPT_DISABLED="1", GH_NO_UPDATE_NOTIFIER="1",
               GH_NO_EXTENSION_UPDATE_NOTIFIER="1", LC_ALL="C", NO_COLOR="1")
    return env


def _source_response(branch: str) -> bytes:
    command = [GITHUB_CLI, "api", "graphql", "--hostname", "github.com",
               "-f", "query=" + SOURCE_QUERY, "-f", "branch=refs/heads/" + _branch(branch)]
    failure = None
    try:
        result = subprocess.run(command, cwd="/", env=_source_environment(),
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
            timeout=SOURCE_TIMEOUT_SECONDS, check=False)
    except subprocess.TimeoutExpired:
        failure = "SOURCE_TIMEOUT"
    except OSError:
        failure = "SOURCE_ADAPTER_UNAVAILABLE"
    # Raise outside the handler: no credential-bearing subprocess exception
    # survives in __context__ / __cause__, not merely a suppressed traceback.
    if failure:
        fail(failure)
    if result.returncode:
        fail("SOURCE_ACCESS_FAILED")
    if len(result.stdout) > MAX_JSON_BYTES:
        fail("INVALID_REMOTE_RESPONSE")
    return result.stdout


def remote_heads(branch: str) -> dict[str, str | None]:
    """Read private GitHub refs through the trusted installed CLI; no fallback."""
    branch = _branch(branch)
    response = _source_response(branch)
    payload = None
    try:
        payload = json.loads(response, object_pairs_hook=_unique_object)
    except (ValueError, UnicodeError, RecursionError):
        pass
    if not isinstance(payload, dict) or payload.get("errors") or not isinstance(payload.get("data"), dict):
        fail("INVALID_REMOTE_RESPONSE")
    repository = payload["data"].get("repository")
    if (not isinstance(repository, dict) or repository.get("nameWithOwner") != REPOSITORY
            or type(repository.get("isPrivate")) is not bool):
        fail("INVALID_REMOTE_RESPONSE")

    def oid(value: Any, name: str) -> str:
        if not isinstance(value, dict) or value.get("name") != name:
            fail("INVALID_REMOTE_RESPONSE")
        target = value.get("target")
        if not isinstance(target, dict) or target.get("__typename") != "Commit":
            fail("INVALID_REMOTE_RESPONSE")
        return _sha(target.get("oid"))

    main = oid(repository.get("main"), "main")
    # Only an explicitly null ref in a successful repository read means a new
    # unpublished branch. Permission failures/partial GraphQL data never do.
    if "branch" not in repository:
        fail("INVALID_REMOTE_RESPONSE")
    remote = None if repository["branch"] is None else oid(repository["branch"], branch)
    return {"main": main, "branch": remote}


def changes(repo: Path, base: str) -> dict[str, list[str]]:
    _sha(base)
    # Separate index and worktree diffs: HEAD->worktree alone loses staged
    # changes that an unstaged edit reverses. Disable rename pairing so BOTH
    # endpoints must fit scope. Include ignored untracked files deliberately.
    common = ("diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-only", "-z")
    return {
        "committed": sorted(set(_paths(repo, *common, base, "HEAD", "--"))),
        "staged": sorted(set(_paths(repo, *common, "--cached", "HEAD", "--"))),
        "unstaged": sorted(set(_paths(repo, *common, "--"))),
        "untracked": sorted(set(_paths(repo, "ls-files", "-z", "--others", "--exclude-standard"))),
        "ignored": sorted(set(_paths(repo, "ls-files", "-z", "--others", "--ignored", "--exclude-standard"))),
    }


def _protected(path: str) -> bool:
    return (path in POLICY_FILES or PurePosixPath(path).name == "AGENTS.md"
        or any(path == p or path.startswith(p + "/") for p in POLICY_PREFIXES))


def _no_symlink(repo: Path, path: str) -> None:
    current = repo
    for part in PurePosixPath(path).parts:
        current /= part
        try:
            mode = current.lstat().st_mode
        except FileNotFoundError:
            continue  # Deletions still undergo tree/index mode checks.
        if stat.S_ISLNK(mode):
            fail("SYMLINK_PATH")
        if not (stat.S_ISREG(mode) or stat.S_ISDIR(mode)):
            fail("SPECIAL_FILE_PATH")


def _modes(repo: Path, revision: str | None) -> dict[str, str]:
    data = _git(repo, "ls-files", "--stage", "-z") if revision is None else _git(
        repo, "ls-tree", "-r", "-z", revision)
    modes = {}
    for row in data.split(b"\0"):
        if not row:
            continue
        try:
            meta, raw = row.split(b"\t", 1)
            fields = meta.decode("ascii").split()
            path = raw.decode("utf-8", errors="strict")
            if revision is None and fields[2] != "0":
                fail("UNMERGED_INDEX")
            modes[path] = fields[0]
        except PreflightError:
            raise
        except (ValueError, UnicodeError, IndexError):
            raise PreflightError("INVALID_GIT_INDEX") from None
    return modes


def _verify_native_installation(repo: Path, main: str) -> None:
    # This comparison detects stale installation data; it does not turn a
    # caller-controlled installation into a trusted security principal.
    for path in NATIVE_INPUTS:
        installed = native.REPO_ROOT / path
        _no_symlink(native.REPO_ROOT, path)
        if installed.read_bytes() != _git(repo, "show", main + ":" + path):
            fail("STALE_NATIVE_POLICY")


def _verify_policy_files(repo: Path, main: str) -> None:
    # Direct byte/mode verification defeats assume-unchanged, skip-worktree and
    # stat-cache hiding of policy edits. The canonical root AGENTS symlink is
    # allowed only with its exact baseline target; parent symlinks never are.
    for row in _git(repo, "ls-tree", "-r", "-z", main).split(b"\0"):
        if not row:
            continue
        metadata, raw_path = row.split(b"\t", 1)
        try:
            path = raw_path.decode("utf-8", errors="strict")
        except UnicodeError:
            raise PreflightError("INVALID_PATH_ENCODING") from None
        if not _protected(path):
            continue
        mode, _, expected = metadata.decode("ascii").split()
        target = repo / path
        _no_symlink(repo, str(PurePosixPath(path).parent))
        try:
            actual_mode = target.lstat().st_mode
            if mode == "120000" and path == "AGENTS.md" and stat.S_ISLNK(actual_mode):
                data = os.readlink(target).encode()
            elif mode in {"100644", "100755"} and stat.S_ISREG(actual_mode):
                if bool(actual_mode & 0o111) != (mode == "100755"):
                    fail("GOVERNANCE_CHANGE")
                data = target.read_bytes()
            else:
                fail("GOVERNANCE_CHANGE")
        except OSError:
            raise PreflightError("GOVERNANCE_CHANGE") from None
        actual = hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
        if actual != expected:
            fail("GOVERNANCE_CHANGE")


def _visible_index(repo: Path) -> None:
    for entry in _git(repo, "ls-files", "-v", "-z").split(b"\0"):
        if entry and (entry[:1].islower() or entry[:1] == b"S"):
            fail("HIDDEN_INDEX_FLAGS")


def inspect_task(worktree: Path, manifest: Mapping[str, Any]) -> dict[str, Any]:
    """Return structural observations only. No auth, state writes or CI acceptance."""
    if not isinstance(manifest, dict):
        fail("INVALID_MANIFEST")
    # Snapshot caller data so later mutation cannot expand the inspected scope.
    try:
        manifest = json.loads(json.dumps(manifest))
    except (TypeError, ValueError):
        raise PreflightError("INVALID_MANIFEST") from None
    try:
        root = worktree.resolve(strict=True)
    except OSError:
        raise PreflightError("INVALID_WORKTREE") from None
    if not worktree.is_absolute() or str(worktree) != str(root):
        fail("WORKTREE_ALIAS")
    if manifest.get("worktree") != str(root) or _text(root, "rev-parse", "--show-toplevel") != str(root):
        fail("WRONG_WORKTREE")
    if manifest.get("repository") != REPOSITORY:
        fail("WRONG_REPOSITORY")
    origins = _text(root, "remote", "get-url", "--all", "origin").splitlines()
    pushes = _text(root, "remote", "get-url", "--push", "--all", "origin").splitlines()
    if len(origins) != 1 or len(pushes) != 1 or origins[0] not in REMOTE_URLS or pushes[0] not in REMOTE_URLS:
        fail("WRONG_ORIGIN")
    branch = _branch(manifest.get("branch"))
    main, head = _sha(manifest.get("authoritativeMain")), _sha(manifest.get("exactHead"))
    observed = remote_heads(branch)
    if observed["main"] != main:
        fail("STALE_MAIN")
    if _text(root, "rev-parse", "HEAD") != head:
        fail("STALE_HEAD")
    _git(root, "merge-base", "--is-ancestor", main, head)
    _verify_native_installation(root, main)
    try:
        native.validate_task_manifest(manifest, observed_main=main, observed_head=head)
    except native.ControlPlaneError:
        raise PreflightError("INVALID_NATIVE_MANIFEST") from None
    status = manifest["status"]
    if status in CERTIFY_STATES:
        fail("TRUSTED_GATE_EVIDENCE_REQUIRED")
    if status in WRITER_STATES:
        if _text(root, "symbolic-ref", "--quiet", "--short", "HEAD") != branch:
            fail("WRONG_WRITER_BRANCH")
        if observed["branch"] is not None:
            # Local commits ahead of an existing remote branch are legitimate.
            # Divergence, an unknown remote object, or a remote ahead blocks.
            _git(root, "merge-base", "--is-ancestor", _sha(observed["branch"]), head)
        lane = "WRITER_IN_PROGRESS"
    elif status in REVIEW_STATES:
        if observed["branch"] != head:
            fail("UNPUBLISHED_REVIEW_HEAD")
        lane = "PUBLISHED_REVIEW_CANDIDATE"
    else:
        fail("UNSUPPORTED_PREFLIGHT_STATE")
    inventory = changes(root, main)
    paths = sorted(set().union(*map(set, inventory.values())))
    scopes = [_path(p.rstrip("/")) for p in manifest["scope"]]
    if not scopes:
        fail("EMPTY_SCOPE")
    if any(_protected(p) for p in paths):
        fail("GOVERNANCE_CHANGE")
    _verify_policy_files(root, main)
    _visible_index(root)
    try:
        native.assert_write_paths_in_scope(paths, scopes, status)
    except native.ControlPlaneError:
        raise PreflightError("OUTSIDE_SCOPE") from None
    modes = [_modes(root, main), _modes(root, head), _modes(root, None)]
    for path in paths:
        _no_symlink(root, path)
        if any(m.get(path) in {"120000", "160000"} for m in modes):
            fail("SYMLINK_OR_SUBMODULE_CHANGE")
    if status in REVIEW_STATES and any(inventory[k] for k in inventory if k != "committed"):
        fail("DIRTY_REVIEW_WORKTREE")
    # Detect observable drift during inspection. No claim of atomicity or lease.
    if (_text(root, "rev-parse", "HEAD") != head or changes(root, main) != inventory
            or remote_heads(branch) != observed):
        fail("SOURCE_MOVED_DURING_PREFLIGHT")
    return {"result": "INSPECTION_ONLY", "structuralStatus": "PASS", "taskId": manifest["taskId"],
        "lane": lane, "repository": REPOSITORY, "worktree": str(root), "branch": branch,
        "authoritativeMain": main, "exactHead": head, "remoteBranchHead": observed["branch"],
        "manifestSha256": hashlib.sha256(json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
        "changes": inventory, "authorization": "UNVERIFIED", "authorized": False,
        "independentReview": "UNVERIFIED", "ciEvidence": "UNVERIFIED",
        "mergeReady": False,
        "limitations": ["No authenticated actor or trusted task-capsule binding (Phases B/C).",
            "No PR/base, required-check, CI-run or review attestation (Phase B).",
            "No branch-protection enforcement or overlapping-lane lease (Phase C).",
            "Point-in-time inspection; no filesystem lock or atomic snapshot."]}


def _unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            fail("DUPLICATE_JSON_KEY")
        result[key] = value
    return result


def read_manifest(path: Path) -> dict[str, Any]:
    if not stat.S_ISREG(path.lstat().st_mode):
        fail("MANIFEST_NOT_REGULAR_FILE")
    with path.open("rb") as stream:
        raw = stream.read(MAX_JSON_BYTES + 1)
    if len(raw) > MAX_JSON_BYTES:
        fail("MANIFEST_TOO_LARGE")
    try:
        value = json.loads(raw, object_pairs_hook=_unique_object)
    except (ValueError, UnicodeError, RecursionError):
        raise PreflightError("INVALID_MANIFEST_JSON") from None
    if not isinstance(value, dict):
        fail("INVALID_MANIFEST")
    return value


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worktree", type=Path, required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        result = native.inspect_task_preflight(args.worktree, read_manifest(args.manifest))
    except (PreflightError, OSError, ValueError, RecursionError) as error:
        code = error.code if isinstance(error, PreflightError) else "INSPECTION_FAILED"
        print(json.dumps({"result": "BLOCKED", "code": code, "authorized": False,
            "mergeReady": False}), file=sys.stderr)
        return 2
    print(json.dumps(result, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
