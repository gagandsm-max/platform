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

expect_refusal() {
  local name="$1" diagnostic="$2" status=0
  shift 2
  "${BUILD[@]}" --out "$REJECT_OUT" "$@" >"${WORKDIR}/${name}.log" 2>&1 || status=$?
  if [[ "$status" != 78 ]] ||
     ! grep -Fq "PLATFORM_AGENT_DEB_BUILD_REFUSED: ${diagnostic}" "${WORKDIR}/${name}.log" ||
     [[ -n "$(find "$REJECT_OUT" -mindepth 1 -print -quit)" ]]; then
    cat "${WORKDIR}/${name}.log" >&2
    echo "FAIL: ${name} expected exit 78 before staging, got ${status}" >&2
    return 1
  fi
  echo "PASS ${name} exit=${status}"
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

"${BUILD[@]}" "${INPUT[@]}" --out "$REPEAT_OUT" | tee "${WORKDIR}/repeat.json"

DEB="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["deb_path"])' "${OUT}/platform-agent-${VERSION}-amd64-deb/build-receipt.json")"
test -f "$DEB"
cmp "$DEB" "${REPEAT_OUT}/platform-agent-${VERSION}-amd64-deb/platform-agent_${VERSION}_amd64.deb"
echo "PASS byte-identical repeat build"
DEB_SHA="$(sha256sum "$DEB" | awk '{print $1}')"
echo "DEB_SHA256=${DEB_SHA}"

LIST="${WORKDIR}/contents.txt"
dpkg-deb -c "$DEB" > "$LIST"
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

# Forbidden /usr/local
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
