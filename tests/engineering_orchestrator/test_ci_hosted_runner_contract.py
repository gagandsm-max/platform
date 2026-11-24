"""Issue #343 hosted-runner CI workflow static regression contract (stdlib unittest).

Static-check limits: inspects only the canonical ``.github/workflows/platform-ci.yml``
shape (unique ``jobs:``, two-space ``  job-id:``, six-space ``      - `` steps, and
``&anchor``/``*alias`` step reuse). Does NOT implement YAML 1.2 or execute ``Makefile``,
``tests/run-all-tests.sh``, Docker, or GitHub Actions; real YAML/CI gates stay separate.
Unmigrated self-hosted ``main`` is expected to fail hosted checks until combined.
"""
from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH, MAKEFILE_PATH, RUN_ALL_PATH = (
    ROOT / ".github/workflows/platform-ci.yml", ROOT / "Makefile", ROOT / "tests/run-all-tests.sh"
)
CANONICAL_JOBS = {
    "static-contracts": "Static contracts", "unit": "Unit tests",
    "ui-build": "UI test and production build", "integration-security": "Integration, PostgreSQL, contract, security",
}
COMPOSE_JOBS = ("static-contracts", "unit", "integration-security")
_ECHO_ONLY_RE = re.compile(r"^(?:echo|printf)\b(?![^#\n]*(?:\||&&|\|\||;))")
_OP_COMPOSE_RE = re.compile(
    r"\bdocker\s+compose\s+(?!-?-?version\b)|run-kafka-storage-contract-test\.sh\b"
    r"|deployment-preflight\.sh\b|\bmake\s+(?:test-static-contracts|test-unit|test-integration|test-clickhouse-replay-migration)\b"
)


def _active_lines(text: str, *, skip_echo: bool = True) -> list[str]:
    return [ln for raw in text.splitlines() if (ln := raw.strip()) and not ln.startswith("#") and not (skip_echo and _ECHO_ONLY_RE.match(ln))]


def _step_run_lines(step: str) -> list[str]:
    m = re.search(r"(?m)^(?P<ind> +(?:-\s+)?)run:\s*(?P<inline>.*)$", step)
    if not m:
        return []
    inline = m.group("inline").strip()
    if inline and not re.match(r"^[|>][+-]?\d*(?:\s+#.*)?$", inline):
        return _active_lines(inline)
    ind, body = len(m.group("ind")), []
    for raw in step[m.end() :].splitlines():
        if raw.strip() and len(raw) - len(raw.lstrip(" ")) <= ind:
            break
        body.append(raw)
    return _active_lines("\n".join(body))


def _parse_workflow(text: str) -> tuple[str, dict[str, str], dict[str, list[str]], list[str]]:
    norm = text.replace("\r\n", "\n")
    hits = list(re.finditer(r"(?m)^jobs:\s*(?:#.*)?$", norm))
    if len(hits) != 1:
        return "", {}, {}, [f"Expected one top-level 'jobs:' block, found {len(hits)}."]
    header, tail = norm[: hits[0].start()], norm[hits[0].end() :]
    end_m = re.search(r"(?m)^[A-Za-z0-9_.-]+:", tail)
    body = tail[: end_m.start()] if end_m else tail
    matches = list(re.finditer(r"(?m)^  ([A-Za-z0-9_-]+):\s*(?:#.*)?$", body))
    jobs, job_steps, errs = {}, {}, []
    anchors = {m.group(1): m.group(2) for m in re.finditer(r"(?ms)^x-[A-Za-z0-9_-]+:\s*&([A-Za-z0-9_-]+)\s*(?:#.*)?\n((?:[ \t]+.*\n|\n)*)", header)}
    for i, m in enumerate(matches):
        jid = m.group(1)
        block = body[m.end() : matches[i + 1].start() if i + 1 < len(matches) else len(body)]
        if jid in jobs:
            errs.append(f"Duplicate job ID: {jid!r}.")
        jobs[jid] = block
        sm = re.search(r"(?m)^    steps:\s*(?:#.*)?$", block)
        if not sm:
            errs.append(f"Job {jid!r} missing 'steps:'.")
            job_steps[jid] = []
            continue
        steps: list[str] = []
        for raw in (s for s in re.split(r"(?m)(?=^      - )", block[sm.end() :]) if s.startswith("      - ")):
            first = raw.splitlines()[0]
            if am := re.match(r"^      - &([A-Za-z0-9_-]+)\s*(?:#.*)?$", first):
                anchors[am.group(1)] = raw
                steps.append(raw)
            elif al := re.match(r"^      - \*([A-Za-z0-9_-]+)\s*(?:#.*)?$", first):
                steps.append(anchors[al.group(1)]) if al.group(1) in anchors else errs.append(f"Undefined alias '*{al.group(1)}' in {jid!r}.")
            else:
                steps.append(raw)
        job_steps[jid] = steps
    return header, jobs, job_steps, errs


def evaluate_contract(wf: str, makefile: str, run_all: str) -> dict[str, list[str]]:
    header, jobs, job_steps, p_errs = _parse_workflow(wf)
    res: dict[str, list[str]] = {k: list(p_errs) for k in ("runners", "security", "compose", "integration")}

    # (1) Canonical job IDs, names, and hosted ubuntu-24.04 selector
    if set(jobs) != set(CANONICAL_JOBS):
        res["runners"].append(f"Expected exact job IDs {sorted(CANONICAL_JOBS)}, got {sorted(jobs)}.")
    for jid, exp_name in CANONICAL_JOBS.items():
        if not (blk := jobs.get(jid, "")):
            continue
        nm = re.search(r"(?m)^    name:\s*['\"]?([^'\"#\n]+?)['\"]?\s*(?:#.*)?$", blk)
        if not nm or nm.group(1).strip() != exp_name:
            res["runners"].append(f"Job {jid!r} must have name {exp_name!r}.")
        rm = re.search(r"(?m)^    runs-on:\s*(.+?)\s*(?:#.*)?$", blk)
        sel = rm.group(1).strip().strip("'\"") if rm else ""
        if sel != "ubuntu-24.04" or "self-hosted" in sel.lower() or "platform-ci" in sel.lower():
            res["runners"].append(f"Job {jid!r} must use 'runs-on: ubuntu-24.04' (no self-hosted), got {sel!r}.")

    # (2) Triggers, read-only permissions, and pinned checkout with persist-credentials: false
    hdr_active = "\n".join(_active_lines(header, skip_echo=False))
    if "pull_request_target" in hdr_active:
        res["security"].append("Privileged trigger 'pull_request_target' is prohibited.")
    on_blk = m.group(1) if (m := re.search(r"(?ms)^on:\s*(?:#.*)?\n((?:  .*\n|\n)*)", header)) else ""
    # Reject unauthorized events, including extra keys/duplicates: an allowlist beats
    # mere presence checks for pull_request and push.
    events = re.findall(r"(?m)^  ([A-Za-z_][A-Za-z0-9_]*):\s*(?:#.*)?$", on_blk)
    if sorted(events) != ["pull_request", "push"]:
        res["security"].append(f"Workflow events must be exactly pull_request and push; got {events!r}.")
    pblk = m.group(1) if (m := re.search(r"(?ms)^  push:\s*(?:#.*)?\n((?:    .*\n|\n)*)", on_blk)) else ""
    main_only = bool(re.search(r"(?m)^    branches:\s*\[\s*['\"]?main['\"]?\s*\]\s*(?:#.*)?$", pblk) or (
        re.search(r"(?m)^    branches:\s*(?:#.*)?$", pblk) and re.findall(r"(?m)^    (?:  )?-\s+['\"]?([^'\"#\n]+?)['\"]?\s*(?:#.*)?$", pblk) == ["main"]
    ))
    if not main_only or "branches-ignore" in pblk:
        res["security"].append("'on.push' must be restricted solely to branch 'main'.")
    perm_blk = m.group(0) if (m := re.search(r"(?ms)^permissions:.*?(?=\n\S|\Z)", header)) else ""
    if not re.search(r"(?m)^  contents:\s*read\b", perm_blk):
        res["security"].append("Workflow must declare 'permissions: contents: read'.")
    if re.search(r"(?i)(?::\s*write\b|write-all)", perm_blk):
        res["security"].append("Workflow permissions must not grant write access.")
    for jid, blk in jobs.items():
        jperm = m.group(0) if (m := re.search(r"(?ms)^    permissions:.*?(?=\n    \S|\Z)", blk)) else ""
        if re.search(r"(?i)(?::\s*write\b|write-all)", jperm):
            res["security"].append(f"Job {jid!r} must not grant write permissions.")
        cos = [s for s in job_steps.get(jid, []) if re.search(r"(?m)^\s+(?:-\s+)?uses:\s*actions/checkout@", s)]
        if not cos:
            res["security"].append(f"Job {jid!r} missing 'actions/checkout'.")
        for s in cos:
            if not re.search(r"actions/checkout@[0-9a-f]{40}\b", s):
                res["security"].append(f"Job {jid!r} 'actions/checkout' must pin a 40-hex SHA.")
            if not re.search(r"(?m)^\s+persist-credentials:\s*false\b", s):
                res["security"].append(f"Job {jid!r} 'actions/checkout' must set 'persist-credentials: false'.")

    # (3) Official Compose 5.5.0 pin + SHA-256 check before operational use (supports &/* aliases)
    for jid in COMPOSE_JOBS:
        pinned = False
        for step in job_steps.get(jid, []):
            rl = _step_run_lines(step)
            op = next((i for i, ln in enumerate(rl) if _OP_COMPOSE_RE.search(ln)), None)
            pre = "\n".join(rl if op is None else rl[:op])
            ctx = f"{hdr_active}\n" + "\n".join(ln for ln in _active_lines(step, skip_echo=False) if not ln.startswith("name:"))
            if (re.search(r"(?<![0-9.])5\.5\.0(?![0-9.])", ctx) and re.search(r"\b(?:curl|wget)\b", pre)
                and re.search(r"(?:github\.com/docker/compose|docker-compose)", pre) and re.search(r"\bsha256sum\b", pre)
                and re.search(r"(?:docker\s+compose|docker-compose)\s+(?:--version|version)\b", pre)):
                pinned = True
            if op is not None:
                break
        if not pinned:
            res["compose"].append(f"Job {jid!r} must install official Compose 5.5.0 with SHA-256 check before use.")

    # (4)-(6) Node pre-pull, integration/Day02/contract/security/Kafka/ClickHouse, diagnostics & cleanup
    prepulled, saw_kafka, has_diag, cleanup, seen = False, False, False, None, set()
    req = {
        "Kafka storage contract": r"run-kafka-storage-contract-test\.sh\b", "ClickHouse replay migration": r"\bmake\s+test-clickhouse-replay-migration\b",
        "20-test integration suite": r"\bmake\s+test-integration\b", "Day02 PostgreSQL suite": r"tests/run-core-day02-tests\.sh\b",
        "API/data contract suite": r"\bmake\s+test-contract\b", "Security smoke suite": r"\bmake\s+test-security-smoke\b",
    }
    for step in job_steps.get("integration-security", []):
        rl, joined = _step_run_lines(step), "\n".join(_step_run_lines(step))
        res["integration"].extend(f"Blanket {b!r} prohibited." for b in ("docker system prune", "docker volume prune", "docker compose down -v") if b in joined)
        if not saw_kafka:
            ki = next((i for i, ln in enumerate(rl) if "run-kafka-storage-contract-test.sh" in ln), None)
            if any(re.search(r"\bdocker\s+pull\b.*\bnode:20-alpine\b", ln) for ln in (rl if ki is None else rl[:ki])):
                prepulled = True
            saw_kafka = ki is not None
        cond = m.group(1).strip().lower() if (m := re.search(r"(?m)^(?:      - |        )if:\s*(.+?)\s*(?:#.*)?$", step)) else ""
        if cond not in ("false", "${{ false }}") and not re.search(r"(?m)^(?:      - |        )continue-on-error:\s*true\b", step):
            for lbl, pat in req.items():
                for ln in rl:
                    if re.search(pat, ln):
                        res["integration"].append(f"{lbl} masked with '|| true'.") if re.search(r"\|\|\s*true\b", ln) else seen.add(lbl)
        if "failure()" in cond and "docker compose ps" in joined and "docker compose logs" in joined:
            has_diag = True
        if "always()" in cond and ("docker volume rm" in joined or "deployment-preflight.sh down" in joined):
            cleanup = (step, joined)
    if not (saw_kafka and prepulled):
        res["integration"].append("Integration job must run 'docker pull node:20-alpine' before Kafka storage contract.")
    res["integration"].extend(f"Missing unmasked {lbl}." for lbl in req if lbl not in seen)
    if not has_diag:
        res["integration"].append("Missing 'if: failure()' diagnostics (ps + logs).")
    if cleanup is None:
        res["integration"].append("Missing 'if: always()' owned cleanup step.")
    else:
        raw_step, joined = cleanup
        res["integration"].extend(f"Cleanup missing {t!r}." for t in ("github.run_id", "github.run_attempt") if t not in raw_step)
        res["integration"].extend(f"Cleanup missing guard {t!r}." for t in (
            "GITHUB_ACTIONS", "deployment-preflight.sh down", "docker volume rm",
            "issue223.owner", "com.docker.compose.project", "com.docker.compose.volume", "--filter",
        ) if t not in joined)
    if not re.search(r"(?ms)^test-integration:.*?\n(?:\t[^\n]*\n)*?\t[^\n]*tests/run-all-tests\.sh\b", makefile):
        res["integration"].append("Makefile 'test-integration' must invoke 'tests/run-all-tests.sh'.")
    suite = "\n".join(_active_lines(run_all))
    res["integration"].extend(f"tests/run-all-tests.sh missing Test 15/20 token {t!r}." for t in (
        "TestProtectionAndRecoveryAfterFreeSpace|TestFixedWatermarkCannotBeChanged",
        "Persistence, Structured JSON Logs, and Workstation Isolation", "platform-isolation-probe-",
        "FOREIGN_PROBE_STARTED", "check-host-runtime-isolation.sh", "--synthetic-test",
        'PERSIST_OK}" = "true"', 'LOGS_OK}" = "true"', 'FOREIGN_OK}" = "true"', 'HOST_ISOLATION_OK}" = "true"',
    ) if t not in suite)
    if "agent-e2e-" in suite:
        res["integration"].append("tests/run-all-tests.sh must not allowlist 'agent-e2e-*' in Test 20.")
    return res


def _job(jid: str, name: str, steps: str) -> str:
    return f"  {jid}:\n    name: {name}\n    runs-on: ubuntu-24.04\n    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n        with:\n          persist-credentials: false\n{steps}"


COMPLIANT_FIXTURE = (
    "name: Platform CI\non:\n  pull_request:\n  push:\n    branches: [main]\npermissions:\n  contents: read\njobs:\n"
    + _job("static-contracts", "Static contracts",
           "      - &install-compose\n        name: Install Compose 5.5.0\n        run: |\n"
           "          curl -fsSL https://github.com/docker/compose/releases/download/v5.5.0/docker-compose-linux-x86_64 -o /tmp/dc\n"
           "          echo \"0000000000000000000000000000000000000000000000000000000000000000  /tmp/dc\" | sha256sum -c -\n"
           "          docker compose version --short\n      - run: |\n          docker compose config -q\n          make test-static-contracts\n")
    + _job("unit", "Unit tests", "      - *install-compose\n      - run: make test-unit\n")
    + _job("ui-build", "UI test and production build", "      - run: docker build --no-cache -t platform-ui:ci ui/\n")
    + _job("integration-security", "Integration, PostgreSQL, contract, security",
           "      - *install-compose\n      - run: docker pull node:20-alpine\n      - run: |\n"
           "          ./tests/run-kafka-storage-contract-test.sh\n          make test-clickhouse-replay-migration\n"
           "          make test-integration\n          ./tests/run-core-day02-tests.sh\n          make test-contract\n          make test-security-smoke\n"
           "      - if: failure()\n        run: |\n          docker compose ps || true\n          docker compose logs --tail=250 || true\n"
           "      - if: always()\n        env:\n          RUN_ID: ${{ github.run_id }}\n          RUN_ATTEMPT: ${{ github.run_attempt }}\n"
           "        run: |\n          [[ \"${GITHUB_ACTIONS:-}\" == \"true\" ]] || exit 1\n          bash deploy/docker/kafka/deployment-preflight.sh down\n"
           "          docker volume inspect -f '{{index .Labels \"issue223.owner\"}}' v\n"
           "          docker volume inspect -f '{{index .Labels \"com.docker.compose.project\"}}|{{index .Labels \"com.docker.compose.volume\"}}' v\n"
           "          docker ps -aq --filter \"volume=v\"\n          docker volume rm v\n")
)


class TestCIHostedRunnerContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.res = evaluate_contract(WORKFLOW_PATH.read_text("utf-8"), MAKEFILE_PATH.read_text("utf-8"), RUN_ALL_PATH.read_text("utf-8"))

    def test_canonical_jobs_and_hosted_runners(self) -> None: self.assertEqual(self.res["runners"], [])
    def test_triggers_permissions_and_checkout(self) -> None: self.assertEqual(self.res["security"], [])
    def test_compose_5_5_0_pin_and_sha256_before_use(self) -> None: self.assertEqual(self.res["compose"], [])
    def test_integration_suites_node_prepull_diagnostics_and_cleanup(self) -> None: self.assertEqual(self.res["integration"], [])


class TestCIHostedRunnerContractMutations(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.makefile, cls.run_all = MAKEFILE_PATH.read_text("utf-8"), RUN_ALL_PATH.read_text("utf-8")

    def test_anchored_compliant_fixture_passes(self) -> None:
        self.assertEqual({k: v for k, v in evaluate_contract(COMPLIANT_FIXTURE, self.makefile, self.run_all).items() if v}, {})

    def test_mutation_removing_hosted_label_fails(self) -> None:
        mut = COMPLIANT_FIXTURE.replace("runs-on: ubuntu-24.04", "runs-on: [self-hosted, Linux]", 1)
        self.assertTrue(any("ubuntu-24.04" in e for e in evaluate_contract(mut, self.makefile, self.run_all)["runners"]))

    def test_mutation_removing_node_prepull_fails(self) -> None:
        mut = COMPLIANT_FIXTURE.replace("run: docker pull node:20-alpine", 'run: echo "docker pull node:20-alpine"')
        self.assertTrue(any("node:20-alpine" in e for e in evaluate_contract(mut, self.makefile, self.run_all)["integration"]))

    def test_mutation_extra_workflow_events_rejected(self) -> None:
        for extra in ("workflow_dispatch", "schedule", "repository_dispatch", "pull_request_target"):
            with self.subTest(event=extra):
                mut = COMPLIANT_FIXTURE.replace("  pull_request:\n", f"  pull_request:\n  {extra}:\n", 1)
                errs = evaluate_contract(mut, self.makefile, self.run_all)["security"]
                self.assertTrue(any("Workflow events must be exactly" in e for e in errs), errs)

    def test_mutation_missing_or_duplicate_event_rejected(self) -> None:
        for name in ("pull_request", "push"):
            with self.subTest(missing=name):
                if name == "push":
                    mut = COMPLIANT_FIXTURE.replace("  push:\n    branches: [main]\n", "", 1)
                else:
                    mut = COMPLIANT_FIXTURE.replace("  pull_request:\n", "", 1)
                errs = evaluate_contract(mut, self.makefile, self.run_all)["security"]
                self.assertTrue(any("Workflow events must be exactly" in e for e in errs), errs)
        dup = COMPLIANT_FIXTURE.replace("  pull_request:\n", "  pull_request:\n  pull_request:\n", 1)
        errs = evaluate_contract(dup, self.makefile, self.run_all)["security"]
        self.assertTrue(any("Workflow events must be exactly" in e for e in errs), errs)

    def test_mutation_privileged_trigger_and_write_permission_fail(self) -> None:
        mut = (
            COMPLIANT_FIXTURE.replace("  pull_request:\n", "  pull_request:\n  pull_request_target:\n")
            .replace("  contents: read\n", "  contents: write\n")
            .replace("persist-credentials: false", "persist-credentials: true", 1)
        )
        errs = evaluate_contract(mut, self.makefile, self.run_all)["security"]
        self.assertTrue(any("pull_request_target" in e for e in errs) and any("write" in e for e in errs) and any("persist-credentials" in e for e in errs), errs)


if __name__ == "__main__":
    unittest.main()
