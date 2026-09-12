"""Guard the Day-02 PostgreSQL runner against host-owned Maven output."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / "tests" / "run-core-day02-tests.sh"
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "platform-ci.yml"


def _executable_lines(text: str) -> str:
    lines = []
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        lines.append(line)
    return "\n".join(lines)


class Day02RunnerIsolationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.raw = SCRIPT.read_text(encoding="utf-8")
        self.body = _executable_lines(self.raw)

    def test_core_source_is_readonly_and_copied(self) -> None:
        self.assertIn('-v "${ROOT_DIR}/core:/src:ro"', self.body)
        self.assertRegex(self.body, r"cp -r /src /build")
        self.assertIn("rm -rf /build/target", self.body)
        self.assertIn("cd /build", self.body)
        self.assertIn("mvn -B -Dgroups=postgres -DfailIfNoSpecifiedTests=true test", self.body)
        copy_at = self.body.index("cp -r /src /build")
        drop_at = self.body.index("rm -rf /build/target")
        mvn_at = self.body.index("mvn -B -Dgroups=postgres")
        self.assertLess(copy_at, drop_at)
        self.assertLess(drop_at, mvn_at)

    def test_rejects_host_writable_core_bind(self) -> None:
        self.assertNotIn('-v "${ROOT_DIR}/core:/app"', self.body)
        self.assertIsNone(re.search(r"core:/[^:\s\"]+(?!.*:ro)", self.body))
        for line in self.body.splitlines():
            if "ROOT_DIR}/core:" in line or "${ROOT_DIR}/core:" in line:
                self.assertIn(":ro", line, line)

    def test_does_not_bind_host_home_m2(self) -> None:
        self.assertNotIn('-v "${HOME}/.m2:/root/.m2"', self.body)
        self.assertIn("platform-day02-maven-repo:/root/.m2", self.body)
        self.assertNotIn("platform_ci_maven_repo", self.body)

    def test_preserves_docker_sock_and_host_network(self) -> None:
        self.assertIn("-v /var/run/docker.sock:/var/run/docker.sock", self.body)
        self.assertNotIn("docker.sock:/var/run/docker.sock:ro", self.body)
        self.assertIn("--network host", self.body)
        self.assertIn("maven:3.9.9-eclipse-temurin-21-alpine", self.body)

    def test_workflow_keeps_checkout_clean_and_only_drops_core_target(self) -> None:
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertNotRegex(text, r"clean:\s*false")
        self.assertNotIn("docker system prune", text)
        self.assertNotIn("docker volume prune", text)
        self.assertNotIn("make dev-reset", text)
        self.assertIn("rm -rf /core/target", text)
        self.assertIn("-v \"${core_dir}:/core\"", text)
        self.assertIn("--network none", text)
        self.assertNotIn("busybox:", text)
        self.assertGreaterEqual(text.count("Remove leftover root-owned Maven target"), 4)
        leftover_at = text.index("Remove leftover root-owned Maven target")
        checkout_at = text.index("name: Checkout")
        self.assertLess(leftover_at, checkout_at)
        self.assertNotIn("chmod -R 777", text)
        self.assertNotIn("sudo rm -rf ${GITHUB_WORKSPACE}", text)
        self.assertNotIn("-v \"${GITHUB_WORKSPACE}:/workspace\"", text)


if __name__ == "__main__":
    unittest.main()
