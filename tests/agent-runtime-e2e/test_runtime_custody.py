from contextlib import redirect_stdout
import hashlib
import io
import json
import os
from pathlib import Path
import signal
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

from isolation import Custody, HEALTH_FORMAT, KAFKA_VOLUME_FORMAT, OWNER, PROJECT, docker_gate, supervise
from test_isolation import PREFIX, TOKEN


FAKE = r"""
import json
from pathlib import Path
import sys
import time

state = Path(sys.argv[1])
data = json.loads(state.read_text())
args = sys.argv[2:]
with state.with_suffix(".trace").open("a") as log:
    log.write(json.dumps(args) + "\n")
kind, action = args[:2]
if data.get("hang") == [kind, action]:
    time.sleep(60)
if data.get("fail") == [kind, action]:
    sys.exit(44)
if kind == "logs":
    sys.exit(0)
objects = data.get(kind, {})
if action == "ls":
    for ref, item in objects.items():
        if "--filter" in args:
            query = args[args.index("--filter") + 1]
            if query.startswith("volume="):
                continue
            label, value = query.removeprefix("label=").split("=", 1)
            labels = item.get("Labels", item.get("Config", {}).get("Labels", {}))
            if labels.get(label) != value:
                continue
        if kind == "image":
            print(ref + "\t" + item["RepoTags"][0].replace(":", "\t"))
        else:
            print(ref if kind == "volume" else ref + "\t" + item["Name"].lstrip("/"))
elif action == "inspect":
    if args[-1] not in objects:
        sys.exit(1)
    print(json.dumps([objects[args[-1]]]))
elif action == "rm":
    del objects[args[-1]]
    state.write_text(json.dumps(data))
elif (kind, action) == ("network", "create"):
    labels = dict(args[i + 1].split("=", 1) for i, v in enumerate(args) if v == "--label")
    objects["a" * 64] = {"Id": "a" * 64, "Name": args[-1], "Driver": "bridge", "Internal": True, "Labels": labels}
    state.write_text(json.dumps(data))
elif kind == "create":
    labels = dict(args[i + 1].split("=", 1) for i, v in enumerate(args) if v == "--label")
    ref = "d" * 64
    data["container"][ref] = {"Id": ref, "Name": "/" + args[args.index("--name") + 1],
                              "Config": {"Labels": labels}, "State": {"Status": "created"}}
    state.write_text(json.dumps(data))
elif kind == "start" and action == "-a":
    if data.get("invalid_exit_proven"):
        data["container"][args[-1]]["State"] = {
            "Status": "exited", "ExitCode": 1, "Error": "", "FinishedAt": "2026-11-26T12:00:00Z",
        }
        state.write_text(json.dumps(data))
    sys.exit(1)
elif kind == "stop":
    data["container"][action]["State"] = {"Status": "exited", "Running": False}
    state.write_text(json.dumps(data))
    sys.exit(1)
elif kind == "exec":
    if data.get("exec_exit_proven"):
        print("\n" + args[args.index("-c") + 2] + ":1", file=sys.stderr)
    sys.exit(1)
else:
    sys.exit(35)
"""


class CustodyTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.directory = Path(tmp.name)
        self.state = self.directory / "docker.json"
        self.trace = self.state.with_suffix(".trace")
        fake = self.directory / "fake.py"
        fake.write_text(FAKE)
        self.custody = Custody([sys.executable, str(fake), str(self.state)], PREFIX, TOKEN, self.directory)
        self.anchor = "a" * 64
        self.container = "b" * 64
        self.labels = {OWNER: TOKEN, PROJECT: PREFIX}
        self.update()

    def update(self, **changes):
        self.state.write_text(json.dumps({"network": {}, **changes}))

    def recorded(self):
        return [json.loads(line) for line in self.trace.read_text().splitlines()]

    def anchor_resource(self):
        return {self.anchor: {"Id": self.anchor, "Name": self.custody.anchor,
                              "Driver": "bridge", "Internal": True, "Labels": self.labels}}

    def reserve(self):
        self.custody.admit()

    def test_failed_query_cannot_authorize_first_mutation(self):
        self.update(fail=["volume", "ls"])
        with self.assertRaises(subprocess.CalledProcessError):
            self.reserve()
        self.assertFalse((self.directory / "reservation-attempted").exists())
        self.assertFalse(any(a[1] == "create" for a in self.recorded()))

    def test_collision_on_unlabelled_exact_name_denies_admission(self):
        self.update(network={self.anchor: {"Name": self.custody.anchor, "Labels": {}}})
        with self.assertRaisesRegex(ValueError, "collision"):
            self.reserve()
        self.assertFalse(any(a[1] == "create" for a in self.recorded()))

    def test_partial_owned_cleanup_by_id_and_empty_inventory(self):
        self.reserve()
        self.update(network=self.anchor_resource(), container={self.container: {
            "Id": self.container, "Name": "/" + PREFIX + "-core",
            "Config": {"Labels": {**self.labels, "com.docker.compose.service": "core"}},
        }})
        self.assertEqual(self.run_work("raise SystemExit(7)"), 7)
        removed = [a for a in self.recorded() if a[1] == "rm"]
        self.assertEqual(removed, [["container", "rm", "--force", self.container], ["network", "rm", self.anchor]])
        self.assertEqual((self.directory / "remaining-networks.txt").read_text(), "")
        self.assert_exits(7, 0)

    def test_wrong_owner_even_with_project_and_name_is_never_removed(self):
        self.reserve()
        self.update(network=self.anchor_resource(), container={self.container: {
            "Id": self.container, "Name": "/" + PREFIX + "-core",
            "Config": {"Labels": {PROJECT: PREFIX, "com.docker.compose.service": "core"}},
        }})
        self.assertEqual(self.run_work("raise SystemExit(23)"), 23)
        self.assert_exits(23, 1)
        self.assertFalse(any(a[1] == "rm" for a in self.recorded()))
        self.assertFalse((self.directory / "remaining-containers.txt").exists())

    def test_missing_reservation_denies_mutation_even_with_empty_inventory(self):
        with self.assertRaisesRegex(ValueError, "reservation missing"):
            self.custody.active()

    def test_helper_mount_denied_before_docker_run(self):
        self.reserve()
        with self.assertRaisesRegex(ValueError, "helper mount"):
            docker_gate(self.custody, ["run", "--rm", "-v", "/var/run/docker.sock:/certs",
                                       "eclipse-temurin:21-jre-alpine"])
        self.assertFalse(any(a[0] == "run" for a in self.recorded()))

    def test_copy_to_host_path_outside_evidence_is_denied(self):
        with self.assertRaisesRegex(ValueError, "outside evidence"):
            docker_gate(self.custody, ["cp", PREFIX + "-core:/fixture", "/tmp/outside-evidence"])
        self.assertFalse(self.trace.exists())

    def test_owned_label_on_bare_name_does_not_authorize_cleanup(self):
        self.reserve()
        self.update(network=self.anchor_resource(), container={self.container: {
            "Id": self.container, "Name": "/core",
            "Config": {"Labels": {**self.labels, "com.docker.compose.service": "core"}},
        }})
        self.assertEqual(self.run_work("pass"), 1)
        self.assertFalse(any(a[1] == "rm" for a in self.recorded()))

    def test_replaced_reservation_denies_all_mutation_and_cleanup(self):
        self.reserve()
        replacement = "c" * 64
        self.update(network={replacement: {**self.anchor_resource()[self.anchor], "Id": replacement}})
        with self.assertRaisesRegex(ValueError, "replaced"):
            docker_gate(self.custody, ["run", "--rm", "node:20-alpine"])
        self.assertEqual(self.run_work("pass"), 1)
        self.assertFalse(any(a[0] == "run" or a[1] == "rm" for a in self.recorded()))

    def test_cleanup_query_failure_preserves_primary_result(self):
        self.reserve()
        self.update(network=self.anchor_resource(), fail=["volume", "ls"])
        self.assertEqual(self.run_work("raise SystemExit(19)"), 19)
        self.assert_exits(19, 1)
        self.assertFalse(any(a[1] == "rm" for a in self.recorded()))

    def test_cleanup_removal_failure_fails_pass_and_keeps_remaining_evidence(self):
        self.reserve()
        self.update(network=self.anchor_resource(), fail=["network", "rm"])
        self.assertEqual(self.run_work("pass"), 1)
        self.assert_exits(0, 1)
        self.assertEqual((self.directory / "remaining-networks.txt").read_text(), self.custody.anchor + "\n")

    def test_kafka_volume_requires_all_provenance_labels(self):
        self.reserve()
        volume = PREFIX + "-platform_kafka_data"
        labels = {OWNER: TOKEN, "issue223.owner": PREFIX, "platform.kafka.project": PREFIX,
                  "platform.kafka.storage-id": "foreign", "platform.kafka.resource": "platform_kafka_data"}
        kafka = {"Name": volume, "Driver": "local", "Labels": labels}
        self.update(network=self.anchor_resource(), volume={volume: kafka})
        self.assertEqual(self.run_work("pass"), 1)
        self.assertFalse(any(a[:2] == ["volume", "rm"] for a in self.recorded()))
        labels["platform.kafka.storage-id"] = PREFIX
        self.update(network=self.anchor_resource(), volume={volume: kafka})
        self.assertEqual(self.run_work("pass"), 0)
        self.assertIn(["volume", "rm", volume], self.recorded())
        self.assertEqual((self.directory / "remaining-volumes.txt").read_text(), "")

    def test_interrupted_mutation_cannot_claim_absence(self):
        self.reserve()
        legacy = self.directory / "pending-fake"
        legacy.write_text("compose -p\n")
        self.assertEqual(self.run_work("pass"), 1)
        self.assert_exits(0, 1)
        self.assertFalse(any(a[1] == "rm" for a in self.recorded()))
        self.assertEqual(legacy.read_text(), "compose -p\n")

    def test_compose_journal_records_only_admitted_identity_and_outcome(self):
        self.reserve()
        config = self.directory / "compose.json"
        config.write_text('{"sensitive":"fixture-secret"}')
        (self.directory / "compose.sha256").write_text(hashlib.sha256(config.read_bytes()).hexdigest())
        compose = ["compose", "-p", PREFIX, "-f", str(config)]
        kafka_run = ["run", "-T", "--rm", "--no-deps", "--user", "1000:1000",
                     "-e", f"PLATFORM_KAFKA_NEW_VOLUME_CONFIRM={PREFIX}", "--entrypoint", "/bin/bash",
                     "kafka", "/platform-kafka/provision-fresh.sh", "--new-empty-volume"]
        cases = (
            (["up", "-d", "postgres", "clickhouse", "kafka", "valkey", "core"],
             ["postgres", "clickhouse", "kafka", "valkey", "core"], 44, {"exit_code": 44}),
            (kafka_run, ["kafka"], -signal.SIGKILL, {}),
            (["run", "--rm", "--no-deps", "unit-contracts"], ["unit-contracts"], None, {}),
        )
        for tail, services, rc, exit_fields in cases:
            with self.subTest(verb=tail[0], rc=rc):
                existing = set(self.directory.glob("pending-*"))
                submitted = {"category": "compose", "verb": tail[0], "services": services,
                             "outcome": "submitted"}

                def pending():
                    receipt, = set(self.directory.glob("pending-*")) - existing
                    self.assertEqual(stat.S_IMODE(receipt.stat().st_mode), 0o600)
                    return receipt

                def client(_args):
                    self.assertEqual(json.loads(pending().read_text()), submitted)
                    if rc is None:
                        raise KeyboardInterrupt
                    return rc

                with patch("isolation.verify"), patch("isolation.subprocess.call", side_effect=client):
                    if rc is None:
                        with self.assertRaises(KeyboardInterrupt):
                            docker_gate(self.custody, compose + tail)
                    else:
                        self.assertEqual(docker_gate(self.custody, compose + tail), rc)
                outcome = "submitted" if rc is None else "returned-nonzero" if rc > 0 else "uncertain"
                self.assertEqual(json.loads(pending().read_text()), {**submitted, "outcome": outcome, **exit_fields})
                self.assertFalse(list(self.directory.glob(".journal-*")))
        receipts = {receipt: receipt.read_bytes() for receipt in self.directory.glob("pending-*")}
        self.assertEqual(self.run_work("pass"), 1)
        self.assertEqual({receipt: receipt.read_bytes() for receipt in self.directory.glob("pending-*")}, receipts)
        self.assertEqual((self.directory / "remaining-networks.txt").read_text(), self.custody.anchor + "\n")

    def test_shared_aggregate_deadlines_bound_hung_work_and_hung_inventory(self):
        self.reserve()
        self.update(network=self.anchor_resource(), hang=["container", "ls"])
        start = time.monotonic()
        self.assertEqual(self.run_work("import time; time.sleep(60)", work=0.25, cleanup=0.5), 124)
        self.assertLess(time.monotonic() - start, 2)
        self.assert_exits(124, 1)

    def test_term_and_int_stop_process_group_before_cleanup(self):
        for sig in (signal.SIGTERM, signal.SIGINT):
            with self.subTest(sig=sig):
                self.update(network=self.anchor_resource())
                (self.directory / "reservation-attempted").touch()
                (self.directory / "reservation-id").write_text(self.anchor + "\n")
                marker = self.directory / "child-survived"
                child = f"import time; time.sleep(1.5); open({str(marker)!r}, 'w').close()"
                code = (
                    "import os, signal, subprocess, sys, time; "
                    f"subprocess.Popen([sys.executable, '-c', {child!r}]); "
                    f"os.kill(os.getppid(), {int(sig)}); time.sleep(60)"
                )
                self.assertEqual(self.run_work(code, work=5), 128 + sig)
                self.assert_exits(128 + sig, 0)
                self.assertFalse(marker.exists())

    def seed_read_resources(self):
        self.reserve()
        self.update(network=self.anchor_resource(), container={
            self.container: {"Id": self.container, "Name": "/" + PREFIX + "-core",
                             "Config": {"Labels": {**self.labels, "com.docker.compose.service": "core"}},
                             "State": {"Running": True}},
            "c" * 64: {"Id": "c" * 64, "Name": "/foreign", "State": {"Running": True}},
        }, volume={"foreign-volume": {"Name": "foreign-volume"}})

    def test_read_gate_rejects_foreign_and_malformed_targets(self):
        self.seed_read_resources()
        for args in (
            ["inspect", "foreign"], ["logs", "foreign"], ["wait", "foreign"],
            ["volume", "inspect", "foreign-volume"],
            ["stats", "--no-stream", "--format", "{{json .}}", "foreign"],
            ["ps", "-aq", "--filter", "name=foreign"], ["volume", "ls", "foreign-volume"],
            ["inspect", PREFIX + "-core", "foreign"], ["logs", "--tail"], ["wait"],
            ["logs", PREFIX + "-core", "--follow"], ["inspect", "-f", "--size", PREFIX + "-core"],
            ["stats", "--no-stream", "--format", "{{json .}}"],
        ):
            with self.subTest(args=args), patch("isolation.os.execv") as execute:
                with self.assertRaises(ValueError):
                    docker_gate(self.custody, args)
                execute.assert_not_called()
        self.assertFalse(any(a[-1] in {"foreign", "foreign-volume", "c" * 64} for a in self.recorded()))

    def test_gate_uses_immutable_id_and_preserves_owned_health_poll_and_lists(self):
        self.seed_read_resources()
        for args, command in (
            (["inspect", "-f", HEALTH_FORMAT, PREFIX + "-core"],
             ["container", "inspect", "--format", HEALTH_FORMAT]),
            (["logs", PREFIX + "-core", "--tail", "80"], ["logs", "--tail", "80"]),
            (["wait", PREFIX + "-core"], ["wait"]),
            (["stats", "--no-stream", "--format", "{{json .}}", PREFIX + "-core"],
             ["stats", "--no-stream", "--format", "{{json .}}"]),
        ):
            with self.subTest(args=args), patch("isolation.os.execv") as execute:
                docker_gate(self.custody, args)
                execute.assert_called_once_with(sys.executable, self.custody.docker + command + [self.container])
        with patch("isolation.os.execv") as execute:
            self.assertEqual(docker_gate(self.custody, ["inspect", "--format", HEALTH_FORMAT,
                                                       PREFIX + "-sensor-infra"]), 1)
            execute.assert_not_called()
        for args, expected in (
            (["ps", "--format", "{{.Names}}"], PREFIX + "-core\n"),
            (["volume", "ls", "--format", "{{.Name}}"], ""),
        ):
            with redirect_stdout(io.StringIO()) as stdout:
                self.assertEqual(docker_gate(self.custody, args), 0)
            self.assertEqual(stdout.getvalue(), expected)

    def test_kafka_reads_preserve_exact_volume_and_preflight_list_filters(self):
        self.reserve()
        volume = PREFIX + "-platform_kafka_data"
        labels = {OWNER: TOKEN, "issue223.owner": PREFIX, "platform.kafka.project": PREFIX,
                  "platform.kafka.storage-id": PREFIX, "platform.kafka.resource": "platform_kafka_data"}
        self.update(network=self.anchor_resource(), volume={
            volume: {"Name": volume, "Driver": "local", "Labels": labels},
            "foreign-volume": {"Name": "foreign-volume"},
        }, container={
            self.container: {"Id": self.container, "Name": "/" + PREFIX + "-kafka",
                             "Config": {"Labels": {**self.labels, "com.docker.compose.service": "kafka"}},
                             "Mounts": [{"Type": "volume", "Name": volume}]},
        })
        with patch("isolation.os.execv") as execute:
            docker_gate(self.custody, ["volume", "inspect", volume, "-f", KAFKA_VOLUME_FORMAT])
            execute.assert_called_once_with(sys.executable, self.custody.docker + [
                "volume", "inspect", "--format", KAFKA_VOLUME_FORMAT, volume,
            ])
        for args, expected in (
            (["volume", "ls", "--format", "{{.Name}}"], volume),
            (["ps", "-aq", "--filter", f"label={PROJECT}={PREFIX}", "--filter",
              "label=com.docker.compose.service=kafka"], self.container),
            (["ps", "-aq", "--filter", f"name=^/{PREFIX}-kafka$"], self.container),
            (["ps", "-aq", "--filter", f"volume={volume}"], self.container),
        ):
            with self.subTest(args=args), redirect_stdout(io.StringIO()) as stdout:
                self.assertEqual(docker_gate(self.custody, args), 0)
                self.assertEqual(stdout.getvalue(), expected + "\n")

    def test_nonzero_mutation_retains_journal_and_reservation_even_after_owned_cleanup(self):
        self.seed_read_resources()
        self.assertEqual(docker_gate(self.custody, ["stop", PREFIX + "-core"]), 1)
        self.assertEqual(len(list(self.directory.glob("pending-*"))), 1)
        self.assertEqual(self.run_work("pass"), 1)
        self.assertEqual(len(list(self.directory.glob("pending-*"))), 1)
        self.assertEqual([a for a in self.recorded() if a[1] == "rm"],
                         [["container", "rm", "--force", self.container]])
        self.assertEqual((self.directory / "remaining-networks.txt").read_text(), self.custody.anchor + "\n")

    def test_invalid_config_requires_exit_and_removal_proof_to_clear_journal(self):
        tag = PREFIX + ":agent-runtime"
        image = "sha256:" + "e" * 64
        args = ["run", "--rm", "--name", PREFIX + "-invalid-config", "--label", f"{PROJECT}={PREFIX}",
                "--network", "none", "--read-only", "--cpus", "0.25", "--memory", "128m",
                "--pids-limit", "32", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                "--entrypoint", "/usr/local/bin/platform-agent-linux", tag]
        self.reserve()
        for proven in (True, False):
            with self.subTest(proven=proven):
                self.update(network=self.anchor_resource(), container={}, invalid_exit_proven=proven,
                            image={image: {"Id": image, "RepoTags": [tag], "Config": {"Labels": self.labels}}})
                if proven:
                    self.assertEqual(docker_gate(self.custody, args), 1)
                    self.assertFalse(list(self.directory.glob("pending-*")))
                    self.assertEqual(json.loads(self.state.read_text())["container"], {})
                    self.assertIn(["container", "rm", "d" * 64], self.recorded())
                    self.assertTrue(any(a[0] == "create" and a[-1] == image for a in self.recorded()))
                else:
                    with self.assertRaisesRegex(ValueError, "exit unconfirmed"):
                        docker_gate(self.custody, args)
                    self.assertEqual(len(list(self.directory.glob("pending-*"))), 1)
                    self.assertIn("d" * 64, json.loads(self.state.read_text())["container"])

    def test_expected_exec_nonzero_requires_in_container_exit_receipt(self):
        self.reserve()
        for proven in (True, False):
            with self.subTest(proven=proven):
                self.update(network=self.anchor_resource(), exec_exit_proven=proven, container={
                    self.container: {"Id": self.container, "Name": "/" + PREFIX + "-agent-runtime",
                                     "Config": {"Labels": {**self.labels,
                                                           "com.docker.compose.service": "agent-runtime"}}},
                })
                self.assertEqual(docker_gate(self.custody, ["exec", PREFIX + "-agent-runtime",
                                                           "wget", "-q", "-O", "-", "http://127.0.0.1:8089/ready"]), 1)
                self.assertEqual(len(list(self.directory.glob("pending-*"))), 0 if proven else 1)

    def run_work(self, code, work=2, cleanup=8):
        return supervise([sys.executable, "-c", code], self.directory, os.environ.copy(),
                         work, cleanup, self.custody)

    def assert_exits(self, workload, cleanup):
        self.assertEqual((self.directory / "exits.txt").read_text(),
                         f"workload_exit={workload}\ncleanup_exit={cleanup}\n")
