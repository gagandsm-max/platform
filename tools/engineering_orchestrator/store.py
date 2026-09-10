"""JSON file persistence for Engineering Orchestrator Phase 1.

Persist JSON only. No pickle/unsafe deserialization. Task/log/evidence text
is treated as untrusted data (never eval/exec).
"""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from typing import Any

from .handoff import Handoff
from .journal import Journal
from .migration_model import MigrationReservationBook
from .models import Evidence, Task
from .paths import LaneRegistry, validate_relative_path


class StoreError(ValueError):
    """Raised for persistence errors."""


class OrchestratorStore:
    """Directory-backed JSON store: task, journal, evidence, lanes, migrations, handoffs."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)
        self.root.mkdir(parents=True, exist_ok=True)

    def _path(self, name: str) -> Path:
        # Store filenames are fixed constants — not user path input.
        if "/" in name or "\\" in name or ".." in name:
            raise StoreError("invalid store object name")
        return self.root / name

    def _atomic_write_json(self, path: Path, payload: Any) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp_name = tempfile.mkstemp(
            prefix=path.name + ".",
            suffix=".tmp",
            dir=str(path.parent),
        )
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                json.dump(payload, fh, indent=2, sort_keys=True, ensure_ascii=False)
                fh.write("\n")
            os.replace(tmp_name, path)
        finally:
            if os.path.exists(tmp_name):
                try:
                    os.remove(tmp_name)
                except OSError:
                    pass

    def _read_json(self, path: Path, default: Any) -> Any:
        if not path.exists():
            return default
        # json.load only — no pickle / yaml / eval.
        with path.open("r", encoding="utf-8") as fh:
            return json.load(fh)

    def save_task(self, task: Task) -> None:
        self._atomic_write_json(self._path("task.json"), task.to_dict())

    def load_task(self) -> Task | None:
        data = self._read_json(self._path("task.json"), None)
        if data is None:
            return None
        if not isinstance(data, dict):
            raise StoreError("task.json must be a JSON object")
        return Task.from_dict(data)

    def save_journal(self, journal: Journal) -> None:
        self._atomic_write_json(self._path("journal.json"), journal.to_list())

    def load_journal(self) -> Journal:
        data = self._read_json(self._path("journal.json"), [])
        if not isinstance(data, list):
            raise StoreError("journal.json must be a JSON array")
        journal = Journal()
        journal.load_list(data)
        return journal

    def save_evidence(self, evidence: list[Evidence]) -> None:
        self._atomic_write_json(
            self._path("evidence.json"),
            [ev.to_dict() for ev in evidence],
        )

    def load_evidence(self) -> list[Evidence]:
        data = self._read_json(self._path("evidence.json"), [])
        if not isinstance(data, list):
            raise StoreError("evidence.json must be a JSON array")
        return [Evidence.from_dict(item) for item in data]

    def save_lanes(self, lanes: LaneRegistry) -> None:
        self._atomic_write_json(self._path("lanes.json"), lanes.to_list())

    def load_lanes(self) -> LaneRegistry:
        data = self._read_json(self._path("lanes.json"), [])
        if not isinstance(data, list):
            raise StoreError("lanes.json must be a JSON array")
        registry = LaneRegistry()
        registry.load_list(data)
        return registry

    def save_migrations(self, book: MigrationReservationBook) -> None:
        self._atomic_write_json(self._path("migrations.json"), book.to_list())

    def load_migrations(self) -> MigrationReservationBook:
        data = self._read_json(self._path("migrations.json"), [])
        if not isinstance(data, list):
            raise StoreError("migrations.json must be a JSON array")
        book = MigrationReservationBook()
        book.load_list(data)
        return book

    def save_handoffs(self, handoffs: list[Handoff]) -> None:
        self._atomic_write_json(
            self._path("handoffs.json"),
            [h.to_dict() for h in handoffs],
        )

    def load_handoffs(self) -> list[Handoff]:
        data = self._read_json(self._path("handoffs.json"), [])
        if not isinstance(data, list):
            raise StoreError("handoffs.json must be a JSON array")
        return [Handoff.from_dict(item) for item in data]

    def validate_user_path_ref(self, path: str) -> str:
        """User-supplied payload_ref / instructions_ref must be relative-safe."""
        return validate_relative_path(path)
