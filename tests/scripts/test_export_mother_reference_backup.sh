#!/usr/bin/env bash
# Deterministic offline test for scripts/export-mother-reference-backup.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/export-mother-reference-backup.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

[[ -x "${SCRIPT}" || -f "${SCRIPT}" ]] || fail "missing exporter script"
chmod +x "${SCRIPT}"

# --- Fixture repo with Platform_00 §3 and mother files ---
FIXTURE="${TMP}/repo"
mkdir -p "${FIXTURE}/project_documentation" "${FIXTURE}/scripts"
cp "${SCRIPT}" "${FIXTURE}/scripts/export-mother-reference-backup.sh"
chmod +x "${FIXTURE}/scripts/export-mother-reference-backup.sh"

cat > "${FIXTURE}/project_documentation/Platform_00_Document_Index.md" <<'MD'
# Index
## 3. Exact 2-file mother-reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `AGENTS.md` | rules |
| 2 | `Platform_00_Document_Index.md` | index |

## 4. Next
MD
# Self-copy index content already written; add AGENTS
echo '# AGENTS fixture' > "${FIXTURE}/project_documentation/AGENTS.md"
# Platform_00 already exists as the index file itself in the table — content already there

git -C "${FIXTURE}" init -q
git -C "${FIXTURE}" add project_documentation scripts
git -C "${FIXTURE}" -c user.email=test@example.com -c user.name=test commit -q -m 'fixture'

BACKUP_DIR="${TMP}/backups"
mkdir -p "${BACKUP_DIR}"

# Snapshot fixture tree before run
find "${FIXTURE}" -type f | sort > "${TMP}/before.txt"

(
  cd "${FIXTURE}"
  PLATFORM_BACKUP_DIR="${BACKUP_DIR}" ./scripts/export-mother-reference-backup.sh > "${TMP}/out.txt"
)

find "${FIXTURE}" -type f | sort > "${TMP}/after.txt"
diff -u "${TMP}/before.txt" "${TMP}/after.txt" >/dev/null || fail "exporter modified repository files"

COMBINED="$(find "${BACKUP_DIR}" -name 'Platform_Mother_Reference_Combined.md' | head -1)"
[[ -n "${COMBINED}" ]] || fail "combined markdown missing"
grep -q 'BEGIN PLATFORM_MOTHER_REFERENCE_BACKUP' "${COMBINED}" || fail "missing BEGIN marker"
grep -q 'END PLATFORM_MOTHER_REFERENCE_BACKUP' "${COMBINED}" || fail "missing END marker"
grep -q 'BEGIN FILE: AGENTS.md' "${COMBINED}" || fail "missing AGENTS begin"
grep -q 'git_head_sha:' "${COMBINED}" || fail "missing HEAD SHA metadata"
grep -q 'generated_utc:' "${COMBINED}" || fail "missing UTC metadata"
grep -q 'freshness_authority: git_head_sha_and_in_document_reconciliation_metadata' "${COMBINED}" || fail "missing freshness-authority metadata"
grep -q 'filesystem_mtime_authoritative: false' "${COMBINED}" || fail "filesystem mtime authority must be false"

SUMS="$(find "${BACKUP_DIR}" -name SHA256SUMS | head -1)"
[[ -f "${SUMS}" ]] || fail "SHA256SUMS missing"
ARCHIVE="$(find "${BACKUP_DIR}" -name 'platform-mother-reference-*.tar.gz' | head -1)"
[[ -f "${ARCHIVE}" ]] || fail "tar.gz missing"

pass "happy-path export"

# --- Missing file fails ---
rm -f "${FIXTURE}/project_documentation/AGENTS.md"
if PLATFORM_BACKUP_DIR="${BACKUP_DIR}/fail1" "${FIXTURE}/scripts/export-mother-reference-backup.sh" >/dev/null 2>"${TMP}/err1.txt"; then
  fail "expected missing-file failure"
fi
grep -qi 'missing' "${TMP}/err1.txt" || fail "missing-file error text unexpected"
pass "missing-file refusal"

# Restore AGENTS; inject duplicate names
cat > "${FIXTURE}/project_documentation/Platform_00_Document_Index.md" <<'MD'
# Index
## 3. Exact 2-file mother-reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `AGENTS.md` | rules |
| 2 | `AGENTS.md` | duplicate |

## 4. Next
MD
echo '# AGENTS fixture' > "${FIXTURE}/project_documentation/AGENTS.md"
if PLATFORM_BACKUP_DIR="${BACKUP_DIR}/fail2" "${FIXTURE}/scripts/export-mother-reference-backup.sh" >/dev/null 2>"${TMP}/err2.txt"; then
  fail "expected duplicate-name failure"
fi
grep -qi 'duplicate' "${TMP}/err2.txt" || fail "duplicate error text unexpected"
pass "duplicate-name refusal"

# Ambiguous path name
cat > "${FIXTURE}/project_documentation/Platform_00_Document_Index.md" <<'MD'
# Index
## 3. Exact 1-file mother-reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `../etc/passwd` | bad |

## 4. Next
MD
if PLATFORM_BACKUP_DIR="${BACKUP_DIR}/fail3" "${FIXTURE}/scripts/export-mother-reference-backup.sh" >/dev/null 2>"${TMP}/err3.txt"; then
  fail "expected ambiguous-name failure"
fi
grep -qi 'ambiguous' "${TMP}/err3.txt" || fail "ambiguous error text unexpected"
pass "ambiguous-name refusal"

echo "ALL TESTS PASSED"
