"""python -m tools.engineering_orchestrator — network-free entrypoint."""

from __future__ import annotations

from .cli import main

if __name__ == "__main__":
    raise SystemExit(main())
