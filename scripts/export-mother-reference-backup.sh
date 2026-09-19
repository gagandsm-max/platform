#!/usr/bin/env bash
# Export the authoritative Mother Reference set discovered from Platform_00 §3.
# Does not modify repository files. Writes artifacts under ${PLATFORM_BACKUP_DIR:-$HOME/platform-backups}/.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INDEX_FILE="${ROOT_DIR}/project_documentation/Platform_00_Document_Index.md"
DOC_DIR="${ROOT_DIR}/project_documentation"
BACKUP_ROOT="${PLATFORM_BACKUP_DIR:-${HOME}/platform-backups}"

if [[ ! -f "${INDEX_FILE}" ]]; then
  echo "ERROR: missing ${INDEX_FILE}" >&2
  exit 1
fi

if [[ ! -d "${DOC_DIR}" ]]; then
  echo "ERROR: missing ${DOC_DIR}" >&2
  exit 1
fi

PARSE_OUT="$(mktemp)"
PARSE_ERR="$(mktemp)"
cleanup_parse() { rm -f "${PARSE_OUT}" "${PARSE_ERR}"; }
trap cleanup_parse EXIT

set +e
python3 - "${INDEX_FILE}" >"${PARSE_OUT}" 2>"${PARSE_ERR}" <<'PY'
import re
import sys
from pathlib import Path

index_path = Path(sys.argv[1])
text = index_path.read_text(encoding="utf-8")
section = re.search(
    r"^## 3\. Exact \d+-file mother-reference set\s*\n(?P<body>.*?)(?=^## |\Z)",
    text,
    re.MULTILINE | re.DOTALL,
)
if section is None:
    print("ERROR: Platform_00 §3 mother-reference table was not found", file=sys.stderr)
    sys.exit(2)
files = re.findall(r"^\|\s*\d+\s*\|\s*`([^`]+)`\s*\|", section.group("body"), re.MULTILINE)
if not files:
    print("ERROR: Platform_00 §3 mother-reference table has no file rows", file=sys.stderr)
    sys.exit(2)
# Refuse ambiguous / duplicate basenames
seen = set()
dupes = []
for name in files:
    if "/" in name or "\\" in name or name.strip() != name or not name:
        print(f"ERROR: refused ambiguous mother-reference name: {name!r}", file=sys.stderr)
        sys.exit(3)
    if name in seen:
        dupes.append(name)
    seen.add(name)
if dupes:
    print(
        "ERROR: duplicate mother-reference names in Platform_00 §3: " + ", ".join(dupes),
        file=sys.stderr,
    )
    sys.exit(3)
for name in files:
    print(name)
PY
PARSE_RC=$?
set -e
if [[ "${PARSE_RC}" -ne 0 ]]; then
  cat "${PARSE_ERR}" >&2 || true
  exit "${PARSE_RC}"
fi
mapfile -t MOTHER_FILES < "${PARSE_OUT}"
cleanup_parse
trap - EXIT
if ((${#MOTHER_FILES[@]} == 0)); then
  echo "ERROR: parsed zero mother-reference files from Platform_00 §3" >&2
  exit 2
fi

missing=()
for name in "${MOTHER_FILES[@]}"; do
  if [[ ! -f "${DOC_DIR}/${name}" ]]; then
    missing+=("${name}")
  fi
done
if ((${#missing[@]})); then
  echo "ERROR: mother-reference file(s) missing under project_documentation/:" >&2
  printf '  - %s\n' "${missing[@]}" >&2
  exit 4
fi

HEAD_SHA="$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || echo UNKNOWN)"
UTC_STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
STAMP_LABEL="${UTC_STAMP}_${HEAD_SHA:0:12}"
OUT_DIR="${BACKUP_ROOT}/mother-reference-${STAMP_LABEL}"
mkdir -p "${OUT_DIR}"

COMBINED="${OUT_DIR}/Platform_Mother_Reference_Combined.md"
SUMS="${OUT_DIR}/SHA256SUMS"
ARCHIVE="${BACKUP_ROOT}/platform-mother-reference-${STAMP_LABEL}.tar.gz"

{
  echo "<!-- BEGIN PLATFORM_MOTHER_REFERENCE_BACKUP -->"
  echo "# Platform Mother Reference Backup"
  echo
  echo "- generated_utc: ${UTC_STAMP}"
  echo "- git_head_sha: ${HEAD_SHA}"
  echo "- freshness_authority: git_head_sha_and_in_document_reconciliation_metadata"
  echo "- filesystem_mtime_authoritative: false"
  echo "- source_index: project_documentation/Platform_00_Document_Index.md §3"
  echo "- file_count: ${#MOTHER_FILES[@]}"
  echo
  for name in "${MOTHER_FILES[@]}"; do
    echo "<!-- BEGIN FILE: ${name} -->"
    cat "${DOC_DIR}/${name}"
    echo
    echo "<!-- END FILE: ${name} -->"
    echo
  done
  echo "<!-- END PLATFORM_MOTHER_REFERENCE_BACKUP -->"
} > "${COMBINED}"

(
  cd "${OUT_DIR}"
  : > SHA256SUMS
  sha256sum "Platform_Mother_Reference_Combined.md" >> SHA256SUMS
  for name in "${MOTHER_FILES[@]}"; do
    # Copy individuals for integrity; do not touch the repo tree.
    cp -a "${DOC_DIR}/${name}" "${OUT_DIR}/${name}"
    sha256sum "${name}" >> SHA256SUMS
  done
)

tar -C "${BACKUP_ROOT}" -czf "${ARCHIVE}" "mother-reference-${STAMP_LABEL}"

echo "Wrote ${COMBINED}"
echo "Wrote ${SUMS}"
echo "Wrote ${ARCHIVE}"
echo "files=${#MOTHER_FILES[@]} head=${HEAD_SHA} utc=${UTC_STAMP}"
