"""Keep packaging systemd units identical to the Agent-owned source."""

from __future__ import annotations

import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class AgentSystemdPublishTests(unittest.TestCase):
    def test_deploy_systemd_matches_agent_source(self) -> None:
        pairs = (
            ("agent/linux/systemd/ebpf/20-ebpf.conf", "deploy/systemd/ebpf/20-ebpf.conf"),
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

    def test_ebpf_profile_uses_only_narrow_ambient_capabilities(self) -> None:
        base = (REPO_ROOT / "agent/linux/systemd/platform-agent.service").read_text()
        profile = (REPO_ROOT / "agent/linux/systemd/ebpf/20-ebpf.conf").read_text()
        self.assertIn("User=platform-agent", base)
        self.assertIn("NoNewPrivileges=true", base)
        self.assertNotIn("CAP_BPF", base)  # Older base-Agent systems need no eBPF capability parser.
        directives = dict(line.split("=", 1) for line in profile.splitlines() if "=" in line)
        self.assertEqual(directives, {
            "CapabilityBoundingSet": "CAP_BPF CAP_PERFMON",
            "AmbientCapabilities": "CAP_BPF CAP_PERFMON",
        })


if __name__ == "__main__":
    unittest.main()
