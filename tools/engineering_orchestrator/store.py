"""Single-bundle JSON persistence for Engineering Orchestrator Phase 1.

Canonical store: bundle.json with schema_version, generation, task, journal,
evidence, lanes, migrations, handoffs.

Mutation model: load → deep-copy candidate → validate → mutate → generation+1
→ temp file + os.replace → only then publish in memory.

Phase 1 assumes ONE writer process. Cross-process locking is Phase 2.
No compatibility migration for the prior six-file draft store.
"""

from __future__ import annotations

import copy
import json
import os
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Optional

from .handoff import Handoff
from .journal import Journal
from .migration_model import MigrationReservationBook
from .models import BUNDLE_SCHEMA_VERSION, Evidence, Task
from .paths import LaneRegistry, validate_relative_path
from .validation import ValidationError, validate_bundle


class StoreError(ValueError):
    """Raised for persistence errors."""


BUNDLE_FILENAME = "bundle.json"


@dataclass
class BundleState:
    schema_version: int
    generation: int
    task: Optional[Task]
    journal: Journal
    evidence: list[Evidence]
    lanes: LaneRegistry
    migrations: MigrationReservationBook
    handoffs: list[Handoff]

    def to_dict(self) -> dict[str, Any]:
        return {
            "schema_version": self.schema_version,
            "generation": self.generation,
            "task": self.task.to_dict() if self.task is not None else None,
            "journal": self.journal.to_list(),
            "evidence": [ev.to_dict() for ev in self.evidence],
            "lanes": self.lanes.to_list(),
            "migrations": self.migrations.to_list(),
            "handoffs": [h.to_dict() for h in self.handoffs],
        }

    def deep_copy(self) -> "BundleState":
        raw = copy.deepcopy(self.to_dict())
        return BundleState.from_dict(raw, validate=False)

    @classmethod
    def empty(cls) -> "BundleState":
        return cls(
            schema_version=BUNDLE_SCHEMA_VERSION,
            generation=0,
            task=None,
            journal=Journal(),
            evidence=[],
            lanes=LaneRegistry(),
            migrations=MigrationReservationBook(),
            handoffs=[],
        )

    @classmethod
    def from_dict(cls, data: dict[str, Any], *, validate: bool = True) -> "BundleState":
        if validate:
            validate_bundle(data)
        journal = Journal()
        journal.load_list(list(data.get("journal") or []))
        lanes = LaneRegistry()
        lanes.load_list(list(data.get("lanes") or []))
        migrations = MigrationReservationBook()
        migrations.load_list(list(data.get("migrations") or []))
        task_data = data.get("task")
        task = Task.from_dict(task_data) if task_data is not None else None
        evidence = [Evidence.from_dict(item) for item in (data.get("evidence") or [])]
        handoffs = [Handoff.from_dict(item) for item in (data.get("handoffs") or [])]
        return cls(
            schema_version=int(data.get("schema_version", BUNDLE_SCHEMA_VERSION)),
            generation=int(data.get("generation", 0)),
            task=task,
            journal=journal,
            evidence=evidence,
            lanes=lanes,
            migrations=migrations,
            handoffs=handoffs,
        )


class OrchestratorStore:
    """Directory-backed single-file bundle store."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)
        self.root.mkdir(parents=True, exist_ok=True)
        self._check_no_legacy_store()

    def _bundle_path(self) -> Path:
        return self.root / BUNDLE_FILENAME

    def _check_no_legacy_store(self) -> None:
        legacy = [
            "task.json",
            "journal.json",
            "evidence.json",
            "lanes.json",
            "migrations.json",
            "handoffs.json",
        ]
        present = [name for name in legacy if (self.root / name).exists()]
        if present and not self._bundle_path().exists():
            raise StoreError(
                "legacy six-file store detected without bundle.json; "
                "Phase 1 P0 requires recreation under single bundle.json "
                f"(found: {present})"
            )

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

    def _read_json(self, path: Path) -> Any:
        if not path.exists():
            return None
        with path.open("r", encoding="utf-8") as fh:
            return json.load(fh)

    def load_bundle(self) -> BundleState:
        path = self._bundle_path()
        if not path.exists():
            return BundleState.empty()
        data = self._read_json(path)
        if not isinstance(data, dict):
            raise StoreError("bundle.json must be a JSON object")
        try:
            return BundleState.from_dict(data, validate=True)
        except (ValidationError, ValueError) as exc:
            raise StoreError(f"bundle validation failed: {exc}") from exc

    def commit_bundle(
        self,
        candidate: BundleState,
        *,
        expected_generation: int,
    ) -> BundleState:
        """Atomically publish candidate if expected_generation matches on-disk.

        On success returns the published state (generation = expected + 1).
        Failed atomic replace leaves the previous bundle intact.
        """
        current = self.load_bundle()
        if current.generation != expected_generation:
            raise StoreError(
                f"stale generation: expected {expected_generation}, "
                f"on-disk {current.generation}"
            )
        candidate.schema_version = BUNDLE_SCHEMA_VERSION
        candidate.generation = expected_generation + 1
        payload = candidate.to_dict()
        try:
            validate_bundle(payload)
        except ValidationError as exc:
            raise StoreError(f"candidate bundle invalid: {exc}") from exc
        self._atomic_write_json(self._bundle_path(), payload)
        return candidate

    def validate_user_path_ref(self, path: str) -> str:
        """User-supplied payload_ref / instructions_ref must be relative-safe."""
        return validate_relative_path(path)
