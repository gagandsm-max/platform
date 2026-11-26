"""Issue #343 Test20 fixture ownership contract (fake Docker only; no daemon/stack)."""

from __future__ import annotations

import hashlib
import json
import os
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "run-all-tests.sh"
LABEL_KEY = "io.amrudai.test20.isolation-probe.run"
FOREIGN_NAME = "unrelated-foreign-fixture"
FOREIGN_ID = "f" * 64
FOREIGN_VOLUME = "unrelated-foreign-volume"


def mock_docker(args: list[str]) -> int:
    state_file = Path(os.environ["FIXTURE_STATE"])
    state = json.loads(state_file.read_text(encoding="utf-8"))
    mode = os.environ["FIXTURE_MODE"]
    containers = state["containers"]
    volumes = state["volumes"]
    events = state["events"]

    def save() -> None:
        state_file.write_text(json.dumps(state), encoding="utf-8")

    def record(event: list[object]) -> None:
        events.append(event)
        save()

    def option(flag: str, default: str = "") -> str:
        return args[args.index(flag) + 1] if flag in args else default

    def find(ref: str):
        for name, item in containers.items():
            if ref in (name, item["id"], item["id"][:12]):
                return name, item
        return "", None

    if args[:1] == ["compose"]:
        record(["compose", args[1:]])
        return 0

    if args[:1] == ["exec"]:
        record(["exec", args[1:]])
        return 0

    if args[:1] == ["logs"]:
        return 0

    if args[:1] == ["volume"]:
        action = args[1]
        if action == "ls":
            for name in sorted(volumes):
                print(name)
            return 0
        if action == "inspect":
            name = args[-1]
            item = volumes.get(name)
            if not item:
                print(f"Error: No such volume: {name}", file=sys.stderr)
                return 1
            fmt = option("-f") or option("--format")
            if "Labels" in fmt:
                print(
                    f'{item["labels"].get("com.docker.compose.project", "")}|'
                    f'{item["labels"].get("issue223.owner", "")}|'
                    f'{item["labels"].get(LABEL_KEY, "")}'
                )
            return 0
        raise AssertionError(f"unexpected volume call: {args}")

    if args[:1] == ["ps"]:
        if "--filter" in args:
            filt = option("--filter")
            if filt.startswith("id="):
                needle = filt[3:]
                for item in containers.values():
                    if item["id"] == needle or item["id"].startswith(needle):
                        print(item["id"])
            return 0
        fmt = option("--format")
        if fmt == "{{.Names}}":
            for name in sorted(containers):
                print(name)
        return 0

    if args[:1] == ["inspect"]:
        fmt = option("-f") or option("--format")
        ref = args[-1]
        typed = "--type" in args and option("--type") == "container"
        if ref == "platform-postgres" and "Config.Image" in (fmt or ""):
            print("postgres:16-alpine")
            record(["inspect_image", ref])
            return 0
        name, item = find(ref)
        if mode == "collision_before_acquire" and (
            ref.startswith("platform-isolation-probe-")
            or (name and name.startswith("platform-isolation-probe-"))
        ):
            if not item:
                containers[ref] = {
                    "id": "a" * 64,
                    "label": "",
                    "running": True,
                    "foreign": True,
                }
                name, item = ref, containers[ref]
                record(["foreign_collision", ref])
            if typed and not fmt:
                return 0
        if not item:
            print(f"Error: No such container: {ref}", file=sys.stderr)
            return 1
        record(["inspect", name or ref])
        if typed and not fmt:
            return 0
        if fmt == "{{.Config.Image}}":
            print("postgres:16-alpine")
            return 0
        if fmt == "{{.Id}}":
            print(item["id"])
            return 0
        if fmt == "{{.State.Running}}":
            print("true" if item.get("running") else "false")
            return 0
        if fmt and "Config.Labels" in fmt and LABEL_KEY in fmt:
            # First ownership inspect is acquire verification; faults apply after that.
            state["owner_inspects"] = int(state.get("owner_inspects", 0)) + 1
            save()
            if (
                mode in ("failed_inspection", "replacement", "post_inspect_name_replacement")
                and state["owner_inspects"] > 1
                and not item.get("foreign")
            ):
                if mode == "failed_inspection":
                    record(["inspect_fail", name or ref])
                    print("fixture inspect failed", file=sys.stderr)
                    return 43
                if mode == "replacement":
                    item["id"] = "b" * 64
                    item["label"] = ""
                    item["foreign"] = True
                    record(["replaced", name or ref])
                elif mode == "post_inspect_name_replacement":
                    # Ownership of the immutable ID remains valid, but the probe
                    # name is reused by an unrelated container before rm.
                    owned_id = item["id"]
                    owned_label = item["label"]
                    owned_name = name
                    del containers[owned_name]
                    containers["_owned_renamed"] = {
                        "id": owned_id,
                        "label": owned_label,
                        "running": True,
                        "foreign": False,
                    }
                    containers[owned_name] = {
                        "id": "e" * 64,
                        "label": "",
                        "running": True,
                        "foreign": True,
                    }
                    record(["name_reused", owned_name, owned_id])
                    save()
                    print(f"{owned_id} {owned_label}")
                    return 0
            label = item.get("label") or "<no value>"
            print(f'{item["id"]} {label}')
            return 0
        print(item["id"])
        return 0

    if args[:1] == ["run"]:
        name = option("--name")
        label = option("--label")
        run_id = label.partition("=")[2] if label.startswith(LABEL_KEY + "=") else ""
        identity = hashlib.sha256(name.encode()).hexdigest()
        if name in containers:
            print("fixture name collision", file=sys.stderr)
            return 125
        stored_label = run_id
        if mode == "wrong_acquire_label":
            stored_label = "0" * 32
        elif mode == "missing_acquire_label":
            stored_label = ""
        containers[name] = {
            "id": identity,
            "label": stored_label,
            "running": True,
            "foreign": False,
        }
        state["acquired"] = True
        record(["create", name, identity, label])
        print(identity)
        return 0

    if args[:1] == ["rm"]:
        refs = [a for a in args[1:] if not a.startswith("-")]
        for ref in refs:
            name, item = find(ref)
            if not item:
                continue
            record(["remove", name, ref, item.get("label", ""), bool(item.get("foreign"))])
            if mode == "failed_removal" and not item.get("foreign"):
                print("fixture remove failed", file=sys.stderr)
                return 42
            if item.get("foreign"):
                # Ownership gates must refuse before deleting foreign fixtures.
                raise AssertionError(f"foreign container removed: {name}")
            del containers[name]
            record(["removed", name])
        save()
        return 0

    raise AssertionError(f"unexpected Docker call: {args}")


class Test20FixtureCleanupContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.raw = SCRIPT.read_text(encoding="utf-8")
        cls.block = cls.raw.split(
            "# Test 20: Persistence, Structured JSON Logs, and Isolation Verification\n",
            1,
        )[1]

    def test_source_has_no_pre_acquisition_deletion(self) -> None:
        # Defect baseline: trap + immediate cleanup_foreign_probe before docker run.
        self.assertNotRegex(
            self.block,
            r"trap cleanup_foreign_probe EXIT\s*\ncleanup_foreign_probe\b",
        )
        acquire_fn = self.block.index("acquire_foreign_probe()")
        run_at = self.block.index('docker run -d --name "${FOREIGN_PROBE_NAME}"', acquire_fn)
        # Inside acquire_foreign_probe, collision must be detected before any rm.
        acquire_body = self.block[acquire_fn:run_at]
        self.assertIn("collision_before_acquire", acquire_body)
        self.assertNotIn("docker rm", acquire_body)
        self.assertIn(LABEL_KEY, self.block)
        # Removal must target verified immutable ID, never the reusable name.
        self.assertIn('docker rm -fv "${verified_id}"', self.block)
        self.assertNotIn('docker rm -fv "${FOREIGN_PROBE_NAME}"', self.block)
        self.assertIn("Never delete by name before acquire", self.block)
        # Failed/ambiguous ownership must not rm create stdout/short IDs.
        self.assertIn("retaining ambiguous resource", self.block)
        self.assertIn("retaining unproven create identity", self.block)

    def test_run_scoped_identity_tokens_remain(self) -> None:
        self.assertIn("platform-isolation-probe-", self.block)
        self.assertIn("FOREIGN_PROBE_STARTED", self.block)
        self.assertIn("FOREIGN_PROBE_ID", self.block)
        self.assertIn("PLATFORM_TEST20_FIXTURE_CONTRACT", self.block)

    def _initial_state(self) -> dict:
        return {
            "containers": {
                FOREIGN_NAME: {
                    "id": FOREIGN_ID,
                    "label": "",
                    "running": True,
                    "foreign": True,
                },
                "platform-postgres": {
                    "id": "c" * 64,
                    "label": "",
                    "running": True,
                    "foreign": False,
                },
            },
            "volumes": {
                FOREIGN_VOLUME: {
                    "labels": {
                        "com.docker.compose.project": "other",
                        "issue223.owner": "other",
                    }
                }
            },
            "events": [],
            "acquired": False,
            "owner_inspects": 0,
        }

    def run_fixture(self, mode: str = "clean"):
        with tempfile.TemporaryDirectory() as directory:
            fixture = Path(directory)
            state_path = fixture / "state.json"
            state_path.write_text(json.dumps(self._initial_state()), encoding="utf-8")
            bash = shutil.which("bash")
            assert bash
            for utility in ("cat", "od", "tr", "printf", "true", "false"):
                src = shutil.which(utility)
                if src:
                    (fixture / utility).symlink_to(src)
            docker = fixture / "docker"
            docker.write_text(
                f"#!{bash}\n"
                f"exec {shlex.quote(sys.executable)} -I "
                f"{shlex.quote(str(Path(__file__).resolve()))} --mock-docker \"$@\"\n",
                encoding="utf-8",
            )
            docker.chmod(0o700)
            env = {
                "PATH": f"{directory}:/usr/bin:/bin",
                "LC_ALL": "C",
                "FIXTURE_STATE": str(state_path),
                "FIXTURE_MODE": mode,
                "PLATFORM_TEST20_FIXTURE_CONTRACT": "1",
                "DOCKER_HOST": f"unix://{fixture}/no-daemon.sock",
            }
            # Extract only Test20 section with a tiny harness prefix.
            body = (
                "PASS_COUNT=0; FAIL_COUNT=0; TOTAL_TESTS=20\n"
                "ROOT_DIR=/fixture\n"
                "pass() { :; }\n"
                "fail() { return 1; }\n"
                + "# Test 20: Persistence, Structured JSON Logs, and Isolation Verification\n"
                + self.block
            )
            script_path = fixture / "test20-fixture.sh"
            script_path.write_text(body, encoding="utf-8")
            result = subprocess.run(
                [bash, str(script_path)],
                env=env,
                capture_output=True,
                text=True,
                check=False,
                timeout=20,
            )
            state = json.loads(state_path.read_text(encoding="utf-8"))
            return result, state

    def test_clean_success_records_id_and_removes_only_owned(self) -> None:
        result, state = self.run_fixture("clean")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("PASS owned acquire/cleanup", result.stdout)
        self.assertIn(FOREIGN_NAME, state["containers"])
        self.assertEqual(state["containers"][FOREIGN_NAME]["id"], FOREIGN_ID)
        self.assertIn(FOREIGN_VOLUME, state["volumes"])
        creates = [e for e in state["events"] if e[0] == "create"]
        removes = [e for e in state["events"] if e[0] == "remove"]
        self.assertEqual(len(creates), 1, state)
        self.assertEqual(len(removes), 1, state)
        self.assertRegex(creates[0][2], r"^[0-9a-f]{64}$")
        self.assertTrue(creates[0][3].startswith(LABEL_KEY + "="))
        # Cleanup must rm by verified full ID, not the probe name.
        self.assertEqual(removes[0][2], creates[0][2], state)
        self.assertRegex(removes[0][2], r"^[0-9a-f]{64}$")
        self.assertFalse(any(e[0] == "remove" and e[4] for e in state["events"]))
        # No removal may precede create (pre-acquisition deletion regression).
        first_create = next(i for i, e in enumerate(state["events"]) if e[0] == "create")
        self.assertFalse(
            any(e[0] == "remove" for e in state["events"][:first_create]),
            state["events"],
        )

    def test_collision_before_acquire_preserves_foreign(self) -> None:
        result, state = self.run_fixture("collision_before_acquire")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("collision", (result.stdout + result.stderr).lower())
        self.assertFalse(any(e[0] == "create" for e in state["events"]), state)
        self.assertFalse(any(e[0] == "remove" for e in state["events"]), state)
        self.assertIn(FOREIGN_NAME, state["containers"])
        # Collision fixture itself must remain (not pre-deleted).
        collision = [
            name
            for name, item in state["containers"].items()
            if name.startswith("platform-isolation-probe-")
        ]
        self.assertTrue(collision, state)
        self.assertTrue(all(state["containers"][n].get("foreign") for n in collision))

    def test_failed_removal_propagates_nonzero(self) -> None:
        result, state = self.run_fixture("failed_removal")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(any(e[0] == "create" for e in state["events"]), state)
        self.assertTrue(any(e[0] == "remove" for e in state["events"]), state)
        self.assertFalse(any(e[0] == "removed" for e in state["events"]), state)
        owned = [
            name
            for name, item in state["containers"].items()
            if name.startswith("platform-isolation-probe-")
        ]
        self.assertTrue(owned, state)
        self.assertIn(FOREIGN_NAME, state["containers"])

    def test_failed_inspection_refuses_removal(self) -> None:
        result, state = self.run_fixture("failed_inspection")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("cleanup refused", result.stdout + result.stderr)
        self.assertFalse(any(e[0] == "remove" for e in state["events"]), state)
        self.assertIn(FOREIGN_NAME, state["containers"])

    def test_replacement_refuses_foreign_deletion(self) -> None:
        result, state = self.run_fixture("replacement")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(any(e[0] == "replaced" for e in state["events"]), state)
        self.assertFalse(any(e[0] == "remove" for e in state["events"]), state)
        self.assertIn(FOREIGN_NAME, state["containers"])
        self.assertEqual(state["containers"][FOREIGN_NAME]["id"], FOREIGN_ID)

    def test_post_inspect_name_replacement_removes_by_verified_id(self) -> None:
        result, state = self.run_fixture("post_inspect_name_replacement")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(any(e[0] == "name_reused" for e in state["events"]), state)
        creates = [e for e in state["events"] if e[0] == "create"]
        removes = [e for e in state["events"] if e[0] == "remove"]
        self.assertEqual(len(creates), 1, state)
        self.assertEqual(len(removes), 1, state)
        # Must remove the owned immutable ID, never the reused name.
        self.assertEqual(removes[0][2], creates[0][2], state)
        self.assertRegex(removes[0][2], r"^[0-9a-f]{64}$")
        self.assertFalse(removes[0][4], state)
        # Foreign container that reused the probe name must survive.
        probe_names = [
            n for n in state["containers"] if n.startswith("platform-isolation-probe-")
        ]
        self.assertTrue(probe_names, state)
        for name in probe_names:
            self.assertTrue(state["containers"][name].get("foreign"), state)
        self.assertIn(FOREIGN_NAME, state["containers"])
        self.assertEqual(state["containers"][FOREIGN_NAME]["id"], FOREIGN_ID)

    def test_wrong_acquire_label_retains_without_removal(self) -> None:
        result, state = self.run_fixture("wrong_acquire_label")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("acquire_label_mismatch", result.stdout + result.stderr)
        self.assertTrue(any(e[0] == "create" for e in state["events"]), state)
        self.assertFalse(any(e[0] == "remove" for e in state["events"]), state)
        retained = [
            n
            for n, item in state["containers"].items()
            if n.startswith("platform-isolation-probe-")
        ]
        self.assertTrue(retained, state)
        self.assertIn(FOREIGN_NAME, state["containers"])

    def test_missing_acquire_label_retains_without_removal(self) -> None:
        result, state = self.run_fixture("missing_acquire_label")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("acquire_label_mismatch", result.stdout + result.stderr)
        self.assertTrue(any(e[0] == "create" for e in state["events"]), state)
        self.assertFalse(any(e[0] == "remove" for e in state["events"]), state)
        retained = [
            n
            for n, item in state["containers"].items()
            if n.startswith("platform-isolation-probe-")
        ]
        self.assertTrue(retained, state)
        self.assertIn(FOREIGN_NAME, state["containers"])


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--mock-docker":
        raise SystemExit(mock_docker(sys.argv[2:]))
    unittest.main()
