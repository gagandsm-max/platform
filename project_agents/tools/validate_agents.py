#!/usr/bin/env python3
"""Validate the Platform agent pack, skill mirrors, and Control Plane v2 enums."""

from __future__ import annotations

import sys
from pathlib import Path

from agent_pack import validate_agent_pack


def main() -> int:
    repo = Path(__file__).resolve().parents[2]
    errors = validate_agent_pack(repo)
    if errors:
        print("FAIL")
        for error in errors:
            print("-", error)
        return 1

    pa = repo / "project_agents"
    import json

    manifest = json.loads((pa / "manifest.json").read_text(encoding="utf-8"))
    skill_manifest = json.loads((pa / "skills" / "manifest.json").read_text(encoding="utf-8"))
    print(
        f"PASS: {len(manifest)} manifest-declared Platform agents and "
        f"{len(skill_manifest)} manifest-declared skills exactly synchronized."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
