"""Issue #343 hosted-runner CI workflow static regression contract (stdlib unittest).

Static-check limits: inspects only the canonical ``.github/workflows/platform-ci.yml``
shape (unique ``jobs:``, two-space ``  job-id:``, six-space ``      - `` steps, and
``&anchor``/``*alias`` step reuse). Does NOT implement YAML 1.2 or execute ``Makefile``,
``tests/run-all-tests.sh``, Docker, or GitHub Actions; real YAML/CI gates stay separate.
Unmigrated self-hosted ``main`` is expected to fail hosted checks until combined.

Behavioral watchdog cases are mock-only (stubbed ``sample``/``snapshot``, no Docker daemon).
"""
from __future__ import annotations

import os
import re
import signal
import subprocess
import tempfile
import textwrap
import time
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
_DISK_HEREDOC_RE = re.compile(
    r"""cat\s+>\s+"\$RUNNER_TEMP/platform-ci-disk"\s+<<'BASH'\n(?P<body>.*?)\n\s*BASH\b""",
    re.DOTALL,
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


def _extract_platform_ci_disk_script(workflow: str) -> str:
    m = _DISK_HEREDOC_RE.search(workflow)
    if not m:
        raise AssertionError("platform-ci-disk BASH heredoc not found in workflow")
    return textwrap.dedent(m.group("body"))


def _step_run_raw(step: str) -> str:
    """Extract the run script body, preserving echo diagnostics used as fail-closed evidence."""
    m = re.search(r"(?m)^(?P<ind> +(?:-\s+)?)run:\s*(?P<inline>.*)$", step)
    if not m:
        return ""
    inline = m.group("inline").strip()
    if inline and not re.match(r"^[|>][+-]?\d*(?:\s+#.*)?$", inline):
        return inline + "\n"
    ind, body = len(m.group("ind")), []
    for raw in step[m.end() :].splitlines():
        if raw.strip() and len(raw) - len(raw.lstrip(" ")) <= ind:
            break
        body.append(raw)
    return textwrap.dedent("\n".join(body)) + ("\n" if body else "")


def _whitespace_step_script(workflow: str) -> str:
    _, _, steps, errors = _parse_workflow(workflow)
    if errors:
        raise AssertionError(errors)
    step = next(v for v in steps["static-contracts"] if "Verify changed-file whitespace" in v)
    return _step_run_raw(step)


def evaluate_contract(wf: str, makefile: str, run_all: str) -> dict[str, list[str]]:
    header, jobs, job_steps, p_errs = _parse_workflow(wf)
    res: dict[str, list[str]] = {
        k: list(p_errs) for k in ("runners", "security", "compose", "integration", "watchdog")
    }

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
    on_blk = m.group(1) if (m := re.search(r"(?m)^on:[ \t]*(?:#[^\n]*)?\n((?:  [^\n]*\n|\n)*)", header)) else ""
    # Reject unauthorized events, including extra keys/duplicates: an allowlist beats
    # mere presence checks for pull_request and push.
    # Fail closed on all direct event declarations. Valid YAML inline mapping,
    # quoted-key or alias forms must not bypass the exact permitted event set.
    event_declarations = [line for line in on_blk.splitlines()
                          if line.startswith("  ") and not line.startswith("    ")
                          and line.strip() and not line.lstrip().startswith("#")]
    events = []
    for declaration in event_declarations:
        key = re.match(r"  ([A-Za-z_][A-Za-z0-9_]*)[ \t]*:", declaration)
        if key:
            events.append(key.group(1))
        if not re.fullmatch(r"  (pull_request|push):[ \t]*(?:#.*)?", declaration):
            res["security"].append(f"Unauthorized or unrecognized workflow event: {declaration!r}.")
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
    # PR-only cancel-in-progress; push/main must not cancel previously accepted main evidence.
    if not re.search(
        r"(?m)^concurrency:\s*(?:#.*)?\n(?:  .*\n)*?"
        r"  cancel-in-progress:\s*\$\{\{\s*github\.event_name\s*==\s*'pull_request'\s*\}\}",
        header,
    ):
        res["security"].append("concurrency.cancel-in-progress must be PR-only (not unconditional true).")
    if re.search(r"(?m)^  cancel-in-progress:\s*true\b", header):
        res["security"].append("concurrency.cancel-in-progress must not be unconditional true.")
    if not re.search(r"github\.event\.pull_request\.number\s*\|\|\s*github\.run_id", header):
        res["security"].append("concurrency.group must isolate push/main by github.run_id.")
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

    # Watchdog: setsid isolation, process-group TERM→KILL reap, sticky failure before TERM,
    # no hardcoded sampler_status=143. Only when hosted disk wrapper heredoc is present.
    integ = jobs.get("integration-security", "")
    disk_match = _DISK_HEREDOC_RE.search(wf)
    if disk_match:
        disk = textwrap.dedent(disk_match.group("body"))
        for token, label in (
            (r"exec\s+setsid\b", "setsid process-group isolation via exec"),
            (r"\bprocess_group_alive\b", "process-group liveness check"),
            (r"\bpid_has_exited\b", "zombie-safe sampler exit detection"),
            (r"kill\s+-KILL\s+--\s+\"-\$\{workload_pid\}\"", "SIGKILL process-group escalation"),
            (r"\breap_workload_group\b", "workload process-group reap"),
            (r"\bstop_helper_pid\b", "helper/sampler KILL reap"),
            (r"sampler_failed", "actual sampler failure status tracking"),
            (r"\bescalator=\$", "sampler-failure SIGKILL escalator"),
            (r"\bpublish_sampler_failure\b", "sticky sampler failure before TERM"),
            (r"\bpublish_cutoff_failure\b", "sticky cutoff failure before TERM"),
            (r"\bwork_cutoff_reached\b", "active work-cutoff predicate"),
            (r"sampler_fail_marker", "sticky failure marker path"),
            (r"sampler_stop_marker", "intentional sampler-stop marker"),
            (r"cutoff_fail_marker", "sticky cutoff failure marker path"),
            (r"PLATFORM_CI_WORK_CUTOFF_EPOCH", "enforceable work cutoff gate"),
        ):
            if not re.search(token, disk):
                res["watchdog"].append(f"platform-ci-disk missing {label}.")
        if re.search(r"(?m)^\s*sampler_status=143\b", disk):
            res["watchdog"].append("platform-ci-disk must not hardcode sampler_status=143.")
        if re.search(r"sampler_status\s*!=\s*143", disk):
            res["watchdog"].append("platform-ci-disk must not treat sampler_status!=143 as the success oracle.")
        # Sticky publish must precede TERM delivery in the sampler failure path.
        fail_path = re.search(
            r"if ! sample; then(.*?)exit 1",
            disk,
            flags=re.DOTALL,
        )
        if not fail_path or "publish_sampler_failure" not in fail_path.group(1) or not re.search(
            r"publish_sampler_failure[\s\S]*?stop_workload_signal",
            fail_path.group(1),
        ):
            res["watchdog"].append(
                "platform-ci-disk must publish sticky sampler failure before stop_workload_signal."
            )
        # Active aggregate deadline (not start-of-command only): monitor while parent waits,
        # sticky-publish cutoff failure, then TERM (and KILL if needed).
        if not (
            re.search(r"cutoff_monitor=\$!", disk)
            and re.search(r"publish_cutoff_failure\s+75", disk)
            and re.search(
                r"publish_cutoff_failure\s+75\s*\n\s*stop_workload_signal",
                disk,
            )
        ):
            res["watchdog"].append(
                "platform-ci-disk must enforce PLATFORM_CI_WORK_CUTOFF_EPOCH during active work "
                "(sticky publish_cutoff_failure before TERM), not only at command start."
            )
        tm = re.search(r"(?m)^    timeout-minutes:\s*(\d+)\s*(?:#.*)?$", integ)
        if not tm:
            res["watchdog"].append("integration-security missing timeout-minutes.")
        elif int(tm.group(1)) != 50:
            res["watchdog"].append(
                "integration-security timeout-minutes must remain 50 without Master "
                f"approval; got {tm.group(1)}."
            )
        if "PLATFORM_CI_WORK_CUTOFF_EPOCH" not in integ or "PLATFORM_CI_RESERVED_TEARDOWN_SECONDS" not in integ:
            res["watchdog"].append(
                "integration-security must establish PLATFORM_CI_WORK_CUTOFF_EPOCH with reserved teardown."
            )
        # Timing arithmetic: work + reserved teardown must equal the 50m ceiling (not 60).
        if not re.search(
            r"reserved_teardown_seconds=\$\(\(6 \* 60\)\).*work_budget_seconds=\$\(\(job_ceiling_seconds - reserved_teardown_seconds\)\)",
            integ,
            flags=re.DOTALL,
        ) and not (
            "reserved_teardown_seconds=$((6 * 60))" in integ
            and "work_budget_seconds=$((job_ceiling_seconds - reserved_teardown_seconds))" in integ
            and "job_ceiling_seconds=$((50 * 60))" in integ
        ):
            res["watchdog"].append(
                "integration-security work cutoff must be 50m ceiling minus 6m reserved teardown (44+6=50)."
            )
        # Cutoff must be established before heavy setup so earlier steps count against budget.
        integ_steps = job_steps.get("integration-security") or []
        if not integ_steps or "Establish hosted work cutoff" not in integ_steps[0]:
            res["watchdog"].append(
                "integration-security must establish work cutoff as the first step "
                "(account for checkout/compose/setup against the job-start epoch)."
            )

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
        if "cleanup_interrupted" not in joined or not re.search(r"trap\s+cleanup_interrupted\s+INT\s+TERM", joined):
            res["integration"].append("Cleanup must trap INT/TERM and fail closed on interrupt.")
        if "Ambiguous/unproven Compose project ownership" not in joined:
            res["integration"].append(
                "Cleanup must fail closed on ambiguous/unproven Compose project ownership."
            )
        if "no volume-delete fallback" not in joined and "never volume-delete" not in joined:
            res["integration"].append(
                "Cleanup marker-absent path must verify residuals without volume-delete fallback."
            )
    # #344 coordination: hosted-ci replay profile only on the ClickHouse Make step.
    ch_steps = [
        s for s in job_steps.get("integration-security", [])
        if re.search(r"\bmake\s+test-clickhouse-replay-migration\b", "\n".join(_step_run_lines(s)))
    ]
    if ch_steps:
        ch = ch_steps[0]
        if not re.search(
            r"(?m)^\s+AMRUDAI_REPLAY_RUNTIME_PROFILE:\s*hosted-ci\s*(?:#.*)?$",
            ch,
        ):
            res["integration"].append(
                "ClickHouse replay Make step must set AMRUDAI_REPLAY_RUNTIME_PROFILE=hosted-ci."
            )
        # Marker must not appear on unrelated integration steps.
        for s in job_steps.get("integration-security", []):
            if s is ch:
                continue
            if "AMRUDAI_REPLAY_RUNTIME_PROFILE" in s:
                res["integration"].append(
                    "AMRUDAI_REPLAY_RUNTIME_PROFILE must be selected only on the ClickHouse replay step."
                )
                break
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
    "name: Platform CI\non:\n  pull_request:\n  push:\n    branches: [main]\npermissions:\n  contents: read\n"
    "concurrency:\n"
    "  group: platform-ci-${{ github.workflow }}-${{ github.event.pull_request.number || github.run_id }}\n"
    "  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n"
    "jobs:\n"
    + _job("static-contracts", "Static contracts",
           "      - &install-compose\n        name: Install Compose 5.5.0\n        run: |\n"
           "          curl -fsSL https://github.com/docker/compose/releases/download/v5.5.0/docker-compose-linux-x86_64 -o /tmp/dc\n"
           "          echo \"0000000000000000000000000000000000000000000000000000000000000000  /tmp/dc\" | sha256sum -c -\n"
           "          docker compose version --short\n      - run: |\n          docker compose config -q\n          make test-static-contracts\n")
    + _job("unit", "Unit tests", "      - *install-compose\n      - run: make test-unit\n")
    + _job("ui-build", "UI test and production build", "      - run: docker build --no-cache -t platform-ui:ci ui/\n")
    + _job("integration-security", "Integration, PostgreSQL, contract, security",
           "      - *install-compose\n      - run: docker pull node:20-alpine\n"
           "      - run: ./tests/run-kafka-storage-contract-test.sh\n"
           "      - name: Prove ClickHouse legacy upgrade and replay-safe canonical reads\n"
           "        env:\n          AMRUDAI_REPLAY_RUNTIME_PROFILE: hosted-ci\n"
           "        run: make test-clickhouse-replay-migration\n"
           "      - run: |\n"
           "          make test-integration\n          ./tests/run-core-day02-tests.sh\n          make test-contract\n          make test-security-smoke\n"
           "      - if: failure()\n        run: |\n          docker compose ps || true\n          docker compose logs --tail=250 || true\n"
           "      - if: always()\n        env:\n          RUN_ID: ${{ github.run_id }}\n          RUN_ATTEMPT: ${{ github.run_attempt }}\n"
           "        run: |\n          cleanup_interrupted() { echo fail-closed; exit 75; }\n"
           "          trap cleanup_interrupted INT TERM\n"
           "          [[ \"${GITHUB_ACTIONS:-}\" == \"true\" ]] || exit 1\n"
           "          echo \"Ambiguous/unproven Compose project ownership; refusing cleanup mutation\"\n"
           "          echo \"Canonical provisioning marker absent; verify residuals only (no volume-delete fallback).\"\n"
           "          bash deploy/docker/kafka/deployment-preflight.sh down\n"
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
    def test_watchdog_process_group_reap_contract(self) -> None: self.assertEqual(self.res["watchdog"], [])


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

    def test_mutation_inline_or_quoted_event_rejected(self) -> None:
        for extra in ('  workflow_dispatch: {}\n', '  schedule: [{cron: "0 0 * * *"}]\n',
                      '  repository_dispatch: {types: [test]}\n',
                      '  "workflow_dispatch":\n'):
            with self.subTest(extra=extra):
                mutated = COMPLIANT_FIXTURE.replace('  pull_request:\n', '  pull_request:\n' + extra, 1)
                errs = evaluate_contract(mutated, self.makefile, self.run_all)['security']
                self.assertTrue(any('Unauthorized or unrecognized workflow event' in e for e in errs), errs)

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


class TestCIChangedFileWhitespaceGate(unittest.TestCase):
    def test_changed_file_whitespace_positive_and_fail_closed(self) -> None:
        workflow = WORKFLOW_PATH.read_text("utf-8")
        script = _whitespace_step_script(workflow)
        self.assertIn("Missing or malformed committed comparison SHA", script)
        self.assertIn("Required committed comparison SHA is unavailable", script)
        self.assertIn("git diff --check", script)

        def call(*cmd: str, cwd: Path) -> subprocess.CompletedProcess[str]:
            return subprocess.run(cmd, cwd=cwd, text=True, capture_output=True, check=False)

        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            self.assertEqual(call("git", "init", "-q", cwd=wd).returncode, 0)
            self.assertEqual(call("git", "config", "user.name", "Test", cwd=wd).returncode, 0)
            self.assertEqual(call("git", "config", "user.email", "test@example.invalid", cwd=wd).returncode, 0)
            (wd / "normal.txt").write_text("good\n", encoding="utf8")
            self.assertEqual(call("git", "add", ".", cwd=wd).returncode, 0)
            self.assertEqual(call("git", "commit", "-qm", "clean parent", cwd=wd).returncode, 0)
            parent = call("git", "rev-parse", "HEAD", cwd=wd).stdout.strip()

            # Positive control: clean committed tree vs parent must pass.
            p_ok = subprocess.run(
                ["bash", "-e", "-c", script],
                cwd=wd,
                env=dict(os.environ, BASE_SHA=parent),
                text=True,
                capture_output=True,
            )
            self.assertEqual(p_ok.returncode, 0, (p_ok.stdout, p_ok.stderr))

            (wd / "normal.txt").write_text("trailing space \n", encoding="utf8")
            self.assertEqual(call("git", "commit", "-qam", "committed defect", cwd=wd).returncode, 0)

            cases = (
                (parent, "trailing whitespace"),
                ("0" * 40, "trailing whitespace"),
                ("f" * 40, "unavailable"),
                ("", "Missing or malformed"),
                ("not-a-sha", "Missing or malformed"),
            )
            for base, needle in cases:
                with self.subTest(base=base, needle=needle):
                    p = subprocess.run(
                        ["bash", "-e", "-c", script],
                        cwd=wd,
                        env=dict(os.environ, BASE_SHA=base),
                        text=True,
                        capture_output=True,
                    )
                    combined = p.stdout + p.stderr
                    self.assertNotEqual(p.returncode, 0, (base, combined))
                    self.assertIn(needle, combined, (base, needle, combined))


class TestCIHostedDiskWatchdogMocks(unittest.TestCase):
    """Mock-only behavioral checks against the extracted platform-ci-disk script."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.raw_script = _extract_platform_ci_disk_script(WORKFLOW_PATH.read_text("utf-8"))

    def _reap_session(self, proc: subprocess.Popen[str] | None) -> None:
        """Positively reap an owned mock wrapper session; no blanket kills."""
        if proc is None or proc.poll() is not None:
            return
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait(timeout=5)

    def _write_mock_script(
        self,
        wd: Path,
        *,
        fail_sample_after: int = 0,
        fail_sample_once_at: int = 0,
        post_fail_delay_seconds: int = 0,
    ) -> Path:
        """Stub docker/df paths; keep the real watchdog/reap logic from the workflow."""
        body = self.raw_script
        body = body.replace(
            "docker_root=$(docker info --format '{{.DockerRootDir}}')",
            'docker_root="/tmp"',
        )
        if fail_sample_once_at > 0:
            cond = f"count == {fail_sample_once_at}"
        elif fail_sample_after > 0:
            cond = f"count >= {fail_sample_after}"
        else:
            cond = "0"
        # Optional post-TERM scheduling delay before sampler exit (sticky-race harness).
        # Dedented extracted body uses six spaces before stop_workload_signal/exit 1
        # (not the sixteen-space YAML heredoc indent). Fail closed unless exactly one hit.
        if post_fail_delay_seconds > 0:
            target = (
                "publish_sampler_failure 1\n"
                "      stop_workload_signal\n"
                "      exit 1"
            )
            replacement = (
                "publish_sampler_failure 1\n"
                "      stop_workload_signal\n"
                "      echo INJECTED_POST_TERM_DELAY\n"
                f"      sleep {post_fail_delay_seconds}\n"
                "      exit 1"
            )
            hits = body.count(target)
            if hits != 1:
                raise AssertionError(
                    "sticky post_fail_delay injection requires exactly one dedented "
                    f"sampler-failure target; found {hits}"
                )
            body = body.replace(target, replacement, 1)
        sample_impl = textwrap.dedent(
            f"""\
            sample() {{
              local marker="$RUNNER_TEMP/platform-ci-sample-count"
              local count=0
              if [[ -f "$marker" ]]; then
                count=$(cat "$marker")
              fi
              count=$((count + 1))
              printf '%s' "$count" > "$marker"
              if (( {cond} )); then
                echo "mock sample failure count=$count"
                return 1
              fi
              return 0
            }}
            """
        )
        snapshot_impl = textwrap.dedent(
            """\
            snapshot() {
              sample
            }
            """
        )
        body = re.sub(r"sample\(\) \{.*?\n\}", sample_impl.rstrip(), body, count=1, flags=re.DOTALL)
        body = re.sub(r"snapshot\(\) \{.*?\n\}", snapshot_impl.rstrip(), body, count=1, flags=re.DOTALL)
        # Drop sudo/df path probes that are irrelevant once sample/snapshot are stubbed.
        body = re.sub(
            r"\npaths=\(.*?\nfi\n",
            "\n",
            body,
            count=1,
            flags=re.DOTALL,
        )
        path = wd / "platform-ci-disk-mock"
        path.write_text(body if body.startswith("#!") else "#!/usr/bin/env bash\n" + body + "\n", encoding="utf8")
        path.chmod(0o755)
        return path

    def _env(self, wd: Path) -> dict[str, str]:
        return {
            **os.environ,
            "GITHUB_WORKSPACE": str(wd),
            "RUNNER_TEMP": str(wd / "runner-temp"),
            "PLATFORM_CI_TERM_GRACE_SECONDS": "2",
            "PLATFORM_CI_SAMPLER_INTERVAL_SECONDS": "1",
        }

    def _write_workload(self, wd: Path, body: str, name: str = "workload.sh") -> Path:
        path = wd / name
        path.write_text("#!/usr/bin/env bash\n" + body, encoding="utf8")
        path.chmod(0o755)
        return path

    def test_workload_success_and_failure_statuses_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            ok_wl = self._write_workload(wd, "exit 0\n", "ok.sh")
            bad_wl = self._write_workload(wd, "exit 7\n", "bad.sh")
            ok = subprocess.run(
                [str(script), str(ok_wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=15,
                start_new_session=True,
            )
            self.assertEqual(ok.returncode, 0, (ok.stdout, ok.stderr))
            bad = subprocess.run(
                [str(script), str(bad_wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=15,
                start_new_session=True,
            )
            self.assertEqual(bad.returncode, 7, (bad.stdout, bad.stderr))

    def test_sampler_failure_preserves_actual_status_and_reaps_group(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            # fail_sample_after=2: preflight sample passes; first periodic sample fails.
            script = self._write_mock_script(wd, fail_sample_after=2)
            env = self._env(wd)
            marker = wd / "runner-temp" / "workload-still-running"
            wl = self._write_workload(
                wd,
                f"touch '{marker}'\nwhile true; do sleep 0.2; done\n",
            )
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=30,
                start_new_session=True,
            )
            combined = p.stdout + p.stderr
            self.assertNotEqual(p.returncode, 0, combined)
            self.assertIn("Hosted disk sampler failed", combined)
            self.assertRegex(combined, r"exit=1\b")
            self.assertNotRegex(combined, r"(?m)^\s*sampler_status=143\b")
            self.assertIn(p.returncode, (143, 137, 1, 75), combined)
            leftover = subprocess.run(
                ["bash", "-c", f"pgrep -af {marker.name} || true"],
                text=True,
                capture_output=True,
                timeout=10,
            )
            self.assertNotIn(str(wl), leftover.stdout)

    def test_sticky_failure_survives_delayed_sampler_and_term_exit_0(self) -> None:
        """Verified counterexample: once-only sample fail + delayed sampler exit + TERM→0."""
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(
                wd,
                fail_sample_once_at=2,
                post_fail_delay_seconds=2,
            )
            # Injection must actually rewrite the dedented failure path.
            self.assertIn("INJECTED_POST_TERM_DELAY", script.read_text(encoding="utf8"))
            env = self._env(wd)
            wl = self._write_workload(
                wd,
                'trap "exit 0" TERM\nwhile :; do :; done\n',
                "term0.sh",
            )
            started = time.time()
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=20,
                start_new_session=True,
            )
            elapsed = time.time() - started
            combined = p.stdout + p.stderr
            self.assertNotEqual(p.returncode, 0, combined)
            self.assertIn("INJECTED_POST_TERM_DELAY", combined)
            self.assertIn("mock sample failure count=2", combined)
            self.assertIn("Hosted disk sampler failed", combined)
            self.assertRegex(combined, r"exit=1\b")
            self.assertGreaterEqual(elapsed, 1.5, f"injected post-TERM delay did not run: {elapsed:.2f}s")
            self.assertFalse(
                p.returncode == 0 and "mock sample failure" in combined,
                f"FALSE_GREEN sticky race: {combined}",
            )

    def test_already_expired_cutoff_rejects_before_work(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            env["PLATFORM_CI_WORK_CUTOFF_EPOCH"] = str(int(time.time()) - 5)
            env["PLATFORM_CI_RESERVED_TEARDOWN_SECONDS"] = "360"
            env["PLATFORM_CI_SAMPLER_INTERVAL_SECONDS"] = "30"
            started_marker = wd / "runner-temp" / "workload-started"
            wl = self._write_workload(
                wd,
                f"touch '{started_marker}'\nsleep 5\nexit 0\n",
                "late.sh",
            )
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=15,
                start_new_session=True,
            )
            combined = p.stdout + p.stderr
            self.assertEqual(p.returncode, 75, combined)
            self.assertIn("Hosted work cutoff reached", combined)
            self.assertFalse(started_marker.exists(), "late work must not start after expired cutoff")

    def test_active_cutoff_rejects_term_exit_0_workload(self) -> None:
        """Counterexample fix: cutoff during sleep must fail even when TERM exits 0."""
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            env["PLATFORM_CI_WORK_CUTOFF_EPOCH"] = str(int(time.time()) + 2)
            env["PLATFORM_CI_RESERVED_TEARDOWN_SECONDS"] = "360"
            env["PLATFORM_CI_SAMPLER_INTERVAL_SECONDS"] = "30"
            env["PLATFORM_CI_CUTOFF_POLL_SECONDS"] = "1"
            env["PLATFORM_CI_TERM_GRACE_SECONDS"] = "2"
            wl = self._write_workload(
                wd,
                'trap "exit 0" TERM\nsleep 8\nexit 0\n',
                "cutoff_term0.sh",
            )
            started = time.time()
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=20,
                start_new_session=True,
            )
            elapsed = time.time() - started
            combined = p.stdout + p.stderr
            self.assertNotEqual(p.returncode, 0, combined)
            self.assertEqual(p.returncode, 75, combined)
            self.assertIn("Hosted work cutoff", combined)
            # Must interrupt the 8s sleep; allow small TERM/reap overhead past the 2s cutoff.
            self.assertLess(elapsed, 6.0, f"cutoff not enforced during work: {elapsed:.2f}s {combined}")

    def test_active_cutoff_kills_term_resistant_workload(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            env["PLATFORM_CI_WORK_CUTOFF_EPOCH"] = str(int(time.time()) + 2)
            env["PLATFORM_CI_RESERVED_TEARDOWN_SECONDS"] = "360"
            env["PLATFORM_CI_SAMPLER_INTERVAL_SECONDS"] = "30"
            env["PLATFORM_CI_CUTOFF_POLL_SECONDS"] = "1"
            env["PLATFORM_CI_TERM_GRACE_SECONDS"] = "2"
            marker = wd / "runner-temp" / "cutoff-resistant"
            wl = self._write_workload(
                wd,
                f'trap "" TERM\ntouch "{marker}"\nwhile true; do sleep 0.2; done\n',
                "cutoff_ignore.sh",
            )
            started = time.time()
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=25,
                start_new_session=True,
            )
            elapsed = time.time() - started
            combined = p.stdout + p.stderr
            self.assertNotEqual(p.returncode, 0, combined)
            self.assertIn("Hosted work cutoff", combined)
            self.assertIn("sending SIGKILL", combined)
            self.assertIn(p.returncode, (75, 137), combined)
            self.assertLess(elapsed, 20, f"TERM-resistant cutoff hung: {elapsed:.1f}s {combined}")
            leftover = subprocess.run(
                ["bash", "-c", f"pgrep -af {marker.name} || true"],
                text=True,
                capture_output=True,
                timeout=10,
            )
            self.assertNotIn(str(wl), leftover.stdout)

    def test_intentional_sampler_stop_after_clean_success(self) -> None:
        """Clean workload success must not be treated as sampler failure."""
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            env["PLATFORM_CI_SAMPLER_INTERVAL_SECONDS"] = "30"
            wl = self._write_workload(wd, "exit 0\n", "clean.sh")
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=15,
                start_new_session=True,
            )
            combined = p.stdout + p.stderr
            self.assertEqual(p.returncode, 0, combined)
            self.assertNotIn("Hosted disk sampler failed", combined)

    def test_sigterm_ignoring_workload_escalates_to_kill(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            (wd / "runner-temp").mkdir()
            script = self._write_mock_script(wd, fail_sample_after=2)
            env = self._env(wd)
            env["PLATFORM_CI_TERM_GRACE_SECONDS"] = "2"
            wl = self._write_workload(wd, 'trap "" TERM\nwhile true; do sleep 0.2; done\n', "ignore.sh")
            started = time.time()
            p = subprocess.run(
                [str(script), str(wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=30,
                start_new_session=True,
            )
            elapsed = time.time() - started
            combined = p.stdout + p.stderr
            self.assertNotEqual(p.returncode, 0, combined)
            self.assertIn("sending SIGKILL", combined)
            self.assertIn("Hosted disk sampler failed", combined)
            self.assertRegex(combined, r"exit=1\b")
            self.assertEqual(p.returncode, 137, combined)
            self.assertLess(elapsed, 25, f"escalation hung: {elapsed:.1f}s {combined}")

    def test_cancel_reaps_without_orphans_and_completion_race(self) -> None:
        proc: subprocess.Popen[str] | None = None
        with tempfile.TemporaryDirectory() as tmp:
            wd = Path(tmp)
            rt = wd / "runner-temp"
            rt.mkdir()
            script = self._write_mock_script(wd)
            env = self._env(wd)
            env["PLATFORM_CI_SAMPLER_INTERVAL_SECONDS"] = "30"

            orphan_flag = rt / "orphan.flag"
            wl = self._write_workload(
                wd,
                f'while true; do echo still >>"{orphan_flag}"; sleep 0.2; done\n',
                "slow.sh",
            )
            try:
                proc = subprocess.Popen(
                    [str(script), str(wl)],
                    cwd=wd,
                    env=env,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    start_new_session=True,
                )
                deadline = time.time() + 5
                while time.time() < deadline and not orphan_flag.exists():
                    time.sleep(0.1)
                self.assertTrue(orphan_flag.exists(), "workload did not start before cancel")
                proc.send_signal(signal.SIGTERM)
                stdout, stderr = proc.communicate(timeout=30)
                combined = (stdout or "") + (stderr or "")
                self.assertEqual(proc.returncode, 130, combined)
                self.assertIn("Hosted test command cancelled", combined)
                time.sleep(0.6)
                before = orphan_flag.read_text(encoding="utf8")
                time.sleep(0.6)
                after = orphan_flag.read_text(encoding="utf8")
                self.assertEqual(before, after, "orphan process group kept writing after cancel")
            finally:
                self._reap_session(proc)

            # Clean completion without cancel.
            done_wl = self._write_workload(wd, "exit 0\n", "done.sh")
            done = subprocess.run(
                [str(script), str(done_wl)],
                cwd=wd,
                env=env,
                text=True,
                capture_output=True,
                timeout=15,
                start_new_session=True,
            )
            self.assertEqual(done.returncode, 0, (done.stdout, done.stderr))

            # Genuine simultaneous cancel/completion: short workload + near-coincident TERM.
            race_proc: subprocess.Popen[str] | None = None
            try:
                race_wl = self._write_workload(wd, "sleep 0.3\nexit 0\n", "race.sh")
                race_proc = subprocess.Popen(
                    [str(script), str(race_wl)],
                    cwd=wd,
                    env=env,
                    text=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    start_new_session=True,
                )
                time.sleep(0.15)
                race_proc.send_signal(signal.SIGTERM)
                r_out, r_err = race_proc.communicate(timeout=15)
                race_combined = (r_out or "") + (r_err or "")
                # Cancel path (130) or clean completion (0) are both coherent; never hang.
                self.assertIn(race_proc.returncode, (0, 130), race_combined)
                if race_proc.returncode == 130:
                    self.assertIn("Hosted test command cancelled", race_combined)
            finally:
                self._reap_session(race_proc)


if __name__ == "__main__":
    unittest.main()
