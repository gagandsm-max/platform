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


def fake_docker(args):
    state_path = Path(os.environ["FIXTURE_STATE"])
    state = json.loads(state_path.read_text())
    volumes = state["volumes"]
    mode = os.environ["FIXTURE_MODE"]

    def save():
        state_path.write_text(json.dumps(state))

    if args[:2] == ["volume", "ls"]:
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
        pass
    else:
        raise AssertionError(args)
    return 0


class KafkaStorageCleanupContract(unittest.TestCase):
    def run_fixture(self, mode):
        source = SCRIPT.read_text()
        # Execute the actual ownership helpers, traps and fourth-volume acquisition.
        helpers = source[source.index("new_volume() {"):source.index("run() {")]
        acquisition = source[source.index("# Independent foreign-provenance fixture:"):
                             source.index("# Source assertion plus direct entrypoint")]
        with tempfile.TemporaryDirectory(prefix="kafka-cleanup-fake-") as tmp:
            root = Path(tmp)
            state = root / "state"
            resources = {UNRELATED: "foreign bytes", PRIMARY:
                         f"{PROJECT}|{PROJECT}|{PROJECT}-identity|platform_kafka_data"}
            if mode == "collision_before_check":
                resources[FOREIGN] = "foreign bytes"
            state.write_text(json.dumps({"volumes": resources, "removals": [], "creates": []}))
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
                f"TASK={shlex.quote(tmp)}\nPLATFORM_KAFKA_STORAGE_ID={PROJECT}-identity\n"
                "DC=(docker compose)\n" + helpers + acquisition
            )
            result = subprocess.run(
                ["/bin/bash", "-c", script], text=True, capture_output=True, timeout=10,
                env={"PATH": tmp, "DOCKER_HOST": f"unix://{tmp}/no-daemon.sock",
                     "FIXTURE_STATE": str(state), "FIXTURE_MODE": mode},
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
