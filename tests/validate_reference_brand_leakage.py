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
    "scripts",
    "compose.yaml",
    ".env.example",
)
REQUIRED_SOURCE_DIRS = ("ui/src", "core/src/main", "sensor/internal", "agent", "schemas", "device-packs")
REQUIRED_SOURCE_FILES = ("ui/index.html", "compose.yaml")
SOURCE_SKIP_DIRS = frozenset({".git", ".test-dist", "node_modules", "target", "dist", "__pycache__", "third_party"})
MAX_MEMBER_BYTES = 32 * 1024 * 1024
MAX_ARTIFACT_BYTES = 256 * 1024 * 1024
MAX_ARCHIVE_MEMBERS = 4096
UNSUPPORTED_PACKAGES = (".deb", ".rpm", ".7z", ".cab", ".msi", ".xz", ".gz", ".bz2", ".zst", ".rar")
SUPPORTED_ARCHIVES = (".zip", ".jar", ".tar", ".tar.gz", ".tar.bz2", ".tar.xz", ".tgz")
NESTED_ARCHIVES = SUPPORTED_ARCHIVES + (
    ".deb",
    ".rpm",
    ".7z",
    ".cab",
    ".msi",
    ".whl",
    ".nupkg",
    ".gz",
    ".bz2",
    ".xz",
    ".zst",
    ".rar",
)
ALLOWED_ZIP_METHODS = frozenset({zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED})
# Known compressed/archive magic used to fail closed without recursive extraction.
COMPRESSED_SIGNATURES: tuple[tuple[bytes, str], ...] = (
    (b"PK\x03\x04", "zip"),
    (b"PK\x05\x06", "zip"),
    (b"PK\x07\x08", "zip"),
    (b"\x1f\x8b", "gzip"),
    (b"\xfd7zXZ\x00", "xz"),
    (b"BZh", "bzip2"),
    (b"7z\xbc\xaf\x27\x1c", "7z"),
    (b"Rar!\x1a\x07", "rar"),
    (b"MSCF", "cab"),
    (b"\x28\xb5\x2f\xfd", "zstd"),
    (b"!<arch>", "ar"),
    (b"\xed\xab\xee\xdb", "rpm"),
)

RULES = {
    "NETAI": re.compile(r"netai|(?<![a-z0-9])net[\s._-]+ai(?![a-z0-9])", re.IGNORECASE),
    "THOUGHTDATA": re.compile(r"thoughtdata|(?<![a-z0-9])thought[\s._-]+data(?![a-z0-9])", re.IGNORECASE),
    "ENTERPRISE360": re.compile(r"enterprise360|(?<![a-z0-9])enterprise[\s._-]+360(?![a-z0-9])", re.IGNORECASE),
}


@dataclass(frozen=True)
class Finding:
    path: str
    problem: str


def _texts_for_scan(data: bytes) -> list[str]:
    """Decode text while preserving token boundaries across malformed bytes."""
    texts: list[str] = []
    # Replacement characters become NUL so invalid sequences cannot glue tokens.
    texts.append(data.decode("utf-8", errors="replace").replace("\ufffd", "\x00"))
    if b"\x00" in data:
        for encoding in ("utf-16-le", "utf-16-be"):
            for offset in (0, 1):
                texts.append(data[offset:].decode(encoding, errors="replace").replace("\ufffd", "\x00"))
    return texts


def detect_brands(data: bytes) -> set[str]:
    brands: set[str] = set()
    for text in _texts_for_scan(data):
        brands.update(name for name, pattern in RULES.items() if pattern.search(text))
    return brands


def inspect_bytes(label: str, data: bytes) -> list[Finding]:
    if len(data) > MAX_MEMBER_BYTES:
        return [Finding(label, "UNSCANNABLE: content exceeds per-member size limit")]
    return [Finding(label, f"BRAND:{name}") for name in sorted(detect_brands(data))]


def compressed_format_name(data: bytes) -> str | None:
    if not data:
        return None
    for magic, name in COMPRESSED_SIGNATURES:
        if data.startswith(magic):
            return name
    return None


def nested_or_compressed_finding(label: str, name: str, data: bytes | None = None) -> Finding | None:
    lower = name.lower()
    if lower.endswith(NESTED_ARCHIVES) or lower.endswith(UNSUPPORTED_PACKAGES):
        return Finding(label, "UNSCANNABLE: nested package/archive")
    if data is not None:
        fmt = compressed_format_name(data)
        if fmt is not None:
            return Finding(label, f"UNSCANNABLE: nested compressed payload ({fmt})")
    return None


def zip_member_kind(info: zipfile.ZipInfo) -> str:
    """Classify ZIP members; preserve ordinary DOS/Windows regular files."""
    if info.is_dir() or info.filename.endswith(("/", "\\")):
        return "dir"
    dos_attr = info.external_attr & 0xFF
    if dos_attr & 0x10:
        return "dir"
    # Unix create_system records mode in the high 16 bits. DOS/Windows ZIPs
    # (create_system 0) must not be misread through Unix mode helpers.
    if info.create_system == 3:
        mode = (info.external_attr >> 16) & 0xFFFF
        file_type = stat.S_IFMT(mode)
        if file_type == 0:
            return "file"
        if file_type == stat.S_IFDIR:
            return "dir"
        if file_type == stat.S_IFLNK:
            return "symlink"
        if file_type != stat.S_IFREG:
            return "special"
    return "file"


def inspect_regular_file(path: Path, label: str) -> list[Finding]:
    try:
        if path.is_symlink():
            return [Finding(label, "UNSCANNABLE: symbolic link")]
        if path.is_dir():
            return []
        if not path.is_file():
            return [Finding(label, "UNSCANNABLE: unusual filesystem node")]
        if path.stat().st_size > MAX_MEMBER_BYTES:
            return [Finding(label, "UNSCANNABLE: file exceeds per-member size limit")]
        data = path.read_bytes()
        findings = inspect_bytes(label, data)
        fmt = compressed_format_name(data)
        lower = path.name.lower()
        if fmt is not None and not lower.endswith(SUPPORTED_ARCHIVES):
            findings.append(
                Finding(label, f"UNSCANNABLE: compressed payload requires supported archive inspection ({fmt})")
            )
        return findings
    except OSError as exc:
        return [Finding(label, f"UNSCANNABLE: {type(exc).__name__}")]


def _read_bounded(member, member_label: str, consumed: int) -> tuple[list[Finding], bytes, int]:
    """Read up to the per-member cap; account actual bytes, not spoofable headers."""
    try:
        data = member.read(MAX_MEMBER_BYTES + 1)
    except Exception as exc:  # noqa: BLE001 — fail closed on any decompressor error
        return (
            [Finding(member_label, f"UNSCANNABLE: malformed compressed stream ({type(exc).__name__})")],
            b"",
            consumed,
        )
    actual = len(data)
    consumed += actual
    findings: list[Finding] = []
    if actual > MAX_MEMBER_BYTES or consumed > MAX_ARTIFACT_BYTES:
        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
        return findings, data[:MAX_MEMBER_BYTES], consumed
    return findings, data, consumed


def inspect_archive(path: Path, label: str) -> list[Finding]:
    findings: list[Finding] = []
    consumed = 0
    if path.is_symlink():
        return [Finding(label, "UNSCANNABLE: symbolic link")]
    try:
        if not path.is_file() or path.stat().st_size == 0:
            return [Finding(label, "UNSCANNABLE: empty or nonregular artifact")]
    except OSError:
        return [Finding(label, "UNSCANNABLE: unreadable artifact")]
    lower_name = path.name.lower()
    if lower_name.endswith(UNSUPPORTED_PACKAGES) and not lower_name.endswith(SUPPORTED_ARCHIVES):
        return [Finding(label, "UNSCANNABLE: package format needs an unpacked release payload")]
    try:
        if zipfile.is_zipfile(path):
            with zipfile.ZipFile(path) as archive:
                if archive.comment:
                    findings.extend(inspect_bytes(f"{label}!ZIP_COMMENT", archive.comment))
                members = archive.infolist()
                if len(members) > MAX_ARCHIVE_MEMBERS:
                    return [Finding(label, "UNSCANNABLE: archive member count limit")]
                substantive_payloads = 0
                for info in members:
                    member_label = f"{label}!{info.filename}"
                    findings.extend(inspect_bytes(member_label, info.filename.encode("utf-8", errors="surrogateescape")))
                    if info.comment:
                        findings.extend(
                            inspect_bytes(
                                f"{member_label}!COMMENT",
                                info.comment if isinstance(info.comment, bytes) else info.comment.encode("utf-8"),
                            )
                        )
                    kind = zip_member_kind(info)
                    if kind == "dir":
                        continue
                    if kind == "symlink":
                        findings.append(Finding(member_label, "UNSCANNABLE: symbolic-link ZIP member"))
                        continue
                    if kind == "special":
                        findings.append(Finding(member_label, "UNSCANNABLE: nonregular ZIP member"))
                        continue
                    if info.compress_type not in ALLOWED_ZIP_METHODS:
                        findings.append(
                            Finding(
                                member_label,
                                f"UNSCANNABLE: unsupported ZIP compression method ({info.compress_type})",
                            )
                        )
                        continue
                    # Header sizes are advisory only; spoofed values must not widen limits.
                    if info.compress_size > MAX_MEMBER_BYTES or info.file_size > MAX_MEMBER_BYTES:
                        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
                        continue
                    if info.compress_size > MAX_ARTIFACT_BYTES - consumed:
                        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
                        continue
                    nested = nested_or_compressed_finding(member_label, info.filename)
                    if nested is not None:
                        findings.append(nested)
                        # Still attempt a bounded read so spoofed nested names cannot hide brands in cleartext
                        # pretending to be archives; nested finding already fails the gate.
                    try:
                        with archive.open(info) as member:
                            read_findings, data, consumed = _read_bounded(member, member_label, consumed)
                    except Exception as exc:  # noqa: BLE001
                        findings.append(
                            Finding(member_label, f"UNSCANNABLE: malformed compressed stream ({type(exc).__name__})")
                        )
                        continue
                    findings.extend(read_findings)
                    if read_findings:
                        continue
                    if nested is None:
                        nested = nested_or_compressed_finding(member_label, info.filename, data)
                        if nested is not None:
                            findings.append(nested)
                    if data:
                        substantive_payloads += 1
                        findings.extend(inspect_bytes(member_label, data))
                if substantive_payloads == 0:
                    findings.append(Finding(label, "UNSCANNABLE: empty or directory-only archive payload"))
            return findings
        if tarfile.is_tarfile(path):
            with tarfile.open(path, mode="r:*") as archive:
                substantive_payloads = 0
                member_count = 0
                for info in archive:
                    member_count += 1
                    if member_count > MAX_ARCHIVE_MEMBERS:
                        findings.append(Finding(label, "UNSCANNABLE: archive member count limit"))
                        break
                    member_label = f"{label}!{info.name}"
                    findings.extend(inspect_bytes(member_label, info.name.encode("utf-8", errors="surrogateescape")))
                    for meta_label, meta_value in (
                        (f"{member_label}!UNAME", info.uname),
                        (f"{member_label}!GNAME", info.gname),
                        (f"{member_label}!LINKNAME", info.linkname),
                    ):
                        if meta_value:
                            findings.extend(inspect_bytes(meta_label, meta_value.encode("utf-8", errors="surrogateescape")))
                    if info.isdir():
                        continue
                    if info.issym() or info.islnk():
                        findings.append(Finding(member_label, "UNSCANNABLE: nonregular archive member"))
                        continue
                    if not info.isfile():
                        findings.append(Finding(member_label, "UNSCANNABLE: nonregular archive member"))
                        continue
                    nested = nested_or_compressed_finding(member_label, info.name)
                    if nested is not None:
                        findings.append(nested)
                    if info.size > MAX_MEMBER_BYTES or consumed + max(info.size, 0) > MAX_ARTIFACT_BYTES:
                        findings.append(Finding(member_label, "UNSCANNABLE: decompression size limit"))
                        continue
                    member = archive.extractfile(info)
                    if member is None:
                        findings.append(Finding(member_label, "UNSCANNABLE: unreadable tar member"))
                        continue
                    with member:
                        read_findings, data, consumed = _read_bounded(member, member_label, consumed)
                    findings.extend(read_findings)
                    if read_findings:
                        continue
                    if nested is None:
                        nested = nested_or_compressed_finding(member_label, info.name, data)
                        if nested is not None:
                            findings.append(nested)
                    if data:
                        substantive_payloads += 1
                        findings.extend(inspect_bytes(member_label, data))
                if substantive_payloads == 0:
                    findings.append(Finding(label, "UNSCANNABLE: empty or directory-only archive payload"))
            return findings
    except (OSError, EOFError, RuntimeError, ValueError, zipfile.BadZipFile, tarfile.TarError) as exc:
        return [Finding(label, f"UNSCANNABLE: corrupt or unreadable archive ({type(exc).__name__})")]
    if lower_name.endswith(SUPPORTED_ARCHIVES):
        return [Finding(label, "UNSCANNABLE: unrecognized or corrupt archive")]
    # Signature-detected compressed payloads without a supported archive wrapper.
    try:
        preview = path.read_bytes()[:64]
    except OSError:
        return [Finding(label, "UNSCANNABLE: unreadable artifact")]
    fmt = compressed_format_name(preview)
    if fmt is not None:
        return [Finding(label, f"UNSCANNABLE: compressed payload requires supported archive inspection ({fmt})")]
    # Non-archive artifacts (binaries, manifests, etc.) still get raw inspection.
    return inspect_regular_file(path, label)


def walk_files(root: Path, *, source: bool) -> list[Path]:
    if root.is_symlink() or root.is_file():
        return [root]
    if not root.is_dir():
        return [root]
    files: list[Path] = []
    for path in sorted(root.rglob("*")):
        if source and any(part in SOURCE_SKIP_DIRS for part in path.relative_to(root).parts):
            continue
        if path.is_symlink():
            files.append(path)
            continue
        if path.is_dir():
            files.append(path)
            continue
        if path.is_file():
            files.append(path)
            continue
        files.append(path)  # fifo/socket/device: do not silently skip
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
            findings.extend(inspect_bytes(label, label.encode("utf-8", errors="surrogateescape")))
            if path.is_dir() and not path.is_symlink():
                continue
            findings.extend(inspect_regular_file(path, label))
    for root in artifacts:
        if not root.exists() and not root.is_symlink():
            findings.append(Finding(str(root), "UNSCANNABLE: artifact missing"))
            continue
        artifact_files = walk_files(root, source=False)
        if not artifact_files:
            findings.append(Finding(str(root), "UNSCANNABLE: empty artifact directory"))
            continue
        scanned_files = 0
        for path in artifact_files:
            visited += 1
            label = path.relative_to(repo).as_posix() if path.is_relative_to(repo) else str(path)
            findings.extend(inspect_bytes(label, label.encode("utf-8", errors="surrogateescape")))
            if path.is_dir() and not path.is_symlink():
                continue
            scanned_files += 1
            findings.extend(inspect_archive(path, label))
        if scanned_files == 0:
            findings.append(Finding(str(root), "UNSCANNABLE: empty or directory-only artifact payload"))
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
    if visited == 0:
        print("[ERROR] No product files were scanned", file=sys.stderr)
        return 2
    for finding in findings:
        print(f"[ERROR] {finding.problem} at {finding.path!r}", file=sys.stderr)
    if findings:
        print(f"BRAND HYGIENE FAIL: {len(findings)} issue(s), {visited} file(s) scanned", file=sys.stderr)
        return 1
    print(f"BRAND HYGIENE PASS: {visited} file(s) scanned; no named reference brands")
    return 0


if __name__ == "__main__":
    sys.exit(main())
