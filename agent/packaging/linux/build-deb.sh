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
  # Hash via stdin so GNU sha256sum never emits backslash-escaped path lines.
  sha256sum <"$1" | awk '{print $1}'
}

# Refuse if any path component (including the leaf) is a symlink.
refuse_symlink_path() {
  local path="$1" label="$2" cur="$1"
  while [[ -n "$cur" && "$cur" != "/" && "$cur" != "." ]]; do
    [[ ! -L "$cur" ]] || die "symlink refused in ${label}: ${cur}"
    cur="$(dirname -- "$cur")"
  done
}

# True when a and b are the same path or one is a parent of the other.
paths_overlap() {
  local a="$1" b="$2"
  [[ "$a" == "$b" || "$a" == "$b"/* || "$b" == "$a"/* ]]
}

usage() {
  cat <<'EOF'
Usage: build-deb.sh --binary PATH --binary-sha256 SHA64 --version VER --source-commit SHA40 --out DIR
                     [--architecture amd64|arm64] [--source-date-epoch EPOCH]
                     [--runtime-version TEXT]

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
    --binary|--version|--source-commit|--out|--architecture|--source-date-epoch|--binary-sha256|--runtime-version)
      [[ $# -ge 2 ]] || die "$1 requires a value"
      ;;
  esac
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --source-commit) SOURCE_COMMIT="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    --architecture) ARCH="$2"; shift 2 ;;
    --source-date-epoch) SOURCE_DATE_EPOCH="$2"; shift 2 ;;
    --binary-sha256)
      BINARY_SHA256="$2"
      shift 2
      ;;
    --runtime-version) RUNTIME_VERSION="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ -n "$BINARY" && -n "$VERSION" && -n "$SOURCE_COMMIT" && -n "$OUT_DIR" ]] || { usage; die "missing required arguments"; }
[[ "$BINARY_SHA256" =~ ^[0123456789abcdef]{64}$ ]] || die "--binary-sha256 is required and must be 64 lowercase hex digits"

REPO_ROOT="$(pwd -P)"
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
need_cmd python3
need_cmd git

# Binary leaf must not be a symlink (OS path ancestors like /bin -> usr/bin are allowed so
# nonexecuted fixtures such as /bin/true remain usable). Output/packaging inputs stay fail-closed.
[[ -f "$BINARY" && ! -L "$BINARY" ]] || die "binary must be a regular non-symlink file"
BIN_SIZE="$(stat -c '%s' "$BINARY")"
[[ "$BIN_SIZE" -gt 0 && "$BIN_SIZE" -le 268435456 ]] || die "binary size out of bounds: ${BIN_SIZE}"
ACTUAL_SHA="$(sha256_file "$BINARY")"
[[ "$ACTUAL_SHA" == "$BINARY_SHA256" ]] || die "binary sha256 mismatch want=${BINARY_SHA256} got=${ACTUAL_SHA}"

[[ "$VERSION" =~ ^[0-9][A-Za-z0-9.+~-]{0,63}$ ]] || die "version unsupported: ${VERSION}"
[[ "$SOURCE_COMMIT" =~ ^[0-9a-f]{40}$ ]] || die "source-commit must be 40 lowercase hex"
[[ "$ARCH" == "amd64" || "$ARCH" == "arm64" ]] || die "architecture unsupported for this slice: ${ARCH}"
[[ "$SOURCE_DATE_EPOCH" =~ ^(0|[1-9][0-9]{0,9})$ ]] || die "source-date-epoch must be integer"
[[ "$SOURCE_DATE_EPOCH" -le 4294967295 ]] || die "source-date-epoch exceeds timestamp range"

# Exact commit object only: refuse missing objects, tags, trees, blobs.
if ! OBJ_TYPE="$(git cat-file -t "$SOURCE_COMMIT" 2>/dev/null)"; then
  die "source-commit is not a git object"
fi
[[ "$OBJ_TYPE" == "commit" ]] || die "source-commit must be a commit object (got ${OBJ_TYPE})"

if ! ELF_ERROR="$(python3 - "$BINARY" "$ARCH" "$BIN_SIZE" <<'PY'
import struct
import sys

path, arch, size = sys.argv[1:]
size = int(size)
expected_machine = {"amd64": 62, "arm64": 183}[arch]

def refuse(message):
    print(message)
    sys.exit(1)

with open(path, "rb") as binary:
    header = binary.read(64)
    if len(header) != 64 or header[:7] != b'\x7fELF\x02\x01\x01':
        refuse("binary must be an ELF64 little-endian executable")

    kind, machine, version, entry, phoff, _, _, ehsize, phentsize, phnum, *_ = (
        struct.unpack_from("<HHIQQQIHHHHHH", header, 16)
    )
    if machine != expected_machine:
        refuse(f"binary ELF machine mismatch for {arch}: expected {expected_machine}, found {machine}")
    if (kind not in (2, 3) or version != 1 or ehsize != 64 or phentsize != 56
            or not entry or not phnum or phoff < ehsize or phoff + phnum * phentsize > size):
        refuse("binary must be an ELF64 little-endian executable with valid program headers")

    # Validate every PT_LOAD for bounds; entry must still be backed by an executable load.
    binary.seek(phoff)
    entry_backed = False
    for _ in range(phnum):
        segment = binary.read(phentsize)
        if len(segment) != phentsize:
            refuse("binary ELF program headers are truncated")
        segment_type, flags, offset, vaddr, _, filesz, memsz, _ = struct.unpack("<IIQQQQQQ", segment)
        if segment_type != 1:
            continue
        if filesz > memsz or offset > size or filesz > size - offset:
            refuse("binary ELF load segment is bounds-invalid")
        if flags & 1 and filesz and vaddr <= entry < vaddr + filesz:
            entry_backed = True
    if not entry_backed:
        refuse("binary ELF entry point is not backed by an executable load segment")
PY
)"; then
  die "${ELF_ERROR:-could not validate binary ELF}"
fi

# Native unit contract: ExecStart must be /opt/platform/agent/platform-agent-linux
grep -qx 'ExecStart=/opt/platform/agent/platform-agent-linux' "$UNIT_SRC" \
  || die "unit ExecStart is not the governed /opt/platform/agent path"
grep -q '/usr/local/' "$UNIT_SRC" && die "unit still references /usr/local"

# Packaging inputs: regular files, no symlink leaf/ancestors (truthful staging inputs).
for _pkg_input in "$LAYOUT" "$UNIT_SRC" "$UNIT_MIRROR" "$ENV_EXAMPLE" "$CONTROL_TEMPLATE" "$LIFECYCLE_SRC" "$README_SRC"; do
  refuse_symlink_path "$_pkg_input" "packaging input"
  [[ -f "$_pkg_input" && ! -L "$_pkg_input" ]] || die "packaging input must be a regular non-symlink file: ${_pkg_input}"
done

[[ -d "$OUT_DIR" ]] || die "--out must be an existing non-symlink directory"
refuse_symlink_path "$OUT_DIR" "output"
[[ ! -L "$OUT_DIR" ]] || die "--out must be an existing non-symlink directory"
OUT_DIR="$(cd -- "$OUT_DIR" && pwd -P)"
BINARY_DIR="$(cd -- "$(dirname -- "$BINARY")" && pwd -P)"
BINARY="${BINARY_DIR}/$(basename -- "$BINARY")"

paths_overlap "$OUT_DIR" "$REPO_ROOT" && die "source/output overlap"
for _git_flag in --absolute-git-dir --git-common-dir; do
  _git_meta="$(git rev-parse "$_git_flag")"
  _git_meta="$(cd -- "$_git_meta" && pwd -P)"
  paths_overlap "$OUT_DIR" "$_git_meta" && die "Git metadata/output overlap"
done
paths_overlap "$OUT_DIR" "$BINARY_DIR" && die "binary/output overlap"

DEST="${OUT_DIR}/platform-agent-${VERSION}-${ARCH}-deb"
[[ ! -e "$DEST" ]] || die "output collision: ${DEST}"

# All validation complete — only now create private output child (never delete existing out contents).
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
[[ "$(sha256_file "${STAGE}/opt/platform/agent/platform-agent-linux")" == "$BINARY_SHA256" ]] \
  || die "staged binary sha256 mismatch"
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

# Provenance via real JSON serialization; argv/path treated as data (not source interpolation).
# binary_sha256 records the packaged input digest only — not compilation provenance from source_commit.
PROVENANCE="$(
  python3 - "$VERSION" "$ARCH" "$SOURCE_COMMIT" "$SOURCE_DATE_EPOCH" "$ACTUAL_SHA" "$BIN_SIZE" \
    "$RUNTIME_VERSION" "$UNIT_SRC" "$LIFECYCLE_SRC" "$LAYOUT" <<'PY'
import hashlib, json, sys

def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

(version, arch, source_commit, source_date_epoch, binary_sha256, binary_bytes,
 runtime_version, unit_src, lifecycle_src, layout) = sys.argv[1:]
runtime = runtime_version if runtime_version != "" else None
doc = {
    "schema": "PLATFORM_AGENT_DEB_PROVENANCE_V1",
    "package": "platform-agent",
    "version": version,
    "architecture": arch,
    "source_commit": source_commit,
    "source_date_epoch": int(source_date_epoch),
    "binary_sha256": binary_sha256,
    "binary_bytes": int(binary_bytes),
    "runtime_version_input": runtime,
    "unit_sha256": sha256_file(unit_src),
    "lifecycle_sha256": sha256_file(lifecycle_src),
    "layout_sha256": sha256_file(layout),
    "signing": "UNSIGNED_INTERNAL_ONLY",
    "forbidden_checked": ["/usr/local"],
    "ebpf_enabled_by_package": False,
}
print(json.dumps(doc, indent=2))
PY
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
DEB_BYTES="$(stat -c '%s' "$DEB_PATH")"

RECEIPT="$(
  python3 - "$VERSION" "$ARCH" "$SOURCE_COMMIT" "$SOURCE_DATE_EPOCH" "$DEB_PATH" "$DEB_SHA" "$DEB_BYTES" "$ACTUAL_SHA" <<'PY'
import json, sys
(version, arch, source_commit, source_date_epoch, deb_path, deb_sha256, deb_bytes, binary_sha256) = sys.argv[1:]
doc = {
    "schema": "PLATFORM_AGENT_DEB_BUILD_RECEIPT_V1",
    "package": "platform-agent",
    "version": version,
    "architecture": arch,
    "source_commit": source_commit,
    "source_date_epoch": int(source_date_epoch),
    "deb_path": deb_path,
    "deb_sha256": deb_sha256,
    "deb_bytes": int(deb_bytes),
    "binary_sha256": binary_sha256,
    "signing": "UNSIGNED_INTERNAL_ONLY",
    "install_performed": False,
    "native_vm_lifecycle": "NOT_RUN_BY_BUILD",
}
print(json.dumps(doc, indent=2))
PY
)"
printf '%s\n' "$RECEIPT" > "${DEST}/build-receipt.json"

# Verify archive listing succeeds before absence checks.
if ! DEB_LISTING="$(dpkg-deb -c "$DEB_PATH")"; then
  die "dpkg-deb listing failed for built package"
fi
if printf '%s\n' "$DEB_LISTING" | grep -E '/usr/local(/|$)'; then
  die "built package unexpectedly contains /usr/local paths"
fi

# Emit receipt as JSON; path is argv data, never interpolated into Python source.
python3 -c 'import json,sys; print(json.dumps(json.load(open(sys.argv[1], encoding="utf-8")), sort_keys=True))' \
  "${DEST}/build-receipt.json"
