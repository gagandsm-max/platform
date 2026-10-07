"""Deterministic Skill eval coverage validator (no model, no network).

Proves routing consistency and semantic-token coverage inside canonical Skill
packages. Does not claim to prove real LLM behavior.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

from skill_resources import (
    MAX_RESOURCE_FILE_BYTES,
    SkillResourceError,
    assert_safe_resource_rel,
)

EVAL_TYPES = frozenset({"TRIGGER", "NEGATIVE", "ADVERSARIAL", "REGRESSION"})
EVAL_ID_RE = re.compile(r"^[a-z][a-z0-9_./-]{1,120}$")
MAX_EVAL_CASES_PER_SKILL = 64
MAX_EVAL_CASE_JSON_BYTES = 16 * 1024
MAX_SEMANTIC_TOKENS = 32
MAX_TOKEN_LEN = 160


class SkillEvalError(ValueError):
    """Deterministic skill-eval failure."""


def _as_str_list(value: Any, label: str) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise SkillEvalError(f"{label} must be a list of strings")
    return list(value)


def load_eval_cases(path: Path) -> list[dict[str, Any]]:
    if path.is_symlink():
        raise SkillEvalError(f"evals file must not be a symlink: {path}")
    if not path.is_file():
        raise SkillEvalError(f"missing evals file: {path}")
    size = path.stat().st_size
    if size > MAX_RESOURCE_FILE_BYTES:
        raise SkillEvalError(f"evals file exceeds size bound: {path} size={size}")
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SkillEvalError(f"evals JSON invalid: {path}: {exc}") from exc
    if not isinstance(payload, dict):
        raise SkillEvalError(f"evals root must be an object: {path}")
    cases = payload.get("cases")
    if not isinstance(cases, list):
        raise SkillEvalError(f"evals.cases must be a list: {path}")
    if len(cases) > MAX_EVAL_CASES_PER_SKILL:
        raise SkillEvalError(
            f"too many eval cases in {path}: {len(cases)} > {MAX_EVAL_CASES_PER_SKILL}"
        )
    return cases


def package_text_corpus(skill_dir: Path) -> str:
    """Concatenate authoritative guidance text for semantic-token search.

    Semantic corpus is SKILL.md + references/* text only. Fixtures and evals are
    excluded so requiredSemanticTokens / requiredInvariants cannot self-satisfy
    from cases.json or scenario fixtures. Generated mirrors are never searched.
    Non-text/binary references are skipped safely. No network/model evaluation.
    """
    chunks: list[str] = []
    skill_md = skill_dir / "SKILL.md"
    if skill_md.is_file() and not skill_md.is_symlink():
        try:
            chunks.append(skill_md.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError):
            pass

    refs_root = skill_dir / "references"
    if refs_root.is_dir() and not refs_root.is_symlink():
        for path in sorted(refs_root.rglob("*")):
            if path.is_symlink() or path.is_dir():
                continue
            if not path.is_file():
                continue
            try:
                rel = path.relative_to(skill_dir).as_posix()
                assert_safe_resource_rel(rel)
            except (ValueError, SkillResourceError):
                continue
            # Text search only; skip binary by decoding strictly.
            try:
                chunks.append(path.read_text(encoding="utf-8"))
            except (OSError, UnicodeDecodeError):
                continue
    return "\n".join(chunks)


def validate_eval_case(
    case: dict[str, Any],
    *,
    skill_name: str,
    known_agents: set[str],
    known_skills: set[str],
    agent_matrix: dict[str, Any],
    corpus: str,
    seen_ids: set[str],
) -> list[str]:
    errors: list[str] = []
    raw = json.dumps(case, ensure_ascii=True, sort_keys=True)
    if len(raw.encode("utf-8")) > MAX_EVAL_CASE_JSON_BYTES:
        errors.append(f"{skill_name}: eval case exceeds size bound")
        return errors
    if not isinstance(case, dict):
        errors.append(f"{skill_name}: eval case must be an object")
        return errors

    case_id = case.get("id")
    if not isinstance(case_id, str) or not EVAL_ID_RE.match(case_id):
        errors.append(f"{skill_name}: invalid eval id {case_id!r}")
    elif case_id in seen_ids:
        errors.append(f"duplicate eval id: {case_id}")
    else:
        seen_ids.add(case_id)

    case_type = case.get("type")
    if case_type not in EVAL_TYPES:
        errors.append(f"{skill_name}: invalid eval type {case_type!r} for {case_id!r}")

    task = case.get("task") or case.get("intent")
    if not isinstance(task, str) or not task.strip():
        errors.append(f"{skill_name}: eval {case_id!r} missing task/intent")

    try:
        owning_agents = _as_str_list(case.get("owningAgents"), f"{skill_name}:{case_id} owningAgents")
        required = _as_str_list(case.get("requiredSkills"), f"{skill_name}:{case_id} requiredSkills")
        forbidden = _as_str_list(case.get("forbiddenSkills"), f"{skill_name}:{case_id} forbiddenSkills")
        tokens = _as_str_list(
            case.get("requiredSemanticTokens") or case.get("requiredInvariants"),
            f"{skill_name}:{case_id} requiredSemanticTokens",
        )
    except SkillEvalError as exc:
        errors.append(str(exc))
        return errors

    if not owning_agents:
        errors.append(f"{skill_name}: eval {case_id!r} requires owningAgents")
    if not required:
        errors.append(f"{skill_name}: eval {case_id!r} requires requiredSkills")

    for agent in owning_agents:
        if agent not in known_agents:
            errors.append(f"{skill_name}: eval {case_id!r} unknown agent {agent}")
    for name in required:
        if name not in known_skills:
            errors.append(f"{skill_name}: eval {case_id!r} unknown required skill {name}")
    for name in forbidden:
        if name not in known_skills:
            errors.append(f"{skill_name}: eval {case_id!r} unknown forbidden skill {name}")

    overlap = sorted(set(required) & set(forbidden))
    if overlap:
        errors.append(
            f"{skill_name}: eval {case_id!r} required/forbidden contradiction: {', '.join(overlap)}"
        )

    # Owning agent must route each required skill as required or conditional.
    for agent in owning_agents:
        spec = agent_matrix.get(agent)
        if not isinstance(spec, dict):
            errors.append(f"{skill_name}: eval {case_id!r} agent {agent} missing from matrix")
            continue
        routed = set(spec.get("requiredSkills") or []) | set(spec.get("conditionalSkills") or [])
        for name in required:
            if name not in routed:
                errors.append(
                    f"{skill_name}: eval {case_id!r} agent {agent} cannot route required skill {name}"
                )

    if len(tokens) > MAX_SEMANTIC_TOKENS:
        errors.append(f"{skill_name}: eval {case_id!r} too many semantic tokens")
    corpus_lower = corpus.lower()
    for token in tokens:
        if not token or len(token) > MAX_TOKEN_LEN:
            errors.append(f"{skill_name}: eval {case_id!r} invalid semantic token {token!r}")
            continue
        if token.lower() not in corpus_lower:
            errors.append(
                f"{skill_name}: eval {case_id!r} missing semantic token in package: {token!r}"
            )

    # Optional resource path references inside case must be safe.
    for key in ("fixture", "fixtures", "reference", "references"):
        value = case.get(key)
        if value is None:
            continue
        paths = value if isinstance(value, list) else [value]
        for item in paths:
            if not isinstance(item, str):
                errors.append(f"{skill_name}: eval {case_id!r} {key} entries must be strings")
                continue
            try:
                assert_safe_resource_rel(item)
            except SkillResourceError as exc:
                errors.append(f"{skill_name}: eval {case_id!r} unsafe resource path: {exc}")

    disposition = case.get("expectedDisposition")
    if disposition is not None and disposition not in {"SAFE", "FAIL_CLOSED", "INCONCLUSIVE", "REJECT"}:
        errors.append(f"{skill_name}: eval {case_id!r} invalid expectedDisposition {disposition!r}")

    return errors


def validate_skill_evals(
    *,
    skill_name: str,
    skill_dir: Path,
    known_agents: set[str],
    known_skills: set[str],
    agent_matrix: dict[str, Any],
    require_evals: bool,
    required_types: set[str] | None = None,
    global_seen_ids: set[str] | None = None,
) -> list[str]:
    """Validate evals/cases.json for one skill when present or required."""
    errors: list[str] = []
    eval_path = skill_dir / "evals" / "cases.json"
    if not eval_path.exists():
        if require_evals:
            errors.append(f"{skill_name}: missing required evals/cases.json")
        return errors

    try:
        cases = load_eval_cases(eval_path)
    except SkillEvalError as exc:
        return [str(exc)]

    corpus = package_text_corpus(skill_dir)
    seen = global_seen_ids if global_seen_ids is not None else set()
    present_types: set[str] = set()
    for case in cases:
        if isinstance(case, dict) and isinstance(case.get("type"), str):
            present_types.add(case["type"])
        errors.extend(
            validate_eval_case(
                case,
                skill_name=skill_name,
                known_agents=known_agents,
                known_skills=known_skills,
                agent_matrix=agent_matrix,
                corpus=corpus,
                seen_ids=seen,
            )
        )

    if required_types:
        missing = sorted(required_types - present_types)
        for case_type in missing:
            errors.append(f"{skill_name}: missing required eval type {case_type}")

    return errors


def load_commercial_depth_policy(path: Path) -> dict[str, Any]:
    if not path.is_file() or path.is_symlink():
        raise SkillEvalError(f"missing commercial depth policy: {path}")
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SkillEvalError(f"commercial depth policy invalid: {exc}") from exc
    if not isinstance(payload, dict):
        raise SkillEvalError("commercial depth policy must be an object")
    return payload
