"""Network-free CLI for Engineering Orchestrator Phase 1.

Uses argparse only. No network imports. Actors are logical/unattested.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Optional, Sequence

from .engine import EngineError, OrchestratorEngine
from .models import (
    Actor,
    EvidenceResult,
    EvidenceType,
    FailureClass,
    RcaStatus,
    ReadinessTarget,
    State,
)
from .store import OrchestratorStore


def _add_store_arg(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--store",
        required=True,
        help="Directory for bundle.json persistence",
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m tools.engineering_orchestrator",
        description=(
            "Platform Engineering Orchestrator Phase 1 — deterministic FSM, "
            "evidence, and hash-chain journal (tamper detection only; not signatures). "
            "Actors are logical/unattested."
        ),
    )
    sub = parser.add_subparsers(dest="command", required=True)

    p_init = sub.add_parser("init-task", help="Initialize a new task store")
    _add_store_arg(p_init)
    p_init.add_argument("--task-id", required=True)
    p_init.add_argument("--title", required=True)
    p_init.add_argument("--head-sha", default="")
    p_init.add_argument("--base-sha", default="")
    p_init.add_argument("--repository", default="")
    p_init.add_argument("--branch", default="")
    p_init.add_argument("--lane-id", default="")
    p_init.add_argument(
        "--path-prefix",
        action="append",
        default=[],
        dest="path_prefixes",
        help="Relative path prefix owned by the lane (repeatable)",
    )
    p_init.add_argument(
        "--required-scope",
        action="append",
        default=[],
        dest="required_scopes",
        help="Legacy scope labels (optional; gates are authoritative)",
    )
    p_init.add_argument(
        "--readiness-target",
        default=ReadinessTarget.IMPLEMENTED.value,
        choices=[t.value for t in ReadinessTarget],
    )
    p_init.add_argument(
        "--allow-migration-reservation",
        action="store_true",
        default=False,
    )

    p_tr = sub.add_parser("transition", help="Apply an authorized FSM transition")
    _add_store_arg(p_tr)
    p_tr.add_argument("--to-state", required=True, choices=[s.value for s in State])
    p_tr.add_argument("--expected-state", required=True, choices=[s.value for s in State])
    p_tr.add_argument("--actor", required=True, choices=[a.value for a in Actor])
    p_tr.add_argument("--rework-target", default=None, choices=[s.value for s in State])
    p_tr.add_argument(
        "--failure-class",
        default=None,
        choices=[f.value for f in FailureClass],
    )
    p_tr.add_argument(
        "--rca-status",
        default=None,
        choices=[r.value for r in RcaStatus],
    )
    p_tr.add_argument(
        "--reality-gate-pass",
        action="store_true",
        help="Reality Checker asserts gate PASS (required for READY_FOR_MERGE_GATE)",
    )
    p_tr.add_argument("--notes", default="")
    p_tr.add_argument("--evidence-id", action="append", default=[], dest="evidence_ids")

    p_rb = sub.add_parser(
        "reconcile-base",
        help="SYSTEM-only FULL_VALIDATION → BASE_RECONCILIATION with HEAD/base update",
    )
    _add_store_arg(p_rb)
    p_rb.add_argument(
        "--expected-state",
        required=True,
        choices=[State.FULL_VALIDATION.value],
    )
    p_rb.add_argument("--expected-head-sha", required=True)
    p_rb.add_argument("--expected-base-sha", required=True)
    p_rb.add_argument("--new-head-sha", required=True)
    p_rb.add_argument("--new-base-sha", required=True)
    p_rb.add_argument(
        "--changed-path",
        action="append",
        default=[],
        dest="changed_paths",
        required=True,
    )
    p_rb.add_argument(
        "--verification-evidence-id",
        action="append",
        default=[],
        dest="verification_evidence_ids",
        required=True,
    )
    p_rb.add_argument("--impact-known", action="store_true", default=False)
    p_rb.add_argument("--impact-unknown", action="store_true", default=False)

    p_ev = sub.add_parser("record-evidence", help="Record evidence JSON record")
    _add_store_arg(p_ev)
    p_ev.add_argument("--evidence-id", required=True)
    p_ev.add_argument("--type", required=True, choices=[t.value for t in EvidenceType])
    p_ev.add_argument("--head-sha", required=True)
    p_ev.add_argument("--claim-scope", required=True)
    p_ev.add_argument("--result", required=True, choices=[r.value for r in EvidenceResult])
    p_ev.add_argument("--collected-at", required=True)
    p_ev.add_argument("--payload-ref", default="")
    p_ev.add_argument("--command-identity", default="")
    p_ev.add_argument("--worker-logical-identity", default="")
    p_ev.add_argument("--reviewer-logical-identity", default="")
    p_ev.add_argument("--ci-run-id", default="")
    p_ev.add_argument("--ci-job-id", default="")
    p_ev.add_argument("--path", action="append", default=[], dest="paths")
    p_ev.add_argument("--dependency", action="append", default=[], dest="dependencies")
    p_ev.add_argument("--reused-from-evidence-id", default="")
    p_ev.add_argument("--reuse-reason", default="")
    p_ev.add_argument("--impact-known", action="store_true", default=True)
    p_ev.add_argument("--impact-unknown", action="store_true", default=False)
    p_ev.add_argument(
        "--promote-from",
        default=None,
        choices=[t.value for t in EvidenceType],
        help="If set, validate promotion from this type (forbidden promotions reject)",
    )

    p_st = sub.add_parser("status", help="Founder/CTO status projection")
    _add_store_arg(p_st)

    p_rp = sub.add_parser("replay", help="Replay journal and report reconstructed state")
    _add_store_arg(p_rp)

    p_vj = sub.add_parser("validate-journal", help="Validate journal hash chain / continuity")
    _add_store_arg(p_vj)

    p_ho = sub.add_parser("handoff-create", help="Create structured manual handoff JSON")
    _add_store_arg(p_ho)
    p_ho.add_argument("--handoff-id", required=True)
    p_ho.add_argument("--actor-role", required=True)
    p_ho.add_argument("--expected-return-state", required=True)
    p_ho.add_argument("--instructions-ref", default="")
    p_ho.add_argument(
        "--inputs-json",
        default="{}",
        help="JSON object of handoff inputs (secrets redacted)",
    )
    p_ho.add_argument(
        "--codex-enabled",
        action="store_true",
        default=False,
        help="Explicitly set codex_enabled (default: false; Phase 1 does not execute AI)",
    )

    return parser


def _engine(store_path: str) -> OrchestratorEngine:
    return OrchestratorEngine(OrchestratorStore(store_path))


def _print_json(payload: Any) -> None:
    print(json.dumps(payload, indent=2, sort_keys=True, ensure_ascii=False))


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)

    try:
        if args.command == "init-task":
            engine = _engine(args.store)
            task = engine.init_task(
                task_id=args.task_id,
                title=args.title,
                head_sha=args.head_sha,
                base_sha=args.base_sha,
                repository=args.repository,
                branch=args.branch,
                lane_id=args.lane_id,
                path_prefixes=args.path_prefixes,
                required_scopes=args.required_scopes or None,
                readiness_target=ReadinessTarget(args.readiness_target),
                migration_reservation_allowed=bool(args.allow_migration_reservation),
            )
            _print_json(task.to_dict())
            return 0

        if args.command == "transition":
            engine = _engine(args.store)
            task = engine.transition(
                State(args.to_state),
                Actor(args.actor),
                expected_state=State(args.expected_state),
                evidence_ids=args.evidence_ids or None,
                rework_target=State(args.rework_target) if args.rework_target else None,
                failure_class=FailureClass(args.failure_class) if args.failure_class else None,
                rca_status=RcaStatus(args.rca_status) if args.rca_status else None,
                reality_gate_pass=bool(args.reality_gate_pass),
                notes=args.notes,
            )
            _print_json(task.to_dict())
            return 0

        if args.command == "reconcile-base":
            engine = _engine(args.store)
            impact_known = True
            if args.impact_unknown:
                impact_known = False
            elif args.impact_known:
                impact_known = True
            else:
                impact_known = False
            task = engine.reconcile_base(
                expected_state=State(args.expected_state),
                expected_head_sha=args.expected_head_sha,
                expected_base_sha=args.expected_base_sha,
                new_head_sha=args.new_head_sha,
                new_base_sha=args.new_base_sha,
                changed_paths=args.changed_paths,
                verification_evidence_ids=args.verification_evidence_ids,
                impact_known=impact_known,
            )
            _print_json(task.to_dict())
            return 0

        if args.command == "record-evidence":
            engine = _engine(args.store)
            impact_known = False if args.impact_unknown else True
            promote_from = EvidenceType(args.promote_from) if args.promote_from else None
            ev = engine.record_evidence(
                evidence_id=args.evidence_id,
                evidence_type=EvidenceType(args.type),
                head_sha=args.head_sha,
                claim_scope=args.claim_scope,
                result=EvidenceResult(args.result),
                collected_at=args.collected_at,
                payload_ref=args.payload_ref,
                impact_known=impact_known,
                promote_from=promote_from,
                paths=args.paths or None,
                dependencies=args.dependencies or None,
                command_identity=args.command_identity,
                worker_logical_identity=args.worker_logical_identity,
                reviewer_logical_identity=args.reviewer_logical_identity,
                ci_run_id=args.ci_run_id,
                ci_job_id=args.ci_job_id,
                reused_from_evidence_id=args.reused_from_evidence_id,
                reuse_reason=args.reuse_reason,
            )
            _print_json(ev.to_dict())
            return 0

        if args.command == "status":
            engine = _engine(args.store)
            _print_json(engine.status())
            return 0

        if args.command == "replay":
            engine = _engine(args.store)
            _print_json(engine.replay())
            return 0

        if args.command == "validate-journal":
            engine = _engine(args.store)
            result = engine.validate_journal()
            _print_json(result)
            return 0 if result.get("ok") else 2

        if args.command == "handoff-create":
            engine = _engine(args.store)
            try:
                inputs = json.loads(args.inputs_json)
            except json.JSONDecodeError as exc:
                raise EngineError(f"invalid --inputs-json: {exc}") from exc
            if not isinstance(inputs, dict):
                raise EngineError("--inputs-json must be a JSON object")
            handoff = engine.create_handoff(
                handoff_id=args.handoff_id,
                actor_role=args.actor_role,
                expected_return_state=args.expected_return_state,
                instructions_ref=args.instructions_ref,
                inputs=inputs,
                codex_enabled=bool(args.codex_enabled),
            )
            _print_json(handoff.to_dict())
            return 0

        parser.error(f"unknown command {args.command}")
        return 2
    except Exception as exc:  # noqa: BLE001 — CLI boundary
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
