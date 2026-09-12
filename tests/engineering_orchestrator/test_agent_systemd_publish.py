"""Keep packaging systemd units identical to the Agent-owned source."""

from __future__ import annotations

import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class AgentSystemdPublishTests(unittest.TestCase):
    def test_deploy_systemd_matches_agent_source(self) -> None:
        pairs = (
            (
                "agent/linux/systemd/platform-agent.service",
                "deploy/systemd/platform-agent.service",
            ),
            (
                "agent/linux/systemd/platform-agent.env.example",
                "deploy/systemd/platform-agent.env.example",
            ),
        )
        for src_rel, dst_rel in pairs:
            src = (REPO_ROOT / src_rel).read_text(encoding="utf-8")
            dst = (REPO_ROOT / dst_rel).read_text(encoding="utf-8")
            self.assertEqual(src, dst, f"{dst_rel} drifted from {src_rel}")


if __name__ == "__main__":
    unittest.main()
