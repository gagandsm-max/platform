#!/usr/bin/env python3
"""Fail closed on reference-brand leakage in first-party shipped source/artifacts.

This is a narrow cleanliness gate, not an intellectual-property scanner. Internal
architecture and provenance documentation is intentionally outside the default
source scope. Release jobs may pass additional built artifacts with --artifact.
"""

from __future__ import annotations

import argparse
import re
import stat
import sys
import tarfile
import zipfile
from dataclasses import dataclass
from pathlib import Path


SOURCE_ROOTS = (
    "ui/src",
    "ui/public",
    "ui/index.html",
    "ui/nginx.conf",
    "ui/Dockerfile",
    "core/src/main",
    "core/Dockerfile",
    "sensor/cmd",
    "sensor/internal",
    "sensor/Dockerfile",
    "agent",
    "deploy",
    "analytics",
    "schemas",
    "device-packs",
    "packaging",
)
REQUIRED_SOURCE_DIRS = ("ui/src", "core/src/main", "sensor/internal", "agent", "schemas", "device-packs")
REQUIRED_SOURCE_FILES = ("ui/index.html",)
SOURCE_SKIP_DIRS = frozenset({".git", ".test-dist", "node_modules", "target", "dist", "__pycache__", "third_party"})
MAX_MEMBER_BYTES = 32 * 1024 * 1024
MAX_ARTIFACT_BYTES = 256 * 1024 * 1024
UNSUPPORTED_PACKAGES = (".deb", ".rpm", ".7z", ".cab", ".msi", ".xz", ".gz")
SUPPORTED_ARCHIVES = (".zip", ".jar", ".tar", ".tar.gz", ".tar.bz2", ".tar.xz", ".tgz")
NESTED_ARCHIVES = SUPPORTED_ARCHIVES + (".deb", ".rpm", ".7z", ".cab", ".msi", ".whl", ".nupkg")

RULES = {
    "NETAI": re.compile(r"netai|(?<![a-z0-9])net[\s._-]+ai(?![a-z0-9])", re.IGNORECASE),
    "THOUGHTDATA": re.compile(r"thoughtdata|(?<![a-z0-9])thought[\s._-]+data(?![a-z0-9])", re.IGNORECASE),
    "ENTERPRISE360": re.compile(r"enterprise360|(?<![a-z0-9])enterprise[\s._-]+360(?![a-z0-9])", re.IGNORECASE),
}


@dataclass(frozen=True)
class Finding:
    path: str
    problem: str


def detect_brands(data: bytes) -> set[str]:
    brands: set[str] = set()

    def check(text: str) -> None:
        brands.update(name for name, pattern in RULES.items() if pattern.search(text))

    check(data.decode("utf-8", errors="ignore"))
    # Windows binary resources can contain UTF-16 at either byte alignment.
    if b"\x00" in data:
        for encoding in ("utf-16-le", "utf-16-be"):
            for offset in (0, 1):
                check(data[offset:].decode(encoding, errors="ignore"))
    return brands


def inspect_bytes(label: str, data: bytes) -> list[Finding]:
    if len(data) > MAX_MEMBER_BYTES:
        return [Finding(label, "UNSCANNABLE: content exceeds per-member size limit")]
    return [Finding(label, f"BRAND:{name}") for name in sorted(detect_brands(data))]


def inspect_regular_file(path: Path, label: str) -> list[Finding]:
    try:
        if path.is_symlink():
            return [Finding(label, "UNSCANNABLE: symbolic link")]
        if not path.is_file():
            return [Finding(label, "UNSCANNABLE: not a regular file")]
        if path.stat().st_size > MAX_MEMBER_BYTES:
            return [Finding(label, "UNSCANNABLE: file exceeds per-member size limit")]
        return inspect_bytes(label, path.read_bytes())
    except OSError as exc:
        return [Finding(label, f"UNSCANNABLE: {type(exc).__name__}")]


def inspect_archive(path: Path, label: str) -> list[Finding]:
    findings: list[Finding] = []
    consumed = 0
    if path.is_symlink():
        return [Finding(label, "UNSCANNABLE: symbolic link")]
    lower_name = path.name.lower()
    if lower_name.endswith(UNSUPPORTED_PACKAGES) and not lower_name.endswith(SUPPORTED_ARCHIVES):
        return [Finding(label, "UNSCANNABLE: package format needs an unpacked release payload")]
    try:
        if zipfile.is_zipfile(path):
            with zipfile.ZipFile(path) as archive:
                for info in archive.infolist():
                    member_label = f"{label}!{info.filename}"
                    findings.extend(inspect_bytes(member_label, info.filename.encode("utf-8")))
                    if info.is_dir():
                        continue
                    if stat.S_ISLNK(info.external_attr >> 16):
                        findings.append(Finding(member_label, "UNSCANNABLE: symbolic-link ZIP member"))
                        continue
                    if info.filename.lower().endswith(NESTED_ARCHIVES):
                        findings.append(Finding(member_label, "UNSCANNABLE: nested package/archive"))
                    consumed += info.file_size
                    if info.file_size > MAX_MEMBER_BYTES or consumed > MAX_ARTIFACT_BYTES:
                        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
                        continue
                    with archive.open(info) as member:
                        findings.extend(inspect_bytes(member_label, member.read(MAX_MEMBER_BYTES + 1)))
            return findings
        if tarfile.is_tarfile(path):
            with tarfile.open(path, mode="r:*") as archive:
                for info in archive:
                    member_label = f"{label}!{info.name}"
                    findings.extend(inspect_bytes(member_label, info.name.encode("utf-8")))
                    if info.isdir():
                        continue
                    if info.name.lower().endswith(NESTED_ARCHIVES):
                        findings.append(Finding(member_label, "UNSCANNABLE: nested package/archive"))
                    if not info.isfile():
                        findings.append(Finding(member_label, "UNSCANNABLE: nonregular archive member"))
                        continue
                    consumed += info.size
                    if info.size > MAX_MEMBER_BYTES or consumed > MAX_ARTIFACT_BYTES:
                        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
                        continue
                    member = archive.extractfile(info)
                    if member is None:
                        findings.append(Finding(member_label, "UNSCANNABLE: unreadable tar member"))
                        continue
                    with member:
                        findings.extend(inspect_bytes(member_label, member.read(MAX_MEMBER_BYTES + 1)))
            return findings
    except (OSError, EOFError, RuntimeError, ValueError, zipfile.BadZipFile, tarfile.TarError):
        return [Finding(label, "UNSCANNABLE: corrupt or unreadable archive")]
    if lower_name.endswith(SUPPORTED_ARCHIVES):
        return [Finding(label, "UNSCANNABLE: unrecognized or corrupt archive")]
    # Non-archive artifacts (binaries, manifests, etc.) still get raw inspection.
    return inspect_regular_file(path, label)


def walk_files(root: Path, *, source: bool) -> list[Path]:
    if root.is_symlink() or root.is_file():
        return [root]
    if not root.is_dir():
        return []
    files = []
    for path in sorted(root.rglob("*")):
        if source and any(part in SOURCE_SKIP_DIRS for part in path.relative_to(root).parts):
            continue
        if path.is_file() or path.is_symlink():
            files.append(path)
    return files


def scan(repo: Path, paths: list[Path], artifacts: list[Path]) -> tuple[list[Finding], int]:
    findings: list[Finding] = []
    visited = 0
    for root in paths:
        if not root.exists() and not root.is_symlink():
            findings.append(Finding(str(root), "UNSCANNABLE: source root missing"))
            continue
        for path in walk_files(root, source=True):
            visited += 1
            label = path.relative_to(repo).as_posix() if path.is_relative_to(repo) else str(path)
            findings.extend(inspect_bytes(label, label.encode("utf-8")))
            findings.extend(inspect_regular_file(path, label))
    for root in artifacts:
        if not root.exists() and not root.is_symlink():
            findings.append(Finding(str(root), "UNSCANNABLE: artifact missing"))
            continue
        for path in walk_files(root, source=False):
            visited += 1
            label = path.relative_to(repo).as_posix() if path.is_relative_to(repo) else str(path)
            findings.extend(inspect_bytes(label, label.encode("utf-8")))
            findings.extend(inspect_archive(path, label))
    return findings, visited


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--path", action="append", default=[], help="Explicit relative source scope (overrides defaults)")
    parser.add_argument("--artifact", action="append", default=[], help="Built file, directory, ZIP or TAR to inspect")
    args = parser.parse_args(argv)
    repo = args.repo.resolve()
    if not args.path:
        missing = [name for name in REQUIRED_SOURCE_DIRS if not (repo / name).is_dir()]
        missing.extend(name for name in REQUIRED_SOURCE_FILES if not (repo / name).is_file())
        if missing:
            print(f"[ERROR] Required product source roots missing: {', '.join(missing)}", file=sys.stderr)
            return 2
    paths = [repo / name for name in (args.path or SOURCE_ROOTS) if (repo / name).exists()]
    if args.path and len(paths) != len(args.path):
        print("[ERROR] An explicitly requested source path does not exist", file=sys.stderr)
        return 2
    if not paths:
        print("[ERROR] No supported first-party product sources found", file=sys.stderr)
        return 2
    artifacts = [repo / name for name in args.artifact]
    findings, visited = scan(repo, paths, artifacts)
    for finding in findings:
        print(f"[ERROR] {finding.problem} at {finding.path!r}", file=sys.stderr)
    if findings:
        print(f"BRAND HYGIENE FAIL: {len(findings)} issue(s), {visited} file(s) scanned", file=sys.stderr)
        return 1
    print(f"BRAND HYGIENE PASS: {visited} file(s) scanned; no named reference brands")
    return 0


if __name__ == "__main__":
    sys.exit(main())
