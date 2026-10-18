#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

NODE_IMAGE="node:20-alpine"

echo "========================================================================"
echo "Platform Static Contract Validation"
echo "========================================================================"

docker run --rm "${NODE_IMAGE}" node --version

validators=(
  "device-packs/tools/validate.mjs"
  "schemas/provisioning/tools/validate.mjs"
  "schemas/presentation/tools/validate.mjs"
  "schemas/edge/tools/validate.mjs"
  "schemas/supportability/tools/validate.mjs"
)

for validator in "${validators[@]}"; do
  if [ ! -f "${validator}" ]; then
    echo "[ERROR] Missing validator: ${validator}"
    exit 1
  fi

  echo "--> Syntax check: ${validator}"
  docker run --rm \
    -v "${ROOT_DIR}:/repo:ro" \
    -w /repo \
    "${NODE_IMAGE}" \
    node --check "${validator}"

  echo "--> Execute validator: ${validator}"
  docker run --rm \
    -v "${ROOT_DIR}:/repo:ro" \
    -w /repo \
    "${NODE_IMAGE}" \
    node "${validator}"
done

echo "--> Parse all governed contract JSON"
docker run --rm -i \
  -v "${ROOT_DIR}:/repo:ro" \
  -w /repo \
  "${NODE_IMAGE}" \
  node - <<'NODE'
const fs = require('fs');
const path = require('path');

const roots = ['device-packs', 'schemas/provisioning', 'schemas/presentation', 'schemas/edge', 'schemas/supportability'];
let count = 0;

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full);
    } else if (entry.isFile() && entry.name.endsWith('.json')) {
      const text = fs.readFileSync(full, 'utf8');
      try {
        JSON.parse(text);
        count += 1;
      } catch (error) {
        console.error(`[ERROR] Invalid JSON: ${full}: ${error.message}`);
        process.exitCode = 1;
      }
    }
  }
}

for (const root of roots) {
  if (!fs.existsSync(root)) {
    console.error(`[ERROR] Missing contract root: ${root}`);
    process.exit(1);
  }
  walk(root);
}

if (process.exitCode) process.exit(process.exitCode);
console.log(`CONTRACT JSON PARSE OK: ${count} file(s).`);
NODE

for dir in device-packs schemas/provisioning schemas/presentation schemas/edge schemas/supportability; do
  if find "${dir}" -name package.json -o -name package-lock.json | grep -q .; then
    echo "[ERROR] Contract validator area must remain dependency-free: ${dir}"
    exit 1
  fi
done

echo "--> Dev disk hygiene guardrails"
bash "${ROOT_DIR}/tests/test-dev-disk-hygiene.sh"
bash "${ROOT_DIR}/tests/test-kafka-health-probe-bounds.sh"

echo "--> Mother Reference validator unit tests"
python3 -m unittest discover -s "${ROOT_DIR}/project_documentation/tools" -p 'test_validate_mother_references.py' -v

echo "--> Mother Reference structural validator (repository)"
python3 "${ROOT_DIR}/project_documentation/tools/validate_mother_references.py"

echo "========================================================================"
echo "STATIC CONTRACT VALIDATION PASSED"
echo "========================================================================"
