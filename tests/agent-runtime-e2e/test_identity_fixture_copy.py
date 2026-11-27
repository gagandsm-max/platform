"""Fake-only regression coverage for the frozen Agent identity fixture."""
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch

import identity_probe
import isolation

PATH = "/var/lib/platform/agent/queue/sequence.meta"
PREFIX = "agent-e2e-" + "1" * 32
REF = "a" * 64
META = {"highestSequence": 42, "agentStreamId": "native-test-stream"}


def archive(rows):
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w") as t:
        for name, body, kind in rows:
            entry = tarfile.TarInfo(name)
            entry.type = kind
            entry.size = len(body)
            t.addfile(entry, io.BytesIO(body))
    return raw.getvalue()


def fixture_copy(name="sequence.meta", meta=META):
    return archive([(name, json.dumps(meta).encode(), tarfile.REGTYPE)])


class SequenceCopyTests(unittest.TestCase):
    def test_streamed_archive_not_host_path(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(
                identity_probe.subprocess, "check_output", return_value=fixture_copy()) as proc:
            dest = Path(tmp) / "sequence.meta"
            self.assertEqual(identity_probe.capture_sequence_metadata(PREFIX, dest), META)
            self.assertEqual(json.loads(dest.read_text()), META)
            proc.assert_called_once_with(["docker", "cp", PREFIX+"-agent-runtime:"+PATH, "-"], timeout=30)

    def test_invalid_archive_does_not_write(self):
        bad = [b"", b"x"*65537, b"not a tar", fixture_copy("../sequence.meta"),
               archive([("sequence.meta", b"", tarfile.SYMTYPE)]),
               archive([("sequence.meta", b"x", tarfile.REGTYPE), ("extra", b"x", tarfile.REGTYPE)]),
               archive([("sequence.meta", b"x"*4097, tarfile.REGTYPE)]),
               fixture_copy(meta={"highestSequence": True, "agentStreamId": "x"}),
               fixture_copy(meta={"highestSequence": -1, "agentStreamId": "x"}),
               fixture_copy(meta={"highestSequence": 1, "agentStreamId": ""})]
        for raw in bad:
            with self.subTest(size=len(raw)), tempfile.TemporaryDirectory() as tmp, patch.object(
                    identity_probe.subprocess, "check_output", return_value=raw):
                dest = Path(tmp) / "sequence.meta"
                with self.assertRaises((ValueError, tarfile.TarError)):
                    identity_probe.capture_sequence_metadata(PREFIX, dest)
                self.assertFalse(dest.exists())

    def test_copy_command_failure_never_creates_evidence(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(
                identity_probe.subprocess, "check_output", side_effect=subprocess.CalledProcessError(1, ["docker", "cp"])):
            dest = Path(tmp) / "sequence.meta"
            with self.assertRaises(subprocess.CalledProcessError):
                identity_probe.capture_sequence_metadata(PREFIX, dest)
            self.assertFalse(dest.exists())

    def test_existing_evidence_is_not_overwritten(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(
                identity_probe.subprocess, "check_output", return_value=fixture_copy()):
            dest = Path(tmp) / "sequence.meta"; dest.write_text("old")
            with self.assertRaises(FileExistsError):
                identity_probe.capture_sequence_metadata(PREFIX, dest)
            self.assertEqual(dest.read_text(), "old")


class CopyAdmissionTests(unittest.TestCase):
    def custody(self, service="agent-runtime", running=True, paused=True):
        name = PREFIX+"-"+service
        obj = {"Id": REF, "Name": "/"+name, "State": {"Running": running, "Paused": paused}}
        return SimpleNamespace(prefix=PREFIX, evidence=Path("/fixture"), docker=["/fake/docker"],
            active=Mock(return_value={"container": {REF: (name, obj)}}),
            inspect=Mock(return_value=obj), check=Mock())

    def test_agent_copy_uses_exact_id_and_rechecks_ownership(self):
        c = self.custody()
        with patch.object(isolation.os, "execv") as call:
            isolation.docker_gate(c, ["cp", PREFIX+"-agent-runtime:"+PATH, "-"])
        c.inspect.assert_called_once_with("container", REF)
        c.check.assert_called_once()
        call.assert_called_once_with(c.docker[0], c.docker+["cp", REF+":"+PATH, "-"])

    def test_copy_denies_unpaused_unknown_or_wrong_resource(self):
        rows = [(self.custody(paused=False), PREFIX+"-agent-runtime:"+PATH, "-"),
                (self.custody(paused=None), PREFIX+"-agent-runtime:"+PATH, "-"),
                (self.custody(running=False), PREFIX+"-agent-runtime:"+PATH, "-"),
                (self.custody(), PREFIX+"-agent-runtime:/var/lib/platform/agent/identity.json", "-"),
                (self.custody(), PREFIX+"-agent-runtime:"+PATH, "/tmp/host-file"),
                (self.custody(), "foreign-agent:"+PATH, "-"),
                (self.custody(service="core"), PREFIX+"-core:"+PATH, "-")]
        for c, src, dest in rows:
            with self.subTest(src=src, dest=dest), patch.object(isolation.os, "execv") as call:
                with self.assertRaises(ValueError): isolation.docker_gate(c, ["cp", src, dest])
                call.assert_not_called()

    def test_rechecked_ownership_failure_denies_copy(self):
        c = self.custody(); c.check.side_effect=ValueError("identity drift")
        with patch.object(isolation.os, "execv") as call:
            with self.assertRaises(ValueError): isolation.docker_gate(c, ["cp", PREFIX+"-agent-runtime:"+PATH, "-"])
            call.assert_not_called()

    def test_existing_kafka_stream_copy_preserved(self):
        c = self.custody(service="kafka", paused=False)
        path = "/tmp/platform-kafka-server.properties"
        with patch.object(isolation.os, "execv") as call:
            isolation.docker_gate(c, ["cp", PREFIX+"-kafka:"+path, "-"])
        call.assert_called_once_with(c.docker[0], c.docker+["cp", REF+":"+path, "-"])


class FrozenDiagnosticsTests(unittest.TestCase):
    def test_terminal_phase_never_execs_paused_agent(self):
        source = Path(__file__).with_name("run.sh").read_text()
        body = source[source.index("dump_evidence() {"):source.index("\nsample_agent_resources()")]
        self.assertIn("IDENTITY_PROBE_FROZEN=1\npython3", source)
        for frozen in ("0", "1"):
            with self.subTest(frozen=frozen), tempfile.TemporaryDirectory() as tmp:
                env = dict(os.environ, EVIDENCE_DIR=tmp, ARTIFACT_DIR=tmp, TRACE=str(Path(tmp)/"trace"),
                           PLATFORM_E2E_PREFIX=PREFIX, IDENTITY_PROBE_FROZEN=frozen)
                stub = 'docker() { printf "%s\\n" "$*" >> "$TRACE"; }\n'
                subprocess.run(["bash", "-c", "set -eu\n"+stub+body+"\ndump_evidence"], env=env, check=True, timeout=10)
                calls = (Path(tmp)/"trace").read_text()
                self.assertEqual("exec "+PREFIX+"-agent-runtime " in calls, frozen == "0")
                self.assertIn("logs "+PREFIX+"-agent-runtime", calls)
                if frozen == "1": self.assertIn("queue exec skipped", (Path(tmp)/"debug.txt").read_text())


if __name__ == "__main__":
    unittest.main()
