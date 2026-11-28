"""Run the Kafka trap and synthetic acquisition against a Docker CLI double."""

import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "run-kafka-storage-contract-test.sh"
PROJECT = "issue223-storage-fixture"
FOREIGN = f"{PROJECT}-foreign"
UNRELATED = f"{FOREIGN}-keep"
PRIMARY = f"ci_{PROJECT}_1_kafka_data"
DIRECT_HELPERS = ("uuid", "config-compare", "interrupted-chown")
OWNED_ID = "a" * 64
FOREIGN_ID = "b" * 64


def fake_docker(args):
    state_path = Path(os.environ["FIXTURE_STATE"])
    state = json.loads(state_path.read_text())
    volumes = state["volumes"]
    mode = os.environ["FIXTURE_MODE"]

    def save():
        state_path.write_text(json.dumps(state))

    containers = state["containers"]
    if args[0] == "run":
        helper = os.environ["FIXTURE_HELPER"]
        name = f"{PROJECT}-{helper}"
        assert args[:11] == [
            "run", "--rm", "--cidfile", str(state_path.parent / f"{helper}.cid"),
            "--name", name, "--label", f"issue223.owner={PROJECT}",
            "--label", f"com.docker.compose.project={PROJECT}", "--network",
        ]
        assert (state_path.parent / f"{helper}.pending").is_file()
        if mode == "name_collision":
            containers[FOREIGN_ID] = [name, "foreign", "foreign"]
            save()
            return 39
        if mode == "unknown_acquisition":
            containers[OWNED_ID] = [name, PROJECT, PROJECT]
            save()
            return 39
        if mode == "unknown_no_inventory":
            Path(args[3]).touch()
            return 39
        Path(args[3]).write_text(OWNED_ID)
        if mode not in ("natural", "lost_after_removal"):
            containers[OWNED_ID] = [name, PROJECT, PROJECT]
        if mode == "foreign_replacement":
            containers.pop(OWNED_ID)
            containers[FOREIGN_ID] = [name, PROJECT, PROJECT]
        if mode == "wrong_owner":
            containers[OWNED_ID][1] = "foreign"
        if mode == "wrong_project":
            containers[OWNED_ID][2] = "foreign"
        save()
        if mode == "cancelled":
            os.kill(int(os.environ["FIXTURE_SHELL"]), signal.SIGTERM)
        if mode in ("lost_response", "lost_after_removal"):
            return 39
        if helper == "uuid":
            print("A" * 22)
    elif args[:2] == ["ps", "--all"]:
        assert args == ["ps", "--all", "--no-trunc", "--format", "{{.ID}}|{{.Names}}"]
        if mode == "malformed_inventory":
            print(f"short-id|{PROJECT}")
        for cid, (name, _, _) in containers.items():
            print(f"{cid}|{name}")
            if mode == "ambiguous_inventory":
                print(f"{cid}|{name}")
        if mode == "inventory_error" or (mode == "final_inventory_error" and state["container_removals"]):
            return 42
    elif args[0] == "inspect":
        assert args[1:4] == ["--type", "container", "--format"]
        assert args[-1] == OWNED_ID
        name, owner, project = containers[args[-1]]
        cid = FOREIGN_ID if mode == "inspect_changed_id" else args[-1]
        print(f"{cid}|/{name}|{owner}|{project}")
        if mode == "inspect_error":
            return 43
    elif args[0] == "rm":
        assert args == ["rm", "--force", OWNED_ID]
        state["container_removals"].append(args[-1])
        if mode != "residual":
            del containers[args[-1]]
        if mode == "late_replacement":
            containers[FOREIGN_ID] = [f"{PROJECT}-{os.environ['FIXTURE_HELPER']}", PROJECT, PROJECT]
        save()
    elif args[:2] == ["volume", "ls"]:
        if mode == "initial_inventory_error" and FOREIGN in volumes:
            return 42
        if mode == "final_inventory_error" and FOREIGN in state["removals"]:
            return 42
        print("\n".join(volumes))
    elif args[:2] == ["volume", "create"]:
        stem = FOREIGN if mode != "collision_after_check" else "foreign-owner"
        volumes[args[-1]] = f"{stem}|{stem}|{stem}|platform_kafka_data"
        state["creates"].append(args[-1])
        save()
        if mode == "interrupt":
            os.kill(os.getppid(), signal.SIGTERM)
        if mode == "lost_response":
            return 39
    elif args[:2] == ["volume", "inspect"]:
        labels = volumes.get(args[-1])
        if labels is None:
            return 1
        print(labels)
        if mode == "failed_inspect" and args[-1] == FOREIGN and "-f" in args:
            return 43
    elif args[:2] == ["volume", "rm"]:
        state["removals"].append(args[-1])
        if mode != "silent_residue" or args[-1] != FOREIGN:
            del volumes[args[-1]]
        save()
    elif args[:2] in (["ps", "-aq"], ["network", "ls"], ["compose", "down"]):
        state["compose_cleanup_calls"].append(args)
        save()
    else:
        raise AssertionError(args)
    return 0


class KafkaStorageCleanupContract(unittest.TestCase):
    def run_fixture(self, mode, helper=None):
        source = SCRIPT.read_text()
        # Execute the real traps and the selected acquisition from the owner script.
        helpers = source[source.index("DIRECT_HELPERS=("):source.index("export PLATFORM_KAFKA_CLUSTER_ID")]
        if helper == "uuid":
            acquisition = source[source.index("export PLATFORM_KAFKA_CLUSTER_ID"):
                                 source.index("export PLATFORM_KAFKA_STORAGE_ID")]
        elif helper == "config-compare":
            acquisition = source[source.index("direct_helper config-compare "):
                                 source.index("for profile in dev test ha")]
        elif helper == "interrupted-chown":
            acquisition = source[source.index("direct_helper interrupted-chown "):
                                 source.index("printf '#!/usr/bin/env bash")]
        else:
            acquisition = source[source.index("# Independent foreign-provenance fixture:"):
                                 source.index("# Source assertion plus direct entrypoint")]
        with tempfile.TemporaryDirectory(prefix="kafka-cleanup-fake-") as tmp:
            root = Path(tmp)
            state = root / "state"
            resources = {UNRELATED: "foreign bytes", PRIMARY:
                         f"{PROJECT}|{PROJECT}|{PROJECT}-identity|platform_kafka_data"}
            if mode == "collision_before_check":
                resources[FOREIGN] = "foreign bytes"
            state.write_text(json.dumps({
                "volumes": resources, "removals": [], "creates": [],
                "containers": {}, "container_removals": [], "compose_cleanup_calls": [],
            }))
            for utility in ("timeout", "grep"):
                (root / utility).symlink_to(shutil.which(utility))
            docker = root / "docker"
            docker.write_text(f"#!{sys.executable}\nimport runpy, sys\n"
                              f"sys.exit(runpy.run_path({str(Path(__file__).resolve())!r})"
                              f"['fake_docker'](sys.argv[1:]))\n")
            docker.chmod(0o700)
            script = (
                "set -euo pipefail\n"
                f"PROJECT={PROJECT}\nVOLUME={PRIMARY}\n"
                f"REPLACEMENT={PROJECT}-replacement\nINTERRUPTED={PROJECT}-interrupted\n"
                f"FOREIGN={FOREIGN}\nFOREIGN_ACQUISITION_STARTED=0\n"
                f"TASK={shlex.quote(tmp)}\nexport FIXTURE_SHELL=$$\n"
                + ('COMPOSE_ACQUISITION_STARTED=0\n' if helper in ("uuid", "config-compare") else
                   f"COMPOSE_ACQUISITION_STARTED=1\nPLATFORM_KAFKA_STORAGE_ID={PROJECT}-identity\n"
                   "DC=(docker compose)\n")
                + helpers + acquisition
            )
            result = subprocess.run(
                ["/bin/bash", "-c", script], text=True, capture_output=True, timeout=10,
                env={"PATH": tmp, "DOCKER_HOST": f"unix://{tmp}/no-daemon.sock",
                     "FIXTURE_STATE": str(state), "FIXTURE_MODE": mode,
                     "FIXTURE_HELPER": helper or ""},
            )
            recorded = json.loads(state.read_text())
            self.assertEqual(recorded["volumes"][UNRELATED], "foreign bytes")
            self.assertNotIn(UNRELATED, recorded["removals"])
            return result, recorded

    def test_normal_exit_and_interrupted_acquisition(self):
        for mode, status in (("normal", 0), ("interrupt", 143), ("lost_response", 39)):
            with self.subTest(mode=mode):
                result, state = self.run_fixture(mode)
                self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                self.assertEqual(state["creates"], [FOREIGN])
                self.assertEqual(set(state["volumes"]), {UNRELATED})
                self.assertIn(FOREIGN, state["removals"])
                self.assertEqual("PASS guarded Kafka storage contract" in result.stdout, status == 0)

    def test_cleanup_failure_and_missing_absence_proof_fail_closed(self):
        for mode in ("initial_inventory_error", "final_inventory_error",
                     "failed_inspect", "silent_residue"):
            with self.subTest(mode=mode):
                result, state = self.run_fixture(mode)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("cleanup or absence proof failed", result.stderr)
                self.assertNotIn("PASS guarded Kafka storage contract", result.stdout)
                if mode in ("initial_inventory_error", "failed_inspect"):
                    self.assertNotIn(FOREIGN, state["removals"])
                if mode == "silent_residue":
                    self.assertIn(FOREIGN, state["volumes"])

    def test_exact_foreign_collision_is_preserved(self):
        for mode, labels in (("collision_before_check", "foreign bytes"),
                             ("collision_after_check",
                              "foreign-owner|foreign-owner|foreign-owner|platform_kafka_data")):
            with self.subTest(mode=mode):
                result, state = self.run_fixture(mode)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(state["volumes"][FOREIGN], labels)
                self.assertNotIn(FOREIGN, state["removals"])
                if mode == "collision_before_check":
                    self.assertEqual(state["creates"], [])

    def test_each_direct_helper_has_early_trap_and_proven_cleanup(self):
        for helper in DIRECT_HELPERS:
            for mode, status, removals in (
                ("natural", 0, []),
                ("cancelled", 143, [OWNED_ID]),
                ("lost_response", 39, [OWNED_ID]),
                ("lost_after_removal", 39, []),
            ):
                with self.subTest(helper=helper, mode=mode):
                    result, state = self.run_fixture(mode, helper)
                    self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                    self.assertEqual(state["containers"], {})
                    self.assertEqual(state["container_removals"], removals)
                    self.assertNotIn("cleanup or absence proof failed", result.stderr)
                    if helper != "interrupted-chown":
                        self.assertEqual(state["compose_cleanup_calls"], [])

    def test_each_direct_helper_fails_closed_on_unknown_identity_or_residue(self):
        for helper in DIRECT_HELPERS:
            for mode, status, surviving, removals in (
                ("name_collision", 39, FOREIGN_ID, []),
                ("unknown_acquisition", 39, OWNED_ID, []),
                ("unknown_no_inventory", 39, None, []),
                ("foreign_replacement", 1, FOREIGN_ID, []),
                ("wrong_owner", 1, OWNED_ID, []),
                ("wrong_project", 1, OWNED_ID, []),
                ("inspect_changed_id", 1, OWNED_ID, []),
                ("inspect_error", 1, OWNED_ID, []),
                ("inventory_error", 1, OWNED_ID, []),
                ("ambiguous_inventory", 1, OWNED_ID, []),
                ("malformed_inventory", 1, OWNED_ID, []),
                ("final_inventory_error", 1, None, [OWNED_ID]),
                ("residual", 1, OWNED_ID, [OWNED_ID]),
                ("late_replacement", 1, FOREIGN_ID, [OWNED_ID]),
            ):
                with self.subTest(helper=helper, mode=mode):
                    result, state = self.run_fixture(mode, helper)
                    self.assertEqual(result.returncode, status, result.stdout + result.stderr)
                    self.assertIn("cleanup or absence proof failed", result.stderr)
                    self.assertNotIn("PASS guarded Kafka storage contract", result.stdout)
                    self.assertEqual(list(state["containers"]), [surviving] if surviving else [])
                    self.assertEqual(state["container_removals"], removals)
                    self.assertEqual(state["compose_cleanup_calls"], [])
                    if mode in ("name_collision", "unknown_acquisition", "unknown_no_inventory"):
                        self.assertIn("Pending/unknown Docker acquisition", result.stderr)
