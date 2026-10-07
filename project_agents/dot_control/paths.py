"""Control-root layout and permission hardening."""

from __future__ import annotations

import os
import stat
from pathlib import Path
from typing import Sequence

from .models import CONTROL_SUBDIRS, DIR_MODE, FILE_MODE, BridgeError


def assert_not_symlink(path: Path, label: str) -> None:
    if path.is_symlink() or (path.exists() and path.is_symlink()):
        raise BridgeError(f"{label} must not be a symlink: {path}")
    # lexists-style: broken symlink still counts
    if os.path.lexists(path) and os.path.islink(path):
        raise BridgeError(f"{label} must not be a symlink: {path}")


def assert_real_directory(path: Path, label: str) -> Path:
    assert_not_symlink(path, label)
    if path.exists() and not path.is_dir():
        raise BridgeError(f"{label} is not a directory: {path}")
    return path


def assert_under_control_root(path: Path, control_root: Path, label: str) -> Path:
    root_r = control_root.resolve(strict=False)
    path_r = path.resolve(strict=False)
    try:
        path_r.relative_to(root_r)
    except ValueError as exc:
        raise BridgeError(f"{label} escapes control root {root_r}: {path}") from exc
    return path_r


def validate_control_root_layout(control_root: Path, *, require_subdirs: bool = False) -> None:
    """Validate control root is not a symlink and subdirs are real directories under root."""
    if not control_root.exists() and not require_subdirs:
        return
    assert_not_symlink(control_root, "control root")
    if control_root.exists() and not control_root.is_dir():
        raise BridgeError(f"control root is not a directory: {control_root}")
    root_r = control_root.resolve(strict=False)
    for name in CONTROL_SUBDIRS:
        sub = control_root / name
        if sub.exists() or os.path.lexists(sub):
            assert_not_symlink(sub, f"control subdirectory {name}")
            if not sub.is_dir():
                raise BridgeError(f"control subdirectory {name} is not a directory: {sub}")
            assert_under_control_root(sub, root_r, f"control subdirectory {name}")
        elif require_subdirs:
            raise BridgeError(f"missing required control subdirectory: {name}")


def chmod_dir(path: Path, mode: int = DIR_MODE) -> None:
    os.chmod(path, mode)


def chmod_file(path: Path, mode: int = FILE_MODE) -> None:
    os.chmod(path, mode)


def init_control_root(control_root: Path) -> list[str]:
    """Create required subdirectories with 0700; reject symlink roots/subdirs."""
    if os.path.lexists(control_root) and os.path.islink(control_root):
        raise BridgeError(f"control root must not be a symlink: {control_root}")
    control_root.mkdir(parents=True, exist_ok=True)
    if control_root.is_symlink():
        raise BridgeError(f"control root must not be a symlink: {control_root}")
    chmod_dir(control_root, DIR_MODE)

    created: list[str] = []
    for name in CONTROL_SUBDIRS:
        path = control_root / name
        if os.path.lexists(path) and os.path.islink(path):
            raise BridgeError(f"control subdirectory must not be a symlink: {path}")
        path.mkdir(parents=True, exist_ok=True)
        if path.is_symlink() or not path.is_dir():
            raise BridgeError(f"control subdirectory unsafe after create: {path}")
        assert_under_control_root(path, control_root, f"control subdirectory {name}")
        chmod_dir(path, DIR_MODE)
        # Reject world/group writable surprises when repairing under init.
        mode = stat.S_IMODE(path.stat().st_mode)
        if mode & 0o077:
            chmod_dir(path, DIR_MODE)
        created.append(str(path))

    validate_control_root_layout(control_root, require_subdirs=True)
    return created


def ensure_subdir(control_root: Path, name: str) -> Path:
    if name not in CONTROL_SUBDIRS:
        raise BridgeError(f"unknown control subdirectory: {name}")
    validate_control_root_layout(control_root, require_subdirs=False)
    path = control_root / name
    if os.path.lexists(path) and os.path.islink(path):
        raise BridgeError(f"control subdirectory must not be a symlink: {path}")
    if not path.exists():
        path.mkdir(parents=True, exist_ok=True)
        chmod_dir(path, DIR_MODE)
    assert_real_directory(path, f"control subdirectory {name}")
    assert_under_control_root(path, control_root, f"control subdirectory {name}")
    return path
