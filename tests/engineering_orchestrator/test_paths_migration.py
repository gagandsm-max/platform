"""Path security, lane ownership, and migration reservation tests."""

from __future__ import annotations

import tempfile
import unittest

from tools.engineering_orchestrator.engine import OrchestratorEngine
from tools.engineering_orchestrator.migration_model import (
    MigrationCollisionError,
    MigrationReservationBook,
)
from tools.engineering_orchestrator.models import Actor, FailureClass, State
from tools.engineering_orchestrator.paths import (
    Lane,
    LaneOwnershipError,
    LaneRegistry,
    PathSecurityError,
    paths_overlap,
    validate_relative_path,
)
from tools.engineering_orchestrator.store import OrchestratorStore


class PathLaneMigrationTests(unittest.TestCase):
    def test_allowed_relative_path(self) -> None:
        self.assertEqual(validate_relative_path("tools/engineering_orchestrator"), "tools/engineering_orchestrator")
        self.assertEqual(validate_relative_path("a/b/./c"), "a/b/c")

    def test_reject_absolute(self) -> None:
        with self.assertRaises(PathSecurityError):
            validate_relative_path("/etc/passwd")

    def test_reject_parent_traversal(self) -> None:
        with self.assertRaises(PathSecurityError):
            validate_relative_path("../secrets")
        with self.assertRaises(PathSecurityError):
            validate_relative_path("foo/../../bar")

    def test_reject_nul(self) -> None:
        with self.assertRaises(PathSecurityError):
            validate_relative_path("foo\x00bar")

    def test_reject_backslash(self) -> None:
        with self.assertRaises(PathSecurityError):
            validate_relative_path("foo\\bar")

    def test_lane_overlap_rejected(self) -> None:
        registry = LaneRegistry()
        registry.claim(
            Lane(
                lane_id="L1",
                task_id="T1",
                path_prefixes=["tools/engineering_orchestrator"],
            )
        )
        with self.assertRaises(LaneOwnershipError):
            registry.claim(
                Lane(
                    lane_id="L2",
                    task_id="T2",
                    path_prefixes=["tools/engineering_orchestrator/schemas"],
                )
            )

    def test_paths_overlap_helper(self) -> None:
        self.assertTrue(paths_overlap("a/b", "a/b/c"))
        self.assertFalse(paths_overlap("a/b", "a/c"))

    def test_engine_lane_overlap_blocks(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(
                task_id="T1",
                title="t",
                head_sha="h1",
                lane_id="L1",
                path_prefixes=["tools/foo"],
            )
            # Simulate second lane claim against same registry.
            with self.assertRaises(LaneOwnershipError):
                engine.lanes.claim(
                    Lane(
                        lane_id="L2",
                        task_id="T2",
                        path_prefixes=["tools/foo/bar"],
                    )
                )

    def test_migration_collision(self) -> None:
        book = MigrationReservationBook()
        book.reserve("r1", "mig-001", "L1", "T1", head_sha="h1")
        with self.assertRaises(MigrationCollisionError):
            book.reserve("r2", "mig-001", "L2", "T2", head_sha="h2")

    def test_engine_migration_collision_blocks_task(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            engine = OrchestratorEngine(OrchestratorStore(tmp))
            engine.init_task(task_id="T1", title="t", head_sha="h1", lane_id="L1")
            # Pre-seed a foreign reservation, then T1 claim collides.
            engine.migrations.reserve("rx", "mig-9", "Lx", "Tother", "h")
            with self.assertRaises(MigrationCollisionError):
                engine.reserve_migration(reservation_id="r2", migration_id="mig-9")
            self.assertEqual(engine.task.state, State.BLOCKED)
            self.assertEqual(engine.task.failure_class, FailureClass.MIGRATION_COLLISION)


if __name__ == "__main__":
    unittest.main()
