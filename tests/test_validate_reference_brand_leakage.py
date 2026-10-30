"""No external packages or application runtime needed for the brand gate tests."""

from __future__ import annotations

import contextlib
import gzip
import io
import os
import stat
import struct
import tarfile
import tempfile
import unittest
import zipfile
import zlib
from pathlib import Path

from validate_reference_brand_leakage import (
    MAX_ARCHIVE_MEMBERS,
    MAX_MEMBER_BYTES,
    REQUIRED_SOURCE_DIRS,
    detect_brands,
    inspect_archive,
    inspect_bytes,
    main,
    scan,
)


class BrandHygieneTest(unittest.TestCase):
    def test_default_source_scope_and_ui_entrypoint_end_to_end(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            for name in REQUIRED_SOURCE_DIRS:
                (repo / name).mkdir(parents=True, exist_ok=True)
                (repo / name / "native.txt").write_text("AmrudAi", encoding="utf-8")
            html = repo / "ui" / "index.html"
            html.write_text("AmrudAi", encoding="utf-8")
            (repo / "compose.yaml").write_text("name: amrudai", encoding="utf-8")
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--repo", temp]), 0)
                html.write_text("NetAI", encoding="utf-8")
                self.assertEqual(main(["--repo", temp]), 1)

    def test_case_punctuation_and_embedded_identifiers(self):
        sample = b"NetAI thought-data Enterprise_360 companyThoughtDataSuffix"
        self.assertEqual(detect_brands(sample), {"NETAI", "THOUGHTDATA", "ENTERPRISE360"})

    def test_nonbrand_internet_ai_does_not_trigger(self):
        self.assertEqual(detect_brands(b"internet AI diagnostics and enterprise scale"), set())

    def test_utf16le_windows_resource(self):
        self.assertIn("NETAI", detect_brands("Reference: NetAI".encode("utf-16-le")))

    def test_malformed_utf8_does_not_glue_brand_tokens(self):
        # Invalid byte between tokens must not become NetAI via errors="ignore".
        self.assertEqual(detect_brands(b"Net\xffAI"), set())
        self.assertEqual(detect_brands(b"Thought\xfeData"), set())

    def test_normal_source_and_filename(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "good.ts").write_text("export const product = 'AmrudAi';", encoding="utf-8")
            findings, count = scan(repo, [root], [])
            self.assertEqual((findings, count), ([], 1))
            (root / "reference-Enterprise360.js").write_text("no string leak", encoding="utf-8")
            findings, count = scan(repo, [root], [])
            self.assertEqual(count, 2)
            self.assertTrue(any("ENTERPRISE360" in item.problem for item in findings))

    def test_filesystem_directory_name_is_inspected(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            (root / "ThoughtData").mkdir(parents=True)
            (root / "ok.ts").write_text("AmrudAi", encoding="utf-8")
            findings, _count = scan(repo, [root], [])
            self.assertTrue(any("THOUGHTDATA" in item.problem for item in findings))

    def test_unusual_filesystem_node_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "ok.ts").write_text("AmrudAi", encoding="utf-8")
            fifo = root / "pipe.node"
            os.mkfifo(fifo)
            findings, _count = scan(repo, [root], [])
            self.assertTrue(any("unusual filesystem node" in item.problem for item in findings))

    def test_internal_documentation_outside_default_source(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            (repo / "project_documentation").mkdir()
            (repo / "project_documentation" / "provenance.md").write_text("ThoughtData", encoding="utf-8")
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "good.ts").write_text("AmrudAi", encoding="utf-8")
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(main(["--repo", str(repo), "--path", "ui/src"]), 0)

    def test_vendored_sources_outside_first_party_scope(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "agent"
            vendor = root / "third_party" / "vendor"
            vendor.mkdir(parents=True)
            (vendor / "external.go").write_text("ThoughtData", encoding="utf-8")
            (root / "first_party.go").write_text("AmrudAi", encoding="utf-8")
            findings, count = scan(repo, [root], [])
            self.assertEqual((findings, count), ([], 1))

    def test_first_party_binary_directory_is_not_skipped(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "agent"
            artifact = root / "bin" / "agent.exe"
            artifact.parent.mkdir(parents=True)
            artifact.write_bytes(b"\x00" + "NetAI".encode("utf-16-le"))
            findings, count = scan(repo, [root], [])
            self.assertGreaterEqual(count, 1)
            self.assertTrue(any("NETAI" in item.problem for item in findings))

    def test_missing_required_source_scope_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--repo", temp]), 2)

    def test_zip_directory_name_detected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "release.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("ThoughtData/", b"")
            self.assertTrue(any("THOUGHTDATA" in item.problem for item in inspect_archive(path, path.name)))

    def test_zip_unix_directory_without_trailing_slash_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "dirs.zip"
            with zipfile.ZipFile(path, "w") as archive:
                for name in ("release", "release/assets"):
                    info = zipfile.ZipInfo(name)
                    info.create_system = 3
                    info.external_attr = (stat.S_IFDIR | 0o755) << 16
                    archive.writestr(info, b"")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_zip_unix_symlink_member_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "link.zip"
            with zipfile.ZipFile(path, "w") as archive:
                info = zipfile.ZipInfo("shadow")
                info.create_system = 3
                info.external_attr = (stat.S_IFLNK | 0o777) << 16
                archive.writestr(info, b"ThoughtData")
                archive.writestr("ok.txt", b"AmrudAi")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("symbolic-link ZIP member" in item.problem for item in findings))

    def test_zip_unix_special_file_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "fifo.zip"
            with zipfile.ZipFile(path, "w") as archive:
                info = zipfile.ZipInfo("pipe")
                info.create_system = 3
                info.external_attr = (stat.S_IFIFO | 0o644) << 16
                archive.writestr(info, b"")
                archive.writestr("ok.txt", b"AmrudAi")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("nonregular ZIP member" in item.problem for item in findings))

    def test_windows_dos_zip_regular_file_still_passes(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "windows.zip"
            with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                info = zipfile.ZipInfo("readme.txt")
                info.create_system = 0
                info.external_attr = 0x20  # MS-DOS archive attribute only
                archive.writestr(info, b"AmrudAi release notes")
            self.assertEqual(inspect_archive(path, path.name), [])

    def test_zip_archive_member_content_and_name(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "bundle.zip"
            with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("safe.txt", "ThoughtData")
                archive.writestr("NetAI-banner.svg", "safe")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("THOUGHTDATA" in item.problem for item in findings))
            self.assertTrue(any("NETAI" in item.problem for item in findings))

    def test_zip_comment_metadata_inspected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "comment.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("ok.txt", b"AmrudAi")
                archive.comment = b"built by ThoughtData"
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("THOUGHTDATA" in item.problem for item in findings))

    def test_tar_archive_member_content(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "bundle.tar.gz"
            with tarfile.open(path, "w:gz") as archive:
                data = b"platform welcome Enterprise360"
                info = tarfile.TarInfo("index.html")
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("ENTERPRISE360" in item.problem for item in findings))

    def test_oversized_content_fails_closed(self):
        findings = inspect_bytes("large.bin", b"x" * (MAX_MEMBER_BYTES + 1))
        self.assertEqual(len(findings), 1)
        self.assertIn("UNSCANNABLE", findings[0].problem)

    def test_unsupported_package_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "app.msi"
            path.write_bytes(b"sample opaque installer")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("UNSCANNABLE" in item.problem for item in findings))

    def test_extensionless_gzip_artifact_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "payload.bin"
            path.write_bytes(gzip.compress(b"ThoughtData hidden"))
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("compressed payload" in item.problem for item in findings))

    def test_nested_extensionless_compressed_member_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "release.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("assets/blob", gzip.compress(b"ThoughtData hidden"))
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("nested compressed payload" in item.problem for item in findings))

    def test_empty_artifact_file_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "empty.zip"
            path.touch()
            self.assertTrue(any("UNSCANNABLE" in item.problem for item in inspect_archive(path, path.name)))

    def test_valid_empty_zip_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "empty-valid.zip"
            with zipfile.ZipFile(path, "w"):
                pass
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_valid_empty_tar_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "empty-valid.tar"
            with tarfile.open(path, "w"):
                pass
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_directory_only_zip_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "dirs-only.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("release/", b"")
                archive.writestr("release/assets/", b"")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_directory_only_tar_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "dirs-only.tar"
            with tarfile.open(path, "w") as archive:
                for name in ("release", "release/assets"):
                    info = tarfile.TarInfo(name)
                    info.type = tarfile.DIRTYPE
                    info.mode = 0o755
                    archive.addfile(info)
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_zero_byte_only_zip_payloads_fail_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "zeros.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("a.txt", b"")
                archive.writestr("b.txt", b"")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("empty or directory-only archive payload" in item.problem for item in findings))

    def test_empty_sibling_with_substantive_payload_passes(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "mixed.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("empty.txt", b"")
                archive.writestr("readme.txt", b"AmrudAi")
            self.assertEqual(inspect_archive(path, path.name), [])

    def test_empty_artifact_directory_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "good.ts").write_text("AmrudAi", encoding="utf-8")
            (repo / "empty-release").mkdir()
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--repo", temp, "--path", "ui/src", "--artifact", "empty-release"]), 1)

    def test_nested_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "release.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("nested.tar.gz", b"hidden payload")
            self.assertTrue(any("nested" in item.problem for item in inspect_archive(path, path.name)))

    def test_supported_compressed_tar_is_scanned(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "release.tar.gz"
            with tarfile.open(path, "w:gz") as archive:
                data = b"Enterprise360"
                info = tarfile.TarInfo("release/manifest.txt")
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            self.assertTrue(any("ENTERPRISE360" in item.problem for item in inspect_archive(path, path.name)))

    def test_ordinary_zip_and_tar_packages_still_pass(self):
        with tempfile.TemporaryDirectory() as temp:
            zip_path = Path(temp) / "ok.zip"
            with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("manifest.json", '{"product":"AmrudAi"}')
            self.assertEqual(inspect_archive(zip_path, zip_path.name), [])
            tar_path = Path(temp) / "ok.tar"
            with tarfile.open(tar_path, "w") as archive:
                data = b"AmrudAi"
                info = tarfile.TarInfo("manifest.txt")
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            self.assertEqual(inspect_archive(tar_path, tar_path.name), [])

    def test_unsupported_zip_compression_method_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "lzma.zip"
            with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_LZMA) as archive:
                archive.writestr("payload.txt", b"AmrudAi")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("unsupported ZIP compression method" in item.problem for item in findings))

    def test_spoofed_zip_uncompressed_size_still_bounds_read(self):
        """Claimed file_size=0 must not hide nonempty deflated content or skip limits."""
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "spoof.zip"
            payload = b"ThoughtData"
            compressed = zlib.compress(payload, 8)[2:-4]  # raw deflate
            # Minimal handcrafted ZIP with mismatched uncompressed size.
            name = b"hidden.txt"
            local = struct.pack(
                "<IHHHHHIIIHH",
                0x04034B50,
                20,
                0,
                8,
                0,
                0,
                zlib.crc32(payload) & 0xFFFFFFFF,
                len(compressed),
                0,  # spoofed uncompressed size
                len(name),
                0,
            )
            local += name + compressed
            central = struct.pack(
                "<IHHHHHHIIIHHHHHII",
                0x02014B50,
                20,
                20,
                0,
                8,
                0,
                0,
                zlib.crc32(payload) & 0xFFFFFFFF,
                len(compressed),
                0,
                len(name),
                0,
                0,
                0,
                0,
                0,
                0,
            )
            central += name
            end = struct.pack("<IHHHHIIH", 0x06054B50, 0, 0, 1, 1, len(central), len(local), 0)
            path.write_bytes(local + central + end)
            findings = inspect_archive(path, path.name)
            self.assertTrue(
                any("THOUGHTDATA" in item.problem or "UNSCANNABLE" in item.problem for item in findings)
            )

    def test_malformed_deflate_stream_returns_structured_finding(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "bad-deflate.zip"
            name = b"broken.txt"
            garbage = b"\x00\x01\x02\x03\x04\x05\x06\x07not-deflate"
            local = struct.pack(
                "<IHHHHHIIIHH",
                0x04034B50,
                20,
                0,
                8,
                0,
                0,
                0,
                len(garbage),
                16,
                len(name),
                0,
            )
            local += name + garbage
            central = struct.pack(
                "<IHHHHHHIIIHHHHHII",
                0x02014B50,
                20,
                20,
                0,
                8,
                0,
                0,
                0,
                len(garbage),
                16,
                len(name),
                0,
                0,
                0,
                0,
                0,
                0,
            )
            central += name
            end = struct.pack("<IHHHHIIH", 0x06054B50, 0, 0, 1, 1, len(central), len(local), 0)
            path.write_bytes(local + central + end)
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("malformed compressed stream" in item.problem for item in findings))

    def test_archive_member_count_limit_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "many.zip"
            with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_STORED) as archive:
                for index in range(MAX_ARCHIVE_MEMBERS + 1):
                    archive.writestr(f"f{index}.txt", b"AmrudAi")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("member count limit" in item.problem for item in findings))

    def test_corrupt_named_archive_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "release.zip"
            path.write_bytes(b"not really a ZIP")
            self.assertTrue(any("UNSCANNABLE" in item.problem for item in inspect_archive(path, path.name)))

    def test_missing_artifact_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "good.ts").write_text("AmrudAi", encoding="utf-8")
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--repo", str(repo), "--path", "ui/src", "--artifact", "missing.zip"]), 1)

    def test_cli_additional_release_zip_scans_payload(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            root = repo / "ui" / "src"
            root.mkdir(parents=True)
            (root / "good.ts").write_text("AmrudAi", encoding="utf-8")
            artifact = repo / "release.zip"
            with zipfile.ZipFile(artifact, "w") as archive:
                archive.writestr("assets/manifest.json", '{"product":"Enterprise360"}')
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--repo", temp, "--path", "ui/src", "--artifact", "release.zip"]), 1)


if __name__ == "__main__":
    unittest.main()
