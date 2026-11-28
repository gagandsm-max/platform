"""Exercise Tests 16 and 18 and security smoke with fake Docker, PKI and curl."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


TESTS = Path(__file__).resolve().parents[1]
ALERT = "curl: (56) OpenSSL SSL_read: error:0A000418:SSL routines::tlsv1 alert unknown ca, errno 0"
VERSIONED_CONNECT_ALERT = "curl: (35) OpenSSL/3.5.5: error:0A000418:SSL routines::tlsv1 alert unknown ca"
VERSIONED_READ_ALERT = (
    "curl: (56) OpenSSL SSL_read: OpenSSL/3.5.5: error:0A000418:SSL routines::tlsv1 alert unknown ca, errno 0"
)
FORBIDDEN = '{"status":"ERROR","code":"ERR_SENSOR_CREDENTIAL_UNKNOWN"}'
SECRET = "fixture-content-must-never-be-logged"
TRUSTED_CONTROLS = ["sensor-net-01", "sensor-infra-01"]
INITIAL_PROBES = TRUSTED_CONTROLS + ["sensor-anonymous", "sensor-rogue"]

UI_HEADER = (
    "Active Internet connections (only servers)\n"
    "Proto Recv-Q Send-Q Local Address           Foreign Address         State      \n"
)
UI_SAFE = UI_HEADER + "tcp        0      0 0.0.0.0:443             0.0.0.0:*               LISTEN\n"

FAKE_TOOLS = r'''
import json
import os
from pathlib import Path
import sys

args = sys.argv[1:]
tool = Path(sys.argv[0]).name
scenario = json.loads(os.environ["MTLS_SCENARIO"])

def argument(flag):
    return args[args.index(flag) + 1]

if tool == "openssl":
    if args[0] in ("x509", "pkey"):
        path = Path(argument("-in"))
        if path.name == scenario.get("invalid"):
            print(os.environ["MTLS_SECRET"], file=sys.stderr)
            sys.exit(1)
        if "-pubkey" in args or "-pubout" in args:
            assert "-noout" in args or "-pubout" in args
            print("mismatch" if path.name == scenario.get("mismatch") else path.stem)
    else:
        assert args[0] == "verify" and argument("-purpose") == "sslclient"
        ca, cert = Path(argument("-trusted")).stem, Path(args[-1]).stem
        if cert == scenario.get("bad_chain"):
            sys.exit(2)
        if cert == "untrusted-client" and ca == "ca":
            if scenario.get("rogue_trusted"):
                sys.exit(0)
            if not scenario.get("verify_error"):
                print("error 20 at 0 depth lookup: unable to get local issuer certificate", file=sys.stderr)
            sys.exit(2)
        assert ca == ("untrusted-ca" if cert == "untrusted-client" else "ca")
    sys.exit(0)

if tool == "docker":
    calls = Path(os.environ["UI_CALLS"])
    previous = calls.read_text().splitlines() if calls.exists() else []
    with calls.open("a") as log:
        log.write(args[0] + "\n")
    container_id = "a" * 64
    ui = scenario.get("ui", {})
    if args[0] == "inspect":
        assert args == ["inspect", "--type", "container", "--format",
                        "{{.Id}}|{{.Name}}|{{.State.Running}}", "platform-ui"]
        phase = "after" if previous else "before"
        rc, output = ui.get(phase, [0, container_id + "|/platform-ui|true"])
    else:
        assert args == ["exec", container_id, "busybox", "netstat", "-lnt"]
        assert previous == ["inspect"]
        rc, output = ui.get("exec", [0, os.environ["UI_SAFE"]])
    print(output, end="")
    if args[0] == "exec":
        sys.stderr.write(ui.get("exec_stderr", ""))
    sys.exit(rc)

if args[-1] == "https://127.0.0.1:23000/":
    assert args == ["-k", "-s", "-o", "/dev/null", "-w", "%{http_code}", args[-1]]
    print("200")
    sys.exit(0)

if args[-1].startswith("http://"):
    assert args[-1] == "http://127.0.0.1:28443/api/v1/version"
    print("400 Bad Request")
    sys.exit(0)

assert args[0] == "--disable" and "-k" not in args
assert argument("--proxy") == "" and argument("--noproxy") == "*"
assert argument("--proto") == "=https"
assert 0 < int(argument("--connect-timeout")) <= int(argument("--max-time")) <= 10
assert Path(argument("--cacert")).name == "ca.crt"
assert argument("--write-out") == "%{http_code}"
assert args[-1] == "https://127.0.0.1:28443/api/v1/sensor/heartbeat"
identity = json.loads(argument("--data"))["sensor_id"]
certs = {"sensor-net-01": "sensor-net", "sensor-infra-01": "sensor-infra",
         "sensor-rogue": "untrusted-client"}
if identity in certs:
    assert Path(argument("--cert")).stem == Path(argument("--key")).stem == certs[identity]
else:
    assert identity == "sensor-anonymous" and "--cert" not in args and "--key" not in args
call_log = Path(os.environ["MTLS_CALLS"])
previous_calls = call_log.read_text().splitlines() if call_log.exists() else []
attempt = previous_calls.count(identity) + 1
with call_log.open("a") as calls:
    calls.write(identity + "\n")
default = [0, "200", json.dumps({"sensor_identity": identity}), ""]
if identity == "sensor-anonymous":
    default = [0, "401", "", ""]
elif identity == "sensor-rogue":
    default = [0, "403", os.environ["MTLS_FORBIDDEN"], ""]
rc, status, body, diagnostic = scenario.get(f"{identity}#{attempt}", scenario.get(identity, default))
Path(argument("--output")).write_text(body)
sys.stdout.write(status)
sys.stderr.write(diagnostic)
sys.exit(rc)
'''


class MtlsGuardTests(unittest.TestCase):
    def run_guard(self, entry, **scenario):
        script = TESTS / "run-all-tests.sh"
        test18 = "TEST_NUM=18\n" + script.read_text().split("TEST_NUM=18\n", 1)[1].split(
            "# Test 19:", 1
        )[0]
        test16 = "TEST_NUM=16\n" + script.read_text().split("TEST_NUM=16\n", 1)[1].split(
            "# Test 17:", 1
        )[0]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            certs = root / "deploy/docker/certs"
            certs.mkdir(parents=True)
            for name in ("ca.crt", "untrusted-ca.crt", "sensor-net.crt", "sensor-net.key",
                         "sensor-infra.crt", "sensor-infra.key", "untrusted-client.crt",
                         "untrusted-client.key"):
                if name != scenario.get("missing"):
                    (certs / name).write_text(SECRET)
            (root / "tests").mkdir()
            shutil.copyfile(TESTS / "run-security-smoke.sh", root / "tests/run-security-smoke.sh")
            (root / "scripts").mkdir()
            (root / "scripts/check-host-runtime-isolation.sh").write_text(
                '[[ "$1" == --synthetic-test ]]\n'
            )
            for name in ("curl", "openssl", "docker"):
                path = root / name
                path.write_text(f"#!{sys.executable}\n" + FAKE_TOOLS)
                path.chmod(0o700)
            calls_path = root / "calls"
            env = dict(
                os.environ, PATH=f"{root}:{os.defpath}", BASH_ENV="/dev/null",
                PLATFORM_CI="1", ROOT_DIR=directory,
                PLATFORM_CORE_HOST_PORT="28443", MTLS_SCENARIO=json.dumps(scenario),
                MTLS_CALLS=str(calls_path), MTLS_FORBIDDEN=FORBIDDEN, MTLS_SECRET=SECRET,
                UI_CALLS=str(root / "ui_calls"), UI_SAFE=UI_SAFE, PLATFORM_UI_HOST_PORT="23000",
            )
            if entry in ("integration", "test16"):
                command = ["bash", "-u", "-c",
                           'pass() { echo GUARD_ACCEPTED; }; fail() { echo GUARD_REJECTED; return 1; }\n'
                           + (test16 if entry == "test16" else test18)]
            else:
                command = ["bash", str(root / "tests/run-security-smoke.sh")]
            result = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True,
                                    timeout=10, check=False)
            calls = calls_path.read_text().splitlines() if calls_path.exists() else []
        output = result.stdout + result.stderr
        self.assertNotIn(SECRET, output)
        return result.returncode, calls, output

    def test_ui_listener_evidence_through_both_entry_points(self):
        identity = "a" * 64 + "|/platform-ui|true"
        cases = [
            ({}, None),
            ({"exec": [0, UI_HEADER + "tcp 0 0 127.0.0.1:8080 192.0.2.1:80 LISTEN\n"
                       "tcp6 0 0 :::443 :::* LISTEN\n"
                       "tcp6 0 0 [::1]:443 [::]:* LISTEN\n"
                       "tcp 0 0 2001:db8::a:23000 ::ffff:192.0.2.2:80 LISTEN\n"]}, None),
            ({"exec": [0, UI_SAFE + "tcp 0 0 127.0.0.1:80 0.0.0.0:* LISTEN\n"]}, "TCP port 80"),
            ({"exec": [0, UI_SAFE + "tcp6 0 0 :::80 :::* LISTEN\n"]}, "TCP port 80"),
            ({"exec": [0, UI_SAFE + "tcp6 0 0 [::1]:80 [::]:* LISTEN\n"]}, "TCP port 80"),
            ({"exec": [0, ""]}, "empty or malformed"),
            ({"exec": [0, UI_HEADER]}, "empty or malformed"),
            ({"exec": [0, UI_SAFE + "tcp 0 0 999.0.0.1:443 0.0.0.0:* LISTEN\n"]}, "empty or malformed"),
            ({"exec": [0, UI_SAFE + "tcp6 0 0 2001:::1:443 :::* LISTEN\n"]}, "empty or malformed"),
            ({"exec": [0, UI_SAFE + "tcp 0 0 127.0.0.1:443 0.0.0.0:* ESTABLISHED\n"]}, "empty or malformed"),
            ({"exec": [127, ""]}, "inventory command failed"),
            ({"exec": [124, UI_SAFE]}, "inventory command failed"),
            ({"exec_stderr": "netstat: /proc/net/tcp6: Permission denied\n"}, "empty or malformed"),
            ({"before": [1, identity]}, "inspection failed"),
            ({"before": [0, identity.replace("/platform-ui", "/other")]}, "exact running"),
            ({"before": [0, identity[52:]]}, "full ID"),
            ({"before": [0, identity.replace("true", "false")]}, "exact running"),
            ({"after": [1, identity]}, "reinspection failed"),
            ({"after": [0, "b" * 64 + "|/platform-ui|true"]}, "identity or running state changed"),
            ({"after": [0, identity.replace("true", "false")]}, "identity or running state changed"),
        ]
        for entry in ("test16", "smoke"):
            for ui, failure in cases:
                with self.subTest(entry=entry, ui=ui):
                    rc, calls, output = self.run_guard(entry, ui=ui)
                    if failure is None:
                        self.assertEqual(rc, 0, output)
                        self.assertIn("GUARD_ACCEPTED" if entry == "test16" else
                                      "no TCP port 80 listener", output)
                        if entry == "smoke":
                            self.assertEqual(calls, INITIAL_PROBES + TRUSTED_CONTROLS)
                    else:
                        self.assertNotEqual(rc, 0, output)
                        self.assertIn(failure, output)
                        self.assertEqual(calls, [])

    def test_fixture_failures_prevent_probing(self):
        cases = [
            ({"missing": "untrusted-client.crt"}, "invalid certificate"),
            ({"missing": "untrusted-client.key"}, "invalid private key"),
            ({"invalid": "untrusted-ca.crt"}, "invalid certificate"),
            ({"invalid": "sensor-infra.key"}, "invalid private key"),
            ({"mismatch": "untrusted-client.key"}, "do not match"),
            ({"bad_chain": "sensor-net"}, "intended CA failed"),
            ({"bad_chain": "untrusted-client"}, "intended CA failed"),
            ({"rogue_trusted": True}, "trusted by the development CA"),
            ({"verify_error": True}, "issuer rejection could not be established"),
        ]
        for entry in ("integration", "smoke"):
            for scenario, message in cases:
                with self.subTest(entry=entry, scenario=scenario):
                    rc, calls, output = self.run_guard(entry, **scenario)
                    self.assertNotEqual(rc, 0, output)
                    self.assertEqual(calls, [])
                    self.assertIn(message, output)

    def test_rogue_false_passes_and_unattributed_responses_fail(self):
        cases = [
            (7, "000", "", "Connection refused"),
            (28, "000", "", "Operation timed out"),
            (58, "000", "", "could not load private key"),
            (0, "000", "", ""),
            (0, "", "", ""),
            (0, "403\n", FORBIDDEN, ""),
            (0, "403403", FORBIDDEN, ""),
            (0, "200", FORBIDDEN, ""),
            (28, "403", FORBIDDEN, "Operation timed out"),
            (0, "403", "Forbidden", ""),
            (35, "000", "", "TLS failed"),
            (56, "000", "", SECRET + " unknown ca"),
            (60, "000", "", ALERT),
            (35, "000", "", VERSIONED_CONNECT_ALERT.replace("OpenSSL/", "LibreSSL/")),
            (56, "000", "", VERSIONED_READ_ALERT.replace("3.5.5", "3.5.x")),
            (56, "000", "", VERSIONED_READ_ALERT.replace("OpenSSL SSL_read:", "TLS SSL_read:")),
            (35, "000", "", VERSIONED_CONNECT_ALERT.replace("0A000418", "0A000416")),
            (35, "000", "", VERSIONED_CONNECT_ALERT.replace(
                "0A000418:SSL routines::tlsv1 alert unknown ca",
                "0A000086:SSL routines::certificate verify failed")),
            (55, "000", "", "curl: (55) Send failure: Broken pipe"),
            (55, "000", "", VERSIONED_READ_ALERT.replace("(56)", "(55)")),
            (35, "000", "", VERSIONED_READ_ALERT),
            (56, "200", "", VERSIONED_READ_ALERT),
        ]
        for entry in ("integration", "smoke"):
            for probe in cases:
                with self.subTest(entry=entry, probe=probe):
                    rc, calls, output = self.run_guard(entry, **{"sensor-rogue": probe})
                    self.assertNotEqual(rc, 0, output)
                    self.assertEqual(calls[-1], "sensor-rogue")
                    self.assertIn(f"curl_exit={probe[0]}", output)

    def test_completed_http_and_explicit_peer_rejections_pass(self):
        for entry in ("integration", "smoke"):
            for rogue, evidence in [
                ((0, "403", FORBIDDEN, ""), "HTTP 403"),
                ((56, "000", "", ALERT), "peer TLS client-certificate alert"),
                ((35, "000", "", VERSIONED_CONNECT_ALERT), "peer TLS client-certificate alert"),
                ((56, "000", "", VERSIONED_READ_ALERT), "peer TLS client-certificate alert"),
                ((35, "000", "", "curl: (35) error:0A000418:SSL routines::tlsv1 alert unknown ca"),
                 "peer TLS client-certificate alert"),
                ((56, "000", "", ALERT.replace("0A000418", "0A000416").replace(
                    "tlsv1 alert unknown ca", "sslv3 alert certificate unknown")),
                 "peer TLS client-certificate alert"),
            ]:
                with self.subTest(entry=entry, rogue=rogue, evidence=evidence):
                    rc, calls, output = self.run_guard(entry, **{"sensor-rogue": rogue})
                    self.assertEqual(rc, 0, output)
                    self.assertEqual(calls, INITIAL_PROBES + TRUSTED_CONTROLS)
                    self.assertIn(evidence, output)
                    self.assertIn("HTTP 401", output)

    def test_controls_require_completed_status_and_exact_json_identity(self):
        cases = [
            ("sensor-net-01", (28, "200", '{"sensor_identity":"sensor-net-01"}', "")),
            ("sensor-net-01", (0, "200", '{"nested":{"sensor_identity":"sensor-net-01"}}', "")),
            ("sensor-net-01", (0, "200", '{"sensor_identity":"sensor-net-01"}garbage', "")),
            ("sensor-infra-01", (0, "200", '{"sensor_identity":"sensor-net-01"}', "")),
            ("sensor-infra-01", (0, "401", '{"sensor_identity":"sensor-infra-01"}', "")),
            ("sensor-anonymous", (28, "401", "", "")),
            ("sensor-anonymous", (0, "200", "", "")),
        ]
        for entry in ("integration", "smoke"):
            for identity, probe in cases:
                with self.subTest(entry=entry, identity=identity, probe=probe):
                    rc, calls, output = self.run_guard(entry, **{identity: probe})
                    self.assertNotEqual(rc, 0, output)
                    self.assertEqual(calls[-1], identity)
                    self.assertIn("expected evidence absent", output)

    def test_post_negative_controls_require_completed_status_and_exact_json_identity(self):
        for entry in ("integration", "smoke"):
            for index, identity in enumerate(TRUSTED_CONTROLS):
                body = json.dumps({"sensor_identity": identity})
                cases = [
                    (28, "200", body, SECRET),
                    (0, "403", body, ""),
                    (0, "200", json.dumps({"sensor_identity": SECRET}), ""),
                ]
                for probe in cases:
                    with self.subTest(entry=entry, identity=identity, probe=probe):
                        rc, calls, output = self.run_guard(entry, **{f"{identity}#2": probe})
                        self.assertNotEqual(rc, 0, output)
                        self.assertEqual(calls, INITIAL_PROBES + TRUSTED_CONTROLS[:index + 1])
                        self.assertIn("expected evidence absent", output)
                        self.assertNotIn("GUARD_ACCEPTED", output)
