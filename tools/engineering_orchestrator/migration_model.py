"""Migration reservation MODEL only — no real migration files or apply.

Phase 1: in-memory/JSON reservation state. Collision → MIGRATION_COLLISION.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any, Optional

from .models import MigrationReservationState


class MigrationCollisionError(ValueError):
    """Raised when two tasks claim the same migration key."""


@dataclass
class MigrationReservation:
    reservation_id: str
    migration_id: str
    lane_id: str
    task_id: str
    state: MigrationReservationState
    head_sha: str = ""

    def to_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["state"] = self.state.value
        return d

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "MigrationReservation":
        return cls(
            reservation_id=str(data["reservation_id"]),
            migration_id=str(data["migration_id"]),
            lane_id=str(data.get("lane_id", "")),
            task_id=str(data["task_id"]),
            state=MigrationReservationState(data["state"]),
            head_sha=str(data.get("head_sha", "")),
        )


class MigrationReservationBook:
    """JSON-persistable reservation book (model only)."""

    def __init__(self) -> None:
        self._by_id: dict[str, MigrationReservation] = {}
        self._active_by_migration: dict[str, str] = {}

    def reserve(
        self,
        reservation_id: str,
        migration_id: str,
        lane_id: str,
        task_id: str,
        head_sha: str = "",
    ) -> MigrationReservation:
        existing_id = self._active_by_migration.get(migration_id)
        if existing_id is not None:
            existing = self._by_id[existing_id]
            if existing.task_id != task_id:
                collided = MigrationReservation(
                    reservation_id=reservation_id,
                    migration_id=migration_id,
                    lane_id=lane_id,
                    task_id=task_id,
                    state=MigrationReservationState.COLLIDED,
                    head_sha=head_sha,
                )
                self._by_id[reservation_id] = collided
                raise MigrationCollisionError(
                    f"migration {migration_id} already reserved by task {existing.task_id}"
                )
            return existing

        reservation = MigrationReservation(
            reservation_id=reservation_id,
            migration_id=migration_id,
            lane_id=lane_id,
            task_id=task_id,
            state=MigrationReservationState.RESERVED,
            head_sha=head_sha,
        )
        self._by_id[reservation_id] = reservation
        self._active_by_migration[migration_id] = reservation_id
        return reservation

    def release(self, reservation_id: str) -> Optional[MigrationReservation]:
        reservation = self._by_id.get(reservation_id)
        if reservation is None:
            return None
        reservation.state = MigrationReservationState.RELEASED
        active = self._active_by_migration.get(reservation.migration_id)
        if active == reservation_id:
            del self._active_by_migration[reservation.migration_id]
        return reservation

    def to_list(self) -> list[dict[str, Any]]:
        return [r.to_dict() for r in self._by_id.values()]

    def load_list(self, items: list[dict[str, Any]]) -> None:
        self._by_id.clear()
        self._active_by_migration.clear()
        for item in items:
            reservation = MigrationReservation.from_dict(item)
            self._by_id[reservation.reservation_id] = reservation
            if reservation.state == MigrationReservationState.RESERVED:
                self._active_by_migration[reservation.migration_id] = (
                    reservation.reservation_id
                )
