"""No external packages or application runtime needed for the brand gate tests."""

from __future__ import annotations

import contextlib
import io
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

from validate_reference_brand_leakage import (
    MAX_MEMBER_BYTES,
    detect_brands,
    inspect_archive,
    inspect_bytes,
    main,
    scan,
)


class BrandHygieneTest(unittest.TestCase):
    def test_case_punctuation_and_embedded_identifiers(self):
        sample = b"NetAI thought-data Enterprise_360 companyThoughtDataSuffix"
        self.assertEqual(detect_brands(sample), {"NETAI", "THOUGHTDATA", "ENTERPRISE360"})

    def test_nonbrand_internet_ai_does_not_trigger(self):
        self.assertEqual(detect_brands(b"internet AI diagnostics and enterprise scale"), set())

    def test_utf16le_windows_resource(self):
        self.assertIn("NETAI", detect_brands("Reference: NetAI".encode("utf-16-le")))

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

    def test_zip_archive_member_content_and_name(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "bundle.zip"
            with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("safe.txt", "ThoughtData")
                archive.writestr("NetAI-banner.svg", "safe")
            findings = inspect_archive(path, path.name)
            self.assertTrue(any("THOUGHTDATA" in item.problem for item in findings))
            self.assertTrue(any("NETAI" in item.problem for item in findings))

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


if __name__ == "__main__":
    unittest.main()
