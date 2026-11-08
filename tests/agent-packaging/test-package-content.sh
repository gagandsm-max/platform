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

BIN="${WORKDIR}/platform-agent-linux"
# Synthetic non-executed payload for archive/content assertions.
printf 'SYNTHETIC_PLATFORM_AGENT_PAYLOAD_NOT_EXECUTED\n' > "$BIN"
chmod 0755 "$BIN"
BIN_SHA="$(sha256sum "$BIN" | awk '{print $1}')"
COMMIT="$(git rev-parse HEAD)"
VERSION="0.0.0+internal.test"
OUT="${WORKDIR}/out"
mkdir -m 0700 "$OUT"

bash agent/packaging/linux/build-deb.sh \
  --binary "$BIN" \
  --version "$VERSION" \
  --source-commit "$COMMIT" \
  --architecture amd64 \
  --source-date-epoch 0 \
  --binary-sha256 "$BIN_SHA" \
  --runtime-version "RUNTIME_INDEPENDENT_OF_PACKAGE_VERSION" \
  --out "$OUT" | tee "${WORKDIR}/build.json"

DEB="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["deb_path"])' "${OUT}/platform-agent-${VERSION}-amd64-deb/build-receipt.json")"
test -f "$DEB"
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
