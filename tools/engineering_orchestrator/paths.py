"""Path security and lane ownership for Engineering Orchestrator Phase 1.

Reject absolute paths, .., NUL, backslash, and ambiguous traversal.
Lane path-prefix overlap → reject / block.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any


class PathSecurityError(ValueError):
    """Raised when a path fails security checks."""


class LaneOwnershipError(ValueError):
    """Raised when lane claims overlap."""


def validate_relative_path(path: str) -> str:
    """Validate and normalize a relative POSIX path. Returns normalized form."""
    if path is None:
        raise PathSecurityError("path is None")
    if not isinstance(path, str):
        raise PathSecurityError("path must be a string")
    if path == "":
        raise PathSecurityError("empty path rejected")
    if "\x00" in path:
        raise PathSecurityError("NUL byte in path rejected")
    if "\\" in path:
        raise PathSecurityError("backslash in path rejected")
    if path.startswith("/"):
        raise PathSecurityError("absolute path rejected")
    # Reject Windows drive / UNC style even without backslash
    if len(path) >= 2 and path[1] == ":":
        raise PathSecurityError("drive-letter path rejected")
    if path.startswith("~"):
        raise PathSecurityError("home-expansion path rejected")

    parts = path.split("/")
    normalized: list[str] = []
    for part in parts:
        if part == "" or part == ".":
            # leading/trailing slash or '.' → ambiguous / empty segment
            if part == "" and not normalized and path.startswith("/"):
                raise PathSecurityError("absolute path rejected")
            # skip '.' but reject empty segments that create ambiguity
            if part == "":
                # allow a single trailing empty from "foo/" by stripping later
                continue
            continue
        if part == "..":
            raise PathSecurityError("parent traversal '..' rejected")
        if part in {".", ".."}:
            raise PathSecurityError("ambiguous traversal rejected")
        normalized.append(part)

    if not normalized:
        raise PathSecurityError("path normalizes to empty")

    result = "/".join(normalized)
    if result.startswith("/") or ".." in result.split("/"):
        raise PathSecurityError("ambiguous traversal rejected")
    return result


def paths_overlap(a: str, b: str) -> bool:
    """True if relative prefixes intersect (equal or one is ancestor of other)."""
    na = validate_relative_path(a)
    nb = validate_relative_path(b)
    if na == nb:
        return True
    a_prefix = na + "/"
    b_prefix = nb + "/"
    return na.startswith(b_prefix) or nb.startswith(a_prefix)


@dataclass
class Lane:
    lane_id: str
    task_id: str
    path_prefixes: list[str] = field(default_factory=list)
    active: bool = True

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Lane":
        return cls(
            lane_id=str(data["lane_id"]),
            task_id=str(data["task_id"]),
            path_prefixes=[validate_relative_path(p) for p in (data.get("path_prefixes") or [])],
            active=bool(data.get("active", True)),
        )


class LaneRegistry:
    """In-memory / JSON-persisted lane ownership registry."""

    def __init__(self) -> None:
        self._lanes: dict[str, Lane] = {}

    def claim(self, lane: Lane) -> None:
        prefixes = [validate_relative_path(p) for p in lane.path_prefixes]
        lane.path_prefixes = prefixes
        for existing in self._lanes.values():
            if not existing.active:
                continue
            if existing.lane_id == lane.lane_id:
                continue
            for p1 in existing.path_prefixes:
                for p2 in prefixes:
                    if paths_overlap(p1, p2):
                        raise LaneOwnershipError(
                            f"lane overlap: {lane.lane_id} vs {existing.lane_id} "
                            f"({p2} overlaps {p1})"
                        )
        self._lanes[lane.lane_id] = lane

    def release(self, lane_id: str) -> None:
        if lane_id in self._lanes:
            self._lanes[lane_id].active = False

    def to_list(self) -> list[dict[str, Any]]:
        return [lane.to_dict() for lane in self._lanes.values()]

    def load_list(self, items: list[dict[str, Any]]) -> None:
        self._lanes.clear()
        for item in items:
            lane = Lane.from_dict(item)
            self._lanes[lane.lane_id] = lane
