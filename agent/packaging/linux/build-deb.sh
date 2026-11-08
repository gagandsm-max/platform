#!/bin/bash
# Build an UNSIGNED/INTERNAL_ONLY platform-agent .deb from a verified binary input.
# Does not sign, does not invent a release version, does not install on the host.
set -euo pipefail

die() {
  echo "PLATFORM_AGENT_DEB_BUILD_REFUSED: $*" >&2
  exit 78
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required tool missing: $1"
}

sha256_file() {
  sha256sum "$1" | awk '{print $1}'
}

usage() {
  cat <<'EOF'
Usage: build-deb.sh --binary PATH --version VER --source-commit SHA40 --out DIR
                     [--architecture amd64] [--source-date-epoch EPOCH]
                     [--binary-sha256 HEX] [--runtime-version TEXT]

Run from repository root. Produces UNSIGNED/INTERNAL_ONLY artifacts only.
EOF
}

BINARY=""
VERSION=""
SOURCE_COMMIT=""
OUT_DIR=""
ARCH="amd64"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}"
BINARY_SHA256=""
RUNTIME_VERSION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --source-commit) SOURCE_COMMIT="${2:-}"; shift 2 ;;
    --out) OUT_DIR="${2:-}"; shift 2 ;;
    --architecture) ARCH="${2:-}"; shift 2 ;;
    --source-date-epoch) SOURCE_DATE_EPOCH="${2:-}"; shift 2 ;;
    --binary-sha256) BINARY_SHA256="${2:-}"; shift 2 ;;
    --runtime-version) RUNTIME_VERSION="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ -n "$BINARY" && -n "$VERSION" && -n "$SOURCE_COMMIT" && -n "$OUT_DIR" ]] || { usage; die "missing required arguments"; }

REPO_ROOT="$(pwd)"
LAYOUT="${REPO_ROOT}/agent/packaging/linux/package-layout.json"
UNIT_SRC="${REPO_ROOT}/agent/linux/systemd/platform-agent.service"
UNIT_MIRROR="${REPO_ROOT}/deploy/systemd/platform-agent.service"
ENV_EXAMPLE="${REPO_ROOT}/agent/linux/systemd/platform-agent.env.example"
CONTROL_TEMPLATE="${REPO_ROOT}/agent/packaging/linux/debian/control"
LIFECYCLE_SRC="${REPO_ROOT}/agent/packaging/linux/scripts/platform-agent-lifecycle.sh"
README_SRC="${REPO_ROOT}/agent/packaging/linux/README.md"

[[ -f "$LAYOUT" ]] || die "run from repository root (package-layout.json missing)"
[[ -f "$UNIT_SRC" && -f "$UNIT_MIRROR" ]] || die "systemd unit sources missing"
[[ -f "$ENV_EXAMPLE" && -f "$CONTROL_TEMPLATE" && -f "$LIFECYCLE_SRC" && -f "$README_SRC" ]] || die "packaging sources missing"
cmp -s "$UNIT_SRC" "$UNIT_MIRROR" || die "unit mirrors are not byte-identical"

need_cmd dpkg-deb
need_cmd sha256sum
need_cmd awk
need_cmd install

[[ -f "$BINARY" && ! -L "$BINARY" ]] || die "binary must be a regular non-symlink file"
BIN_SIZE="$(stat -c '%s' "$BINARY")"
[[ "$BIN_SIZE" -gt 0 && "$BIN_SIZE" -le 268435456 ]] || die "binary size out of bounds: ${BIN_SIZE}"
ACTUAL_SHA="$(sha256_file "$BINARY")"
if [[ -n "$BINARY_SHA256" ]]; then
  [[ "$ACTUAL_SHA" == "$BINARY_SHA256" ]] || die "binary sha256 mismatch want=${BINARY_SHA256} got=${ACTUAL_SHA}"
fi

[[ "$VERSION" =~ ^[0-9][A-Za-z0-9.+~-]{0,63}$ ]] || die "version unsupported: ${VERSION}"
[[ "$SOURCE_COMMIT" =~ ^[0-9a-f]{40}$ ]] || die "source-commit must be 40 lowercase hex"
[[ "$ARCH" == "amd64" || "$ARCH" == "arm64" ]] || die "architecture unsupported for this slice: ${ARCH}"
[[ "$SOURCE_DATE_EPOCH" =~ ^[0-9]+$ ]] || die "source-date-epoch must be integer"

# Native unit contract: ExecStart must be /opt/platform/agent/platform-agent-linux
grep -qx 'ExecStart=/opt/platform/agent/platform-agent-linux' "$UNIT_SRC" \
  || die "unit ExecStart is not the governed /opt/platform/agent path"
grep -q '/usr/local/' "$UNIT_SRC" && die "unit still references /usr/local"

[[ -d "$OUT_DIR" && ! -L "$OUT_DIR" ]] || die "--out must be an existing non-symlink directory"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
DEST="${OUT_DIR}/platform-agent-${VERSION}-${ARCH}-deb"
[[ ! -e "$DEST" ]] || die "output collision: ${DEST}"
mkdir -m 0700 "$DEST"
STAGE="${DEST}/payload"
mkdir -m 0755 "$STAGE"

install -d -m 0755 \
  "${STAGE}/opt/platform/agent" \
  "${STAGE}/usr/lib/systemd/system" \
  "${STAGE}/usr/lib/platform/agent/packaging" \
  "${STAGE}/usr/share/doc/platform-agent" \
  "${STAGE}/DEBIAN"

install -d -m 0755 "${STAGE}/usr/share/platform-agent/systemd"
install -m 0644 "${REPO_ROOT}/agent/linux/systemd/ebpf/20-ebpf.conf" "${STAGE}/usr/share/platform-agent/systemd/20-ebpf.conf"

install -m 0755 "$BINARY" "${STAGE}/opt/platform/agent/platform-agent-linux"
install -m 0644 "$UNIT_SRC" "${STAGE}/usr/lib/systemd/system/platform-agent.service"
install -m 0755 "$LIFECYCLE_SRC" "${STAGE}/usr/lib/platform/agent/packaging/platform-agent-lifecycle.sh"
install -m 0644 "$LAYOUT" "${STAGE}/usr/lib/platform/agent/packaging/package-layout.json"
install -m 0644 "$ENV_EXAMPLE" "${STAGE}/usr/share/doc/platform-agent/platform-agent.env.example"
install -m 0644 "$README_SRC" "${STAGE}/usr/share/doc/platform-agent/README.packaging.md"

# Operator-facing honesty markers (not secrets).
{
  echo "Package: platform-agent"
  echo "Version: ${VERSION}"
  echo "Source-Commit: ${SOURCE_COMMIT}"
  echo "Architecture: ${ARCH}"
  echo "Signing: UNSIGNED_INTERNAL_ONLY"
  echo "Runtime-Version-Metadata: ${RUNTIME_VERSION:-NOT_SUPPLIED_INDEPENDENT_OF_PACKAGE_VERSION}"
  echo "Note: package Version does not rewrite embedded runtime version strings."
} > "${STAGE}/usr/share/doc/platform-agent/ARTIFACT_TRUTH.txt"
chmod 0644 "${STAGE}/usr/share/doc/platform-agent/ARTIFACT_TRUTH.txt"

RUNTIME_JSON="null"
if [[ -n "$RUNTIME_VERSION" ]]; then
  RUNTIME_JSON="$(printf '%s' "$RUNTIME_VERSION" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')"
fi
PROVENANCE="$(cat <<EOF
{
  "schema": "PLATFORM_AGENT_DEB_PROVENANCE_V1",
  "package": "platform-agent",
  "version": "${VERSION}",
  "architecture": "${ARCH}",
  "source_commit": "${SOURCE_COMMIT}",
  "source_date_epoch": ${SOURCE_DATE_EPOCH},
  "binary_sha256": "${ACTUAL_SHA}",
  "binary_bytes": ${BIN_SIZE},
  "runtime_version_input": ${RUNTIME_JSON},
  "unit_sha256": "$(sha256_file "$UNIT_SRC")",
  "lifecycle_sha256": "$(sha256_file "$LIFECYCLE_SRC")",
  "layout_sha256": "$(sha256_file "$LAYOUT")",
  "signing": "UNSIGNED_INTERNAL_ONLY",
  "forbidden_checked": ["/usr/local"],
  "ebpf_enabled_by_package": false
}
EOF
)"
printf '%s\n' "$PROVENANCE" > "${STAGE}/usr/lib/platform/agent/packaging/provenance.json"
chmod 0644 "${STAGE}/usr/lib/platform/agent/packaging/provenance.json"

sed -e "s/@VERSION@/${VERSION}/g" -e "s/@DEB_ARCH@/${ARCH}/g" \
  "$CONTROL_TEMPLATE" > "${STAGE}/DEBIAN/control"
chmod 0644 "${STAGE}/DEBIAN/control"

# Empty conffiles intentionally omitted: operator-created agent.env is not a package conffile.

# Embed lifecycle source into each maintainer script so preinst works before unpack.
hook() {
  local phase="$1"
  local hook_path="${STAGE}/DEBIAN/${phase}"
  {
    echo '#!/bin/bash'
    echo '# Embedded platform-agent lifecycle (preinst-safe; does not depend on unpacked helpers).'
    echo 'set -euo pipefail'
    # Strip shebang, original set, and original main invocation; call with fixed phase.
    sed -e '1{/^#!/d;}' \
        -e '/^set -euo pipefail$/d' \
        -e '/^main "\$@"$/d' \
        "$LIFECYCLE_SRC"
    printf 'main %q "$@"\n' "$phase"
  } > "$hook_path"
  chmod 0755 "$hook_path"
}

hook preinst
hook postinst
hook prerm
hook postrm

# Deterministic timestamps when SOURCE_DATE_EPOCH is provided.
find "$STAGE" -exec touch -d "@${SOURCE_DATE_EPOCH}" {} +

DEB_PATH="${DEST}/platform-agent_${VERSION}_${ARCH}.deb"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH}" dpkg-deb --root-owner-group --build "$STAGE" "$DEB_PATH" >/dev/null
DEB_SHA="$(sha256_file "$DEB_PATH")"

RECEIPT="$(cat <<EOF
{
  "schema": "PLATFORM_AGENT_DEB_BUILD_RECEIPT_V1",
  "package": "platform-agent",
  "version": "${VERSION}",
  "architecture": "${ARCH}",
  "source_commit": "${SOURCE_COMMIT}",
  "source_date_epoch": ${SOURCE_DATE_EPOCH},
  "deb_path": "${DEB_PATH}",
  "deb_sha256": "${DEB_SHA}",
  "deb_bytes": $(stat -c '%s' "$DEB_PATH"),
  "binary_sha256": "${ACTUAL_SHA}",
  "signing": "UNSIGNED_INTERNAL_ONLY",
  "install_performed": false,
  "native_vm_lifecycle": "NOT_RUN_BY_BUILD"
}
EOF
)"
printf '%s\n' "$RECEIPT" > "${DEST}/build-receipt.json"

# Fail closed if the archive contains /usr/local.
if dpkg-deb -c "$DEB_PATH" | grep -E '/usr/local(/|$)'; then
  die "built package unexpectedly contains /usr/local paths"
fi

python3 - <<PY
import json
print(json.dumps(json.load(open("${DEST}/build-receipt.json")), sort_keys=True))
PY
