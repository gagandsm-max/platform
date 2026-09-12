"""Control Plane v2 contracts: state machine, task manifests, and handoffs.

Validation is explicit and deterministic. It uses only the Python standard
library. JSON schemas in project_agents/schemas/ remain the human/machine
contract documents; this module enforces them without generating those files.
"""

from __future__ import annotations

import hashlib
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any, Mapping

REPO_ROOT = Path(__file__).resolve().parents[2]
AGENTS_ROOT = REPO_ROOT / "project_agents"
STATE_MACHINE_PATH = AGENTS_ROOT / "control_plane_state_machine.json"
TASK_SCHEMA_PATH = AGENTS_ROOT / "schemas" / "task_manifest.schema.json"
HANDOFF_SCHEMA_PATH = AGENTS_ROOT / "schemas" / "handoff.schema.json"
MANIFEST_PATH = AGENTS_ROOT / "manifest.json"
SKILLS_MANIFEST_PATH = AGENTS_ROOT / "skills" / "manifest.json"

SCHEMA_VERSION = "v2"
GIT_SHA_RE = re.compile(r"^[0-9a-f]{40}$")
ISO_TIME_RE = re.compile(
    r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$"
)
TASK_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{1,127}$")
AGENT_NAME_RE = re.compile(r"^[a-z][a-z0-9-]*$")
SKILL_NAME_RE = re.compile(r"^[a-z][a-z0-9_]*$")

WRITER_SELF_CERTIFY_FORBIDDEN = frozenset(
    {"REALITY_GATE", "MERGE_READY", "POST_MERGE_VERIFY", "COMPLETE"}
)
EVIDENCE_REQUIRED_CLAIM_TYPES = frozenset(
    {"SOURCE_FACT", "EXECUTION_EVIDENCE", "CI_EVIDENCE"}
)
UNVERIFIED = "UNVERIFIED"
CLAIM_TYPES = frozenset(
    {"SOURCE_FACT", "EXECUTION_EVIDENCE", "CI_EVIDENCE", "INFERENCE", "UNVERIFIED"}
)
PROVISIONAL_SCOPE_PHASES = frozenset({"INTAKE", "REBASELINE", "REQUIREMENTS", "DIAGNOSIS"})
EXCEPTION_STATES = frozenset({"BLOCKED", "REJECTED"})
HEAD_REQUIRED_STATES = frozenset(
    {
        "ROUTING",
        "IMPLEMENTATION",
        "LOCAL_VALIDATION",
        "SOURCE_REVIEW",
        "ADVERSARIAL_RUNTIME",
        "SECURITY_DOMAIN_REVIEW",
        "CI_VALIDATION",
        "REALITY_GATE",
        "MERGE_READY",
        "POST_MERGE_VERIFY",
        "COMPLETE",
    }
)
FROZEN_SCOPE_STATES = frozenset(
    {
        "IMPLEMENTATION",
        "LOCAL_VALIDATION",
        "SOURCE_REVIEW",
        "ADVERSARIAL_RUNTIME",
        "SECURITY_DOMAIN_REVIEW",
        "CI_VALIDATION",
        "REALITY_GATE",
        "MERGE_READY",
        "POST_MERGE_VERIFY",
        "COMPLETE",
    }
)
VALIDATION_GATE_STATES = frozenset(
    {
        "LOCAL_VALIDATION",
        "SOURCE_REVIEW",
        "ADVERSARIAL_RUNTIME",
        "SECURITY_DOMAIN_REVIEW",
        "CI_VALIDATION",
        "REALITY_GATE",
        "MERGE_READY",
        "POST_MERGE_VERIFY",
        "COMPLETE",
    }
)


class ControlPlaneError(ValueError):
    """Deterministic Control Plane v2 contract failure."""


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def load_state_machine(path: Path = STATE_MACHINE_PATH) -> dict[str, Any]:
    data = load_json(path)
    if not isinstance(data, dict):
        raise ControlPlaneError("state machine must be a JSON object")
    return data


def authoritative_states(machine: Mapping[str, Any] | None = None) -> list[str]:
    machine = machine or load_state_machine()
    states = machine.get("states")
    if not isinstance(states, list) or not states or not all(isinstance(item, str) for item in states):
        raise ControlPlaneError("state machine states must be a non-empty string array")
    return list(states)


def ordered_flow(machine: Mapping[str, Any] | None = None) -> list[str]:
    machine = machine or load_state_machine()
    flow = machine.get("orderedFlow")
    if not isinstance(flow, list) or not all(isinstance(item, str) for item in flow):
        raise ControlPlaneError("state machine orderedFlow must be a string array")
    return list(flow)


def legal_transitions(machine: Mapping[str, Any] | None = None) -> dict[str, list[str]]:
    machine = machine or load_state_machine()
    raw = machine.get("transitions")
    if not isinstance(raw, dict):
        raise ControlPlaneError("state machine transitions must be an object")
    out: dict[str, list[str]] = {}
    for source, targets in raw.items():
        if not isinstance(source, str) or not isinstance(targets, list) or not all(
            isinstance(item, str) for item in targets
        ):
            raise ControlPlaneError("state machine transitions must map strings to string arrays")
        out[source] = list(targets)
    return out


def _manifest_entries(data: Any, label: str) -> list[dict[str, Any]]:
    if isinstance(data, list):
        entries = data
    elif isinstance(data, dict):
        for key in ("agents", "skills"):
            if isinstance(data.get(key), list):
                entries = data[key]
                break
        else:
            raise ControlPlaneError(f"{label} must be an array or an object with an agents/skills array")
    else:
        raise ControlPlaneError(f"{label} must be a JSON array")
    out: list[dict[str, Any]] = []
    for item in entries:
        if not isinstance(item, dict) or not isinstance(item.get("name"), str):
            raise ControlPlaneError(f"{label} entries must include a string name")
        out.append(item)
    return out


def load_agent_names(path: Path = MANIFEST_PATH) -> set[str]:
    return {item["name"] for item in _manifest_entries(load_json(path), "agent manifest")}


def load_skill_names(path: Path = SKILLS_MANIFEST_PATH) -> set[str]:
    return {item["name"] for item in _manifest_entries(load_json(path), "skill manifest")}


def schema_enum(schema: Mapping[str, Any], *path: str) -> list[str]:
    cursor: Any = schema
    for key in path:
        if not isinstance(cursor, dict) or key not in cursor:
            raise ControlPlaneError(f"schema path {'.'.join(path)} is missing")
        cursor = cursor[key]
    if not isinstance(cursor, list) or not cursor or not all(isinstance(item, str) for item in cursor):
        raise ControlPlaneError(f"schema path {'.'.join(path)} must be a non-empty string enum")
    return list(cursor)


def assert_schema_state_enums_match_machine(
    machine: Mapping[str, Any] | None = None,
    task_schema: Mapping[str, Any] | None = None,
    handoff_schema: Mapping[str, Any] | None = None,
) -> None:
    states = authoritative_states(machine)
    task_schema = task_schema or load_json(TASK_SCHEMA_PATH)
    handoff_schema = handoff_schema or load_json(HANDOFF_SCHEMA_PATH)
    task_status = schema_enum(task_schema, "properties", "status", "enum")
    handoff_phase = schema_enum(handoff_schema, "properties", "phase", "enum")
    handoff_next = schema_enum(handoff_schema, "properties", "nextState", "enum")
    for label, values in (
        ("task_manifest.status", task_status),
        ("handoff.phase", handoff_phase),
        ("handoff.nextState", handoff_next),
    ):
        if values != states:
            raise ControlPlaneError(
                f"{label} enum {values!r} does not match authoritative state machine {states!r}"
            )


def is_legal_transition(source: str, target: str, machine: Mapping[str, Any] | None = None) -> bool:
    transitions = legal_transitions(machine)
    return target in transitions.get(source, [])


def assert_legal_transition(
    source: str,
    target: str,
    *,
    actor_role: str | None = None,
    machine: Mapping[str, Any] | None = None,
) -> None:
    machine = machine or load_state_machine()
    states = set(authoritative_states(machine))
    if source not in states:
        raise ControlPlaneError(f"unknown source state {source}")
    if target not in states:
        raise ControlPlaneError(f"unknown target state {target}")
    if not is_legal_transition(source, target, machine):
        raise ControlPlaneError(f"illegal control-plane transition {source} -> {target}")
    if actor_role == "writer" and target in WRITER_SELF_CERTIFY_FORBIDDEN:
        raise ControlPlaneError(
            f"writer cannot self-certify transition into {target}"
        )


def _require_object(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ControlPlaneError(f"{label} must be an object")
    return value


def _require_string(value: Any, label: str, *, allow_empty: bool = False) -> str:
    if not isinstance(value, str):
        raise ControlPlaneError(f"{label} must be a string")
    if not allow_empty and not value.strip():
        raise ControlPlaneError(f"{label} must be a non-empty string")
    return value


def _require_string_array(value: Any, label: str) -> list[str]:
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ControlPlaneError(f"{label} must be an array of strings")
    return list(value)


def _require_iso_time(value: Any, label: str) -> str:
    text = _require_string(value, label)
    if not ISO_TIME_RE.match(text):
        raise ControlPlaneError(f"{label} must be an ISO-8601 timestamp")
    datetime.fromisoformat(text.replace("Z", "+00:00"))
    return text


def _require_sha(value: Any, label: str) -> str:
    text = _require_string(value, label)
    if not GIT_SHA_RE.match(text):
        raise ControlPlaneError(f"{label} must be a 40-character lowercase git SHA")
    return text


def _require_enum(value: Any, label: str, allowed: list[str] | set[str]) -> str:
    text = _require_string(value, label)
    if text not in set(allowed):
        raise ControlPlaneError(f"{label} must be one of {sorted(set(allowed))}")
    return text


def _unknown_keys(obj: Mapping[str, Any], allowed: set[str], label: str) -> None:
    extra = sorted(set(obj) - allowed)
    if extra:
        raise ControlPlaneError(f"{label} has unsupported fields: {extra}")


def validate_selected_agents(names: list[str], allowed: set[str] | None = None) -> list[str]:
    allowed = allowed if allowed is not None else load_agent_names()
    seen: set[str] = set()
    for name in names:
        if not AGENT_NAME_RE.match(name):
            raise ControlPlaneError(f"selectedAgents entry {name!r} is not a valid agent name")
        if name not in allowed:
            raise ControlPlaneError(f"selectedAgents entry {name!r} is not declared in the agent manifest")
        if name in seen:
            raise ControlPlaneError(f"selectedAgents contains duplicate {name!r}")
        seen.add(name)
    return names


def validate_selected_skills(names: list[str], allowed: set[str] | None = None) -> list[str]:
    allowed = allowed if allowed is not None else load_skill_names()
    seen: set[str] = set()
    for name in names:
        if not SKILL_NAME_RE.match(name):
            raise ControlPlaneError(f"selectedSkills entry {name!r} is not a valid skill name")
        if name not in allowed:
            raise ControlPlaneError(f"selectedSkills entry {name!r} is not declared in the skill manifest")
        if name in seen:
            raise ControlPlaneError(f"selectedSkills contains duplicate {name!r}")
        seen.add(name)
    return names


def validate_task_manifest(
    manifest: Mapping[str, Any],
    *,
    observed_main: str | None = None,
    observed_head: str | None = None,
    agent_names: set[str] | None = None,
    skill_names: set[str] | None = None,
    machine: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    obj = _require_object(manifest, "task manifest")
    states = authoritative_states(machine)
    required_always = {
        "schemaVersion",
        "taskId",
        "taskType",
        "objective",
        "repository",
        "authoritativeMain",
        "branch",
        "worktree",
        "governingRequirements",
        "selectedAgents",
        "selectedSkills",
        "scope",
        "nonGoals",
        "securityBoundaries",
        "migrationReservations",
        "dependencies",
        "requiredTests",
        "requiredEvidence",
        "stopConditions",
        "writerTool",
        "verifierTool",
        "gatekeeper",
        "status",
        "createdAt",
        "updatedAt",
    }
    missing = sorted(required_always - set(obj))
    if missing:
        raise ControlPlaneError(f"task manifest missing required fields: {missing}")
    if "writer" in obj or "verifier" in obj or "state" in obj:
        raise ControlPlaneError("task manifest must use writerTool/verifierTool/status; aliases are forbidden")
    _unknown_keys(
        obj,
        required_always | {"exactHead"},
        "task manifest",
    )
    if _require_string(obj["schemaVersion"], "schemaVersion") != SCHEMA_VERSION:
        raise ControlPlaneError(f"schemaVersion must be {SCHEMA_VERSION}")
    task_id = _require_string(obj["taskId"], "taskId")
    if not TASK_ID_RE.match(task_id):
        raise ControlPlaneError("taskId has an invalid format")
    status = _require_enum(obj["status"], "status", states)
    _require_string(obj["taskType"], "taskType")
    _require_string(obj["objective"], "objective")
    _require_string(obj["repository"], "repository")
    authoritative_main = _require_sha(obj["authoritativeMain"], "authoritativeMain")
    _require_string(obj["branch"], "branch")
    _require_string(obj["worktree"], "worktree")
    _require_string_array(obj["governingRequirements"], "governingRequirements")
    selected_agents = validate_selected_agents(
        _require_string_array(obj["selectedAgents"], "selectedAgents"),
        agent_names,
    )
    selected_skills = validate_selected_skills(
        _require_string_array(obj["selectedSkills"], "selectedSkills"),
        skill_names,
    )
    scope = _require_string_array(obj["scope"], "scope")
    _require_string_array(obj["nonGoals"], "nonGoals")
    _require_string_array(obj["securityBoundaries"], "securityBoundaries")
    _require_string_array(obj["migrationReservations"], "migrationReservations")
    _require_string_array(obj["dependencies"], "dependencies")
    _require_string_array(obj["requiredTests"], "requiredTests")
    required_evidence = _require_string_array(obj["requiredEvidence"], "requiredEvidence")
    _require_string_array(obj["stopConditions"], "stopConditions")
    _require_string(obj["writerTool"], "writerTool")
    _require_string(obj["verifierTool"], "verifierTool")
    _require_string(obj["gatekeeper"], "gatekeeper")
    created_at = _require_iso_time(obj["createdAt"], "createdAt")
    updated_at = _require_iso_time(obj["updatedAt"], "updatedAt")
    if datetime.fromisoformat(updated_at.replace("Z", "+00:00")) < datetime.fromisoformat(
        created_at.replace("Z", "+00:00")
    ):
        raise ControlPlaneError("updatedAt must not precede createdAt")

    exact_head: str | None = None
    if "exactHead" in obj and obj["exactHead"] not in (None, ""):
        exact_head = _require_sha(obj["exactHead"], "exactHead")
    if status in HEAD_REQUIRED_STATES and not exact_head:
        raise ControlPlaneError(
            f"exactHead is required before leaving diagnosis; status={status}"
        )
    if status in FROZEN_SCOPE_STATES and not scope:
        raise ControlPlaneError(
            "scope must be a non-empty allowed write surface before IMPLEMENTATION"
        )
    if observed_main is not None and observed_main != authoritative_main:
        raise ControlPlaneError(
            f"authoritativeMain drift: manifest={authoritative_main} observed={observed_main}"
        )
    if exact_head is not None and observed_head is not None and observed_head != exact_head:
        raise ControlPlaneError(
            f"exactHead drift: manifest={exact_head} observed={observed_head}"
        )
    if not selected_agents and status in HEAD_REQUIRED_STATES:
        raise ControlPlaneError("selectedAgents must be resolved before ROUTING")
    if not selected_skills and status in HEAD_REQUIRED_STATES:
        raise ControlPlaneError("selectedSkills must be resolved before ROUTING")
    if status in VALIDATION_GATE_STATES and not required_evidence:
        raise ControlPlaneError("requiredEvidence must be declared before validation gates")
    return dict(obj)


def assert_write_paths_in_scope(
    files_changed: list[str],
    scope: list[str],
    status: str,
) -> None:
    """Reject silent scope expansion once IMPLEMENTATION has frozen the write surface."""
    if status not in FROZEN_SCOPE_STATES:
        return
    if not scope:
        raise ControlPlaneError("frozen scope is empty; return through DIAGNOSIS/ROUTING")
    illegal = [path for path in files_changed if not any(path == item or path.startswith(item.rstrip("/") + "/") for item in scope)]
    if illegal:
        raise ControlPlaneError(
            "write path outside frozen scope; return through DIAGNOSIS/ROUTING to amend scope: "
            + ", ".join(illegal)
        )


def _validate_claim(claim: Any, index: int) -> dict[str, Any]:
    obj = _require_object(claim, f"claims[{index}]")
    _unknown_keys(obj, {"claimType", "statement", "evidencePointer"}, f"claims[{index}]")
    claim_type = _require_enum(obj.get("claimType"), f"claims[{index}].claimType", CLAIM_TYPES)
    statement = _require_string(obj.get("statement"), f"claims[{index}].statement")
    pointer = obj.get("evidencePointer")
    if claim_type in EVIDENCE_REQUIRED_CLAIM_TYPES:
        if not isinstance(pointer, str) or not pointer.strip():
            raise ControlPlaneError(
                f"claims[{index}] {claim_type} requires evidencePointer"
            )
    elif pointer is not None and pointer != "":
        _require_string(pointer, f"claims[{index}].evidencePointer")
    if claim_type == "INFERENCE" and (not isinstance(pointer, str) or not pointer.strip()):
        raise ControlPlaneError(
            f"claims[{index}] INFERENCE must cite supporting evidence where applicable"
        )
    return {
        "claimType": claim_type,
        "statement": statement,
        "evidencePointer": pointer if isinstance(pointer, str) else None,
    }


def claims_satisfy_required_evidence(
    claims: list[Mapping[str, Any]],
    required_evidence: list[str],
) -> None:
    if not required_evidence:
        return
    qualifying = [
        claim
        for claim in claims
        if claim.get("claimType") in EVIDENCE_REQUIRED_CLAIM_TYPES
        and isinstance(claim.get("evidencePointer"), str)
        and claim.get("evidencePointer", "").strip()
    ]
    unverified = [claim for claim in claims if claim.get("claimType") == UNVERIFIED]
    if unverified and not qualifying:
        raise ControlPlaneError("UNVERIFIED claims cannot satisfy a required evidence gate")
    covered = {claim.get("evidencePointer") for claim in qualifying}
    missing = [item for item in required_evidence if item not in covered]
    if missing:
        if unverified:
            raise ControlPlaneError(
                "UNVERIFIED claims cannot satisfy a required evidence gate"
            )
        raise ControlPlaneError(f"required evidence is not covered by qualifying claims: {missing}")


def validate_handoff(
    handoff: Mapping[str, Any],
    *,
    required_evidence: list[str] | None = None,
    actor_role: str | None = None,
    previous_state: str | None = None,
    machine: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    obj = _require_object(handoff, "handoff")
    states = authoritative_states(machine)
    required = {
        "schemaVersion",
        "taskId",
        "phase",
        "inputHead",
        "outputHead",
        "filesChanged",
        "requirementsAddressed",
        "testsExecuted",
        "testResults",
        "runtimeEvidence",
        "securityEvidence",
        "knownFailures",
        "unresolvedRisks",
        "claims",
        "nextState",
        "nextOwner",
        "timestamp",
    }
    missing = sorted(required - set(obj))
    if missing:
        raise ControlPlaneError(f"handoff missing required fields: {missing}")
    _unknown_keys(obj, required, "handoff")
    if _require_string(obj["schemaVersion"], "schemaVersion") != SCHEMA_VERSION:
        raise ControlPlaneError(f"schemaVersion must be {SCHEMA_VERSION}")
    task_id = _require_string(obj["taskId"], "taskId")
    if not TASK_ID_RE.match(task_id):
        raise ControlPlaneError("taskId has an invalid format")
    phase = _require_enum(obj["phase"], "phase", states)
    next_state = _require_enum(obj["nextState"], "nextState", states)
    _require_sha(obj["inputHead"], "inputHead")
    _require_sha(obj["outputHead"], "outputHead")
    _require_string_array(obj["filesChanged"], "filesChanged")
    _require_string_array(obj["requirementsAddressed"], "requirementsAddressed")
    _require_string_array(obj["testsExecuted"], "testsExecuted")
    test_results = obj["testResults"]
    if not isinstance(test_results, list):
        raise ControlPlaneError("testResults must be an array")
    for index, result in enumerate(test_results):
        item = _require_object(result, f"testResults[{index}]")
        _require_string(item.get("name"), f"testResults[{index}].name")
        _require_enum(item.get("status"), f"testResults[{index}].status", {"PASS", "FAIL", "SKIP", "ERROR"})
    _require_string_array(obj["runtimeEvidence"], "runtimeEvidence")
    _require_string_array(obj["securityEvidence"], "securityEvidence")
    _require_string_array(obj["knownFailures"], "knownFailures")
    _require_string_array(obj["unresolvedRisks"], "unresolvedRisks")
    claims_raw = obj["claims"]
    if not isinstance(claims_raw, list):
        raise ControlPlaneError("claims must be an array")
    claims = [_validate_claim(claim, index) for index, claim in enumerate(claims_raw)]
    _require_string(obj["nextOwner"], "nextOwner")
    _require_iso_time(obj["timestamp"], "timestamp")
    source_state = previous_state or phase
    assert_legal_transition(source_state, next_state, actor_role=actor_role, machine=machine)
    claims_satisfy_required_evidence(claims, required_evidence or [])
    unverified_only = claims and all(claim["claimType"] == UNVERIFIED for claim in claims)
    if unverified_only and next_state in {
        "LOCAL_VALIDATION",
        "SOURCE_REVIEW",
        "CI_VALIDATION",
        "REALITY_GATE",
        "MERGE_READY",
        "POST_MERGE_VERIFY",
        "COMPLETE",
    }:
        raise ControlPlaneError("handoffs cannot silently promote UNVERIFIED claims")
    return dict(obj)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()
