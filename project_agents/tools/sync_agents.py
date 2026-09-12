#!/usr/bin/env python3
"""Regenerate Cursor / Antigravity / Codex adapters and root skill mirrors."""

from __future__ import annotations

from pathlib import Path

from agent_pack import AgentPackError, sync_agent_pack


def main() -> int:
    repo = Path(__file__).resolve().parents[2]
    try:
        print(sync_agent_pack(repo))
    except AgentPackError as exc:
        raise SystemExit(str(exc)) from exc
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
