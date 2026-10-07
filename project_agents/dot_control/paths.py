"""Control-root layout and permission hardening."""

from __future__ import annotations

import os
import stat
from pathlib import Path
from typing import Optional, Sequence

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


def _mode_has_group_or_other(mode: int) -> bool:
    return bool(stat.S_IMODE(mode) & 0o077)


def assert_dir_mode_owner(path: Path, label: str, *, expected_uid: Optional[int] = None) -> None:
    """Reject group/other permissions and wrong ownership. Never repairs."""
    assert_not_symlink(path, label)
    if not path.is_dir():
        raise BridgeError(f"{label} is not a directory: {path}")
    st = path.stat()
    mode = stat.S_IMODE(st.st_mode)
    if _mode_has_group_or_other(mode):
        raise BridgeError(
            f"{label} exposes group/other permissions (mode={oct(mode)}); "
            f"required {oct(DIR_MODE)} with no group/other bits"
        )
    uid = os.geteuid() if expected_uid is None else expected_uid
    if hasattr(st, "st_uid") and st.st_uid != uid:
        raise BridgeError(
            f"{label} owner uid {st.st_uid} != bridge uid {uid}"
        )


def assert_durable_file_mode(path: Path, label: str, *, expected_uid: Optional[int] = None) -> None:
    """Require regular non-symlink file with mode 0600 and matching owner."""
    if path.is_symlink() or os.path.islink(path):
        raise BridgeError(f"{label} must not be a symlink: {path}")
    if not path.is_file():
        raise BridgeError(f"{label} is not a regular file: {path}")
    st = path.stat()
    if not stat.S_ISREG(st.st_mode):
        raise BridgeError(f"{label} is not a regular file: {path}")
    mode = stat.S_IMODE(st.st_mode)
    if mode != FILE_MODE:
        raise BridgeError(
            f"{label} mode {oct(mode)} != required {oct(FILE_MODE)}"
        )
    uid = os.geteuid() if expected_uid is None else expected_uid
    if hasattr(st, "st_uid") and st.st_uid != uid:
        raise BridgeError(
            f"{label} owner uid {st.st_uid} != bridge uid {uid}"
        )


def validate_control_root_layout(
    control_root: Path,
    *,
    require_subdirs: bool = False,
    check_modes: bool = True,
    expected_uid: Optional[int] = None,
) -> None:
    """Validate control root is not a symlink and subdirs are real directories under root.

    When ``check_modes`` is True (normal operation), reject unsafe modes/ownership
    without repairing. Explicit ``init_control_root`` is the only repair path.
    """
    if not control_root.exists() and not require_subdirs:
        return
    assert_not_symlink(control_root, "control root")
    if control_root.exists() and not control_root.is_dir():
        raise BridgeError(f"control root is not a directory: {control_root}")
    root_r = control_root.resolve(strict=False)
    if check_modes and control_root.exists():
        assert_dir_mode_owner(control_root, "control root", expected_uid=expected_uid)
    for name in CONTROL_SUBDIRS:
        sub = control_root / name
        if sub.exists() or os.path.lexists(sub):
            assert_not_symlink(sub, f"control subdirectory {name}")
            if not sub.is_dir():
                raise BridgeError(f"control subdirectory {name} is not a directory: {sub}")
            assert_under_control_root(sub, root_r, f"control subdirectory {name}")
            if check_modes:
                assert_dir_mode_owner(
                    sub, f"control subdirectory {name}", expected_uid=expected_uid
                )
        elif require_subdirs:
            raise BridgeError(f"missing required control subdirectory: {name}")


def chmod_dir(path: Path, mode: int = DIR_MODE) -> None:
    os.chmod(path, mode)


def chmod_file(path: Path, mode: int = FILE_MODE) -> None:
    os.chmod(path, mode)


def init_control_root(control_root: Path) -> list[str]:
    """Explicit init: create/repair required subdirectories with 0700; reject symlinks."""
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
        mode = stat.S_IMODE(path.stat().st_mode)
        if mode & 0o077:
            chmod_dir(path, DIR_MODE)
        created.append(str(path))

    # After explicit init, modes must validate cleanly.
    validate_control_root_layout(control_root, require_subdirs=True, check_modes=True)
    return created


def ensure_subdir(control_root: Path, name: str) -> Path:
    """Return an existing approved subdirectory. Does not repair modes."""
    if name not in CONTROL_SUBDIRS:
        raise BridgeError(f"unknown control subdirectory: {name}")
    validate_control_root_layout(control_root, require_subdirs=True, check_modes=True)
    path = control_root / name
    if os.path.lexists(path) and os.path.islink(path):
        raise BridgeError(f"control subdirectory must not be a symlink: {path}")
    if not path.exists():
        raise BridgeError(
            f"missing control subdirectory {name}; run explicit init first"
        )
    assert_real_directory(path, f"control subdirectory {name}")
    assert_under_control_root(path, control_root, f"control subdirectory {name}")
    assert_dir_mode_owner(path, f"control subdirectory {name}")
    return path
