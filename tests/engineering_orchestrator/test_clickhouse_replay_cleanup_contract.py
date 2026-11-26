"""Exercise the replay harness with a stateful Docker CLI double."""
import json
import hashlib
import os
import signal
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "run-clickhouse-replay-migration-test.sh"
LABEL = "io.amrudai.clickhouse-replay.run"


def mock_docker(args):
    state_file = Path(os.environ["FIXTURE_STATE"])
    state = json.loads(state_file.read_text())
    resources = state["resources"]
    mode = os.environ["FIXTURE_MODE"]
    kind, action, *rest = args if args[0] in ("container", "network", "volume") else (
        "container", *args
    )

    if args[:2] == ["context", "show"]:
        print(os.environ.get("FIXTURE_CONTEXT", "default"))
        return 0
    if args[:2] == ["context", "inspect"]:
        print(os.environ.get("FIXTURE_ENDPOINT", "unix:///var/run/docker.sock"))
        return 0

    def option(flag, default=""):
        return rest[rest.index(flag) + 1] if flag in rest else default

    def record(event):
        state["events"].append(event)
        state_file.write_text(json.dumps(state))

    def matching(ref):
        return next(((name, item) for name, item in resources.items()
                     if item["kind"] == kind and ref in (name, item["id"])), ("", None))

    if action == "ls":
        if mode in ("inventory_error", "signal_term_inventory_error"):
            print("fixture inventory failed", file=sys.stderr)
            return 45
        filter_value = option("--filter")
        label = filter_value[6:] if filter_value.startswith("label=") else ""
        filter_value = option("--filter")
        id_filter = filter_value[3:] if filter_value.startswith("id=") else ""
        name_filter = filter_value[5:] if filter_value.startswith("name=") else ""
        for name, item in resources.items():
            if (item["kind"] == kind and (not label or item["label"] == label)
                    and (not id_filter or item["id"] == id_filter)
                    and (not name_filter or name_filter in name)):
                print(item["id"] if kind != "volume" else name)
        return 0
    if action == "inspect":
        inspect_format = option("--format")
        if inspect_format and f'"{LABEL}"' not in inspect_format and inspect_format != "{{.Name}}":
            print("fixture ownership format missing label key", file=sys.stderr)
            return 44
        name, item = matching(rest[-1])
        if not item:
            print(f"Error: No such {kind}: {rest[-1]}", file=sys.stderr)
            return 1
        if mode == "inspect_error" and kind == "volume":
            print("fixture inspect failed", file=sys.stderr)
            return 43
        record(["inspected", name])
        if inspect_format == "{{.Name}}":
            print(name)
            return 0
        value = item["label"].partition("=")[2]
        print(f'{item["id"]} {value or "<no value>"}')
        if (mode == "inspect_nonzero_matching" and kind == "volume"
                and sum(event[0] == "inspected" and event[1] == name
                        for event in state["events"]) > 1):
            print("fixture inspect returned matching stdout with failure", file=sys.stderr)
            return 43
        return 0
    if action in ("create", "run"):
        name = option("--name") if action == "run" else rest[-1]
        if ((mode == "foreign" and kind == "volume")
                or (mode == "partial" and kind == "container")):
            resources.setdefault(name, {"kind": kind, "id": name, "label": "foreign"})
            record(["foreign", name])
            if kind == "container":
                print("fixture setup failure", file=sys.stderr)
                return 37
        if name not in resources:
            label = option("--label")
            if kind == "volume" and mode in ("missing_label", "wrong_label"):
                label = "" if mode == "missing_label" else LABEL + "=foreign"
            identity = name if kind == "volume" else hashlib.sha256(name.encode()).hexdigest()
            resources[name] = {"kind": kind, "id": identity, "label": label}
            record(["create", name])
        if ((mode == "signal_network_int" and kind == "network")
                or (mode == "signal_volume_int" and kind == "volume")
                or (mode == "signal_server_int" and kind == "container"
                    and "-migrate-" not in name)):
            os.kill(os.getppid(), signal.SIGINT)
        if mode == "network_after_create_error" and kind == "network":
            print("fixture network create response lost", file=sys.stderr)
            return 38
        if mode == "volume_after_create_error" and kind == "volume":
            print("fixture volume create response lost", file=sys.stderr)
            return 39
        if "--rm" in rest:
            del resources[name]
            record(["auto-remove", name])
        else:
            print(resources[name]["id"])
        if mode == "signal_migrator_int" and "-migrate-" in name:
            os.kill(os.getppid(), signal.SIGINT)
        return 0
    if action == "rm":
        for ref in (value for value in rest if not value.startswith("-")):
            name, item = matching(ref)
            if item:
                if mode == "hang_volume_remove" and kind == "volume":
                    record(["remove_attempt", name])
                    os.execv("/bin/sleep", ["sleep", "30"])
                if (mode == "replaced_after_check" and kind == "volume"
                        and ["inspected", name] in state["events"]):
                    item["id"] = "foreign-volume-id"
                    item["label"] = "foreign"
                    record(["foreign", name])
                record(["remove", name, ref, item["label"]])
                if mode in ("remove_error", "silent_residue") and kind == "volume":
                    return 42 if mode == "remove_error" else 0
                del resources[name]
                record(["removed", name])
        return 0
    if action == "restart":
        if mode == "signal_int":
            os.kill(os.getppid(), signal.SIGINT)
        elif mode in ("signal_term", "signal_term_inventory_error"):
            os.kill(os.getppid(), signal.SIGTERM)
        return 0
    if action in ("exec", "start"):
        if action == "start":
            return 0
        sql = option("--query")
        if "must.fail" in sql:
            return 1
        if sql.startswith("SELECT engine"):
            print("View" if "name='metric_numeric'" in sql or "name='metric_counter'" in sql else "ReplacingMergeTree")
        elif sql.startswith("SELECT sorting_key"):
            print("tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id")
        elif sql.startswith("SELECT create_table_query"):
            print("metric_counter_raw FINAL metric_numeric_raw FINAL")
        elif sql.startswith("SELECT type"):
            print("UInt64")
        elif sql.startswith("SELECT value"):
            print(next((value for suffix, value in (("-b'", "9223372036854775808"),
                                                    ("-c'", "18446744073709551615"))
                        if sql.split(" FORMAT")[0].endswith(suffix)), "9007199254740993"))
        elif sql.startswith("SELECT count()"):
            if "v002_legacy" in sql:
                print(0)
            elif ("WHERE" in sql and "FROM metric_numeric WHERE metric_id='" in sql
                  or "FROM metric_counter WHERE" in sql):
                print(1)
            else:
                print(2)
        return 0
    raise AssertionError(f"unexpected Docker call: {args}")


class ClickHouseReplayCleanupContract(unittest.TestCase):
    def run_fixture(self, mode="clean", mock=True, docker_host=None, profile="mock",
                    hosted=False, docker_context=None, mock_value=None,
                    hosted_config="valid", fixture_context="default"):
        with tempfile.TemporaryDirectory() as directory:
            fixture = Path(directory)
            state = fixture / "state.json"
            state.write_text('{"resources": {}, "events": []}')
            bash = shutil.which("bash")
            for utility in ("cat", "seq", "timeout", "od", "tr", "python3"):
                (fixture / utility).symlink_to(shutil.which(utility))
            if mode == "hang_volume_remove":
                timeout_wrapper = fixture / "timeout"
                timeout_wrapper.unlink()
                timeout_wrapper.write_text(
                    f"#!{bash}\n"
                    "if [[ \"$*\" == *'docker volume rm '* ]]; then\n"
                    "  printf '%s\\n' \"$@\" > \"$FIXTURE_TIMEOUT_ARGS\"\n"
                    "  shift 3\n"
                    "  exec /usr/bin/timeout --signal=TERM --kill-after=1 1 \"$@\"\n"
                    "fi\n"
                    "exec /usr/bin/timeout \"$@\"\n"
                )
                timeout_wrapper.chmod(0o700)
            for name, body in {
                "dirname": 'printf "%s\\n" "${1%/*}"',
                "sleep": "exit 0",
            }.items():
                (fixture / name).write_text(f"#!{bash}\n{body}\n")
                (fixture / name).chmod(0o700)
            docker = fixture / "docker"
            docker.write_text(f"#!{bash}\nexec {shlex.quote(sys.executable)} -I "
                              f'{shlex.quote(str(Path(__file__).resolve()))} --mock-docker "$@"\n')
            docker.chmod(0o700)
            fixture_env = {"PATH": directory, "LC_ALL": "C", "FIXTURE_STATE": str(state),
                           "FIXTURE_MODE": mode,
                           "DOCKER_HOST": docker_host or f"unix://{fixture}/no-daemon.sock"}
            fixture_env["FIXTURE_TIMEOUT_ARGS"] = str(fixture / "timeout-args")
            (fixture / "home" / ".docker").mkdir(parents=True)
            fixture_env.update({"HOME": str(fixture / "home"),
                                "DOCKER_CONFIG": str(fixture / "home" / ".docker"),
                                "AMRUDAI_REPLAY_RUNTIME_PROFILE": profile})
            if mock:
                fixture_env["REPLAY_HARNESS_MOCK"] = mock_value if mock_value is not None else "1"
            elif mock_value is not None:
                fixture_env["REPLAY_HARNESS_MOCK"] = mock_value
            else:
                fixture_env["PLATFORM_ISOLATED_RUNTIME"] = "approved"
            if hosted:
                workflow_config = fixture / "platform-ci-docker-cli"
                workflow_config.mkdir()
                config_path = workflow_config / "config.json"
                if hosted_config == "empty_object":
                    config_path.write_text("{}")
                elif hosted_config == "non_default":
                    config_path.write_text('{"currentContext":"remote"}')
                elif hosted_config == "malformed":
                    config_path.write_text('{"currentContext":')
                elif hosted_config == "non_object":
                    config_path.write_text('[]')
                elif hosted_config == "missing_directory":
                    workflow_config.rmdir()
                fixture_env.update({"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted",
                                    "RUNNER_OS": "Linux", "GITHUB_RUN_ID": "1234",
                                    "GITHUB_RUN_ATTEMPT": "1", "GITHUB_WORKSPACE": str(fixture),
                                    "RUNNER_TEMP": str(fixture), "DOCKER_CONFIG": str(workflow_config),
                                    "DOCKER_HOST": "", "FIXTURE_CONTEXT": fixture_context})
                if hosted_config == "wrong_path":
                    fixture_env["DOCKER_CONFIG"] = str(fixture / "home" / ".docker")
                if hosted_config == "escaping_directory_symlink":
                    workflow_config.rmdir()
                    workflow_config.symlink_to(fixture / "home" / ".docker", target_is_directory=True)
            if docker_context:
                fixture_env["DOCKER_CONTEXT"] = docker_context
            try:
                process = subprocess.run(
                    [bash, os.environ.get("REPLAY_SCRIPT_UNDER_TEST", str(SCRIPT))],
                    env=fixture_env,
                    capture_output=True, text=True, timeout=120,
                )
            except subprocess.TimeoutExpired as error:
                raise AssertionError(f"fixture exceeded 120s: state={state.read_text()} "
                                     f"stdout={error.stdout!r} stderr={error.stderr!r}") from error
            state_data = json.loads(state.read_text())
            timeout_args = fixture / "timeout-args"
            if timeout_args.exists():
                state_data["timeout_args"] = timeout_args.read_text().splitlines()
            return process, state_data

    def assert_foreign_retained(self, state):
        foreign = {event[1] for event in state["events"] if event[0] == "foreign"}
        foreign.update(name for name, resource in state["resources"].items()
                       if resource["label"] in ("", LABEL + "=foreign"))
        self.assertTrue(foreign)
        self.assertTrue(foreign <= state["resources"].keys(), state)
        self.assertFalse(any(event[0] == "remove" and event[1] in foreign
                             for event in state["events"]), state)

    def test_complete_fixture_success_and_owned_cleanup(self):
        result, state = self.run_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(" PASS", result.stdout)
        self.assertFalse(state["resources"], state)
        self.assertTrue(any(event[0] == "removed" for event in state["events"]), state)

    def test_cleanup_failure_after_primary_success(self):
        result, state = self.run_fixture("remove_error")
        self.assertIn("rerun V003 idempotently", result.stdout)
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["volume"])
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertNotIn(" PASS", result.stdout)
        self.assertIn(next(iter(state["resources"])), result.stdout)

    def test_successful_removal_response_with_residue(self):
        result, state = self.run_fixture("silent_residue")
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["volume"])
        self.assertNotEqual(result.returncode, 0, result.stdout)

    def test_foreign_same_name_is_rejected(self):
        result, state = self.run_fixture("foreign")
        self.assert_foreign_retained(state)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertFalse(any(e[0] == "create" and "platform-ch-replay-" in e[1] for e in state["events"]))

    def test_run_scoped_identity_and_owner_label_are_unique(self):
        result, state = self.run_fixture()
        self.assertEqual(result.returncode, 0, result.stderr)
        created = [event[1] for event in state["events"] if event[0] == "create"]
        self.assertTrue(created)
        labels = [event[3] for event in state["events"] if event[0] == "remove"]
        self.assertTrue(labels)
        run_ids = {label.partition("=")[2] for label in labels}
        self.assertEqual(len(run_ids), 1)
        self.assertRegex(next(iter(run_ids)), r"^[0-9a-f]{32}$")

    def test_hostile_replacement_is_security_counterexample_not_safety_evidence(self):
        """Record TOCTOU deletion as a counterexample, never as safety evidence."""
        result, state = self.run_fixture("replaced_after_check")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(any(event[0] == "inspected" for event in state["events"]), state)
        self.assertTrue(any(event[0] == "foreign" for event in state["events"]), state)
        self.assertTrue(any(event[0] == "remove" and event[3] == "foreign"
                            for event in state["events"]), state)
        self.assertFalse(state["resources"], state)
        self.assertIn("hostile-daemon safety is not certified", result.stdout)
        self.assertNotIn("safety PASS", result.stdout)

    def test_partial_setup_preserves_primary_failure(self):
        result, state = self.run_fixture("partial")
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertIn("fixture setup failure", result.stderr)
        self.assert_foreign_retained(state)
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["container"])

    def test_partial_network_acquisition_is_identified_cleaned_and_status_preserved(self):
        result, state = self.run_fixture("network_after_create_error")
        self.assertEqual(result.returncode, 38, result.stderr)
        self.assertIn("network create response lost", result.stderr)
        self.assertFalse(state["resources"], state)

    def test_partial_volume_acquisition_is_identified_cleaned_and_status_preserved(self):
        result, state = self.run_fixture("volume_after_create_error")
        self.assertEqual(result.returncode, 39, result.stderr)
        self.assertIn("volume create response lost", result.stderr)
        self.assertFalse(state["resources"], state)

    def test_absent_or_wrong_label_is_rejected(self):
        for mode in ("missing_label", "wrong_label"):
            with self.subTest(mode=mode):
                result, state = self.run_fixture(mode)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assert_foreign_retained(state)

    def test_inspect_error_is_failure_without_volume_removal(self):
        result, state = self.run_fixture("inspect_error")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("fixture inspect failed", result.stderr)
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["volume"])
        self.assertFalse(any(e[0] == "remove" and e[1] in state["resources"] for e in state["events"]))

    def test_nonzero_inspect_with_matching_stdout_never_authorizes_volume_removal(self):
        result, state = self.run_fixture("inspect_nonzero_matching")
        volume_names = [name for name, item in state["resources"].items()
                        if item["kind"] == "volume"]
        self.assertEqual(len(volume_names), 1, state)
        self.assertIn("successful inspect", result.stdout)
        self.assertEqual(sum(event[0] == "inspected" and event[1] == volume_names[0]
                             for event in state["events"]), 3, state)
        self.assertFalse(any(event[0] == "remove" and event[1] == volume_names[0]
                             for event in state["events"]), state)

    def test_cleanup_reports_missing_volume_identity(self):
        result, state = self.run_fixture("inspect_error")
        self.assertIn("volume identity unavailable", result.stdout)
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["volume"])

    def test_cleanup_timeout_bounds_hanging_fake_docker_call(self):
        result, state = self.run_fixture("hang_volume_remove")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("cleanup failed removing volume", result.stdout)
        self.assertTrue(any(event[0] == "remove_attempt" for event in state["events"]), state)
        self.assertEqual(state["timeout_args"][:3], ["--signal=TERM", "--kill-after=2", "13"])
        self.assertEqual([item["kind"] for item in state["resources"].values()], ["volume"])

    def test_inventory_error_is_not_misreported_as_absence(self):
        result, state = self.run_fixture("inventory_error")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("cleanup absence query failed", result.stdout)
        self.assertNotIn(" PASS", result.stdout)

    def test_term_preserves_signal_status_when_cleanup_absence_is_unverified(self):
        result, _ = self.run_fixture("signal_term_inventory_error")
        self.assertEqual(result.returncode, 143, result.stdout + result.stderr)
        self.assertIn("cleanup incomplete: owned resource absence is unverified", result.stdout)
        self.assertNotIn(" PASS", result.stdout)

    def test_cleanup_stops_dispatch_at_absolute_deadline(self):
        source = SCRIPT.read_text()
        bounded_docker = source[source.index("bounded_docker() {"):source.index("inspect_owner() {")]
        # Use the real deadline arithmetic and a deterministic shell clock seam.
        # Setting SECONDS before each call removes real waits and flaky timing.
        result = subprocess.run(
            ["/bin/bash", "-c", bounded_docker + """
            timeout() { printf '%s\\n' "$*"; }
            CLEANUP_ACTIVE=1
            CLEANUP_DEADLINE=45
            SECONDS=0
            bounded_docker volume inspect owned || exit $?
            SECONDS=37
            bounded_docker volume rm owned || exit $?
            SECONDS=43
            bounded_docker volume ls
            """],
            env={"PATH": "", "DOCKER_HOST": "unix:///tmp/no-daemon.sock"},
            capture_output=True, text=True, timeout=5,
        )
        self.assertEqual(result.returncode, 124)
        self.assertEqual(result.stdout.splitlines(), [
            "--signal=TERM --kill-after=2 13 docker volume inspect owned",
            "--signal=TERM --kill-after=2 6 docker volume rm owned",
        ])

    def test_mock_mode_requires_dead_end_socket(self):
        result, state = self.run_fixture(docker_host="unix:///tmp/real-docker.sock")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("FAIL:", result.stderr)
        self.assertFalse(state["events"], state)

    def test_arbitrary_environment_claim_does_not_authorize_real_runtime(self):
        result, state = self.run_fixture(mock=False, profile="")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("shared native runtime awaits the process-bound canonical flock", result.stderr)
        self.assertFalse(state["events"], state)

    def test_hosted_ci_fixture_profile_routes_only_with_consistent_metadata(self):
        result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(state["resources"], state)
        self.assertIn("runtime isolation is not certified", result.stdout)

    def test_hosted_ci_accepts_exact_workflow_config_with_default_context(self):
        result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True,
                                         hosted_config="empty_object")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(state["resources"], state)

    def test_hosted_ci_rejects_missing_or_wrong_workflow_config(self):
        for config in ("missing_directory", "wrong_path"):
            with self.subTest(config=config):
                result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True,
                                                 hosted_config=config)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("exact workflow Docker config directory", result.stderr)
                self.assertFalse(state["events"], state)

    def test_hosted_ci_rejects_non_default_context_and_malformed_config(self):
        cases = (("valid", "remote", "default Docker context"),
                 ("non_default", "default", "selects a non-default context"),
                 ("malformed", "default", "malformed Docker config"),
                 ("non_object", "default", "must be a JSON object"))
        for config, context, diagnostic in cases:
            with self.subTest(config=config, context=context):
                result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True,
                                                 hosted_config=config, fixture_context=context)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(diagnostic, result.stderr)
                self.assertFalse(state["events"], state)

    def test_hosted_ci_rejects_config_directory_symlink(self):
        result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True,
                                         hosted_config="escaping_directory_symlink")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("escapes the exact workflow job-local path", result.stderr)
        self.assertFalse(state["events"], state)

    def test_environment_claim_without_hosted_runner_metadata_never_authorizes(self):
        result, state = self.run_fixture(mock=False, profile="hosted-ci")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("lacks expected GitHub-hosted job metadata", result.stderr)
        self.assertFalse(state["events"], state)

    def test_unknown_or_mixed_profiles_are_rejected(self):
        for profile, mock, mock_value in (("typo", False, None), ("hosted-ci", True, None),
                                          ("hosted-ci", False, "true"),
                                          ("hosted-ci", False, "0")):
            with self.subTest(profile=profile, mock=mock, mock_value=mock_value):
                result, state = self.run_fixture(mock=mock, profile=profile, mock_value=mock_value)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                if mock_value or (profile == "hosted-ci" and mock):
                    self.assertIn("REPLAY_HARNESS_MOCK", result.stderr)
                else:
                    self.assertIn("unknown or mixed runtime profile", result.stderr)
                self.assertFalse(state["events"], state)

    def test_hosted_ci_rejects_remote_context_override(self):
        result, state = self.run_fixture(mock=False, profile="hosted-ci", hosted=True,
                                         docker_context="remote")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("remote Docker endpoint/context overrides", result.stderr)
        self.assertFalse(state["events"], state)

    def test_int_signal_runs_deterministic_cleanup_and_keeps_signal_status(self):
        result, state = self.run_fixture("signal_int")
        self.assertEqual(result.returncode, 130, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)

    def test_term_signal_runs_deterministic_cleanup_and_keeps_signal_status(self):
        result, state = self.run_fixture("signal_term")
        self.assertEqual(result.returncode, 143, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)

    def test_interrupt_during_migrator_acquisition_cleans_recorded_name(self):
        result, state = self.run_fixture("signal_migrator_int")
        self.assertEqual(result.returncode, 130, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)
        self.assertTrue(any(event[0] == "removed" and "-migrate-" in event[1]
                            for event in state["events"]), state)

    def test_interrupt_after_network_acquisition_is_recovered_and_cleaned(self):
        result, state = self.run_fixture("signal_network_int")
        self.assertEqual(result.returncode, 130, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)

    def test_interrupt_after_volume_acquisition_is_recovered_and_cleaned(self):
        result, state = self.run_fixture("signal_volume_int")
        self.assertEqual(result.returncode, 130, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)

    def test_interrupt_after_server_acquisition_is_recovered_and_cleaned(self):
        result, state = self.run_fixture("signal_server_int")
        self.assertEqual(result.returncode, 130, result.stdout + result.stderr)
        self.assertFalse(state["resources"], state)


if __name__ == "__main__":
    if sys.argv[1:2] == ["--mock-docker"]:
        sys.exit(mock_docker(sys.argv[2:]))
    unittest.main()
