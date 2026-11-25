#!/usr/bin/env bash
# Package content / provenance / no-/usr/local / no-secrets gate for LNX-PKG-01.
# Does not install packages on the host and is not native reboot certification.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "MISSING_TOOL: $1" >&2; exit 78; }; }
need dpkg-deb
need sha256sum
need python3

WORKDIR="$(mktemp -d /tmp/platform-agent-pkg-content.XXXXXX)"
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

# Real ELF fixture for archive/content assertions. The fixture is never executed.
BIN=/bin/true
BIN_SHA="$(sha256sum "$BIN" | awk '{print $1}')"
COMMIT="$(git rev-parse HEAD)"
VERSION="0.0.0+internal.test"
OUT="${WORKDIR}/out"
REPEAT_OUT="${WORKDIR}/repeat"
REJECT_OUT="${WORKDIR}/reject"
mkdir -m 0700 "$OUT" "$REPEAT_OUT" "$REJECT_OUT"

BUILD=(
  bash agent/packaging/linux/build-deb.sh
  --version "$VERSION"
  --source-commit "$COMMIT"
  --source-date-epoch 0
  --runtime-version "RUNTIME_INDEPENDENT_OF_PACKAGE_VERSION"
)
INPUT=(
  --binary "$BIN"
  --architecture amd64
  --binary-sha256 "$BIN_SHA"
)
"${BUILD[@]}" "${INPUT[@]}" --out "$OUT" | tee "${WORKDIR}/build.json"

expect_refusal_at() {
  local name="$1" diagnostic="$2" out_dir="$3" status=0
  shift 3
  "${BUILD[@]}" --out "$out_dir" "$@" >"${WORKDIR}/${name}.log" 2>&1 || status=$?
  if [[ "$status" != 78 ]] ||
     ! grep -Fq "PLATFORM_AGENT_DEB_BUILD_REFUSED: ${diagnostic}" "${WORKDIR}/${name}.log" ||
     [[ -n "$(find "$out_dir" -mindepth 1 -print -quit 2>/dev/null || true)" ]]; then
    cat "${WORKDIR}/${name}.log" >&2
    echo "FAIL: ${name} expected exit 78 before staging, got ${status}" >&2
    return 1
  fi
  echo "PASS ${name} exit=${status}"
}

expect_refusal() {
  local name="$1" diagnostic="$2"
  shift 2
  expect_refusal_at "$name" "$diagnostic" "$REJECT_OUT" "$@"
}

DIGEST_DIAGNOSTIC="--binary-sha256 is required and must be 64 lowercase hex digits"
expect_refusal missing-digest "$DIGEST_DIAGNOSTIC" --binary "$BIN"
expect_refusal malformed-digest "$DIGEST_DIAGNOSTIC" --binary "$BIN" --binary-sha256 "$(printf '%064d' 0 | tr 0 A)"
WRONG_SHA="0${BIN_SHA:1}"
if [[ "$WRONG_SHA" == "$BIN_SHA" ]]; then
  WRONG_SHA="1${BIN_SHA:1}"
fi
expect_refusal hash-mismatch "binary sha256 mismatch" --binary "$BIN" --binary-sha256 "$WRONG_SHA"
TEXT_BIN="${WORKDIR}/not-elf"
printf 'This is a text payload.\n' > "$TEXT_BIN"
TEXT_SHA="$(sha256sum "$TEXT_BIN" | awk '{print $1}')"
expect_refusal non-elf "binary must be an ELF64 little-endian executable" --binary "$TEXT_BIN" --binary-sha256 "$TEXT_SHA"
expect_refusal wrong-architecture "binary ELF machine mismatch for arm64: expected 183, found 62" \
  --binary "$BIN" --binary-sha256 "$BIN_SHA" --architecture arm64

# Nonexistent commit object (40 hex that is not in the object store).
expect_refusal nonexistent-commit "source-commit is not a git object" \
  --binary "$BIN" --binary-sha256 "$BIN_SHA" \
  --source-commit "ffffffffffffffffffffffffffffffffffffffff"

# Wrong object type: tree SHA must not be accepted as source-commit.
TREE_SHA="$(git rev-parse 'HEAD^{tree}')"
expect_refusal wrong-object-type "source-commit must be a commit object" \
  --binary "$BIN" --binary-sha256 "$BIN_SHA" \
  --source-commit "$TREE_SHA"

# Invalid epoch / missing argument arity (refuse before writes).
expect_refusal invalid-epoch "source-date-epoch must be integer" \
  --binary "$BIN" --binary-sha256 "$BIN_SHA" --source-date-epoch "-1"
expect_refusal missing-argument "--binary-sha256 requires a value" \
  --binary "$BIN" --binary-sha256

# Symlink output leaf and symlink-ancestor output must fail closed before staging.
SYMLINK_OUT="${WORKDIR}/out-symlink"
ln -s "$REJECT_OUT" "$SYMLINK_OUT"
status=0
"${BUILD[@]}" --binary "$BIN" --binary-sha256 "$BIN_SHA" --out "$SYMLINK_OUT" \
  >"${WORKDIR}/symlink-out.log" 2>&1 || status=$?
if [[ "$status" != 78 ]] ||
   ! grep -Fq "PLATFORM_AGENT_DEB_BUILD_REFUSED: symlink refused in output" "${WORKDIR}/symlink-out.log" ||
   [[ -n "$(find "$REJECT_OUT" -mindepth 1 -print -quit)" ]]; then
  cat "${WORKDIR}/symlink-out.log" >&2
  echo "FAIL: symlink-out expected exit 78 before staging, got ${status}" >&2
  exit 1
fi
echo "PASS symlink-out exit=${status}"

REAL_PARENT="${WORKDIR}/real-parent"
mkdir -m 0700 "$REAL_PARENT" "${REAL_PARENT}/out"
LINK_PARENT="${WORKDIR}/link-parent"
ln -s "$REAL_PARENT" "$LINK_PARENT"
status=0
"${BUILD[@]}" --binary "$BIN" --binary-sha256 "$BIN_SHA" --out "${LINK_PARENT}/out" \
  >"${WORKDIR}/symlink-ancestor.log" 2>&1 || status=$?
if [[ "$status" != 78 ]] ||
   ! grep -Fq "PLATFORM_AGENT_DEB_BUILD_REFUSED: symlink refused in output" "${WORKDIR}/symlink-ancestor.log" ||
   [[ -n "$(find "${REAL_PARENT}/out" -mindepth 1 -print -quit)" ]]; then
  cat "${WORKDIR}/symlink-ancestor.log" >&2
  echo "FAIL: symlink-ancestor expected exit 78 before staging, got ${status}" >&2
  exit 1
fi
echo "PASS symlink-ancestor exit=${status}"

# Source/output overlap: refuse writing into the repository tree (do not scan ROOT contents).
status=0
"${BUILD[@]}" --binary "$BIN" --binary-sha256 "$BIN_SHA" --out "$ROOT" \
  >"${WORKDIR}/source-overlap.log" 2>&1 || status=$?
if [[ "$status" != 78 ]] ||
   ! grep -Fq "PLATFORM_AGENT_DEB_BUILD_REFUSED: source/output overlap" "${WORKDIR}/source-overlap.log" ||
   [[ -e "${ROOT}/platform-agent-${VERSION}-amd64-deb" ]]; then
  cat "${WORKDIR}/source-overlap.log" >&2
  echo "FAIL: source-overlap expected exit 78 before staging, got ${status}" >&2
  exit 1
fi
echo "PASS source-overlap exit=${status}"

# Malformed later PT_LOAD: earlier executable load backs entry, later PT_LOAD is bounds-invalid.
BAD_ELF="${WORKDIR}/bad-later-pt-load"
python3 - "$BAD_ELF" <<'PY'
import struct, sys
path = sys.argv[1]
# Minimal ELF64 LE ET_EXEC, amd64, two PT_LOAD entries.
# Entry at vaddr 0x1000 backed by first PT_LOAD; second PT_LOAD claims file bytes past EOF.
ehsize = 64
phentsize = 56
phnum = 2
phoff = 64
entry = 0x1000
# File content: header + 2 program headers + 16 bytes of "payload" for first load.
payload = b"\x90" * 16
size = ehsize + phnum * phentsize + len(payload)

e_ident = b"\x7fELF" + bytes([2, 1, 1, 0]) + bytes(8)
header = e_ident + struct.pack("<HHIQQQIHHHHHH",
    2,      # e_type ET_EXEC
    62,     # e_machine EM_X86_64
    1,      # e_version
    entry,  # e_entry
    phoff,  # e_phoff
    0,      # e_shoff
    0,      # e_flags
    ehsize,
    phentsize,
    phnum,
    0, 0, 0,
)
# PT_LOAD #0: executable, covers entry, in-bounds.
ph0 = struct.pack("<IIQQQQQQ",
    1,          # PT_LOAD
    5,          # PF_R|PF_X
    ehsize + phnum * phentsize,  # p_offset
    0x1000,     # p_vaddr
    0x1000,     # p_paddr
    len(payload),
    len(payload),
    0x1000,     # p_align
)
# PT_LOAD #1: bounds-invalid (offset + filesz > size).
ph1 = struct.pack("<IIQQQQQQ",
    1,          # PT_LOAD
    4,          # PF_R
    size - 8,   # p_offset near EOF
    0x2000,
    0x2000,
    64,         # filesz extends past EOF
    64,
    0x1000,
)
open(path, "wb").write(header + ph0 + ph1 + payload)
PY
BAD_ELF_SHA="$(sha256sum "$BAD_ELF" | awk '{print $1}')"
expect_refusal bad-later-pt-load "binary ELF load segment is bounds-invalid" \
  --binary "$BAD_ELF" --binary-sha256 "$BAD_ELF_SHA"

"${BUILD[@]}" "${INPUT[@]}" --out "$REPEAT_OUT" | tee "${WORKDIR}/repeat.json"

DEB="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["deb_path"])' "${OUT}/platform-agent-${VERSION}-amd64-deb/build-receipt.json")"
test -f "$DEB"
cmp "$DEB" "${REPEAT_OUT}/platform-agent-${VERSION}-amd64-deb/platform-agent_${VERSION}_amd64.deb"
echo "PASS byte-identical repeat build"
DEB_SHA="$(sha256sum "$DEB" | awk '{print $1}')"
echo "DEB_SHA256=${DEB_SHA}"

# Quote / backslash output paths must produce valid JSON receipts (argv/path as data).
QUOTE_OUT="${WORKDIR}/out\"quote"
BS_OUT="${WORKDIR}/out\\slash"
mkdir -m 0700 "$QUOTE_OUT" "$BS_OUT"
"${BUILD[@]}" "${INPUT[@]}" --out "$QUOTE_OUT" | tee "${WORKDIR}/quote.json"
"${BUILD[@]}" "${INPUT[@]}" --out "$BS_OUT" | tee "${WORKDIR}/bs.json"
python3 - "$QUOTE_OUT" "$BS_OUT" "$VERSION" "$BIN_SHA" <<'PY'
import json, sys, pathlib
quote_out, bs_out, version, bin_sha = sys.argv[1:]
for out in (quote_out, bs_out):
    receipt_path = pathlib.Path(out) / f"platform-agent-{version}-amd64-deb" / "build-receipt.json"
    receipt = json.loads(receipt_path.read_text(encoding="utf-8"))
    assert receipt["signing"] == "UNSIGNED_INTERNAL_ONLY"
    assert receipt["binary_sha256"] == bin_sha
    assert pathlib.Path(receipt["deb_path"]).is_file()
    # Round-trip proves the builder emitted actual JSON, not broken source interpolation.
    assert json.loads(json.dumps(receipt))["deb_sha256"] == receipt["deb_sha256"]
print("PASS quote/backslash output path JSON safety")
PY

LIST="${WORKDIR}/contents.txt"
# Archive listing must succeed before absence checks.
if ! dpkg-deb -c "$DEB" > "$LIST"; then
  echo "FAIL: dpkg-deb -c listing failed" >&2
  exit 1
fi
dpkg-deb -I "$DEB" > "${WORKDIR}/info.txt"
dpkg-deb -f "$DEB" Package Version Architecture > "${WORKDIR}/fields.txt"

grep -q 'Package: platform-agent' "${WORKDIR}/info.txt"
grep -q "Version: ${VERSION}" "${WORKDIR}/info.txt"
grep -q 'Architecture: amd64' "${WORKDIR}/info.txt"

# Required payload paths
grep -E '\./opt/platform/agent/platform-agent-linux$' "$LIST"
grep -E '\./usr/lib/systemd/system/platform-agent\.service$' "$LIST"
grep -E '\./usr/lib/platform/agent/packaging/platform-agent-lifecycle\.sh$' "$LIST"
grep -E '\./usr/lib/platform/agent/packaging/provenance\.json$' "$LIST"
grep -E '\./usr/share/doc/platform-agent/platform-agent\.env\.example$' "$LIST"

# Forbidden /usr/local (listing already succeeded above)
if grep -E '/usr/local(/|$)' "$LIST"; then
  echo "FAIL: package contains /usr/local path" >&2
  exit 1
fi

# Extract and inspect sensitive content
EXTRACT="${WORKDIR}/extract"
mkdir "$EXTRACT"
dpkg-deb -x "$DEB" "$EXTRACT"
dpkg-deb -e "$DEB" "${EXTRACT}/DEBIAN"

# Modes
python3 - <<PY
import os, stat, pathlib, json, hashlib
root = pathlib.Path("${EXTRACT}")
binary = root / "opt/platform/agent/platform-agent-linux"
unit = root / "usr/lib/systemd/system/platform-agent.service"
assert binary.is_file() and not binary.is_symlink()
assert binary.read_bytes() == pathlib.Path("${BIN}").read_bytes()
assert unit.is_file() and not unit.is_symlink()
mode = stat.S_IMODE(binary.stat().st_mode)
assert mode == 0o755, oct(mode)
umode = stat.S_IMODE(unit.stat().st_mode)
assert umode == 0o644, oct(umode)
unit_text = unit.read_text()
assert "ExecStart=/opt/platform/agent/platform-agent-linux" in unit_text
assert "/usr/local/" not in unit_text
assert "CapabilityBoundingSet=" not in unit_text
assert "AmbientCapabilities=" not in unit_text
profile = root / "usr/share/platform-agent/systemd/20-ebpf.conf"
assert profile.read_bytes() == pathlib.Path("agent/linux/systemd/ebpf/20-ebpf.conf").read_bytes()
assert not (root / "etc/systemd/system/platform-agent.service.d").exists()
# Unit must match repo sources byte-for-byte
repo_unit = pathlib.Path("agent/linux/systemd/platform-agent.service").read_bytes()
deploy_unit = pathlib.Path("deploy/systemd/platform-agent.service").read_bytes()
assert repo_unit == deploy_unit == unit.read_bytes()
prov = json.loads((root/"usr/lib/platform/agent/packaging/provenance.json").read_text())
assert prov["signing"] == "UNSIGNED_INTERNAL_ONLY"
assert prov["ebpf_enabled_by_package"] is False
assert prov["binary_sha256"] == "${BIN_SHA}"
assert prov["source_commit"] == "${COMMIT}"
# No secrets in packaged text assets
forbidden = ["BEGIN PRIVATE KEY", "PRIVATE KEY-----", "password=", "-----BEGIN"]
for path in root.rglob("*"):
    if not path.is_file():
        continue
    if path.suffix in {".json", ".md", ".txt", ".example", ".service", ".sh"} or path.name in {"control", "preinst", "postinst", "prerm", "postrm"}:
        text = path.read_text(errors="ignore")
        for token in forbidden:
            assert token not in text, (path, token)
print("CONTENT_ASSERTIONS_OK")
print("binary_sha256", prov["binary_sha256"])
print("deb_sha256", "${DEB_SHA}")
PY

# Maintainer scripts executable
for h in preinst postinst prerm postrm; do
  test -x "${EXTRACT}/DEBIAN/${h}"
done

echo "PASS test-package-content.sh"
echo "EVIDENCE deb_sha256=${DEB_SHA} binary_sha256=${BIN_SHA} source_commit=${COMMIT}"
