"""Exercise the real Test-15 shell branch without starting Docker or a stack."""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "run-all-tests.sh"
PROTECTION = "--- PASS: TestProtectionAndRecoveryAfterFreeSpace (0.00s)\n"
FIXED = "--- PASS: TestFixedWatermarkCannotBeChanged (0.00s)\n"


class WatermarkGuardTests(unittest.TestCase):
    def run_guard(self, output: str, status: int = 0, error: str = ""):
        raw = SCRIPT.read_text(encoding="utf-8")
        block = "TEST_NUM=15\n" + raw.split("TEST_NUM=15\n", 1)[1].split(
            "# Test 16:", 1
        )[0]
        shell = r'''
docker() {
    printf '%s\0' "$@" > "$FAKE_ARGS"
    printf '%s' "$FAKE_OUTPUT"
    printf '%s' "$FAKE_ERROR" >&2
    return "$FAKE_STATUS"
}
source "$WATERMARK_SCRIPT"
ROOT_DIR="$FAKE_ROOT"
pass() { printf 'GUARD_ACCEPTED\n'; }
fail() { printf 'GUARD_REJECTED\n'; return 1; }
'''
        with tempfile.TemporaryDirectory() as tmp:
            args_path = Path(tmp) / "docker-args"
            env = dict(os.environ, WATERMARK_SCRIPT=str(SCRIPT), FAKE_OUTPUT=output,
                       FAKE_ERROR=error, FAKE_STATUS=str(status),
                       FAKE_ROOT="/fixture/repository with spaces", FAKE_ARGS=str(args_path))
            result = subprocess.run(
                ["bash", "-eu", "-c", shell + block], env=env,
                capture_output=True, text=True, check=False, timeout=10,
            )
            args = args_path.read_bytes().decode().rstrip("\0").split("\0")
        return result, args

    def assert_rejected(self, output: str, status: int = 0, error: str = ""):
        result, _ = self.run_guard(output, status, error)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("GUARD_REJECTED", result.stdout)
        self.assertNotIn("GUARD_ACCEPTED", result.stdout)
        return result

    def test_both_required_cases_pass(self):
        result, args = self.run_guard(PROTECTION + "PASS\nok example/spool 0.01s\n" + FIXED)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("GUARD_ACCEPTED", result.stdout)
        self.assertIn("/fixture/repository with spaces/sensor:/app:ro", args)

    def test_nonzero_exit_rejects_even_after_both_cases_pass(self):
        self.assert_rejected(PROTECTION + FIXED + "FAIL example/other\n", 1)

    def test_docker_failure_rejects_pass_output_and_keeps_diagnostics(self):
        result = self.assert_rejected(PROTECTION + FIXED, 125, "daemon unavailable\n")
        self.assertIn("daemon unavailable", result.stdout + result.stderr)

    def test_zero_matched_tests_rejects_package_pass(self):
        self.assert_rejected("testing: warning: no tests to run\nPASS\nok example 0.01s\n")

    def test_each_required_case_must_pass(self):
        for output in (PROTECTION, FIXED):
            with self.subTest(output=output):
                self.assert_rejected(output + "PASS\n")

    def test_skip_cannot_replace_required_pass(self):
        self.assert_rejected(PROTECTION + "--- SKIP: TestFixedWatermarkCannotBeChanged (0.00s)\nPASS\n")

    def test_similarly_named_tests_cannot_satisfy_guard(self):
        for output in (
            PROTECTION.replace("FreeSpace (", "FreeSpaceExtra (") + FIXED,
            PROTECTION + FIXED.replace("Changed (", "ChangedExtra ("),
        ):
            with self.subTest(output=output):
                self.assert_rejected(output)

    def test_incidental_pass_text_and_empty_output_reject(self):
        for output in ("PASSWORD=fixture\n", "", "PASS\n"):
            with self.subTest(output=output):
                self.assert_rejected(output)


if __name__ == "__main__":
    unittest.main()
